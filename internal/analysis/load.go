package analysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/tools/go/packages"
)

// ModuleMode selects a non-mutating Go module resolution policy.
type ModuleMode string

const (
	// ModuleReadonly refuses go.mod and go.sum updates.
	ModuleReadonly ModuleMode = "readonly"
	// ModuleVendor loads dependencies from the selected vendor tree.
	ModuleVendor ModuleMode = "vendor"
	// DefaultMaxPackages bounds the complete package graph retained by one
	// typed analysis load.
	DefaultMaxPackages = 10_000
	// DefaultMaxSourceFiles bounds unique physical Go sources retained by one
	// typed analysis load.
	DefaultMaxSourceFiles = 20_000
	// DefaultMaxSourceBytes bounds aggregate unique physical Go source bytes
	// retained by one typed analysis load.
	DefaultMaxSourceBytes int64 = 256 << 20
)

// PackageLoadOptions defines one run-owned Go package loading request.
type PackageLoadOptions struct {
	Dir string
	Patterns []string
	Requirement rules.Requirement
	Tests bool
	LoadDependencySyntax bool
	LoadEffectFacts bool
	BuildTags []string
	ModuleMode ModuleMode
	Env []string
	Overlay map[string][]byte
	AllowNetwork bool
	GOOS string
	GOARCH string
	MaxPackages int
	MaxSourceFiles int
	MaxSourceBytes int64
	Contracts contracts.Set
	compactDependencySource bool
}

// PackageDiagnostic is one canonical package-loading or type-checking error.
type PackageDiagnostic struct {
	PackageID string
	Targets []string
	Position string
	Message string
	Kind packages.ErrorKind
}

// PackageLoadResult owns one compatible typed package graph for a run.
type PackageLoadResult struct {
	Requirement rules.Requirement
	Packages []*packages.Package
	Diagnostics []PackageDiagnostic
	Sources PackageSourceSet
	effectFacts *nativeEffectFacts
}

// PackageSourceSet is one immutable index of the exact bytes parsed by a
// package load.
type PackageSourceSet struct {
	paths []string
	files map[string]*source.File
	problems []PackageSourceProblem
}

// PackageSourceProblem records why one captured source is diagnostic-only.
type PackageSourceProblem struct {
	Path string
	Digest source.Digest
	Targets []string
	Message string
}

func clonePackageLoadOptions(options PackageLoadOptions) PackageLoadOptions {
	result := options
	result.Patterns = slices.Clone(options.Patterns)
	result.BuildTags = slices.Clone(options.BuildTags)
	result.Env = slices.Clone(options.Env)
	result.Overlay = cloneOverlay(options.Overlay)
	return result
}

// Paths returns normalized physical source identities in canonical order.
func (s PackageSourceSet) Paths() []string {
	return slices.Clone(s.paths)
}

// Lookup returns the immutable source version parsed for path.
func (s PackageSourceSet) Lookup(path string) (*source.File, bool) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, false
	}
	file, found := s.files[path]
	return file, found
}

// Problems returns source-model failures in canonical order.
func (s PackageSourceSet) Problems() []PackageSourceProblem {
	result := slices.Clone(s.problems)
	for index := range result {
		result[index].Targets = slices.Clone(result[index].Targets)
	}
	return result
}

// WithProblemTargets returns the same immutable source set with one canonical
// target set attached to every source-model problem.
func (s PackageSourceSet) WithProblemTargets(targets []string) (PackageSourceSet, error) {
	if err := validateProblemTargets(targets); err != nil {
		return PackageSourceSet{}, err
	}
	result := s
	result.problems = slices.Clone(s.problems)
	for index := range result.problems {
		result.problems[index].Targets = slices.Clone(targets)
	}
	return result, nil
}

// MergePackageSourceSets combines compatible immutable source indexes.
func MergePackageSourceSets(sets ...PackageSourceSet) (PackageSourceSet, error) {
	files := make(map[string]*source.File)
	type problemIdentity struct {
		path string
		digest source.Digest
		message string
	}
	problemsByIdentity := make(map[problemIdentity]PackageSourceProblem)
	for _, set := range sets {
		for _, path := range set.paths {
			file, found := set.files[path]
			if !found || file == nil {
				return PackageSourceSet{}, fmt.Errorf(
					"package source set is missing %q",
					path,
				)
			}
			if previous, duplicate := files[path]; duplicate {
				if previous.Digest() != file.Digest() {
					return PackageSourceSet{}, fmt.Errorf(
						"package source sets contain incompatible versions of %q",
						path,
					)
				}
				if !previous.CanFormat() && file.CanFormat() {
					files[path] = file
				}
				continue
			}
			files[path] = file
		}
		for _, problem := range set.problems {
			if err := validateProblemTargets(problem.Targets); err != nil {
				return PackageSourceSet{}, fmt.Errorf(
					"package source problem %q targets: %w",
					problem.Path,
					err,
				)
			}
			identity := problemIdentity{
				path: problem.Path,
				digest: problem.Digest,
				message: problem.Message,
			}
			if previous, found := problemsByIdentity[identity]; found {
				previous.Targets = append(previous.Targets, problem.Targets...)
				sort.Strings(previous.Targets)
				previous.Targets = slices.Compact(previous.Targets)
				problemsByIdentity[identity] = previous
				continue
			}
			problem.Targets = slices.Clone(problem.Targets)
			problemsByIdentity[identity] = problem
		}
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	problems := make([]PackageSourceProblem, 0, len(problemsByIdentity))
	for _, problem := range problemsByIdentity {
		problems = append(problems, problem)
	}
	sort.Slice(
		problems,
		func(left, right int) bool {
			if problems[left].Path != problems[right].Path {
				return problems[left].Path < problems[right].Path
			}
			if problems[left].Message != problems[right].Message {
				return problems[left].Message < problems[right].Message
			}
			return slices.Compare(problems[left].Targets, problems[right].Targets) < 0
		},
	)
	return PackageSourceSet{paths: paths, files: files, problems: problems}, nil
}

func validateProblemTargets(targets []string) error {
	for index, target := range targets {
		if strings.TrimSpace(target) == "" || strings.TrimSpace(target) != target {
			return fmt.Errorf("target %d is empty or not canonical", index)
		}
		if index > 0 && targets[index - 1] >= target {
			return fmt.Errorf("targets are not strictly sorted")
		}
	}
	return nil
}

// LoadPackages loads the typed prerequisite shared by types, CFG, and SSA
// tiers. Lexical and syntax-only execution must use the file-owned frontend and
// never reach this boundary.
func LoadPackages(ctx context.Context, options PackageLoadOptions) (PackageLoadResult, error) {
	if ctx == nil {
		return PackageLoadResult{}, fmt.Errorf("package loading requires a context")
	}
	if err := ctx.Err(); err != nil {
		return PackageLoadResult{}, err
	}
	if options.Requirement < rules.RequireTypes || options.Requirement > rules.RequireSSA {
		return PackageLoadResult{}, fmt.Errorf(
			"package loading requires types, control flow, or SSA; got %s",
			options.Requirement,
		)
	}
	if options.Dir == "" ||
		!filepath.IsAbs(options.Dir) ||
		filepath.Clean(options.Dir) != options.Dir {
		return PackageLoadResult{}, fmt.Errorf(
			"package loading directory %q is not normalized absolute",
			options.Dir,
		)
	}
	if len(options.Patterns) == 0 {
		return PackageLoadResult{}, fmt.Errorf(
			"package loading requires at least one pattern",
		)
	}
	patterns := slices.Clone(options.Patterns)
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			return PackageLoadResult{}, fmt.Errorf(
				"package loading patterns must not be empty",
			)
		}
	}
	if err := validatePackageOverlay(options.Overlay); err != nil {
		return PackageLoadResult{}, err
	}
	limits, err := resolvePackageResourceLimits(options)
	if err != nil {
		return PackageLoadResult{}, err
	}
	buildFlags, err := packageBuildFlags(options)
	if err != nil {
		return PackageLoadResult{}, err
	}
	sourceCollector := newPackageSourceCollector(limits, options.compactDependencySource)

	loaded, err := packages.Load(
		&packages.Config{
			Context: ctx,
			Mode: packageLoadMode(options),
			Dir: options.Dir,
			Env: packageLoadEnvironment(options),
			BuildFlags: buildFlags,
			Tests: options.Tests,
			Overlay: cloneOverlay(options.Overlay),
			ParseFile: sourceCollector.parseFile,
		},
		patterns...,
	)
	if contextErr := ctx.Err(); contextErr != nil {
		return PackageLoadResult{}, contextErr
	}
	if err != nil {
		return PackageLoadResult{}, fmt.Errorf("load Go packages: %w", err)
	}
	if err := validatePackageGraphLimit(loaded, limits.maxPackages); err != nil {
		return PackageLoadResult{}, err
	}
	ordered, err := canonicalPackages(loaded)
	if err != nil {
		return PackageLoadResult{}, err
	}
	if err := captureProfilePhase(ctx, ProfilePhasePackages); err != nil {
		return PackageLoadResult{}, err
	}
	if err := capturePackageOriginalSources(ordered, options.Overlay, sourceCollector);
		err != nil {
		return PackageLoadResult{}, err
	}
	sources, err := sourceCollector.result(options.compactDependencySource)
	if err != nil {
		return PackageLoadResult{}, err
	}
	if err := captureProfilePhase(ctx, ProfilePhaseSourceModel); err != nil {
		return PackageLoadResult{}, err
	}
	diagnostics := packageDiagnostics(ordered)
	cgoDiagnostics, err := cgoBoundaryDiagnostics(ordered, sources)
	if err != nil {
		return PackageLoadResult{}, err
	}
	diagnostics = append(diagnostics, cgoDiagnostics...)
	orderPackageDiagnostics(diagnostics)
	var effects *nativeEffectFacts
	if options.LoadEffectFacts {
		effects, err = loadNativeEffectFacts(ctx, options, ordered, sources)
		if err != nil {
			return PackageLoadResult{}, err
		}
	}
	if err := captureProfilePhase(ctx, ProfilePhaseEffectFacts); err != nil {
		return PackageLoadResult{}, err
	}
	return PackageLoadResult{
		Requirement: options.Requirement,
		Packages: ordered,
		Diagnostics: diagnostics,
		Sources: sources,
		effectFacts: effects,
	}, nil
}

func validatePackageOverlay(overlay map[string][]byte) error {
	paths := make([]string, 0, len(overlay))
	for path := range overlay {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf(
				"package overlay path %q is not normalized absolute",
				path,
			)
		}
		if err := source.ValidateSize(int64(len(overlay[path]))); err != nil {
			return fmt.Errorf("package overlay %q: %w", path, err)
		}
	}
	return nil
}

func resolvePackageResourceLimits(options PackageLoadOptions) (packageResourceLimits, error) {
	if options.MaxPackages < 0 {
		return packageResourceLimits{}, fmt.Errorf(
			"maximum package count must not be negative",
		)
	}
	if options.MaxSourceFiles < 0 {
		return packageResourceLimits{}, fmt.Errorf(
			"maximum typed source file count must not be negative",
		)
	}
	if options.MaxSourceBytes < 0 {
		return packageResourceLimits{}, fmt.Errorf(
			"maximum typed source byte count must not be negative",
		)
	}
	if options.MaxPackages > DefaultMaxPackages {
		return packageResourceLimits{}, fmt.Errorf(
			"maximum package count must not exceed %d",
			DefaultMaxPackages,
		)
	}
	if options.MaxSourceFiles > DefaultMaxSourceFiles {
		return packageResourceLimits{}, fmt.Errorf(
			"maximum typed source file count must not exceed %d",
			DefaultMaxSourceFiles,
		)
	}
	if options.MaxSourceBytes > DefaultMaxSourceBytes {
		return packageResourceLimits{}, fmt.Errorf(
			"maximum typed source byte count must not exceed %d",
			DefaultMaxSourceBytes,
		)
	}
	limits := defaultPackageResourceLimits()
	if options.MaxPackages != 0 {
		limits.maxPackages = options.MaxPackages
	}
	if options.MaxSourceFiles != 0 {
		limits.maxSourceFiles = options.MaxSourceFiles
	}
	if options.MaxSourceBytes != 0 {
		limits.maxSourceBytes = options.MaxSourceBytes
	}
	return limits, nil
}

func defaultPackageResourceLimits() packageResourceLimits {
	return packageResourceLimits{
		maxPackages: DefaultMaxPackages,
		maxSourceFiles: DefaultMaxSourceFiles,
		maxSourceBytes: DefaultMaxSourceBytes,
	}
}

func validatePackageGraphLimit(roots []*packages.Package, limit int) error {
	visited := make(map[string]struct{})
	stack := slices.Clone(roots)
	for len(stack) > 0 {
		pkg := stack[len(stack) - 1]
		stack = stack[:len(stack) - 1]
		if pkg == nil {
			continue
		}
		identity := pkg.ID
		if identity == "" {
			identity = fmt.Sprintf("%p", pkg)
		}
		if _, found := visited[identity]; found {
			continue
		}
		visited[identity] = struct{}{}
		if len(visited) > limit {
			return fmt.Errorf("package graph exceeds %d-package limit", limit)
		}
		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		sort.Strings(imports)
		for index := len(imports) - 1; index >= 0; index-- {
			stack = append(stack, pkg.Imports[imports[index]])
		}
	}
	return nil
}

type packageSourceCollector struct {
	mu sync.Mutex
	inputs map[string][]byte
	files map[string]*source.File
	problems map[string]PackageSourceProblem
	errors map[string]map[string]error
	seen map[string]sourceReservation
	limits packageResourceLimits
	bytes int64
	deferSourceIndexing bool
}

type packageResourceLimits struct {
	maxPackages int
	maxSourceFiles int
	maxSourceBytes int64
}

type sourceReservation struct {
	digest [sha256.Size]byte
	size int64
}

func newPackageSourceCollector(
	limits packageResourceLimits,
	deferSourceIndexing bool,
) *packageSourceCollector {
	return &packageSourceCollector{
		inputs: make(map[string][]byte),
		files: make(map[string]*source.File),
		problems: make(map[string]PackageSourceProblem),
		errors: make(map[string]map[string]error),
		seen: make(map[string]sourceReservation),
		limits: limits,
		deferSourceIndexing: deferSourceIndexing,
	}
}

func (c *packageSourceCollector) parseFile(
	fileSet *token.FileSet,
	filename string,
	input []byte,
) (*ast.File, error) {
	if err := c.admit(filename, input); err != nil {
		return nil, scanner.ErrorList{
			&scanner.Error{
				Pos: token.Position{Filename: filename, Line: 1, Column: 1},
				Msg: err.Error(),
			},
		}
	}
	if !c.deferSourceIndexing {
		physical, sourceErr := source.Load(filename, input)
		c.add(filename, physical, sourceErr)
		if physical == nil {
			return nil, scanner.ErrorList{
				&scanner.Error{
					Pos: token.Position{Filename: filename, Line: 1, Column: 1},
					Msg: sourceErr.Error(),
				},
			}
		}
	} else {
		c.remember(filename, input)
	}
	return parser.ParseFile(fileSet, filename, input, parser.AllErrors | parser.ParseComments)
}

func (c *packageSourceCollector) remember(filename string, input []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := filepath.Clean(filename)
	if _, found := c.inputs[path]; !found {
		c.inputs[path] = bytes.Clone(input)
	}
}

func (c *packageSourceCollector) input(path string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	input, found := c.inputs[path]
	return bytes.Clone(input), found
}

func (c *packageSourceCollector) admit(filename string, input []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := filepath.Clean(filename)
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		err := fmt.Errorf("package parser returned non-normalized source path %q", path)
		c.addError(path, err)
		return err
	}
	reservation := sourceReservation{digest: sha256.Sum256(input), size: int64(len(input))}
	if previous, found := c.seen[path]; found {
		if previous != reservation {
			err := fmt.Errorf(
				"package parser returned incompatible source versions for %q",
				path,
			)
			c.addError(path, err)
			return err
		}
		return nil
	}
	if len(c.seen) + 1 > c.limits.maxSourceFiles {
		err := fmt.Errorf("typed source set exceeds %d-file limit", c.limits.maxSourceFiles)
		c.addError("", err)
		return err
	}
	if int64(len(input)) > c.limits.maxSourceBytes - c.bytes {
		err := fmt.Errorf("typed source set exceeds %d-byte limit", c.limits.maxSourceBytes)
		c.addError("", err)
		return err
	}
	c.seen[path] = reservation
	c.bytes += int64(len(input))
	return nil
}

func (c *packageSourceCollector) captured(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, found := c.files[path]
	return found
}

func capturePackageOriginalSources(
	packages_ []*packages.Package,
	overlay map[string][]byte,
	collector *packageSourceCollector,
) error {
	paths := make(map[string]struct{})
	for _, pkg := range packages_ {
		for _, path := range pkg.GoFiles {
			path = filepath.Clean(path)
			if !filepath.IsAbs(path) || filepath.Clean(path) != path {
				return fmt.Errorf(
					"package %q has non-normalized original source path %q",
					pkg.ID,
					path,
				)
			}
			paths[path] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		if collector.captured(path) {
			continue
		}
		input, found := collector.input(path)
		if !found {
			input, found = overlay[path]
		}
		if !found {
			var err error
			input, err = source.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read original package source %q: %w", path, err)
			}
		}
		if err := collector.admit(path, input); err != nil {
			return err
		}
		file, sourceErr := source.Load(path, input)
		collector.add(path, file, sourceErr)
	}
	return nil
}

func (c *packageSourceCollector) add(filename string, file *source.File, sourceErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := filepath.Clean(filename)
	if file != nil {
		path = file.Path()
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		c.addError(
			path,
			fmt.Errorf("package parser returned non-normalized source path %q", path),
		)
		return
	}
	if file == nil {
		c.addError(
			path,
			fmt.Errorf("package parser did not capture source %q: %w", path, sourceErr),
		)
		return
	}
	if previous, found := c.files[path]; found {
		if previous.Digest() != file.Digest() {
			c.addError(
				path,
				fmt.Errorf(
					"package parser returned incompatible source versions for %q",
					path,
				),
			)
		}
	} else {
		c.files[path] = file
	}
	if sourceErr != nil {
		problem := PackageSourceProblem{
			Path: path,
			Digest: file.Digest(),
			Message: sourceErr.Error(),
		}
		if previous, found := c.problems[path];
			found &&
				(previous.Path != problem.Path ||
					previous.Digest != problem.Digest ||
					previous.Message != problem.Message ||
					!slices.Equal(previous.Targets, problem.Targets)) {
			c.addError(
				path,
				fmt.Errorf(
					"package parser returned incompatible source problems for %q",
					path,
				),
			)
		} else {
			c.problems[path] = problem
		}
	}
}

func (c *packageSourceCollector) addError(key string, err error) {
	if err == nil {
		return
	}
	if c.errors[key] == nil {
		c.errors[key] = make(map[string]error)
	}
	c.errors[key][err.Error()] = err
}

func (c *packageSourceCollector) result(compactDependencySource bool) (PackageSourceSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.errors) > 0 {
		keys := make([]string, 0, len(c.errors))
		for key := range c.errors {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		failures := make([]error, 0, len(keys))
		for _, key := range keys {
			messages := make([]string, 0, len(c.errors[key]))
			for message := range c.errors[key] {
				messages = append(messages, message)
			}
			sort.Strings(messages)
			for _, message := range messages {
				failures = append(failures, c.errors[key][message])
			}
		}
		return PackageSourceSet{}, errors.Join(failures...)
	}
	files := make(map[string]*source.File, len(c.inputs))
	for path, file := range c.files {
		files[path] = file
	}
	for path, input := range c.inputs {
		if _, found := files[path]; found {
			continue
		}
		var file *source.File
		var sourceErr error
		if compactDependencySource {
			file, sourceErr = source.CaptureParsedBytes(path, input)
		} else {
			file, sourceErr = source.Load(path, input)
		}
		if sourceErr != nil {
			return PackageSourceSet{}, fmt.Errorf(
				"capture package source %q: %w",
				path,
				sourceErr,
			)
		}
		files[path] = file
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	problems := make([]PackageSourceProblem, 0, len(c.problems))
	for _, problem := range c.problems {
		problems = append(problems, problem)
	}
	sort.Slice(
		problems,
		func(left, right int) bool {
			if problems[left].Path != problems[right].Path {
				return problems[left].Path < problems[right].Path
			}
			return problems[left].Message < problems[right].Message
		},
	)
	return PackageSourceSet{paths: paths, files: files, problems: problems}, nil
}

const typedPackageLoadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedEmbedFiles |
	packages.NeedEmbedPatterns |
	packages.NeedImports |
	packages.NeedExportFile |
	packages.NeedSyntax |
	packages.NeedTypes |
	packages.NeedTypesSizes |
	packages.NeedTypesInfo |
	packages.NeedForTest |
	packages.NeedModule

func packageLoadMode(options PackageLoadOptions) packages.LoadMode {
	mode := typedPackageLoadMode
	if options.LoadDependencySyntax {
		mode |= packages.NeedDeps
	}
	return mode
}

func packageBuildFlags(options PackageLoadOptions) ([]string, error) {
	mode := options.ModuleMode
	if mode == "" {
		mode = ModuleReadonly
	}
	if mode != ModuleReadonly && mode != ModuleVendor {
		return nil, fmt.Errorf("unsupported package module mode %q", mode)
	}
	tags := slices.Clone(options.BuildTags)
	for _, tag := range tags {
		if !validBuildTag(tag) {
			return nil, fmt.Errorf("invalid package build tag %q", tag)
		}
	}
	slices.Sort(tags)
	tags = slices.Compact(tags)
	flags := []string{"-mod=" + string(mode)}
	if len(tags) > 0 {
		flags = append(flags, "-tags=" + strings.Join(tags, ","))
	}
	return flags, nil
}

func validBuildTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, character := range tag {
		if !unicode.IsLetter(character) &&
			!unicode.IsDigit(character) &&
			character != '_' &&
			character != '.' {
			return false
		}
	}
	return true
}

func packageLoadEnvironment(options PackageLoadOptions) []string {
	environment := options.Env
	if environment == nil {
		environment = os.Environ()
	}
	replacements := map[string]string{"GOPACKAGESDRIVER": "off"}
	if !options.AllowNetwork {
		replacements["GOPROXY"] = "off"
		replacements["GONOPROXY"] = "none"
		replacements["GOSUMDB"] = "off"
		replacements["GOTOOLCHAIN"] = "local"
		replacements["GOVCS"] = "off"
	}
	if options.GOOS != "" {
		replacements["GOOS"] = options.GOOS
	}
	if options.GOARCH != "" {
		replacements["GOARCH"] = options.GOARCH
	}
	values := make(map[string]string, len(environment) + len(replacements))
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found && name != "" {
			values[name] = value
		}
	}
	for name, value := range replacements {
		values[name] = value
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

func cloneOverlay(overlay map[string][]byte) map[string][]byte {
	if overlay == nil {
		return nil
	}
	result := make(map[string][]byte, len(overlay))
	for path, contents := range overlay {
		result[path] = bytes.Clone(contents)
	}
	return result
}

func canonicalPackages(loaded []*packages.Package) ([]*packages.Package, error) {
	byID := make(map[string]*packages.Package, len(loaded))
	for index, pkg := range loaded {
		if pkg == nil {
			return nil, fmt.Errorf("loaded package %d is nil", index)
		}
		if pkg.ID == "" {
			return nil, fmt.Errorf("loaded package %d has no ID", index)
		}
		if previous, found := byID[pkg.ID]; found && previous != pkg {
			return nil, fmt.Errorf(
				"package load returned incompatible duplicate ID %q",
				pkg.ID,
			)
		}
		byID[pkg.ID] = pkg
	}
	testMainIDs := make(map[string]struct{})
	for _, pkg := range byID {
		if pkg.ForTest != "" {
			testMainIDs[pkg.ForTest + ".test"] = struct{}{}
		}
	}
	for id := range testMainIDs {
		pkg, found := byID[id]
		if found && pkg.ID == pkg.PkgPath && pkg.Name == "main" && pkg.ForTest == "" {
			delete(byID, id)
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*packages.Package, len(ids))
	for index, id := range ids {
		result[index] = byID[id]
	}
	return result, nil
}

func packageDiagnostics(roots []*packages.Package) []PackageDiagnostic {
	visited := make(map[string]struct{})
	diagnostics := make([]PackageDiagnostic, 0)
	var visit func(*packages.Package)
	visit = func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		if _, found := visited[pkg.ID]; found {
			return
		}
		visited[pkg.ID] = struct{}{}
		for _, issue := range pkg.Errors {
			diagnostics = append(
				diagnostics,
				PackageDiagnostic{
					PackageID: pkg.ID,
					Position: issue.Pos,
					Message: issue.Msg,
					Kind: issue.Kind,
				},
			)
		}
		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		sort.Strings(imports)
		for _, path := range imports {
			visit(pkg.Imports[path])
		}
	}
	for _, root := range roots {
		visit(root)
	}
	orderPackageDiagnostics(diagnostics)
	return diagnostics
}

func cgoBoundaryDiagnostics(
	roots []*packages.Package,
	sources PackageSourceSet,
) ([]PackageDiagnostic, error) {
	diagnostics := make([]PackageDiagnostic, 0)
	owned, err := canonicalPackageSourceFiles(roots, sources)
	if err != nil {
		return nil, err
	}
	for _, work := range owned {
		pkg, file := work.package_, work.source
		compiled := make(map[string]struct{}, len(pkg.Syntax))
		for _, syntax := range pkg.Syntax {
			if syntax == nil || pkg.Fset == nil {
				continue
			}
			compiled[filepath.Clean(
				pkg.Fset.PositionFor(syntax.Pos(), false).Filename,
			)] = struct{}{}
		}
		if _, found := compiled[file.Path()]; found || !sourceImportsC(file) {
			continue
		}
		diagnostics = append(
			diagnostics,
			PackageDiagnostic{
				PackageID: pkg.ID,
				Position: file.Path(),
				Message: "typed analysis is unavailable for cgo source; syntax analysis remains available",
				Kind: packages.UnknownError,
			},
		)
	}
	orderPackageDiagnostics(diagnostics)
	return diagnostics, nil
}

func sourceImportsC(file *source.File) bool {
	if file == nil {
		return false
	}
	found := false
	_ = file.ReadSyntax(
		func(syntax *ast.File) error {
			for _, imported := range syntax.Imports {
				path, err := strconv.Unquote(imported.Path.Value)
				if err == nil && path == "C" {
					found = true
					break
				}
			}
			return nil
		},
	)
	return found
}

func orderPackageDiagnostics(diagnostics []PackageDiagnostic) {
	sort.Slice(
		diagnostics,
		func(left, right int) bool {
			first, second := diagnostics[left], diagnostics[right]
			if first.PackageID != second.PackageID {
				return first.PackageID < second.PackageID
			}
			if first.Position != second.Position {
				return first.Position < second.Position
			}
			if first.Message != second.Message {
				return first.Message < second.Message
			}
			if first.Kind != second.Kind {
				return first.Kind < second.Kind
			}
			return slices.Compare(first.Targets, second.Targets) < 0
		},
	)
}
