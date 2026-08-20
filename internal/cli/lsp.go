package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/config"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	glippyformat "github.com/faustbrian/glippy/internal/format"
	"github.com/faustbrian/glippy/internal/goversion"
	"github.com/faustbrian/glippy/internal/lsp"
	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

const lspUsage = "glippy: expected 'lsp [--fix-suggestions] [--fix-unsafe] [--config=<path>]'\n"

const lintRuleDocumentationURL = "https://github.com/faustbrian/gox/blob/main/docs/lint-rules.md#"

const maximumLSPWorkspacePackageEntries = 8

const maximumLSPWorkspaceAccountedBytes int64 = 128 << 20

const maximumLSPWorkspaceChangedFiles = 4096

const indexedLSPSourceMemoryFactor int64 = 16

const compactLSPSourceMemoryFactor int64 = 2

type lspInvocation struct {
	configPath string
	fixSuggestions bool
	fixUnsafe bool
}

type lspAnalysisState struct {
	task lintTask
	packageTask *lintPackageTask
	result analysis.Result
	overlay map[string][]byte
}

type lspPackageGroupKey struct {
	root string
	packageDirectory string
	packageName string
	configuration cache.Digest
	sourceVersion string
	requirement rules.Requirement
}

type lspPackageGroup struct {
	key lspPackageGroupKey
	task lintTask
	members []int
	documents map[string]source.Digest
}

type lspWorkspacePackageEntry struct {
	result analysis.PackageResult
	documents map[string]source.Digest
	rootPackagePaths []string
	dependencyPackagePaths []string
	filesystemFiles map[string]lspWorkspaceFileSnapshot
	sourceDirectories map[string]cache.Digest
	accountedBytes int64
	used uint64
}

type lspWorkspaceFileSnapshot struct {
	digest source.Digest
	exists bool
}

type lspWorkspaceSession struct {
	entries map[lspPackageGroupKey]lspWorkspacePackageEntry
	clock uint64
}

type lspBackend struct {
	registry *rules.Registry
	invocation lspInvocation
	workspaceMu sync.Mutex
	workspace lspWorkspaceSession
	workspaceChangesMu sync.Mutex
	workspaceChangedFiles map[string]struct{}
	workspaceInvalidateAll bool
	packageSessionMu sync.Mutex
	packageSession *analysis.PackageSession
	runPackageAnalysis func(
		context.Context,
		*rules.Registry,
		lintPackageTask,
		map[string][]byte,
	) (analysis.PackageResult, error)
	runPackageGraphDiscovery func(
		context.Context,
		analysis.PackageLoadOptions,
	) (analysis.PackageGraphResult, error)
}

func (b *lspBackend) typedPackageSession() *analysis.PackageSession {
	b.packageSessionMu.Lock()
	defer b.packageSessionMu.Unlock()
	if b.packageSession == nil {
		b.packageSession = analysis.NewPackageSession()
	}
	return b.packageSession
}

func parseLSPInvocation(arguments []string) (lspInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "lsp" {
		return lspInvocation{}, false
	}
	result := lspInvocation{}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--fix-suggestions" && !result.fixSuggestions:
			result.fixSuggestions = true
		case argument == "--fix-unsafe" && !result.fixUnsafe:
			result.fixUnsafe = true
		case strings.HasPrefix(argument, "--config=") && result.configPath == "":
			result.configPath = strings.TrimPrefix(argument, "--config=")
			if result.configPath == "" {
				return lspInvocation{}, false
			}
		case argument == "--config" &&
			result.configPath == "" &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			result.configPath = arguments[index]
		default:
			return lspInvocation{}, false
		}
	}
	return result, true
}

func runLSP(
	ctx context.Context,
	invocation lspInvocation,
	stdin io.Reader,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	err := lsp.Serve(
		ctx,
		stdin,
		stdout,
		&lspBackend{registry: registry, invocation: invocation},
	)
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return report(stderr, ExitCanceled, "glippy lsp: %v\n", err)
	}
	return report(stderr, ExitInternalError, "glippy lsp: %v\n", err)
}

func (b *lspBackend) Analyze(ctx context.Context, document lsp.Document) (lsp.Analysis, error) {
	return b.analyzeWorkspaceDocument(ctx, document, []lsp.Document{document})
}

func (b *lspBackend) AnalyzeWorkspace(
	ctx context.Context,
	documents []lsp.Document,
) ([]lsp.WorkspaceAnalysis, error) {
	b.workspaceMu.Lock()
	defer b.workspaceMu.Unlock()
	changedFiles, invalidateAll := b.takeWorkspaceFileChanges()
	consumedChanges := false
	defer func() {
		if !consumedChanges {
			b.restoreWorkspaceFileChanges(changedFiles, invalidateAll)
		}
	}()

	results := make([]lsp.WorkspaceAnalysis, len(documents))
	groups := make(map[lspPackageGroupKey]int)
	orderedGroups := make([]lspPackageGroup, 0)
	for index, document := range documents {
		results[index].Document = document
		file, err := source.Load(document.Path, document.Text)
		if err != nil {
			results[index].Err = err
			continue
		}
		packageName := ""
		if err := file.ReadSyntax(
			func(syntax *ast.File) error {
				if syntax.Name == nil || syntax.Name.Name == "" {
					return errors.New("editor source has no package name")
				}
				packageName = syntax.Name.Name
				return nil
			},
		);
			err != nil {
			results[index].Err = err
			continue
		}
		task, requirement, err := b.task(document.Path)
		if err != nil {
			results[index].Err = err
			continue
		}
		if requirement < rules.RequireTypes {
			analysis_, err := b.analyzeWorkspaceDocument(ctx, document, documents)
			results[index].Analysis = analysis_
			results[index].Err = err
			continue
		}
		key := lspPackageGroupKey{
			root: task.root,
			packageDirectory: filepath.Dir(document.Path),
			packageName: packageName,
			configuration: task.options.configurationDigest,
			sourceVersion: task.options.sourceGoVersion,
			requirement: requirement,
		}
		groupIndex, found := groups[key]
		if !found {
			groupIndex = len(orderedGroups)
			groups[key] = groupIndex
			orderedGroups = append(
				orderedGroups,
				lspPackageGroup{
					key: key,
					task: task,
					documents: make(map[string]source.Digest),
				},
			)
		}
		orderedGroups[groupIndex].members = append(orderedGroups[groupIndex].members, index)
		orderedGroups[groupIndex].documents[document.Path] = file.Digest()
	}
	sort.Slice(
		orderedGroups,
		func(left, right int) bool {
			return compareLSPPackageGroupKey(
				orderedGroups[left].key,
				orderedGroups[right].key,
			) <
				0
		},
	)
	invalidatedPackages, invalidateRoots := lspWorkspaceInvalidation(
		b.workspace.entries,
		orderedGroups,
	)
	lspWorkspaceWatchedInvalidation(
		b.workspace.entries,
		changedFiles,
		invalidatedPackages,
		invalidateRoots,
	)
	if invalidateAll {
		for _, group := range orderedGroups {
			invalidateRoots[group.key.root] = true
		}
	}
	nextEntries := make(map[lspPackageGroupKey]lspWorkspacePackageEntry)
	type workspaceOverlay struct {
		files map[string][]byte
		err error
	}
	overlays := make(map[string]workspaceOverlay)
	prepareGroup := func(group lspPackageGroup) (lintPackageTask, map[string][]byte, error) {
		patterns := make([]string, 0, len(group.members))
		for _, index := range group.members {
			patterns = append(patterns, "file=" + documents[index].Path)
		}
		slices.Sort(patterns)
		patterns = slices.Compact(patterns)
		packageTask := lintPackageTask{
			root: group.task.root,
			patterns: patterns,
			options: group.task.options,
		}
		resolved, found := overlays[group.task.root]
		if !found {
			resolved.files, resolved.err = editorWorkspaceOverlay(
				group.task.root,
				documents[group.members[0]],
				documents,
			)
			overlays[group.task.root] = resolved
		}
		return packageTask, resolved.files, resolved.err
	}
	type packageAttempt struct {
		result analysis.PackageResult
		err error
	}
	attempts := make(map[lspPackageGroupKey]packageAttempt)
	// Analyze cache misses before making reuse decisions. Their authoritative
	// package paths can then invalidate only actual reverse dependants instead
	// of every retained result under the workspace root.
	for _, group := range orderedGroups {
		if _, cached := b.workspace.entries[group.key]; cached {
			continue
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		packageTask, overlay, packageErr := prepareGroup(group)
		exactOverlay := packageErr == nil
		var packageResult analysis.PackageResult
		if packageErr == nil {
			packageResult, packageErr = b.analyzePackageSnapshot(
				ctx,
				packageTask,
				overlay,
			)
		}
		attempts[group.key] = packageAttempt{result: packageResult, err: packageErr}
		rootPackagePaths := packageResult.RootPackagePaths
		if len(rootPackagePaths) == 0 && packageErr == nil {
			packageErr = errors.New(
				"typed package analysis returned no root package identity",
			)
			attempts[group.key] = packageAttempt{result: packageResult, err: packageErr}
		}
		if len(rootPackagePaths) == 0 && exactOverlay {
			graph, graphErr := b.discoverPackageGraph(
				ctx,
				packageLoadOptions(packageTask, overlay),
			)
			if errors.Is(graphErr, context.Canceled) ||
				errors.Is(graphErr, context.DeadlineExceeded) {
				return nil, graphErr
			}
			if graphErr == nil {
				rootPackagePaths = graph.RootPackagePaths
			}
		}
		if len(rootPackagePaths) == 0 {
			invalidateRoots[group.key.root] = true
			continue
		}
		paths := invalidatedPackages[group.key.root]
		if paths == nil {
			paths = make(map[string]struct{})
			invalidatedPackages[group.key.root] = paths
		}
		for _, path := range rootPackagePaths {
			paths[path] = struct{}{}
		}
	}
	for _, group := range orderedGroups {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		packageTask, overlay, err := prepareGroup(group)
		if err == nil {
			entry, cached := b.workspace.entries[group.key]
			reused := cached &&
				equalLSPDocumentDigests(entry.documents, group.documents) &&
				!invalidateRoots[group.key.root] &&
				!lspPackageEntryIntersects(
					entry,
					invalidatedPackages[group.key.root],
				)
			packageResult := entry.result
			var packageErr error
			if attempt, found := attempts[group.key]; found {
				packageResult = attempt.result
				packageErr = attempt.err
			} else if !reused {
				packageResult, packageErr = b.analyzePackageSnapshot(
					ctx,
					packageTask,
					overlay,
				)
			}
			if packageErr != nil {
				err = packageErr
			} else {
				byPath := make(map[string]analysis.Result, len(packageResult.Files))
				for _, result := range packageResult.Files {
					byPath[result.Path] = result
				}
				complete := true
				for _, index := range group.members {
					document := documents[index]
					result, found := byPath[document.Path]
					if !found {
						complete = false
						results[index].Err = fmt.Errorf(
							"typed editor analysis did not return %q",
							document.Path,
						)
						continue
					}
					bound, found := packageResult.Sources.Lookup(document.Path)
					if !found || !bytes.Equal(bound.Bytes(), document.Text) {
						complete = false
						results[index].Err = errors.New(
							"typed editor analysis does not match the document source",
						)
						continue
					}
					memberTask := group.task
					results[index].Analysis = editorAnalysis(
						bound,
						result,
						&lspAnalysisState{
							task: memberTask,
							packageTask: &packageTask,
							result: result,
							overlay: overlay,
						},
					)
				}
				filesystemFiles, sourceDirectories, snapshotErr := captureLSPWorkspaceFilesystem(
					packageTask,
					packageResult,
					overlay,
				)
				if complete && snapshotErr == nil {
					b.workspace.clock++
					nextEntries[group.key] = lspWorkspacePackageEntry{
						result: packageResult,
						documents: cloneLSPDocumentDigests(group.documents),
						rootPackagePaths: slices.Clone(
							packageResult.RootPackagePaths,
						),
						dependencyPackagePaths: slices.Clone(
							packageResult.DependencyPackagePaths,
						),
						filesystemFiles: filesystemFiles,
						sourceDirectories: sourceDirectories,
						accountedBytes: lspWorkspacePackageAccountedBytes(
							packageResult,
						),
						used: b.workspace.clock,
					}
				}
				continue
			}
		}
		for _, index := range group.members {
			results[index].Err = err
		}
	}
	b.workspace.entries = boundLSPWorkspaceEntries(nextEntries)
	consumedChanges = true
	return results, nil
}

func (b *lspBackend) WorkspaceFilesChanged(ctx context.Context, paths []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("workspace file path %q is not absolute and clean", path)
		}
	}
	b.typedPackageSession().InvalidateAll()
	b.workspaceChangesMu.Lock()
	defer b.workspaceChangesMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.workspaceInvalidateAll {
		return nil
	}
	if b.workspaceChangedFiles == nil {
		b.workspaceChangedFiles = make(map[string]struct{}, len(paths))
	}
	for _, path := range paths {
		b.workspaceChangedFiles[path] = struct{}{}
		if len(b.workspaceChangedFiles) > maximumLSPWorkspaceChangedFiles {
			b.workspaceChangedFiles = nil
			b.workspaceInvalidateAll = true
			return nil
		}
	}
	return nil
}

func (b *lspBackend) takeWorkspaceFileChanges() (map[string]struct{}, bool) {
	b.workspaceChangesMu.Lock()
	defer b.workspaceChangesMu.Unlock()
	files := b.workspaceChangedFiles
	invalidateAll := b.workspaceInvalidateAll
	b.workspaceChangedFiles = nil
	b.workspaceInvalidateAll = false
	return files, invalidateAll
}

func (b *lspBackend) restoreWorkspaceFileChanges(files map[string]struct{}, invalidateAll bool) {
	b.workspaceChangesMu.Lock()
	defer b.workspaceChangesMu.Unlock()
	if invalidateAll || b.workspaceInvalidateAll {
		b.workspaceChangedFiles = nil
		b.workspaceInvalidateAll = true
		return
	}
	if b.workspaceChangedFiles == nil {
		b.workspaceChangedFiles = make(map[string]struct{}, len(files))
	}
	for path := range files {
		b.workspaceChangedFiles[path] = struct{}{}
		if len(b.workspaceChangedFiles) > maximumLSPWorkspaceChangedFiles {
			b.workspaceChangedFiles = nil
			b.workspaceInvalidateAll = true
			return
		}
	}
}

func captureLSPWorkspaceFilesystem(
	task lintPackageTask,
	result analysis.PackageResult,
	overlay map[string][]byte,
) (map[string]lspWorkspaceFileSnapshot, map[string]cache.Digest, error) {
	files := make(map[string]lspWorkspaceFileSnapshot)
	directories := make(map[string]cache.Digest)
	directoryPaths := make(map[string]struct{})
	for _, path := range result.Sources.Paths() {
		if _, overlaid := overlay[path]; !overlaid {
			file, found := result.Sources.Lookup(path)
			if !found {
				return nil, nil, fmt.Errorf(
					"workspace package source %q is missing",
					path,
				)
			}
			files[path] = lspWorkspaceFileSnapshot{digest: file.Digest(), exists: true}
		}
		directory := filepath.Dir(path)
		if pathWithinLSPWorkspace(task.root, directory) {
			directoryPaths[directory] = struct{}{}
		}
	}
	controlFiles := []string{
		filepath.Join(task.root, "go.mod"),
		filepath.Join(task.root, "go.sum"),
		filepath.Join(task.root, "go.work"),
		filepath.Join(task.root, "go.work.sum"),
	}
	if task.options.baseline.Path != "" {
		controlFiles = append(
			controlFiles,
			filepath.Join(task.root, filepath.FromSlash(task.options.baseline.Path)),
		)
	}
	for _, path := range controlFiles {
		contents, err := source.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			files[path] = lspWorkspaceFileSnapshot{}
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		files[path] = lspWorkspaceFileSnapshot{
			digest: source.Digest(sha256.Sum256(contents)),
			exists: true,
		}
	}
	for directory := range directoryPaths {
		digest, err := lspGoDirectoryDigest(directory)
		if err != nil {
			return nil, nil, err
		}
		directories[directory] = digest
	}
	return files, directories, nil
}

func lspWorkspaceFilesystemCurrent(entry lspWorkspacePackageEntry) bool {
	for path, expected := range entry.filesystemFiles {
		contents, err := source.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			if expected.exists {
				return false
			}
			continue
		}
		if err != nil ||
			!expected.exists ||
			source.Digest(sha256.Sum256(contents)) != expected.digest {
			return false
		}
	}
	for directory, expected := range entry.sourceDirectories {
		current, err := lspGoDirectoryDigest(directory)
		if err != nil || current != expected {
			return false
		}
	}
	return true
}

func lspGoDirectoryDigest(directory string) (cache.Digest, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return cache.Digest{}, err
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".go" {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return cache.DigestOf([]byte(strings.Join(names, "\x00"))), nil
}

func pathWithinLSPWorkspace(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".." + string(filepath.Separator))
}

func lspWorkspaceInvalidation(
	previous map[lspPackageGroupKey]lspWorkspacePackageEntry,
	groups []lspPackageGroup,
) (map[string]map[string]struct{}, map[string]bool) {
	invalidated := make(map[string]map[string]struct{})
	invalidateRoots := make(map[string]bool)
	current := make(map[lspPackageGroupKey]lspPackageGroup, len(groups))
	add := func(root string, entry lspWorkspacePackageEntry) {
		paths := invalidated[root]
		if paths == nil {
			paths = make(map[string]struct{})
			invalidated[root] = paths
		}
		if len(entry.rootPackagePaths) == 0 {
			invalidateRoots[root] = true
			return
		}
		for _, path := range entry.rootPackagePaths {
			paths[path] = struct{}{}
		}
	}
	for _, group := range groups {
		current[group.key] = group
		entry, found := previous[group.key]
		if !found {
			continue
		}
		if !equalLSPDocumentDigests(entry.documents, group.documents) ||
			!lspWorkspaceFilesystemCurrent(entry) {
			add(group.key.root, entry)
		}
	}
	for key, entry := range previous {
		if _, found := current[key]; !found {
			add(key.root, entry)
		}
	}
	return invalidated, invalidateRoots
}

func lspWorkspaceWatchedInvalidation(
	previous map[lspPackageGroupKey]lspWorkspacePackageEntry,
	changedFiles map[string]struct{},
	invalidated map[string]map[string]struct{},
	invalidateRoots map[string]bool,
) {
	for key, entry := range previous {
		affected := false
		for path := range changedFiles {
			if _, found := entry.filesystemFiles[path]; found {
				affected = true
				break
			}
			if _, found := entry.sourceDirectories[filepath.Dir(path)]; found {
				affected = true
				break
			}
		}
		if !affected {
			continue
		}
		if len(entry.rootPackagePaths) == 0 {
			invalidateRoots[key.root] = true
			continue
		}
		paths := invalidated[key.root]
		if paths == nil {
			paths = make(map[string]struct{})
			invalidated[key.root] = paths
		}
		for _, path := range entry.rootPackagePaths {
			paths[path] = struct{}{}
		}
	}
}

func lspPackageEntryIntersects(entry lspWorkspacePackageEntry, paths map[string]struct{}) bool {
	for _, path := range entry.rootPackagePaths {
		if _, found := paths[path]; found {
			return true
		}
	}
	for _, path := range entry.dependencyPackagePaths {
		if _, found := paths[path]; found {
			return true
		}
	}
	return false
}

func equalLSPDocumentDigests(left, right map[string]source.Digest) bool {
	if len(left) != len(right) {
		return false
	}
	for path, digest := range left {
		if right[path] != digest {
			return false
		}
	}
	return true
}

func cloneLSPDocumentDigests(input map[string]source.Digest) map[string]source.Digest {
	result := make(map[string]source.Digest, len(input))
	for path, digest := range input {
		result[path] = digest
	}
	return result
}

func boundLSPWorkspaceEntries(
	entries map[lspPackageGroupKey]lspWorkspacePackageEntry,
) map[lspPackageGroupKey]lspWorkspacePackageEntry {
	keys := make([]lspPackageGroupKey, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Slice(
		keys,
		func(left, right int) bool {
			leftEntry, rightEntry := entries[keys[left]], entries[keys[right]]
			if leftEntry.used != rightEntry.used {
				return leftEntry.used > rightEntry.used
			}
			return compareLSPPackageGroupKey(keys[left], keys[right]) < 0
		},
	)
	result := make(
		map[lspPackageGroupKey]lspWorkspacePackageEntry,
		maximumLSPWorkspacePackageEntries,
	)
	var accountedBytes int64
	for _, key := range keys {
		if len(result) == maximumLSPWorkspacePackageEntries {
			break
		}
		entry := entries[key]
		if len(result) != 0 &&
			(entry.accountedBytes > maximumLSPWorkspaceAccountedBytes ||
				accountedBytes >
					maximumLSPWorkspaceAccountedBytes - entry.accountedBytes) {
			continue
		}
		result[key] = entries[key]
		accountedBytes += entry.accountedBytes
	}
	return result
}

func lspWorkspacePackageAccountedBytes(result analysis.PackageResult) int64 {
	var retained int64
	for _, path := range result.Sources.Paths() {
		file, found := result.Sources.Lookup(path)
		if !found {
			return maximumLSPWorkspaceAccountedBytes + 1
		}
		factor := compactLSPSourceMemoryFactor
		if file.CanFormat() {
			factor = indexedLSPSourceMemoryFactor
		}
		size := file.ByteSize()
		if size > (maximumLSPWorkspaceAccountedBytes + 1 - retained) / factor {
			return maximumLSPWorkspaceAccountedBytes + 1
		}
		retained += size * factor
	}
	return retained
}

func compareLSPPackageGroupKey(left, right lspPackageGroupKey) int {
	values := [][2]string{
		{left.root, right.root},
		{left.packageDirectory, right.packageDirectory},
		{left.packageName, right.packageName},
		{left.sourceVersion, right.sourceVersion},
	}
	for _, value := range values {
		if value[0] < value[1] {
			return -1
		}
		if value[0] > value[1] {
			return 1
		}
	}
	if comparison := bytes.Compare(left.configuration[:], right.configuration[:]);
		comparison != 0 {
		return comparison
	}
	if left.requirement < right.requirement {
		return -1
	}
	if left.requirement > right.requirement {
		return 1
	}
	return 0
}

func (b *lspBackend) analyzePackage(
	ctx context.Context,
	task lintPackageTask,
	overlay map[string][]byte,
) (analysis.PackageResult, error) {
	task.options.analysis.PackageSession = b.typedPackageSession()
	if b.runPackageAnalysis != nil {
		return b.runPackageAnalysis(ctx, b.registry, task, overlay)
	}
	return runPackageAnalysisWithOverlay(ctx, b.registry, task, overlay)
}

func (b *lspBackend) analyzePackageSnapshot(
	ctx context.Context,
	task lintPackageTask,
	overlay map[string][]byte,
) (analysis.PackageResult, error) {
	result, err := b.analyzePackage(ctx, task, overlay)
	if err != nil {
		return result, err
	}
	if err := applyConfiguredPackageBaseline(task, &result, b.registry); err != nil {
		return result, err
	}
	if err := validateLintPackagePrerequisites(result); err != nil {
		return result, err
	}
	return result, nil
}

func (b *lspBackend) discoverPackageGraph(
	ctx context.Context,
	options analysis.PackageLoadOptions,
) (analysis.PackageGraphResult, error) {
	if b.runPackageGraphDiscovery != nil {
		return b.runPackageGraphDiscovery(ctx, options)
	}
	return analysis.DiscoverPackageGraph(ctx, options)
}

func (b *lspBackend) analyzeWorkspaceDocument(
	ctx context.Context,
	document lsp.Document,
	documents []lsp.Document,
) (lsp.Analysis, error) {
	file, err := source.Load(document.Path, document.Text)
	if err != nil {
		return lsp.Analysis{}, err
	}
	task, requirement, err := b.task(document.Path)
	if err != nil {
		return lsp.Analysis{}, err
	}
	if requirement < rules.RequireTypes {
		result, err := analysis.Run(ctx, file, b.registry, task.options.analysis)
		if err != nil {
			return lsp.Analysis{}, err
		}
		inputs := []glippyreport.LintTextInput{{File: file, Result: result}}
		results := []analysis.Result{result}
		if err := applyConfiguredBaselines([]lintTask{task}, inputs, results, b.registry);
			err != nil {
			return lsp.Analysis{}, err
		}
		return editorAnalysis(
			file,
			results[0],
			&lspAnalysisState{task: task, result: results[0]},
		), nil
	}
	if task.root == "" {
		return lsp.Analysis{}, errors.New(
			"typed editor diagnostics require a module, workspace, or repository root",
		)
	}
	packageTask := lintPackageTask{
		root: task.root,
		patterns: []string{"file=" + document.Path},
		options: task.options,
	}
	packageTask.options.analysis.PackageSession = b.typedPackageSession()
	overlay, err := editorWorkspaceOverlay(task.root, document, documents)
	if err != nil {
		return lsp.Analysis{}, err
	}
	packageResult, err := runPackageAnalysisWithOverlay(ctx, b.registry, packageTask, overlay)
	if err != nil {
		return lsp.Analysis{}, err
	}
	if err := applyConfiguredPackageBaseline(packageTask, &packageResult, b.registry);
		err != nil {
		return lsp.Analysis{}, err
	}
	if err := validateLintPackagePrerequisites(packageResult); err != nil {
		return lsp.Analysis{}, err
	}
	for _, result := range packageResult.Files {
		if result.Path != document.Path {
			continue
		}
		bound, found := packageResult.Sources.Lookup(result.Path)
		if !found || bound.Digest() != file.Digest() {
			return lsp.Analysis{}, errors.New(
				"typed editor analysis does not match the document source",
			)
		}
		return editorAnalysis(
			bound,
			result,
			&lspAnalysisState{
				task: task,
				packageTask: &packageTask,
				result: result,
				overlay: overlay,
			},
		), nil
	}
	return lsp.Analysis{}, fmt.Errorf("typed editor analysis did not return %q", document.Path)
}

func editorWorkspaceOverlay(
	root string,
	target lsp.Document,
	documents []lsp.Document,
) (map[string][]byte, error) {
	overlay := make(map[string][]byte, len(documents) + 1)
	add := func(document lsp.Document) error {
		path := filepath.Clean(document.Path)
		if !filepath.IsAbs(path) || path != document.Path {
			return fmt.Errorf(
				"editor document path %q is not normalized absolute",
				document.Path,
			)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve editor document %q: %w", path, err)
		}
		if relative == ".." ||
			strings.HasPrefix(relative, ".." + string(filepath.Separator)) {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if previous, found := overlay[path];
			found && !bytes.Equal(previous, document.Text) {
			return fmt.Errorf(
				"editor workspace contains incompatible buffers for %q",
				path,
			)
		}
		overlay[path] = slices.Clone(document.Text)
		return nil
	}
	for _, document := range documents {
		if err := add(document); err != nil {
			return nil, err
		}
	}
	if _, found := overlay[target.Path]; !found {
		if err := add(target); err != nil {
			return nil, err
		}
	}
	if _, found := overlay[target.Path]; !found {
		return nil, fmt.Errorf(
			"typed editor target %q is outside workspace root %q",
			target.Path,
			root,
		)
	}
	return overlay, nil
}

func (b *lspBackend) CodeActions(
	ctx context.Context,
	document lsp.Document,
	analyzed lsp.Analysis,
	requested source.Range,
) ([]lsp.CodeAction, error) {
	state, valid := analyzed.State.(*lspAnalysisState)
	if !valid || state == nil || analyzed.File == nil {
		return nil, errors.New("editor analysis state is unavailable")
	}
	if analyzed.File.Path() != document.Path ||
		!bytes.Equal(analyzed.File.Bytes(), document.Text) {
		return nil, errors.New("editor analysis source is stale")
	}
	if analyzed.File.Metadata().Generated {
		return []lsp.CodeAction{}, nil
	}
	actions := make([]lsp.CodeAction, 0)
	for _, diagnostic := range state.result.Diagnostics {
		if !rangesIntersect(diagnostic.Range, requested) {
			continue
		}
		for _, offered := range diagnostic.Fixes {
			if !b.authorized(offered.Safety) {
				continue
			}
			coordinated, err := b.coordinateEditorFixes(
				ctx,
				analyzed.File,
				state,
				[]fixengine.Selection{
					{Diagnostic: diagnostic, FixName: offered.Name},
				},
				b.invocation.fixSuggestions,
				b.invocation.fixUnsafe,
			)
			if err != nil {
				return nil, err
			}
			if len(coordinated.Applied) != 1 ||
				len(coordinated.Rejected) != 0 ||
				bytes.Equal(coordinated.Bytes, analyzed.File.Bytes()) {
				continue
			}
			actions = append(
				actions,
				lsp.CodeAction{
					Title: fmt.Sprintf(
						"%s: %s [%s]",
						diagnostic.RuleID,
						offered.Name,
						offered.Safety,
					),
					Kind: "quickfix",
					Preferred: offered.Safety == rules.FixSafe,
					DiagnosticCode: diagnostic.RuleID,
					DiagnosticRange: diagnostic.Range,
					DiagnosticSeverity: editorSeverity(diagnostic.Severity),
					DiagnosticMessage: diagnostic.Message,
					DiagnosticDocumentationURI: lintRuleDocumentationURL +
						diagnostic.RuleID,
					DiagnosticWithheldFixes: editorWithheldFixes(
						diagnostic.WithheldFixes,
					),
					NewText: coordinated.Bytes,
				},
			)
		}
	}
	safeSelections, err := fixengine.SelectSafe(state.result.Diagnostics)
	if err != nil {
		safeSelections = nil
	}
	if len(safeSelections) > 0 {
		coordinated, err := b.coordinateEditorFixes(
			ctx,
			analyzed.File,
			state,
			safeSelections,
			false,
			false,
		)
		if err != nil {
			return nil, err
		}
		if len(coordinated.Applied) == len(safeSelections) &&
			len(coordinated.Rejected) == 0 &&
			!bytes.Equal(coordinated.Bytes, analyzed.File.Bytes()) {
			actions = append(
				actions,
				lsp.CodeAction{
					Title: "Fix all safe Glippy findings",
					Kind: "source.fixAll.glippy",
					Preferred: true,
					NewText: coordinated.Bytes,
				},
			)
		}
	}
	return actions, nil
}

func (b *lspBackend) coordinateEditorFixes(
	ctx context.Context,
	file *source.File,
	state *lspAnalysisState,
	selections []fixengine.Selection,
	allowSuggestion bool,
	allowUnsafe bool,
) (fixengine.Result, error) {
	options := fixengine.Options{
		AllowSuggestion: allowSuggestion,
		AllowUnsafe: allowUnsafe,
		Format: state.task.options.format,
	}
	options.Validate = func(formatted *source.File) error {
		if state.packageTask != nil {
			_, _, err := validateLintPackageFix(
				ctx,
				b.registry,
				*state.packageTask,
				formatted,
				state.overlay,
			)
			return err
		}
		result, err := analysis.Run(ctx, formatted, b.registry, state.task.options.analysis)
		if err != nil {
			return err
		}
		inputs := []glippyreport.LintTextInput{{File: formatted, Result: result}}
		results := []analysis.Result{result}
		return applyConfiguredBaselines([]lintTask{state.task}, inputs, results, b.registry)
	}
	return fixengine.Coordinate(file, selections, options)
}

func (b *lspBackend) Format(ctx context.Context, document lsp.Document) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selection, err := config.DiscoverFileContext(document.Path, b.invocation.configPath)
	if err != nil {
		return nil, err
	}
	if _, err := goversion.Resolve(document.Path, selection.Root); err != nil {
		return nil, err
	}
	options, _, err := formatOptionsForSelection(selection)
	if err != nil {
		return nil, err
	}
	file, err := source.Load(document.Path, document.Text)
	if err != nil {
		return nil, err
	}
	formatted, err := glippyformat.File(file, options)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return formatted, nil
}

func (b *lspBackend) task(path string) (lintTask, rules.Requirement, error) {
	selection, err := config.DiscoverFileContext(path, b.invocation.configPath)
	if err != nil {
		return lintTask{}, 0, err
	}
	language, err := goversion.Resolve(path, selection.Root)
	if err != nil {
		return lintTask{}, 0, err
	}
	options, _, err := lintOptionsForSelection(
		selection,
		language.Language,
		b.registry,
		nil,
		nil,
		nil,
	)
	if err != nil {
		return lintTask{}, 0, err
	}
	resolution, err := options.analysis.RuleResolution()
	if err != nil {
		return lintTask{}, 0, err
	}
	selected, err := b.registry.ResolveOptions(resolution)
	if err != nil {
		return lintTask{}, 0, err
	}
	return lintTask{
		root: selection.Root,
		options: options,
	}, rules.MaximumRequirement(selected), nil
}

func (b *lspBackend) authorized(safety rules.FixSafety) bool {
	switch safety {
	case rules.FixSafe:
		return true
	case rules.FixSuggestion:
		return b.invocation.fixSuggestions
	case rules.FixUnsafe:
		return b.invocation.fixUnsafe
	default:
		return false
	}
}

func editorAnalysis(
	file *source.File,
	result analysis.Result,
	state *lspAnalysisState,
) lsp.Analysis {
	diagnostics := make([]lsp.Diagnostic, 0, len(result.Diagnostics))
	for _, diagnostic := range analysis.OrderDiagnostics(result.Diagnostics) {
		related := make([]lsp.Related, len(diagnostic.Related))
		for index, item := range diagnostic.Related {
			related[index] = lsp.Related{Range: item.Range, Message: item.Message}
		}
		severity := editorSeverity(diagnostic.Severity)
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: diagnostic.Range,
				Severity: severity,
				Code: diagnostic.RuleID,
				DocumentationURI: lintRuleDocumentationURL + diagnostic.RuleID,
				Message: diagnostic.Message,
				Related: related,
				WithheldFixes: editorWithheldFixes(diagnostic.WithheldFixes),
			},
		)
	}
	for _, problem := range result.SuppressionProblems {
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: problem.Range,
				Severity: lsp.SeverityWarning,
				Code: "suppression:" + string(problem.Kind),
				Message: problem.Message,
			},
		)
	}
	for _, directive := range result.UnusedSuppressions {
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: directive.Range,
				Severity: lsp.SeverityWarning,
				Code: "suppression:unused",
				Message: "unused suppression for " + directive.RuleID,
			},
		)
	}
	for _, problem := range result.BaselineProblems {
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: source.Range{},
				Severity: lsp.SeverityWarning,
				Code: "baseline:" + string(problem.Kind),
				Message: fmt.Sprintf(
					"%s/%s has %d unmatched occurrence(s)",
					problem.Entry.RuleID,
					problem.Entry.MessageKey,
					problem.Remaining,
				),
			},
		)
	}
	return lsp.Analysis{File: file, Diagnostics: diagnostics, State: state}
}

func editorWithheldFixes(fixes []rules.WithheldFix) []lsp.WithheldFix {
	result := make([]lsp.WithheldFix, len(fixes))
	for index, fix := range fixes {
		result[index] = lsp.WithheldFix{
			Name: fix.Name,
			Reason: string(fix.Reason),
			Message: fix.Message,
		}
	}
	return result
}

func editorSeverity(severity rules.Severity) lsp.Severity {
	if severity == rules.SeverityError {
		return lsp.SeverityError
	}
	return lsp.SeverityWarning
}

func rangesIntersect(left, right source.Range) bool {
	if left.Start == left.End {
		return left.Start >= right.Start && left.Start <= right.End
	}
	if right.Start == right.End {
		return right.Start >= left.Start && right.Start <= left.End
	}
	return left.Start < right.End && right.Start < left.End
}
