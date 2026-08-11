package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/build"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/cache"
	"github.com/faustbrian/gox/internal/rules"
	goxversion "github.com/faustbrian/gox/internal/version"
)

const (
	cacheDirectoryEnvironment = "GOX_CACHE_DIR"
	sourceGoVersion           = "1.26"
	formatterCacheMode        = "gox-v1"
)

var (
	developmentCacheToolIdentityOnce sync.Once
	developmentCacheToolIdentity     string
	developmentCacheToolIdentityErr  error
)

type packageAnalysisError struct {
	exitCode int
	err      error
}

func (e *packageAnalysisError) Error() string { return e.err.Error() }

func (e *packageAnalysisError) Unwrap() error { return e.err }

func newPackageAnalysisError(exitCode int, format string, arguments ...any) error {
	return &packageAnalysisError{exitCode: exitCode, err: fmt.Errorf(format, arguments...)}
}

func packageAnalysisErrorExitCode(err error) int {
	exitCode := exitCodeForError(ExitInternalError, err)
	if exitCode == ExitCanceled {
		return exitCode
	}
	var classified *packageAnalysisError
	if errors.As(err, &classified) {
		return classified.exitCode
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return ExitFilesystemError
	}
	return exitCode
}

func runPackageAnalysis(
	ctx context.Context,
	registry *rules.Registry,
	task lintPackageTask,
) (analysis.PackageResult, error) {
	loadOptions := analysis.PackageLoadOptions{
		Dir:        task.root,
		Patterns:   task.patterns,
		Tests:      true,
		ModuleMode: analysis.ModuleReadonly,
	}
	if !task.options.cache.Enabled {
		return analysis.RunPackages(ctx, registry, task.options.analysis, loadOptions)
	}

	cgoEnabled, err := configuredCGOEnabled()
	if err != nil {
		return analysis.PackageResult{}, newPackageAnalysisError(
			ExitInvalidInvocation,
			"%w",
			err,
		)
	}
	loadOptions.GOOS = configuredTarget("GOOS", runtime.GOOS)
	loadOptions.GOARCH = configuredTarget("GOARCH", runtime.GOARCH)
	loadOptions.Env = packageCacheEnvironment(cgoEnabled)
	root, err := packageCacheRoot()
	if err != nil {
		return analysis.PackageResult{}, err
	}
	if err := requireExternalCacheRoot(root, task.root); err != nil {
		return analysis.PackageResult{}, err
	}
	toolIdentity, err := currentCacheToolIdentity()
	if err != nil {
		return analysis.PackageResult{}, newPackageAnalysisError(
			ExitFilesystemError,
			"identify development binary for analysis cache: %w",
			err,
		)
	}
	store, err := cache.Open(root)
	if err != nil {
		return analysis.PackageResult{}, newPackageAnalysisError(
			ExitFilesystemError,
			"open analysis cache: %w",
			err,
		)
	}
	runOptions := task.options.analysis
	runOptions.Cache = &analysis.PackageCacheOptions{
		Store:           store,
		ToolVersion:     toolIdentity,
		BuildGoVersion:  runtime.Version(),
		SourceGoVersion: sourceGoVersion,
		Configuration:   task.options.configurationDigest,
		CGOEnabled:      cgoEnabled,
		FormatterMode:   formatterCacheMode,
	}
	result, runErr := analysis.RunPackages(ctx, registry, runOptions, loadOptions)
	var pruneErr error
	if ctx.Err() == nil {
		_, pruneErr = store.Prune(ctx, cache.PruneOptions{
			MaxEntries: task.options.cache.MaxEntries,
			MaxBytes:   task.options.cache.MaxBytes,
		})
		if pruneErr != nil {
			pruneErr = newPackageAnalysisError(
				ExitFilesystemError,
				"prune analysis cache: %w",
				pruneErr,
			)
		}
	}
	closeErr := store.Close()
	if closeErr != nil {
		closeErr = newPackageAnalysisError(
			ExitFilesystemError,
			"close analysis cache: %w",
			closeErr,
		)
	}
	return result, errors.Join(runErr, pruneErr, closeErr)
}

func currentCacheToolIdentity() (string, error) {
	version := goxversion.Current()
	if version != "devel" {
		return version, nil
	}
	developmentCacheToolIdentityOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			developmentCacheToolIdentityErr = err
			return
		}
		file, err := os.Open(path)
		if err != nil {
			developmentCacheToolIdentityErr = err
			return
		}
		developmentCacheToolIdentity, developmentCacheToolIdentityErr =
			resolvedCacheToolIdentity(version, file)
		if closeErr := file.Close(); closeErr != nil {
			developmentCacheToolIdentityErr = errors.Join(
				developmentCacheToolIdentityErr,
				closeErr,
			)
		}
	})
	return developmentCacheToolIdentity, developmentCacheToolIdentityErr
}

func resolvedCacheToolIdentity(version string, executable io.Reader) (string, error) {
	if version != "devel" {
		return version, nil
	}
	if executable == nil {
		return "", fmt.Errorf("development binary is required")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, executable); err != nil {
		return "", err
	}
	return "devel-sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func packageCacheRoot() (string, error) {
	if root := os.Getenv(cacheDirectoryEnvironment); root != "" {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return "", newPackageAnalysisError(
				ExitInvalidInvocation,
				"%s must be a normalized absolute path",
				cacheDirectoryEnvironment,
			)
		}
		return root, nil
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", newPackageAnalysisError(
			ExitFilesystemError,
			"resolve user cache directory: %w",
			err,
		)
	}
	root, err = filepath.Abs(filepath.Join(root, "gox", "analysis"))
	if err != nil {
		return "", newPackageAnalysisError(
			ExitFilesystemError,
			"resolve analysis cache directory: %w",
			err,
		)
	}
	return filepath.Clean(root), nil
}

func configuredCGOEnabled() (bool, error) {
	value, configured := os.LookupEnv("CGO_ENABLED")
	if !configured || value == "" {
		return build.Default.CgoEnabled, nil
	}
	switch value {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("CGO_ENABLED must be 0 or 1 when persistent caching is enabled")
	}
}

func configuredTarget(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func packageCacheEnvironment(cgoEnabled bool) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if found && name != "" {
			values[name] = value
		}
	}
	values["GOENV"] = "off"
	if cgoEnabled {
		values["CGO_ENABLED"] = "1"
	} else {
		values["CGO_ENABLED"] = "0"
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, len(names))
	for index, name := range names {
		result[index] = name + "=" + values[name]
	}
	return result
}

func requireExternalCacheRoot(root, projectRoot string) error {
	resolvedRoot, err := resolveProspectivePath(root)
	if err != nil {
		return newPackageAnalysisError(
			ExitFilesystemError,
			"resolve analysis cache root: %w",
			err,
		)
	}
	resolvedProject, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return newPackageAnalysisError(
			ExitFilesystemError,
			"resolve project root for analysis cache: %w",
			err,
		)
	}
	inside, err := pathWithin(resolvedProject, resolvedRoot)
	if err != nil {
		return newPackageAnalysisError(ExitFilesystemError, "%w", err)
	}
	if inside {
		return newPackageAnalysisError(
			ExitInvalidInvocation,
			"analysis cache root must remain outside project root %q",
			projectRoot,
		)
	}
	return nil
}

func resolveProspectivePath(path string) (string, error) {
	suffix := make([]string, 0)
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}

func pathWithin(root, candidate string) (bool, error) {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, fmt.Errorf("compare analysis cache and project roots: %w", err)
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}
