package cli

import (
	"errors"
	"fmt"
	"go/version"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/faustbrian/glippy/internal/analysis"
	"golang.org/x/mod/modfile"
)

func automaticPackageModuleMode(
	directory string,
	environment []string,
) (analysis.ModuleMode, error) {
	controlPath, workspace, err := effectivePackageControlFile(directory, environment)
	if err != nil || controlPath == "" {
		return analysis.ModuleReadonly, err
	}
	contents, err := os.ReadFile(controlPath)
	if err != nil {
		return analysis.ModuleReadonly, fmt.Errorf(
			"read module selection file %q: %w",
			controlPath,
			err,
		)
	}
	goVersion, err := packageControlGoVersion(controlPath, contents, workspace)
	if err != nil {
		return analysis.ModuleReadonly, err
	}
	if goVersion == "" || version.Compare("go" + goVersion, "go1.14") < 0 {
		return analysis.ModuleReadonly, nil
	}
	vendorRoot := filepath.Join(filepath.Dir(controlPath), "vendor")
	info, err := os.Stat(vendorRoot)
	if errors.Is(err, os.ErrNotExist) || err == nil && !info.IsDir() {
		return analysis.ModuleReadonly, nil
	}
	if err != nil {
		return analysis.ModuleReadonly, fmt.Errorf(
			"inspect vendor directory %q: %w",
			vendorRoot,
			err,
		)
	}
	manifest := filepath.Join(vendorRoot, "modules.txt")
	manifestWorkspace, found := packageVendorManifestWorkspace(manifest)
	if !found || manifestWorkspace != workspace {
		return analysis.ModuleReadonly, nil
	}
	return analysis.ModuleVendor, nil
}

func effectivePackageControlFile(directory string, environment []string) (string, bool, error) {
	goWork := packageEnvironmentValue(environment, "GOWORK")
	switch goWork {
	case "off":
	case "", "auto":
		workspace, err := findPackageControlFile(directory, "go.work")
		if err != nil {
			return "", false, err
		}
		if workspace != "" {
			return workspace, true, nil
		}
	default:
		if !filepath.IsAbs(goWork) || filepath.Clean(goWork) != goWork {
			return "", false, fmt.Errorf(
				"GOWORK must be off, auto, or a normalized absolute path",
			)
		}
		info, err := os.Stat(goWork)
		if err != nil {
			return "", false, fmt.Errorf("inspect GOWORK file %q: %w", goWork, err)
		}
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("GOWORK path %q is not a regular file", goWork)
		}
		return goWork, true, nil
	}
	module, err := findPackageControlFile(directory, "go.mod")
	return module, false, err
}

func findPackageControlFile(directory, name string) (string, error) {
	for current := directory; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, name)
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.Mode().IsRegular():
			return candidate, nil
		case err == nil:
			return "", fmt.Errorf(
				"module selection path %q is not a regular file",
				candidate,
			)
		case !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf(
				"inspect module selection path %q: %w",
				candidate,
				err,
			)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil
		}
	}
}

func packageControlGoVersion(path string, contents []byte, workspace bool) (string, error) {
	if workspace {
		parsed, err := modfile.ParseWork(path, contents, nil)
		if err != nil {
			return "", fmt.Errorf("parse module selection file %q: %w", path, err)
		}
		if parsed.Go != nil {
			return parsed.Go.Version, nil
		}
		return "", nil
	}
	parsed, err := modfile.Parse(path, contents, nil)
	if err != nil {
		return "", fmt.Errorf("parse module selection file %q: %w", path, err)
	}
	if parsed.Go != nil {
		return parsed.Go.Version, nil
	}
	return "", nil
}

func packageVendorManifestWorkspace(path string) (bool, bool) {
	file, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer file.Close()
	var prefix [512]byte
	count, readErr := file.Read(prefix[:])
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, false
	}
	line, _, _ := strings.Cut(string(prefix[:count]), "\n")
	annotations, annotated := strings.CutPrefix(line, "## ")
	if !annotated {
		return false, true
	}
	for _, annotation := range strings.Split(annotations, ";") {
		if strings.TrimSpace(annotation) == "workspace" {
			return true, true
		}
	}
	return false, true
}

func packageEnvironmentValue(environment []string, name string) string {
	prefix := name + "="
	for index := len(environment) - 1; index >= 0; index-- {
		if strings.HasPrefix(environment[index], prefix) {
			return strings.TrimPrefix(environment[index], prefix)
		}
	}
	return ""
}
