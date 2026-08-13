// Package goversion resolves and validates the Go source language version for
// one project path without invoking the Go command.
package goversion

import (
	"errors"
	"fmt"
	"go/version"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

const (
	// Minimum is the oldest source language supported by this Gox release line.
	Minimum = "go1.25"
	// Maximum is the newest source language supported by this Gox release line.
	Maximum = "go1.26"
	// Default applies when no project language directive is available.
	Default = Maximum
)

// Selection is the normalized source language and the directive that selected
// it. Path is empty when the documented default applies.
type Selection struct {
	Language string
	Path string
}

type filesystemError struct {
	err error
}

func (e *filesystemError) Error() string {
	return e.err.Error()
}

func (e *filesystemError) Unwrap() error {
	return e.err
}

// IsFilesystemError reports whether resolving the source version failed while
// inspecting or reading the filesystem rather than because of project policy.
func IsFilesystemError(err error) bool {
	var classified *filesystemError
	return errors.As(err, &classified)
}

// Resolve selects the nearest owning go.mod, then the project go.work, and
// finally Default. inputPath may identify a file that does not exist yet.
func Resolve(inputPath, root string) (Selection, error) {
	start, boundary, err := searchBounds(inputPath, root)
	if err != nil {
		return Selection{}, err
	}
	for directory := start; directory != ""; directory = parentWithin(directory, boundary) {
		path := filepath.Join(directory, "go.mod")
		contents, readErr := os.ReadFile(path)
		switch {
		case readErr == nil:
			parsed, parseErr := modfile.Parse(path, contents, nil)
			if parseErr != nil {
				return Selection{}, fmt.Errorf(
					"parse source language file %q: %w",
					path,
					parseErr,
				)
			}
			if parsed.Go == nil {
				return Selection{Language: Default}, nil
			}
			return validate(parsed.Go.Version, path)
		case !os.IsNotExist(readErr):
			return Selection{}, &filesystemError{
				err: fmt.Errorf("read source language file %q: %w", path, readErr),
			}
		}
	}
	if boundary != "" {
		path := filepath.Join(boundary, "go.work")
		contents, readErr := os.ReadFile(path)
		switch {
		case readErr == nil:
			parsed, parseErr := modfile.ParseWork(path, contents, nil)
			if parseErr != nil {
				return Selection{}, fmt.Errorf(
					"parse source language file %q: %w",
					path,
					parseErr,
				)
			}
			if parsed.Go == nil {
				return Selection{Language: Default}, nil
			}
			return validate(parsed.Go.Version, path)
		case !os.IsNotExist(readErr):
			return Selection{}, &filesystemError{
				err: fmt.Errorf("read source language file %q: %w", path, readErr),
			}
		}
	}
	return Selection{Language: Default}, nil
}

func searchBounds(inputPath, root string) (string, string, error) {
	absoluteInput, err := filepath.Abs(inputPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve source path %q: %w", inputPath, err)
	}
	start := absoluteInput
	if info, statErr := os.Stat(absoluteInput); statErr == nil && info.IsDir() {
		start = absoluteInput
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", "", &filesystemError{
			err: fmt.Errorf("inspect source path %q: %w", absoluteInput, statErr),
		}
	} else {
		start = filepath.Dir(absoluteInput)
	}
	if root == "" {
		return start, "", nil
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve project root %q: %w", root, err)
	}
	relative, err := filepath.Rel(absoluteRoot, start)
	if err != nil ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		(len(relative) > 3 && relative[:3] == ".." + string(filepath.Separator)) {
		return "", "", fmt.Errorf(
			"source path %q is outside project root %q",
			absoluteInput,
			absoluteRoot,
		)
	}
	return start, absoluteRoot, nil
}

func parentWithin(directory, boundary string) string {
	if directory == boundary {
		return ""
	}
	parent := filepath.Dir(directory)
	if parent == directory {
		return ""
	}
	return parent
}

func validate(raw, path string) (Selection, error) {
	language := version.Lang("go" + raw)
	if !version.IsValid(language) {
		return Selection{}, fmt.Errorf(
			"source language file %q has invalid Go version %q",
			path,
			raw,
		)
	}
	if version.Compare(language, Minimum) < 0 || version.Compare(language, Maximum) > 0 {
		return Selection{}, fmt.Errorf(
			"source language file %q selects %s; Gox supports Go 1.25 through Go 1.26",
			path,
			language[2:],
		)
	}
	return Selection{Language: language, Path: path}, nil
}
