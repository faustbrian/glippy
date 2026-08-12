package analysis

import (
	"bytes"
	"context"
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
	"strings"
	"sync"
	"unicode"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
	"golang.org/x/tools/go/packages"
)

// ModuleMode selects a non-mutating Go module resolution policy.
type ModuleMode string

const (
	// ModuleReadonly refuses go.mod and go.sum updates.
	ModuleReadonly ModuleMode = "readonly"
	// ModuleVendor loads dependencies from the selected vendor tree.
	ModuleVendor ModuleMode = "vendor"
)

// PackageLoadOptions defines one run-owned Go package loading request.
type PackageLoadOptions struct {
	Dir                  string
	Patterns             []string
	Requirement          rules.Requirement
	Tests                bool
	LoadDependencySyntax bool
	BuildTags            []string
	ModuleMode           ModuleMode
	Env                  []string
	Overlay              map[string][]byte
	AllowNetwork         bool
	GOOS                 string
	GOARCH               string
}

// PackageDiagnostic is one canonical package-loading or type-checking error.
type PackageDiagnostic struct {
	PackageID string
	Position  string
	Message   string
	Kind      packages.ErrorKind
}

// PackageLoadResult owns one compatible typed package graph for a run.
type PackageLoadResult struct {
	Requirement rules.Requirement
	Packages    []*packages.Package
	Diagnostics []PackageDiagnostic
	Sources     PackageSourceSet
}

// PackageSourceSet is one immutable index of the exact bytes parsed by a
// package load.
type PackageSourceSet struct {
	paths    []string
	files    map[string]*source.File
	problems []PackageSourceProblem
}

// PackageSourceProblem records why one captured source is diagnostic-only.
type PackageSourceProblem struct {
	Path    string
	Digest  source.Digest
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
func (s PackageSourceSet) Paths() []string { return slices.Clone(s.paths) }

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
	return slices.Clone(s.problems)
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
	if options.Dir == "" || !filepath.IsAbs(options.Dir) || filepath.Clean(options.Dir) != options.Dir {
		return PackageLoadResult{}, fmt.Errorf("package loading directory %q is not normalized absolute", options.Dir)
	}
	if len(options.Patterns) == 0 {
		return PackageLoadResult{}, fmt.Errorf("package loading requires at least one pattern")
	}
	patterns := slices.Clone(options.Patterns)
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			return PackageLoadResult{}, fmt.Errorf("package loading patterns must not be empty")
		}
	}
	if err := validatePackageOverlay(options.Overlay); err != nil {
		return PackageLoadResult{}, err
	}
	buildFlags, err := packageBuildFlags(options)
	if err != nil {
		return PackageLoadResult{}, err
	}
	sourceCollector := newPackageSourceCollector()

	loaded, err := packages.Load(&packages.Config{
		Context:    ctx,
		Mode:       packageLoadMode(options),
		Dir:        options.Dir,
		Env:        packageLoadEnvironment(options),
		BuildFlags: buildFlags,
		Tests:      options.Tests,
		Overlay:    cloneOverlay(options.Overlay),
		ParseFile:  sourceCollector.parseFile,
	}, patterns...)
	if contextErr := ctx.Err(); contextErr != nil {
		return PackageLoadResult{}, contextErr
	}
	if err != nil {
		return PackageLoadResult{}, fmt.Errorf("load Go packages: %w", err)
	}
	sources, err := sourceCollector.result()
	if err != nil {
		return PackageLoadResult{}, err
	}
	ordered, err := canonicalPackages(loaded)
	if err != nil {
		return PackageLoadResult{}, err
	}
	return PackageLoadResult{
		Requirement: options.Requirement,
		Packages:    ordered,
		Diagnostics: packageDiagnostics(ordered),
		Sources:     sources,
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
			return fmt.Errorf("package overlay path %q is not normalized absolute", path)
		}
		if err := source.ValidateSize(int64(len(overlay[path]))); err != nil {
			return fmt.Errorf("package overlay %q: %w", path, err)
		}
	}
	return nil
}

type packageSourceCollector struct {
	mu       sync.Mutex
	files    map[string]*source.File
	problems map[string]PackageSourceProblem
	errors   map[string]map[string]error
}

func newPackageSourceCollector() *packageSourceCollector {
	return &packageSourceCollector{
		files:    make(map[string]*source.File),
		problems: make(map[string]PackageSourceProblem),
		errors:   make(map[string]map[string]error),
	}
}

func (c *packageSourceCollector) parseFile(
	fileSet *token.FileSet,
	filename string,
	input []byte,
) (*ast.File, error) {
	physical, sourceErr := source.Load(filename, input)
	c.add(filename, physical, sourceErr)
	if physical == nil {
		return nil, scanner.ErrorList{&scanner.Error{
			Pos: token.Position{Filename: filename, Line: 1, Column: 1},
			Msg: sourceErr.Error(),
		}}
	}
	return parser.ParseFile(fileSet, filename, input, parser.AllErrors|parser.ParseComments)
}

func (c *packageSourceCollector) add(filename string, file *source.File, sourceErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := filepath.Clean(filename)
	if file != nil {
		path = file.Path()
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		c.addError(path, fmt.Errorf("package parser returned non-normalized source path %q", path))
		return
	}
	if file == nil {
		c.addError(path, fmt.Errorf("package parser did not capture source %q: %w", path, sourceErr))
		return
	}
	if previous, found := c.files[path]; found {
		if previous.Digest() != file.Digest() {
			c.addError(path, fmt.Errorf("package parser returned incompatible source versions for %q", path))
		}
	} else {
		c.files[path] = file
	}
	if sourceErr != nil {
		problem := PackageSourceProblem{Path: path, Digest: file.Digest(), Message: sourceErr.Error()}
		if previous, found := c.problems[path]; found && previous != problem {
			c.addError(path, fmt.Errorf("package parser returned incompatible source problems for %q", path))
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

func (c *packageSourceCollector) result() (PackageSourceSet, error) {
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
	paths := make([]string, 0, len(c.files))
	files := make(map[string]*source.File, len(c.files))
	for path, file := range c.files {
		paths = append(paths, path)
		files[path] = file
	}
	sort.Strings(paths)
	problems := make([]PackageSourceProblem, 0, len(c.problems))
	for _, problem := range c.problems {
		problems = append(problems, problem)
	}
	sort.Slice(problems, func(left, right int) bool {
		if problems[left].Path != problems[right].Path {
			return problems[left].Path < problems[right].Path
		}
		return problems[left].Message < problems[right].Message
	})
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
		flags = append(flags, "-tags="+strings.Join(tags, ","))
	}
	return flags, nil
}

func validBuildTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, character := range tag {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			character != '_' && character != '.' {
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
	replacements := map[string]string{
		"GOPACKAGESDRIVER": "off",
	}
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
	values := make(map[string]string, len(environment)+len(replacements))
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
			return nil, fmt.Errorf("package load returned incompatible duplicate ID %q", pkg.ID)
		}
		byID[pkg.ID] = pkg
	}
	testMainIDs := make(map[string]struct{})
	for _, pkg := range byID {
		if pkg.ForTest != "" {
			testMainIDs[pkg.ForTest+".test"] = struct{}{}
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
			diagnostics = append(diagnostics, PackageDiagnostic{
				PackageID: pkg.ID,
				Position:  issue.Pos,
				Message:   issue.Msg,
				Kind:      issue.Kind,
			})
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
	sort.Slice(diagnostics, func(left, right int) bool {
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
		return first.Kind < second.Kind
	})
	return diagnostics
}
