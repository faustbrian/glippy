// Package changed resolves source lines owned by a Git change relative to a
// deterministic merge base.
package changed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	glippydiff "github.com/faustbrian/glippy/internal/diff"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

var hunkHeader = regexp.MustCompile(`(?m)^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

var unifiedHunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+[0-9]+(?:,[0-9]+)? @@`)

// LineRange is one inclusive one-based physical line interval.
type LineRange struct {
	Start int
	End int
}

type fileChange struct {
	ranges []LineRange
	whole bool
	digest source.Digest
	versioned bool
}

// Scope is one immutable repository change relative to a resolved merge base.
type Scope struct {
	root string
	mergeBase string
	files map[string]fileChange
}

// Resolve discovers the containing Git repository and computes current source
// lines changed since the merge base shared by base and HEAD.
func Resolve(ctx context.Context, anchor, base string) (*Scope, error) {
	if ctx == nil {
		return nil, errors.New("changed-code scope requires a context")
	}
	if strings.TrimSpace(base) == "" {
		return nil, errors.New("changed-code base is required")
	}
	if strings.HasPrefix(base, "-") {
		return nil, errors.New("changed-code base must not begin with '-'")
	}
	if strings.IndexFunc(base, unicode.IsControl) >= 0 {
		return nil, errors.New("changed-code base must not contain control characters")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	anchor, err := changedAnchor(anchor)
	if err != nil {
		return nil, err
	}
	rootOutput, err := runGit(ctx, anchor, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve Git repository for %q: %w", anchor, err)
	}
	root, err := filepath.Abs(strings.TrimSpace(string(rootOutput)))
	if err != nil {
		return nil, fmt.Errorf("normalize Git repository root: %w", err)
	}
	root = filepath.Clean(root)
	mergeOutput, err := runGit(ctx, root, "merge-base", "--all", base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve merge base for %q: %w", base, err)
	}
	mergeBases := strings.Fields(string(mergeOutput))
	if len(mergeBases) == 0 {
		return nil, fmt.Errorf("resolve merge base for %q: no common ancestor", base)
	}
	sort.Strings(mergeBases)
	scope := &Scope{root: root, mergeBase: mergeBases[0], files: make(map[string]fileChange)}
	filterOverrides, err := gitFilterOverrides(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("resolve Git filter policy: %w", err)
	}
	status, err := runGitConfigured(
		ctx,
		root,
		filterOverrides,
		"diff",
		"--name-status",
		"-z",
		"--find-renames=50%",
		"--no-ext-diff",
		"--no-textconv",
		scope.mergeBase,
		"--",
	)
	if err != nil {
		return nil, fmt.Errorf("list files changed from %s: %w", scope.mergeBase, err)
	}
	paths, err := currentChangedPaths(status)
	if err != nil {
		return nil, err
	}
	for _, changed := range paths {
		path, err := scope.repositoryPath(changed.current)
		if err != nil {
			return nil, err
		}
		arguments := []string{
			"diff",
			"--unified=0",
			"--text",
			"--find-renames=50%",
			"--no-ext-diff",
			"--no-textconv",
			scope.mergeBase,
			"--",
		}
		if changed.previous != "" {
			arguments = append(arguments, changed.previous)
		}
		arguments = append(arguments, changed.current)
		patch, err := runGitConfigured(ctx, root, filterOverrides, arguments...)
		if err != nil {
			return nil, fmt.Errorf(
				"inspect changed lines for %q: %w",
				changed.current,
				err,
			)
		}
		change, err := bindSourceVersion(
			path,
			fileChange{ranges: parseNewLineRanges(patch)},
		)
		if err != nil {
			return nil, err
		}
		scope.files[path] = change
	}
	untracked, err := runGitConfigured(
		ctx,
		root,
		filterOverrides,
		"ls-files",
		"--others",
		"--exclude-standard",
		"-z",
		"--",
	)
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}
	for _, relative := range splitNUL(untracked) {
		path, err := scope.repositoryPath(relative)
		if err != nil {
			return nil, err
		}
		change, err := bindSourceVersion(path, fileChange{whole: true})
		if err != nil {
			return nil, err
		}
		scope.files[path] = change
	}
	return scope, nil
}

// Root returns the normalized absolute repository root.
func (s *Scope) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// MergeBase returns the selected full Git object identity.
func (s *Scope) MergeBase() string {
	if s == nil {
		return ""
	}
	return s.mergeBase
}

// Changed reports whether path is a current changed or untracked path.
func (s *Scope) Changed(path string) bool {
	if s == nil {
		return false
	}
	_, found := s.files[canonicalPath(path)]
	return found
}

// Contains reports whether path resolves within the repository root.
func (s *Scope) Contains(path string) bool {
	if s == nil {
		return false
	}
	relative, err := filepath.Rel(s.root, canonicalPath(path))
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".." + string(filepath.Separator))
}

// WholeFile reports whether every line of path is owned by the change.
func (s *Scope) WholeFile(path string) bool {
	if s == nil {
		return false
	}
	change, found := s.files[canonicalPath(path)]
	return found && change.whole
}

// Lines returns changed physical lines for path.
func (s *Scope) Lines(path string) []LineRange {
	if s == nil {
		return nil
	}
	return slices.Clone(s.files[canonicalPath(path)].ranges)
}

// FilterDiagnostics retains diagnostics intersecting changed lines, records
// pre-existing diagnostics separately, and exposes only fixes wholly owned by
// the changed lines.
func (s *Scope) FilterDiagnostics(
	file *source.File,
	diagnostics []rules.Diagnostic,
) ([]rules.Diagnostic, []rules.Diagnostic, error) {
	if s == nil || file == nil {
		return nil, nil, errors.New(
			"changed-code filtering requires a scope and source file",
		)
	}
	visible := make([]rules.Diagnostic, 0, len(diagnostics))
	preexisting := make([]rules.Diagnostic, 0)
	for _, diagnostic := range diagnostics {
		if diagnostic.Path != file.Path() || diagnostic.Digest != file.Digest() {
			return nil, nil, fmt.Errorf(
				"diagnostic source identity does not match %q",
				file.Path(),
			)
		}
		intersects, err := s.intersects(file, diagnostic.Range)
		if err != nil {
			return nil, nil, err
		}
		if !intersects {
			preexisting = append(preexisting, diagnostic)
			continue
		}
		filtered := diagnostic
		filtered.Fixes = make([]rules.Fix, 0, len(diagnostic.Fixes))
		for _, fix := range diagnostic.Fixes {
			owned := true
			for _, edit := range fix.Edits {
				editOwned, err := s.owns(file, edit.Range)
				if err != nil {
					return nil, nil, err
				}
				if !editOwned {
					owned = false
					break
				}
			}
			if owned {
				filteredFix := fix
				filteredFix.Edits = slices.Clone(fix.Edits)
				filtered.Fixes = append(filtered.Fixes, filteredFix)
			}
		}
		visible = append(visible, filtered)
	}
	return visible, preexisting, nil
}

// OwnsTransformation reports whether every original source line changed by
// after belongs to the changed-code scope.
func (s *Scope) OwnsTransformation(file *source.File, after []byte) (bool, error) {
	if s == nil || file == nil {
		return false, errors.New(
			"changed-code transformation requires a scope and source file",
		)
	}
	if !s.Contains(file.Path()) {
		return false, fmt.Errorf(
			"changed-code source %q is outside Git root %q",
			file.Path(),
			s.Root(),
		)
	}
	change, found, err := s.sourceChange(file)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if change.whole {
		return true, nil
	}
	before := file.Bytes()
	if bytes.Equal(before, after) {
		return true, nil
	}
	lineCount := bytes.Count(before, []byte{'\n'})
	if len(before) > 0 && before[len(before) - 1] != '\n' {
		lineCount++
	}
	for _, line := range transformationLines(before, after, lineCount) {
		owned := false
		for _, candidate := range change.ranges {
			if candidate.Start <= line && line <= candidate.End {
				owned = true
				break
			}
		}
		if !owned {
			return false, nil
		}
	}
	return true, nil
}

func transformationLines(before, after []byte, lineCount int) []int {
	difference := glippydiff.Unified("before", "after", before, after)
	lines := strings.Split(difference, "\n")
	changedLines := make([]int, 0)
	oldLine := 0
	lastDeleted := 0
	inHunk := false
	for _, line := range lines {
		if match := unifiedHunkHeader.FindStringSubmatch(line); match != nil {
			oldLine, _ = strconv.Atoi(match[1])
			lastDeleted = 0
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		switch line[0] {
		case ' ':
			oldLine++
			lastDeleted = 0
		case '-':
			if oldLine >= 1 && oldLine <= lineCount {
				changedLines = append(changedLines, oldLine)
				lastDeleted = oldLine
			}
			oldLine++
		case '+':
			anchor := lastDeleted
			if anchor == 0 {
				anchor = min(max(oldLine, 1), max(lineCount, 1))
			}
			changedLines = append(changedLines, anchor)
		case '\\':
		default:
			inHunk = false
		}
	}
	sort.Ints(changedLines)
	return slices.Compact(changedLines)
}

func (s *Scope) intersects(file *source.File, range_ source.Range) (bool, error) {
	change, found, err := s.sourceChange(file)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if change.whole {
		return true, nil
	}
	start, end, err := physicalLineSpan(file, range_)
	if err != nil {
		return false, err
	}
	for _, candidate := range change.ranges {
		if start <= candidate.End && candidate.Start <= end {
			return true, nil
		}
	}
	return false, nil
}

func (s *Scope) owns(file *source.File, range_ source.Range) (bool, error) {
	change, found, err := s.sourceChange(file)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if change.whole {
		return true, nil
	}
	start, end, err := physicalLineSpan(file, range_)
	if err != nil {
		return false, err
	}
	for line := start; line <= end; line++ {
		owned := false
		for _, candidate := range change.ranges {
			if candidate.Start <= line && line <= candidate.End {
				owned = true
				break
			}
		}
		if !owned {
			return false, nil
		}
	}
	return true, nil
}

func (s *Scope) sourceChange(file *source.File) (fileChange, bool, error) {
	change, found := s.files[canonicalPath(file.Path())]
	if found && change.versioned && change.digest != file.Digest() {
		return fileChange{}, false, fmt.Errorf(
			"changed-code source %q changed after scope resolution",
			file.Path(),
		)
	}
	return change, found, nil
}

func bindSourceVersion(path string, change fileChange) (fileChange, error) {
	if change.whole || filepath.Ext(path) != ".go" {
		return change, nil
	}
	input, err := source.ReadFile(path)
	if err != nil {
		return fileChange{}, fmt.Errorf("read changed-code source %q: %w", path, err)
	}
	change.digest = source.Digest(sha256.Sum256(input))
	change.versioned = true
	return change, nil
}

func physicalLineSpan(file *source.File, range_ source.Range) (int, int, error) {
	bytes_ := file.Bytes()
	if range_.Start < 0 || range_.End < range_.Start || range_.End > len(bytes_) {
		return 0, 0, fmt.Errorf("source range %#v is invalid for %q", range_, file.Path())
	}
	start, found := file.Position(range_.Start)
	if !found {
		return 0, 0, fmt.Errorf(
			"source range start is not a UTF-8 boundary in %q",
			file.Path(),
		)
	}
	endOffset := range_.End
	if endOffset > range_.Start {
		endOffset--
		for endOffset > range_.Start && !utf8.RuneStart(bytes_[endOffset]) {
			endOffset--
		}
	}
	end, found := file.Position(endOffset)
	if !found {
		return 0, 0, fmt.Errorf(
			"source range end is not a UTF-8 boundary in %q",
			file.Path(),
		)
	}
	return start.Line, end.Line, nil
}

func changedAnchor(anchor string) (string, error) {
	if anchor == "" {
		anchor = "."
	}
	absolute, err := filepath.Abs(anchor)
	if err != nil {
		return "", fmt.Errorf("normalize changed-code anchor: %w", err)
	}
	info, err := os.Stat(absolute)
	if err == nil && !info.IsDir() {
		absolute = filepath.Dir(absolute)
	}
	return canonicalPath(absolute), nil
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(absolute)
}

func (s *Scope) repositoryPath(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("Git returned invalid repository path %q", relative)
	}
	path := filepath.Clean(filepath.Join(s.root, filepath.FromSlash(relative)))
	relativeToRoot, err := filepath.Rel(s.root, path)
	if err != nil ||
		relativeToRoot == ".." ||
		strings.HasPrefix(relativeToRoot, ".." + string(filepath.Separator)) {
		return "", fmt.Errorf("Git path %q escapes repository root", relative)
	}
	return path, nil
}

type changedPath struct {
	current string
	previous string
}

func currentChangedPaths(output []byte) ([]changedPath, error) {
	fields := splitNUL(output)
	paths := make([]changedPath, 0, len(fields) / 2)
	for index := 0; index < len(fields); {
		status := fields[index]
		index++
		if status == "" {
			continue
		}
		if index >= len(fields) {
			return nil, errors.New("Git changed-file status is truncated")
		}
		kind := status[0]
		if kind == 'R' || kind == 'C' {
			previous := fields[index]
			index++
			if index >= len(fields) {
				return nil, errors.New("Git rename status is truncated")
			}
			paths = append(
				paths,
				changedPath{current: fields[index], previous: previous},
			)
			index++
			continue
		}
		path := fields[index]
		index++
		if kind != 'D' {
			paths = append(paths, changedPath{current: path})
		}
	}
	sort.Slice(
		paths,
		func(left, right int) bool {
			return paths[left].current < paths[right].current
		},
	)
	return slices.CompactFunc(
		paths,
		func(left, right changedPath) bool {
			return left.current == right.current && left.previous == right.previous
		},
	), nil
}

func parseNewLineRanges(patch []byte) []LineRange {
	matches := hunkHeader.FindAllSubmatch(patch, -1)
	ranges := make([]LineRange, 0, len(matches))
	for _, match := range matches {
		start, err := strconv.Atoi(string(match[1]))
		if err != nil {
			continue
		}
		count := 1
		if len(match[2]) > 0 {
			count, err = strconv.Atoi(string(match[2]))
			if err != nil {
				continue
			}
		}
		if count == 0 {
			continue
		}
		ranges = append(ranges, LineRange{Start: start, End: start + count - 1})
	}
	sort.Slice(
		ranges,
		func(left, right int) bool {
			return ranges[left].Start < ranges[right].Start
		},
	)
	merged := make([]LineRange, 0, len(ranges))
	for _, candidate := range ranges {
		if len(merged) == 0 || candidate.Start > merged[len(merged) - 1].End + 1 {
			merged = append(merged, candidate)
			continue
		}
		merged[len(merged) - 1].End = max(merged[len(merged) - 1].End, candidate.End)
	}
	return merged
}

func splitNUL(output []byte) []string {
	if len(output) == 0 {
		return nil
	}
	parts := bytes.Split(output, []byte{0})
	if len(parts[len(parts) - 1]) == 0 {
		parts = parts[:len(parts) - 1]
	}
	result := make([]string, len(parts))
	for index, part := range parts {
		result[index] = string(part)
	}
	return result
}

func runGit(ctx context.Context, root string, arguments ...string) ([]byte, error) {
	return runGitConfigured(ctx, root, nil, arguments...)
}

func runGitConfigured(
	ctx context.Context,
	root string,
	configuration []string,
	arguments ...string,
) ([]byte, error) {
	prefix := []string{
		"-C",
		root,
		"-c",
		"core.attributesFile=/dev/null",
		"-c",
		"diff.algorithm=myers",
		"-c",
		"diff.indentHeuristic=false",
		"-c",
		"diff.renames=true",
		"-c",
		"diff.renameLimit=1000",
	}
	for _, option := range configuration {
		prefix = append(prefix, "-c", option)
	}
	command := exec.CommandContext(ctx, "git", append(prefix, arguments...)...)
	command.Env = sanitizedGitEnvironment()
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}

func gitFilterOverrides(ctx context.Context, root string) ([]string, error) {
	output, err := runGit(ctx, root, "config", "--local", "--includes", "--null", "--list")
	if err != nil {
		return nil, err
	}
	drivers := make(map[string]struct{})
	for _, entry := range splitNUL(output) {
		key, _, _ := strings.Cut(entry, "\n")
		if !strings.HasPrefix(key, "filter.") {
			continue
		}
		for _, suffix := range []string{".clean", ".process", ".required", ".smudge"} {
			if driver, found := strings.CutSuffix(key, suffix);
				found && driver != "filter." {
				drivers[driver] = struct{}{}
				break
			}
		}
	}
	names := make([]string, 0, len(drivers))
	for driver := range drivers {
		names = append(names, driver)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names) * 4)
	for _, driver := range names {
		result = append(
			result,
			driver + ".clean=",
			driver + ".process=",
			driver + ".required=false",
			driver + ".smudge=",
		)
	}
	return result, nil
}

func sanitizedGitEnvironment() []string {
	result := make([]string, 0, len(os.Environ()) + 8)
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.HasPrefix(name, "GIT_") || name == "LC_ALL") {
			continue
		}
		result = append(result, entry)
	}
	return append(
		result,
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_EXTERNAL_DIFF=",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"LC_ALL=C",
	)
}
