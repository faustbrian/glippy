// Package rules defines native lint-rule contracts and canonical metadata.
package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"github.com/faustbrian/gox/internal/source"
	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/ssa"
)

// Requirement is the most expensive representation a rule requires.
type Requirement uint8

const (
	RequireLexical Requirement = iota
	RequireSyntax
	RequireTypes
	RequireControlFlow
	RequireSSA
)

func (r Requirement) String() string {
	switch r {
	case RequireLexical:
		return "lexical source"
	case RequireSyntax:
		return "syntax"
	case RequireTypes:
		return "types"
	case RequireControlFlow:
		return "control flow"
	case RequireSSA:
		return "SSA"
	default:
		return fmt.Sprintf("requirement(%d)", r)
	}
}

// Severity is the configured diagnostic level for one rule.
type Severity string

const (
	SeverityOff   Severity = "off"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Preset is one coherent built-in rule group.
type Preset string

const (
	PresetCorrectness Preset = "correctness"
	PresetSuspicious  Preset = "suspicious"
	PresetPerformance Preset = "performance"
	PresetComplexity  Preset = "complexity"
	PresetStyle       Preset = "style"
	PresetMigration   Preset = "migration"
)

// Category classifies the observable concern reported by a rule.
type Category string

const (
	CategoryCorrectness     Category = "correctness"
	CategorySafety          Category = "safety"
	CategorySuspicious      Category = "suspicious"
	CategoryPerformance     Category = "performance"
	CategoryComplexity      Category = "complexity"
	CategoryStyle           Category = "style"
	CategoryMigration       Category = "migration"
	CategoryMaintainability Category = "maintainability"
)

// FixSafety controls whether a fix may run without an explicit opt-in.
type FixSafety string

const (
	FixSafe       FixSafety = "safe"
	FixSuggestion FixSafety = "suggestion"
	FixUnsafe     FixSafety = "unsafe"
)

// FixMetadata describes one named transformation a rule may offer.
type FixMetadata struct {
	Name        string
	Description string
	Safety      FixSafety
}

// OptionKind is the scalar type accepted by one rule option.
type OptionKind string

const (
	OptionBoolean OptionKind = "boolean"
	OptionInteger OptionKind = "integer"
	OptionString  OptionKind = "string"
	OptionStrings OptionKind = "strings"
)

// OptionMetadata is one typed configuration field owned by a rule.
type OptionMetadata struct {
	Name     string
	Summary  string
	Kind     OptionKind
	Required bool
	Default  *OptionValue
}

// Deprecation records a rule replacement without silently changing its ID.
type Deprecation struct {
	Since       string
	Replacement string
	Message     string
}

// Example is one paired incorrect and correct rule example.
type Example struct {
	Title     string
	Incorrect string
	Correct   string
}

// Metadata is the canonical rule documentation and scheduling contract.
type Metadata struct {
	ID                       string
	Summary                  string
	Documentation            string
	DefaultSeverity          Severity
	Presets                  []Preset
	MinimumGoVersion         string
	Requirement              Requirement
	NodeInterests            []NodeKind
	RequiresDependencySyntax bool
	RunOnGenerated           bool
	RunDespiteTypeErrors     bool
	Categories               []Category
	Fixes                    []FixMetadata
	Options                  []OptionMetadata
	Deprecation              *Deprecation
	KnownLimitations         []string
	Examples                 []Example
}

// Rule is the common metadata boundary implemented by every native rule.
type Rule interface {
	Metadata() Metadata
}

// SyntaxRule receives only nodes matching its declared interests.
type SyntaxRule interface {
	Rule
	RunSyntax(*Context, ast.Node) ([]Finding, error)
}

// SyntaxFileRule runs once over one immutable source-backed syntax view.
type SyntaxFileRule interface {
	Rule
	RunSyntaxFile(*Context) ([]Finding, error)
}

// TypesRule receives package AST nodes with shared type information and exact
// physical source identity.
type TypesRule interface {
	Rule
	RunTypes(*TypesContext, ast.Node) ([]Finding, error)
}

// PackageRule runs once for each selected typed package and may report against
// the package's canonically owned physical source files.
type PackageRule interface {
	Rule
	RunPackage(*PackageContext) ([]PackageFinding, error)
}

// ControlFlowRule runs once for each function declaration and function literal
// through a graph shared with every enabled control-flow rule.
type ControlFlowRule interface {
	Rule
	RunControlFlow(*ControlFlowContext) ([]Finding, error)
}

// SSARule runs once for each source function through a program shared with
// every enabled SSA rule.
type SSARule interface {
	Rule
	RunSSA(*SSAContext) ([]Finding, error)
}

// Context is the immutable per-file syntax rule context.
type Context struct {
	file    *source.File
	options OptionSet
}

// NewContext creates an immutable per-file rule context.
func NewContext(file *source.File, options OptionSet) *Context {
	return &Context{file: file, options: options}
}

// TypesContext binds one package AST to its exact immutable physical source.
type TypesContext struct {
	file      *source.File
	fileSet   *token.FileSet
	packageID string
	package_  *types.Package
	info      *types.Info
	illTyped  bool
	options   OptionSet
}

// PackageFile binds one package AST to its exact immutable physical source and
// records whether the current package invocation owns reporter-visible output
// for that file.
type PackageFile struct {
	file      *source.File
	syntax    *ast.File
	fileSet   *token.FileSet
	target    bool
	contextID *packageContextID
}

type packageContextID struct{ marker byte }

// PackageContext exposes one shared typed package and its canonical physical
// source mapping to a package-wide native rule.
type PackageContext struct {
	fileSet      *token.FileSet
	packageID    string
	package_     *types.Package
	info         *types.Info
	sizes        types.Sizes
	illTyped     bool
	files        []PackageFile
	dependencies []PackageDependency
	options      OptionSet
	contextID    *packageContextID
}

// PackageDependency is one immutable dependency package view exposed only to
// package-wide rules that declare dependency syntax in canonical metadata.
type PackageDependency struct {
	fileSet   *token.FileSet
	packageID string
	package_  *types.Package
	info      *types.Info
	sizes     types.Sizes
	illTyped  bool
	files     []PackageFile
}

// PackageFinding binds one ordinary finding to the owned physical file it
// targets.
type PackageFinding struct {
	File    PackageFile
	Finding Finding
}

// NewPackageFile constructs one read-only package-file view.
func NewPackageFile(
	file *source.File,
	syntax *ast.File,
	fileSet *token.FileSet,
	target bool,
) PackageFile {
	return PackageFile{file: file, syntax: syntax, fileSet: fileSet, target: target}
}

// NewPackageContext constructs one read-only package-wide rule context.
func NewPackageContext(
	fileSet *token.FileSet,
	packageID string,
	package_ *types.Package,
	info *types.Info,
	sizes types.Sizes,
	illTyped bool,
	files []PackageFile,
	dependencies []PackageDependency,
	options OptionSet,
) *PackageContext {
	contextID := &packageContextID{}
	files = slices.Clone(files)
	for index := range files {
		files[index].contextID = contextID
	}
	return &PackageContext{
		fileSet:      fileSet,
		packageID:    packageID,
		package_:     package_,
		info:         info,
		sizes:        sizes,
		illTyped:     illTyped,
		files:        files,
		dependencies: slices.Clone(dependencies),
		options:      options,
		contextID:    contextID,
	}
}

// NewPackageDependency constructs one read-only dependency-package view.
func NewPackageDependency(
	fileSet *token.FileSet,
	packageID string,
	package_ *types.Package,
	info *types.Info,
	sizes types.Sizes,
	illTyped bool,
	files []PackageFile,
) PackageDependency {
	files = slices.Clone(files)
	for index := range files {
		files[index].target = false
		files[index].contextID = nil
	}
	return PackageDependency{
		fileSet: fileSet, packageID: packageID, package_: package_, info: info,
		sizes: sizes, illTyped: illTyped, files: files,
	}
}

// Source returns the exact immutable source captured for this package file.
func (f PackageFile) Source() *source.File { return f.file }

// Syntax returns the shared package AST for this physical file.
func (f PackageFile) Syntax() *ast.File { return f.syntax }

// Target reports whether this package invocation owns diagnostics for the file.
func (f PackageFile) Target() bool { return f.target }

// Range maps an AST node to this package file's exact physical byte range.
func (f PackageFile) Range(node ast.Node) (source.Range, error) {
	if node == nil {
		return source.Range{}, fmt.Errorf("package range requires a syntax node")
	}
	return f.PositionRange(node.Pos(), node.End())
}

// PositionRange maps package positions to this exact physical source.
func (f PackageFile) PositionRange(start, end token.Pos) (source.Range, error) {
	if f.file == nil || f.fileSet == nil {
		return source.Range{}, fmt.Errorf("package range requires source and package positions")
	}
	if !start.IsValid() || !end.IsValid() {
		return source.Range{}, fmt.Errorf("package range positions are invalid")
	}
	physicalStart := f.fileSet.PositionFor(start, false)
	physicalEnd := f.fileSet.PositionFor(end, false)
	if physicalStart.Filename != f.file.Path() || physicalEnd.Filename != f.file.Path() {
		return source.Range{}, fmt.Errorf("package range positions belong to another source file")
	}
	range_ := source.Range{Start: physicalStart.Offset, End: physicalEnd.Offset}
	if _, valid := f.file.Slice(range_); !valid {
		return source.Range{}, fmt.Errorf("package positions map to an invalid physical range")
	}
	return range_, nil
}

// TokenRange maps a package position to this file's exact lexical token.
func (f PackageFile) TokenRange(position token.Pos) (source.Range, error) {
	if f.file == nil || f.fileSet == nil || !position.IsValid() {
		return source.Range{}, fmt.Errorf("package token range requires source and a package position")
	}
	physical := f.fileSet.PositionFor(position, false)
	if physical.Filename != f.file.Path() {
		return source.Range{}, fmt.Errorf("package token position belongs to another source file")
	}
	range_, found := f.file.TokenRangeAtOffset(physical.Offset)
	if !found {
		return source.Range{}, fmt.Errorf("package token position does not identify a physical token")
	}
	return range_, nil
}

// Files returns independent descriptors in canonical physical path order.
func (c *PackageContext) Files() []PackageFile {
	if c == nil {
		return nil
	}
	return slices.Clone(c.files)
}

// Dependencies returns dependency packages in deterministic dependency-first
// order. Rules without a declared dependency-syntax requirement receive none.
func (c *PackageContext) Dependencies() []PackageDependency {
	if c == nil {
		return nil
	}
	return slices.Clone(c.dependencies)
}

// PackageID returns the opaque go/packages identity for this dependency.
func (d PackageDependency) PackageID() string { return d.packageID }

// Package returns the shared read-only go/types package for this dependency.
func (d PackageDependency) Package() *types.Package { return d.package_ }

// Info returns the shared read-only type information for dependency AST nodes.
func (d PackageDependency) Info() *types.Info { return d.info }

// Sizes returns the dependency's architecture-specific type sizes.
func (d PackageDependency) Sizes() types.Sizes { return d.sizes }

// FileSet returns the dependency's shared read-only position mapping.
func (d PackageDependency) FileSet() *token.FileSet { return d.fileSet }

// IllTyped reports whether dependency loading encountered type errors.
func (d PackageDependency) IllTyped() bool { return d.illTyped }

// Files returns independent, non-target dependency file descriptors in
// canonical physical path order.
func (d PackageDependency) Files() []PackageFile { return slices.Clone(d.files) }

// OwnsTarget reports whether a descriptor came from this exact callback and is
// eligible for reporter-visible output.
func (c *PackageContext) OwnsTarget(file PackageFile) bool {
	if c == nil || c.contextID == nil || file.contextID != c.contextID || !file.target {
		return false
	}
	for _, candidate := range c.files {
		if candidate.contextID == file.contextID && candidate.target &&
			candidate.file == file.file && candidate.syntax == file.syntax &&
			candidate.fileSet == file.fileSet {
			return true
		}
	}
	return false
}

// PackageID returns the opaque go/packages identity for this package.
func (c *PackageContext) PackageID() string {
	if c == nil {
		return ""
	}
	return c.packageID
}

// Package returns the shared read-only go/types package.
func (c *PackageContext) Package() *types.Package {
	if c == nil {
		return nil
	}
	return c.package_
}

// Info returns the shared read-only type information for package AST nodes.
func (c *PackageContext) Info() *types.Info {
	if c == nil {
		return nil
	}
	return c.info
}

// Sizes returns the shared architecture-specific type-size implementation.
func (c *PackageContext) Sizes() types.Sizes {
	if c == nil {
		return nil
	}
	return c.sizes
}

// FileSet returns the shared read-only package position mapping.
func (c *PackageContext) FileSet() *token.FileSet {
	if c == nil {
		return nil
	}
	return c.fileSet
}

// IllTyped reports whether package loading encountered type errors.
func (c *PackageContext) IllTyped() bool { return c != nil && c.illTyped }

// BooleanOption returns one configured boolean rule option.
func (c *PackageContext) BooleanOption(name string) (bool, bool) {
	if c == nil {
		return false, false
	}
	return c.options.Boolean(name)
}

// IntegerOption returns one configured integer rule option.
func (c *PackageContext) IntegerOption(name string) (int64, bool) {
	if c == nil {
		return 0, false
	}
	return c.options.Integer(name)
}

// StringOption returns one configured string rule option.
func (c *PackageContext) StringOption(name string) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.options.String(name)
}

// StringsOption returns one independently owned string-list rule option.
func (c *PackageContext) StringsOption(name string) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	return c.options.Strings(name)
}

// ControlFlowContext binds one function graph to its shared typed package and
// exact immutable physical source.
type ControlFlowContext struct {
	typesContext *TypesContext
	function     ast.Node
	body         *ast.BlockStmt
	graph        *cfg.CFG
}

// SSAContext binds one source function to its shared SSA program, typed
// package, and exact immutable physical source.
type SSAContext struct {
	typesContext *TypesContext
	program      *ssa.Program
	ssaPackage   *ssa.Package
	function     *ssa.Function
	syntax       ast.Node
}

// NewSSAContext constructs one read-only SSA rule context.
func NewSSAContext(
	typesContext *TypesContext,
	program *ssa.Program,
	ssaPackage *ssa.Package,
	function *ssa.Function,
	syntax ast.Node,
) *SSAContext {
	return &SSAContext{
		typesContext: typesContext,
		program:      program,
		ssaPackage:   ssaPackage,
		function:     function,
		syntax:       syntax,
	}
}

// Program returns the shared read-only SSA program for the package load.
func (c *SSAContext) Program() *ssa.Program {
	if c == nil {
		return nil
	}
	return c.program
}

// SSAPackage returns the shared read-only SSA package for the function.
func (c *SSAContext) SSAPackage() *ssa.Package {
	if c == nil {
		return nil
	}
	return c.ssaPackage
}

// Function returns the shared read-only SSA function.
func (c *SSAContext) Function() *ssa.Function {
	if c == nil {
		return nil
	}
	return c.function
}

// Syntax returns the function declaration or literal represented by Function.
func (c *SSAContext) Syntax() ast.Node {
	if c == nil {
		return nil
	}
	return c.syntax
}

// IllTyped reports whether package loading encountered type errors.
func (c *SSAContext) IllTyped() bool {
	return c != nil && c.typesContext.IllTyped()
}

// File returns the exact immutable source version for the current package AST.
func (c *SSAContext) File() *source.File {
	if c == nil {
		return nil
	}
	return c.typesContext.File()
}

// PackageID returns the opaque go/packages identity for the current package.
func (c *SSAContext) PackageID() string {
	if c == nil {
		return ""
	}
	return c.typesContext.PackageID()
}

// Package returns the shared read-only go/types package.
func (c *SSAContext) Package() *types.Package {
	if c == nil {
		return nil
	}
	return c.typesContext.Package()
}

// Info returns the shared read-only type information for package AST nodes.
func (c *SSAContext) Info() *types.Info {
	if c == nil {
		return nil
	}
	return c.typesContext.Info()
}

// FileSet returns the shared read-only package position mapping.
func (c *SSAContext) FileSet() *token.FileSet {
	if c == nil || c.typesContext == nil {
		return nil
	}
	return c.typesContext.fileSet
}

// Range maps a package AST node to its current physical source range.
func (c *SSAContext) Range(node ast.Node) (source.Range, error) {
	if c == nil || c.typesContext == nil {
		return source.Range{}, fmt.Errorf("SSA range requires a context")
	}
	return c.typesContext.Range(node)
}

// PositionRange maps package positions to the exact current physical source.
func (c *SSAContext) PositionRange(start, end token.Pos) (source.Range, error) {
	if c == nil || c.typesContext == nil {
		return source.Range{}, fmt.Errorf("SSA range requires a context")
	}
	return c.typesContext.PositionRange(start, end)
}

// TokenRange maps a package position to the exact physical lexical token.
func (c *SSAContext) TokenRange(position token.Pos) (source.Range, error) {
	if c == nil || c.typesContext == nil {
		return source.Range{}, fmt.Errorf("SSA token range requires a context")
	}
	return c.typesContext.TokenRange(position)
}

// BooleanOption returns one configured boolean rule option.
func (c *SSAContext) BooleanOption(name string) (bool, bool) {
	if c == nil || c.typesContext == nil {
		return false, false
	}
	return c.typesContext.BooleanOption(name)
}

// IntegerOption returns one configured integer rule option.
func (c *SSAContext) IntegerOption(name string) (int64, bool) {
	if c == nil || c.typesContext == nil {
		return 0, false
	}
	return c.typesContext.IntegerOption(name)
}

// StringOption returns one configured string rule option.
func (c *SSAContext) StringOption(name string) (string, bool) {
	if c == nil || c.typesContext == nil {
		return "", false
	}
	return c.typesContext.StringOption(name)
}

// StringsOption returns one independently owned string-list rule option.
func (c *SSAContext) StringsOption(name string) ([]string, bool) {
	if c == nil || c.typesContext == nil {
		return nil, false
	}
	return c.typesContext.StringsOption(name)
}

// NewControlFlowContext constructs one read-only control-flow rule context.
func NewControlFlowContext(
	typesContext *TypesContext,
	function ast.Node,
	body *ast.BlockStmt,
	graph *cfg.CFG,
) *ControlFlowContext {
	return &ControlFlowContext{
		typesContext: typesContext,
		function:     function,
		body:         body,
		graph:        graph,
	}
}

// Function returns the function declaration or literal that owns the graph.
func (c *ControlFlowContext) Function() ast.Node {
	if c == nil {
		return nil
	}
	return c.function
}

// Body returns the function body used to construct the graph.
func (c *ControlFlowContext) Body() *ast.BlockStmt {
	if c == nil {
		return nil
	}
	return c.body
}

// Graph returns the shared read-only control-flow graph for the function.
func (c *ControlFlowContext) Graph() *cfg.CFG {
	if c == nil {
		return nil
	}
	return c.graph
}

// IllTyped reports whether package loading encountered type errors.
func (c *ControlFlowContext) IllTyped() bool {
	return c != nil && c.typesContext.IllTyped()
}

// File returns the exact immutable source version for the current package AST.
func (c *ControlFlowContext) File() *source.File {
	if c == nil {
		return nil
	}
	return c.typesContext.File()
}

// PackageID returns the opaque go/packages identity for the current package.
func (c *ControlFlowContext) PackageID() string {
	if c == nil {
		return ""
	}
	return c.typesContext.PackageID()
}

// Package returns the shared read-only go/types package.
func (c *ControlFlowContext) Package() *types.Package {
	if c == nil {
		return nil
	}
	return c.typesContext.Package()
}

// Info returns the shared read-only type information for package AST nodes.
func (c *ControlFlowContext) Info() *types.Info {
	if c == nil {
		return nil
	}
	return c.typesContext.Info()
}

// Range maps a package AST node to its current physical source range.
func (c *ControlFlowContext) Range(node ast.Node) (source.Range, error) {
	if c == nil || c.typesContext == nil {
		return source.Range{}, fmt.Errorf("control-flow range requires a context")
	}
	return c.typesContext.Range(node)
}

// PositionRange maps package positions to the exact current physical source.
func (c *ControlFlowContext) PositionRange(start, end token.Pos) (source.Range, error) {
	if c == nil || c.typesContext == nil {
		return source.Range{}, fmt.Errorf("control-flow range requires a context")
	}
	return c.typesContext.PositionRange(start, end)
}

// BooleanOption returns one configured boolean rule option.
func (c *ControlFlowContext) BooleanOption(name string) (bool, bool) {
	if c == nil || c.typesContext == nil {
		return false, false
	}
	return c.typesContext.BooleanOption(name)
}

// IntegerOption returns one configured integer rule option.
func (c *ControlFlowContext) IntegerOption(name string) (int64, bool) {
	if c == nil || c.typesContext == nil {
		return 0, false
	}
	return c.typesContext.IntegerOption(name)
}

// StringOption returns one configured string rule option.
func (c *ControlFlowContext) StringOption(name string) (string, bool) {
	if c == nil || c.typesContext == nil {
		return "", false
	}
	return c.typesContext.StringOption(name)
}

// StringsOption returns one independently owned string-list rule option.
func (c *ControlFlowContext) StringsOption(name string) ([]string, bool) {
	if c == nil || c.typesContext == nil {
		return nil, false
	}
	return c.typesContext.StringsOption(name)
}

// NewTypesContext constructs one read-only typed rule context.
func NewTypesContext(
	file *source.File,
	fileSet *token.FileSet,
	packageID string,
	package_ *types.Package,
	info *types.Info,
	illTyped bool,
	options OptionSet,
) *TypesContext {
	return &TypesContext{
		file:      file,
		fileSet:   fileSet,
		packageID: packageID,
		package_:  package_,
		info:      info,
		illTyped:  illTyped,
		options:   options,
	}
}

// IllTyped reports whether package loading encountered type errors.
func (c *TypesContext) IllTyped() bool {
	return c != nil && c.illTyped
}

// File returns the exact immutable source version for the current package AST.
func (c *TypesContext) File() *source.File {
	if c == nil {
		return nil
	}
	return c.file
}

// BooleanOption returns one configured boolean rule option.
func (c *TypesContext) BooleanOption(name string) (bool, bool) {
	if c == nil {
		return false, false
	}
	return c.options.Boolean(name)
}

// IntegerOption returns one configured integer rule option.
func (c *TypesContext) IntegerOption(name string) (int64, bool) {
	if c == nil {
		return 0, false
	}
	return c.options.Integer(name)
}

// StringOption returns one configured string rule option.
func (c *TypesContext) StringOption(name string) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.options.String(name)
}

// StringsOption returns one independently owned string-list rule option.
func (c *TypesContext) StringsOption(name string) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	return c.options.Strings(name)
}

// PackageID returns the opaque go/packages identity for the current package.
func (c *TypesContext) PackageID() string {
	if c == nil {
		return ""
	}
	return c.packageID
}

// Package returns the shared read-only go/types package.
func (c *TypesContext) Package() *types.Package {
	if c == nil {
		return nil
	}
	return c.package_
}

// Info returns the shared read-only type information for package AST nodes.
func (c *TypesContext) Info() *types.Info {
	if c == nil {
		return nil
	}
	return c.info
}

// Range maps a package AST node to its current physical source range.
func (c *TypesContext) Range(node ast.Node) (source.Range, error) {
	if node == nil {
		return source.Range{}, fmt.Errorf("typed range requires a syntax node")
	}
	return c.PositionRange(node.Pos(), node.End())
}

// PositionRange maps package positions to the exact current physical source.
func (c *TypesContext) PositionRange(start, end token.Pos) (source.Range, error) {
	if c == nil || c.file == nil || c.fileSet == nil {
		return source.Range{}, fmt.Errorf("typed range requires source and package positions")
	}
	if !start.IsValid() || !end.IsValid() {
		return source.Range{}, fmt.Errorf("typed range positions are invalid")
	}
	physicalStart := c.fileSet.PositionFor(start, false)
	physicalEnd := c.fileSet.PositionFor(end, false)
	if physicalStart.Filename != c.file.Path() || physicalEnd.Filename != c.file.Path() {
		return source.Range{}, fmt.Errorf("typed range positions belong to another source file")
	}
	range_ := source.Range{Start: physicalStart.Offset, End: physicalEnd.Offset}
	if _, valid := c.file.Slice(range_); !valid {
		return source.Range{}, fmt.Errorf("typed positions map to an invalid physical range")
	}
	return range_, nil
}

// TokenRange maps a package position to the exact physical lexical token.
func (c *TypesContext) TokenRange(position token.Pos) (source.Range, error) {
	if c == nil || c.file == nil || c.fileSet == nil || !position.IsValid() {
		return source.Range{}, fmt.Errorf("typed token range requires source and a package position")
	}
	physical := c.fileSet.PositionFor(position, false)
	if physical.Filename != c.file.Path() {
		return source.Range{}, fmt.Errorf("typed token position belongs to another source file")
	}
	range_, found := c.file.TokenRangeAtOffset(physical.Offset)
	if !found {
		return source.Range{}, fmt.Errorf("typed token position does not identify a physical token")
	}
	return range_, nil
}

// Range maps an isolated syntax node to its exact physical byte range.
func (c *Context) Range(node ast.Node) (source.Range, error) {
	if c == nil || c.file == nil || node == nil {
		return source.Range{}, fmt.Errorf("range requires a source file and syntax node")
	}
	start, startFound := c.file.PhysicalOffset(node.Pos())
	end, endFound := c.file.PhysicalOffset(node.End())
	result := source.Range{Start: start, End: end}
	if !startFound || !endFound {
		return source.Range{}, fmt.Errorf("syntax node positions do not map to physical source")
	}
	if _, valid := c.file.Slice(result); !valid {
		return source.Range{}, fmt.Errorf("syntax node maps to an invalid physical range")
	}
	return result, nil
}

// Related identifies a secondary physical source range.
type Related struct {
	Range   source.Range
	Message string
}

// Edit is one exact byte replacement proposed by a fix.
type Edit struct {
	Range   source.Range
	NewText string
}

// Fix is one named, safety-classified set of source edits.
type Fix struct {
	Name   string
	Safety FixSafety
	Edits  []Edit
}

// Finding is the rule-owned portion of a diagnostic.
type Finding struct {
	MessageKey string
	Message    string
	Range      source.Range
	Related    []Related
	Notes      []string
	Help       string
	Fixes      []Fix
}

// Diagnostic is one source-versioned, reporter-ready lint diagnostic.
type Diagnostic struct {
	RuleID     string
	Severity   Severity
	MessageKey string
	Message    string
	Path       string
	Digest     source.Digest
	Range      source.Range
	Related    []Related
	Notes      []string
	Help       string
	Fixes      []Fix
}

// Selection is one enabled rule with its resolved severity and cost.
type Selection struct {
	ID          string
	Severity    Severity
	Requirement Requirement
	Options     OptionSet
}

// File returns the exact immutable source version for the syntax rule.
func (c *Context) File() *source.File {
	if c == nil {
		return nil
	}
	return c.file
}

// BooleanOption returns one configured boolean rule option.
func (c *Context) BooleanOption(name string) (bool, bool) {
	if c == nil {
		return false, false
	}
	return c.options.Boolean(name)
}

// IntegerOption returns one configured integer rule option.
func (c *Context) IntegerOption(name string) (int64, bool) {
	if c == nil {
		return 0, false
	}
	return c.options.Integer(name)
}

// StringOption returns one configured string rule option.
func (c *Context) StringOption(name string) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.options.String(name)
}

// StringsOption returns one independently owned string-list rule option.
func (c *Context) StringsOption(name string) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	return c.options.Strings(name)
}

// PositionRange converts a token range when a caller already owns positions.
func (c *Context) PositionRange(start, end token.Pos) (source.Range, error) {
	if c == nil || c.file == nil {
		return source.Range{}, fmt.Errorf("range requires a source file")
	}
	physicalStart, startFound := c.file.PhysicalOffset(start)
	physicalEnd, endFound := c.file.PhysicalOffset(end)
	result := source.Range{Start: physicalStart, End: physicalEnd}
	if !startFound || !endFound {
		return source.Range{}, fmt.Errorf("positions do not map to physical source")
	}
	if _, valid := c.file.Slice(result); !valid {
		return source.Range{}, fmt.Errorf("positions map to an invalid physical range")
	}
	return result, nil
}

// PreviousSignificantToken returns the lexical token before a parsed position,
// ignoring comments and automatically inserted semicolons.
func (c *Context) PreviousSignificantToken(position token.Pos) (source.Token, bool) {
	if c == nil || c.file == nil {
		return source.Token{}, false
	}
	return c.file.PreviousSignificantToken(position)
}
