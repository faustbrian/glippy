// Package version resolves deterministic binary version metadata.
package version

import "runtime/debug"

var linked string

// Current returns the release, installed-module, or development version.
func Current() string {
	build, available := debug.ReadBuildInfo()
	if !available {
		build = nil
	}
	return Resolve(linked, build)
}

// Resolve gives explicit release metadata precedence over Go module build data.
func Resolve(release string, build *debug.BuildInfo) string {
	if release != "" {
		return release
	}
	if build != nil && build.Main.Version != "" && build.Main.Version != "(devel)" {
		return build.Main.Version
	}
	return "devel"
}
