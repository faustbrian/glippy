package analysis

import (
	"context"
	"fmt"

	"golang.org/x/tools/go/packages"
)

const packageGraphMetadataLoadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedDeps |
	packages.NeedForTest |
	packages.NeedModule

// PackageGraphResult is one metadata-only package identity snapshot.
type PackageGraphResult struct {
	RootPackagePaths []string
	DependencyPackagePaths []string
	Diagnostics []PackageDiagnostic
}

// DiscoverPackageGraph resolves package identities without retaining syntax,
// types, control flow, SSA, export data, or effect facts.
func DiscoverPackageGraph(
	ctx context.Context,
	options PackageLoadOptions,
) (PackageGraphResult, error) {
	if ctx == nil {
		return PackageGraphResult{}, fmt.Errorf(
			"package graph discovery requires a context",
		)
	}
	if err := ctx.Err(); err != nil {
		return PackageGraphResult{}, err
	}
	patterns, limits, buildFlags, err := preparePackageLoadRequest(options)
	if err != nil {
		return PackageGraphResult{}, err
	}
	loaded, err := packages.Load(
		&packages.Config{
			Context: ctx,
			Mode: packageGraphMetadataLoadMode,
			Dir: options.Dir,
			Env: packageLoadEnvironment(options),
			BuildFlags: buildFlags,
			Tests: options.Tests,
			Overlay: cloneOverlay(options.Overlay),
		},
		patterns...,
	)
	if contextErr := ctx.Err(); contextErr != nil {
		return PackageGraphResult{}, contextErr
	}
	if err != nil {
		return PackageGraphResult{}, fmt.Errorf("discover Go package graph: %w", err)
	}
	if err := validatePackageGraphLimit(loaded, limits.maxPackages); err != nil {
		return PackageGraphResult{}, err
	}
	ordered, err := canonicalPackages(loaded)
	if err != nil {
		return PackageGraphResult{}, err
	}
	rootPaths, dependencyPaths, err := packageGraphPaths(ordered)
	if err != nil {
		return PackageGraphResult{}, err
	}
	return PackageGraphResult{
		RootPackagePaths: rootPaths,
		DependencyPackagePaths: dependencyPaths,
		Diagnostics: packageDiagnostics(ordered),
	}, nil
}
