// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"

	"golang.org/x/pkgsite/internal/licenses"
)

// DataSource is the interface used by the frontend to interact with module data.
type DataSource interface {
	// See the internal/postgres package for further documentation of these
	// methods, particularly as they pertain to the main postgres implementation.

	// GetDirectoryNew returns information about a directory, which may also be a module and/or package.
	// The module and version must both be known.
	GetDirectoryNew(ctx context.Context, dirPath, modulePath, version string) (_ *VersionedDirectory, err error)
	// GetImports returns a slice of import paths imported by the package
	// specified by path and version.
	GetImports(ctx context.Context, pkgPath, modulePath, version string) ([]string, error)
	// GetModuleInfo returns the LegacyModuleInfo corresponding to modulePath and
	// version.
	GetModuleInfo(ctx context.Context, modulePath, version string) (*LegacyModuleInfo, error)
	// GetPathInfo returns information about a path.
	GetPathInfo(ctx context.Context, path, inModulePath, inVersion string) (outModulePath, outVersion string, isPackage bool, err error)
	// GetPseudoVersionsForModule returns LegacyModuleInfo for all known
	// pseudo-versions for the module corresponding to modulePath.
	GetPseudoVersionsForModule(ctx context.Context, modulePath string) ([]*LegacyModuleInfo, error)
	// GetPseudoVersionsForModule returns LegacyModuleInfo for all known
	// pseudo-versions for any module containing a package with the given import
	// path.
	GetPseudoVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*LegacyModuleInfo, error)
	// GetTaggedVersionsForModule returns LegacyModuleInfo for all known tagged
	// versions for the module corresponding to modulePath.
	GetTaggedVersionsForModule(ctx context.Context, modulePath string) ([]*LegacyModuleInfo, error)
	// GetTaggedVersionsForModule returns LegacyModuleInfo for all known tagged
	// versions for any module containing a package with the given import path.
	GetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*LegacyModuleInfo, error)

	// TODO(b/155474770): Deprecate these methods.
	//
	// GetDirectory returns packages whose import path is in a (possibly
	// nested) subdirectory of the given directory path. When multiple
	// package paths satisfy this query, it should prefer the module with
	// the longest path.
	GetDirectory(ctx context.Context, dirPath, modulePath, version string, fields FieldSet) (_ *LegacyDirectory, err error)
	// GetModuleLicenses returns all top-level Licenses for the given modulePath
	// and version. (i.e., Licenses contained in the module root directory)
	GetModuleLicenses(ctx context.Context, modulePath, version string) ([]*licenses.License, error)
	// GetPackage returns the LegacyVersionedPackage corresponding to the given package
	// pkgPath, modulePath, and version. When multiple package paths satisfy this query, it
	// should prefer the module with the longest path.
	GetPackage(ctx context.Context, pkgPath, modulePath, version string) (*LegacyVersionedPackage, error)
	// GetPackageLicenses returns all Licenses that apply to pkgPath, within the
	// module version specified by modulePath and version.
	GetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) ([]*licenses.License, error)
	// GetPackagesInModule returns LegacyPackages contained in the module version
	// specified by modulePath and version.
	GetPackagesInModule(ctx context.Context, modulePath, version string) ([]*LegacyPackage, error)
}
