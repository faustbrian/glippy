// Package discovery owns deterministic filesystem input selection.
package discovery

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is one normalized Go source selected from an immutable discovery pass.
type File struct {
	Path     string
	Explicit bool
	Symlink  bool
}

// Options defines the authorized recursive discovery boundary.
type Options struct {
	Root string
}

// GoFiles returns normalized Go source paths in deterministic order.
func GoFiles(ctx context.Context, inputs []string, options Options) ([]File, error) {
	root, err := normalizeRoot(options.Root)
	if err != nil {
		return nil, err
	}
	selected := make(map[string]File)
	for _, input := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path, err := filepath.Abs(input)
		if err != nil {
			return nil, fmt.Errorf("resolve input path %q: %w", input, err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect input path %q: %w", path, err)
		}
		if root != "" && withinRoot(root, path) && excludedWithinRoot(root, path) {
			return nil, fmt.Errorf("input %q is excluded by project policy", path)
		}
		if info.IsDir() {
			if root == "" || !withinRoot(root, path) {
				return nil, fmt.Errorf("recursive input %q is outside the authorized project root", path)
			}
			hasSymlink, err := hasSymlinkComponent(root, path)
			if err != nil {
				return nil, fmt.Errorf("inspect recursive input %q: %w", path, err)
			}
			if hasSymlink {
				return nil, fmt.Errorf("recursive input %q crosses a symlink", path)
			}
			if err := walkGoFiles(ctx, path, fixtureWithinRoot(root, path), selected); err != nil {
				return nil, err
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Stat(path)
			if err != nil {
				return nil, fmt.Errorf("inspect symlink input %q: %w", path, err)
			}
			if !target.Mode().IsRegular() {
				return nil, fmt.Errorf("input path %q is not a regular file", path)
			}
		} else if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("input path %q is not a regular file", path)
		}
		if filepath.Ext(path) != ".go" {
			return nil, fmt.Errorf("input path %q is not a Go source file", path)
		}
		selected[path] = File{
			Path:     path,
			Explicit: true,
			Symlink:  info.Mode()&os.ModeSymlink != 0,
		}
	}
	files := make([]File, 0, len(selected))
	for _, file := range selected {
		files = append(files, file)
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	return files, nil
}

func normalizeRoot(root string) (string, error) {
	if root == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root %q: %w", root, err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect project root %q: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root %q is not a directory", absolute)
	}
	return absolute, nil
}

func withinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func excludedWithinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if excludedDirectory(component) {
			return true
		}
	}
	return false
}

func fixtureWithinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if isFixtureDirectory(component) {
			return true
		}
	}
	return false
}

func hasSymlinkComponent(root, path string) (bool, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}

func walkGoFiles(ctx context.Context, root string, includeFixtures bool, selected map[string]File) error {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path != root && entry.IsDir() {
			if excludedDirectory(entry.Name()) || (!includeFixtures && isFixtureDirectory(entry.Name())) {
				return filepath.SkipDir
			}
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(path) != ".go" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			if _, exists := selected[path]; !exists {
				selected[path] = File{Path: path}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("discover Go files below %q: %w", root, err)
	}
	return nil
}

func excludedDirectory(name string) bool {
	switch name {
	case "vendor", ".git", ".hg", ".svn", ".bzr", ".jj":
		return true
	default:
		return false
	}
}

func isFixtureDirectory(name string) bool {
	return name == "testdata" || name == "fixtures"
}
