// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue provides queue implementations that can be used for
// asynchronous scheduling of fetch actions.
package queue

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A Queue provides an interface for asynchronous scheduling of fetch actions.
type Queue interface {
	ScheduleFetch(ctx context.Context, modulePath, version, suffix string) error
}

// GCP provides a Queue implementation backed by the Google Cloud Tasks
// API.
type GCP struct {
	cfg     *config.Config
	client  *cloudtasks.Client
	queueID string
}

// NewGCP returns a new Queue that can be used to enqueue tasks using the
// cloud tasks API.  The given queueID should be the name of the queue in the
// cloud tasks console.
func NewGCP(cfg *config.Config, client *cloudtasks.Client, queueID string) *GCP {
	return &GCP{
		cfg:     cfg,
		client:  client,
		queueID: queueID,
	}
}

// ScheduleFetch enqueues a task on GCP to fetch the given modulePath and
// version. It returns an error if there was an error hashing the task name, or
// an error pushing the task to GCP.
func (q *GCP) ScheduleFetch(ctx context.Context, modulePath, version, suffix string) error {
	// the new taskqueue API requires a deadline of <= 30s
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	queueName := fmt.Sprintf("projects/%s/locations/%s/queues/%s", q.cfg.ProjectID, q.cfg.LocationID, q.queueID)
	u := fmt.Sprintf("/fetch/%s/@v/%s", modulePath, version)
	taskID := newTaskID(modulePath, version, time.Now())
	req := &taskspb.CreateTaskRequest{
		Parent: queueName,
		Task: &taskspb.Task{
			Name: fmt.Sprintf("%s/tasks/%s", queueName, taskID),
			MessageType: &taskspb.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
					HttpMethod:  taskspb.HttpMethod_POST,
					RelativeUri: u,
					AppEngineRouting: &taskspb.AppEngineRouting{
						Service: os.Getenv("GAE_SERVICE"),
					},
				},
			},
		},
	}
	// If suffix is non-empty, append it to the task name. This lets us force reprocessing
	// of tasks that would normally be de-duplicated.
	if suffix != "" {
		req.Task.Name += "-" + suffix
	}

	if _, err := q.client.CreateTask(ctx, req); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			log.Infof(ctx, "ignoring duplicate task ID %s", taskID)
		} else {
			return fmt.Errorf("q.client.CreateTask(ctx, req): %v", err)
		}
	}
	return nil
}

// How often the task ID for a given module and version will change.
const taskIDChangeInterval = 3 * time.Hour

// Create a task ID for the given module path and version.
// Task IDs can contain only letters ([A-Za-z]), numbers ([0-9]), hyphens (-), or underscores (_).
// Also include a truncated time in the hash, so it changes periodically.
//
// Since we truncate the time to the nearest taskIDChangeInterval, it's still possible
// for two identical tasks to appear within that time period (for example, one at 2:59
// and the other at 3:01) -- each is part of a different taskIDChangeInterval-sized chunk
// of time. But there will never be a third identical task in that interval.
func newTaskID(modulePath, version string, now time.Time) string {
	t := now.Truncate(taskIDChangeInterval)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(modulePath+"@"+version+"-"+t.String())))
}

type moduleVersion struct {
	modulePath, version string
}

// InMemory is a Queue implementation that schedules in-process fetch
// operations. Unlike the GCP task queue, it will not automatically retry tasks
// on failure.
//
// This should only be used for local development.
type InMemory struct {
	proxyClient *proxy.Client
	db          *postgres.DB

	queue chan moduleVersion
	sem   chan struct{}
}

// NewInMemory creates a new InMemory that asynchronously fetches
// from proxyClient and stores in db. It uses workerCount parallelism to
// execute these fetches.
func NewInMemory(ctx context.Context, proxyClient *proxy.Client, db *postgres.DB, workerCount int,
	processFunc func(context.Context, string, string, *proxy.Client, *postgres.DB) (int, error)) *InMemory {
	q := &InMemory{
		proxyClient: proxyClient,
		db:          db,
		queue:       make(chan moduleVersion, 1000),
		sem:         make(chan struct{}, workerCount),
	}
	go q.process(ctx, processFunc)
	return q
}

func (q *InMemory) process(ctx context.Context, processFunc func(context.Context, string, string, *proxy.Client, *postgres.DB) (int, error)) {

	for v := range q.queue {
		select {
		case <-ctx.Done():
			return
		case q.sem <- struct{}{}:
		}

		// If a worker is available, make a request to the fetch service inside a
		// goroutine and wait for it to finish.
		go func(v moduleVersion) {
			defer func() { <-q.sem }()

			log.Infof(ctx, "Fetch requested: %q %q (workerCount = %d)", v.modulePath, v.version, cap(q.sem))

			fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			if _, err := processFunc(fetchCtx, v.modulePath, v.version, q.proxyClient, q.db); err != nil {
				log.Error(fetchCtx, err)
			}
		}(v)
	}
}

// ScheduleFetch pushes a fetch task into the local queue to be processed
// asynchronously.
func (q *InMemory) ScheduleFetch(ctx context.Context, modulePath, version, suffix string) error {
	q.queue <- moduleVersion{modulePath, version}
	return nil
}

// WaitForTesting waits for all queued requests to finish. It should only be
// used by test code.
func (q InMemory) WaitForTesting(ctx context.Context) {
	for i := 0; i < cap(q.sem); i++ {
		select {
		case <-ctx.Done():
			return
		case q.sem <- struct{}{}:
		}
	}
	close(q.queue)
}