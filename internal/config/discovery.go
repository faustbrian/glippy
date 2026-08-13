package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Selection identifies the project boundary and configuration for one input.
type Selection struct {
	Root string
	Path string
	Explicit bool
}

// Discover selects one project configuration for an input path.
func Discover(inputPath, explicitPath string) (Selection, error) {
	return discover(inputPath, explicitPath, true)
}

// DiscoverFileContext selects configuration for an editor file path that may not exist yet.
func DiscoverFileContext(inputPath, explicitPath string) (Selection, error) {
	return discover(inputPath, explicitPath, false)
}

func discover(inputPath, explicitPath string, requireInput bool) (Selection, error) {
	absoluteInput, err := filepath.Abs(inputPath)
	if err != nil {
		return Selection{}, fmt.Errorf("resolve input path %q: %w", inputPath, err)
	}
	info, err := os.Stat(absoluteInput)
	if err != nil && (requireInput || !os.IsNotExist(err)) {
		return Selection{}, fmt.Errorf("inspect input path %q: %w", absoluteInput, err)
	}
	if !requireInput && err == nil && info.IsDir() {
		return Selection{}, fmt.Errorf("editor file path %q is a directory", absoluteInput)
	}
	start := absoluteInput
	if err != nil || !info.IsDir() {
		start = filepath.Dir(start)
	}
	root, err := findProjectRoot(start)
	if err != nil {
		return Selection{}, err
	}
	if explicitPath != "" {
		absoluteConfiguration, err := filepath.Abs(explicitPath)
		if err != nil {
			return Selection{}, fmt.Errorf(
				"resolve configuration path %q: %w",
				explicitPath,
				err,
			)
		}
		info, err := os.Stat(absoluteConfiguration)
		if err != nil {
			return Selection{}, fmt.Errorf(
				"inspect configuration path %q: %w",
				absoluteConfiguration,
				err,
			)
		}
		if !info.Mode().IsRegular() {
			return Selection{}, fmt.Errorf(
				"configuration path %q is not a regular file",
				absoluteConfiguration,
			)
		}
		return Selection{Root: root, Path: absoluteConfiguration, Explicit: true}, nil
	}
	if root == "" {
		return Selection{}, nil
	}
	configurationPath, err := findConfiguration(root)
	if err != nil {
		return Selection{}, err
	}
	return Selection{Root: root, Path: configurationPath}, nil
}

func findProjectRoot(start string) (string, error) {
	for directory := start; ; directory = filepath.Dir(directory) {
		for _, marker := range []string{"go.work", "go.mod", ".git"} {
			_, err := os.Lstat(filepath.Join(directory, marker))
			switch {
			case err == nil:
				return directory, nil
			case !os.IsNotExist(err):
				return "", fmt.Errorf(
					"inspect project marker in %q: %w",
					directory,
					err,
				)
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", nil
		}
	}
}

func findConfiguration(root string) (string, error) {
	var canonical string
	var legacy string
	for directory := root; ; directory = filepath.Dir(directory) {
		for _, name := range []string{Filename, LegacyFilename} {
			candidate := filepath.Join(directory, name)
			info, err := os.Stat(candidate)
			switch {
			case err == nil && !info.Mode().IsRegular():
				return "", fmt.Errorf(
					"configuration path %q is not a regular file",
					candidate,
				)
			case err == nil && name == Filename && canonical == "":
				canonical = candidate
			case err == nil && name == LegacyFilename && legacy == "":
				legacy = candidate
			case err != nil && !os.IsNotExist(err):
				return "", fmt.Errorf(
					"inspect configuration path %q: %w",
					candidate,
					err,
				)
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
	}
	if canonical != "" && legacy != "" {
		return "", fmt.Errorf(
			"both %s and %s were found; remove the legacy configuration or select one explicitly with --config",
			Filename,
			LegacyFilename,
		)
	}
	if canonical != "" {
		return canonical, nil
	}
	return legacy, nil
}
