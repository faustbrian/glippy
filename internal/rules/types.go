// Package rules defines native lint-rule contracts and canonical metadata.
package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"sync"

	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/mod/module"
	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"
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
	SeverityOff Severity = "off"
	SeverityWarn Severity = "warn"
	SeverityError Severity = "error"
)

// LintLevel is one command-line diagnostic policy compatible with Clippy's
// allow, warn, deny, and forbid model.
type LintLevel string

const (
	LintAllow LintLevel = "allow"
	LintWarn LintLevel = "warn"
	LintDeny LintLevel = "deny"
	LintForbid LintLevel = "forbid"
)

// LintLevelDirective applies one ordered level to exact rule IDs or preset
// groups. A forbidden rule cannot be lowered by a later directive.
type LintLevelDirective struct {
	Level LintLevel
	Targets []string
}

// Preset is one coherent built-in rule group.
type Preset string

const (
	PresetCorrectness Preset = "correctness"
	PresetSuspicious Preset = "suspicious"
	PresetPerformance Preset = "performance"
	PresetComplexity Preset = "complexity"
	PresetStyle Preset = "style"
	PresetPedantic Preset = "pedantic"
	PresetNursery Preset = "nursery"
	PresetRestriction Preset = "restriction"
	PresetMigration Preset = "migration"
)

// Category classifies the observable concern reported by a rule.
type Category string

const (
	CategoryCorrectness Category = "correctness"
	CategorySafety Category = "safety"
	CategorySuspicious Category = "suspicious"
	CategoryPerformance Category = "performance"
	CategoryComplexity Category = "complexity"
	CategoryStyle Category = "style"
	CategoryMigration Category = "migration"
	CategoryMaintainability Category = "maintainability"
)

// FixSafety controls whether a fix may run without an explicit opt-in.
type FixSafety string

const (
	FixSafe FixSafety = "safe"
	FixSuggestion FixSafety = "suggestion"
	FixUnsafe FixSafety = "unsafe"
)

// FixMetadata describes one named transformation a rule may offer.
type FixMetadata struct {
	Name string
	Description string
	Safety FixSafety
}

// OptionKind is the scalar type accepted by one rule option.
type OptionKind string

const (
	OptionBoolean OptionKind = "boolean"
	OptionInteger OptionKind = "integer"
	OptionString OptionKind = "string"
	OptionStrings OptionKind = "strings"
)

// OptionMetadata is one typed configuration field owned by a rule.
type OptionMetadata struct {
	Name string
	Summary string
	Kind OptionKind
	Required bool
	Default *OptionValue
	Minimum *int64
	Maximum *int64
}

// Deprecation records a rule replacement without silently changing its ID.
type Deprecation struct {
	Since string
	Replacement string
	Message string
}

// Example is one paired incorrect and correct rule example.
type Example struct {
	Title string
	Incorrect string
	Correct string
}

// Metadata is the canonical rule documentation and scheduling contract.
type Metadata struct {
	ID string
	Summary string
	Documentation string
	DefaultSeverity Severity
	Presets []Preset
	MinimumGoVersion string
	Requirement Requirement
	NodeInterests []NodeKind
	RequiresDependencySyntax bool
	RequiresEffectFacts bool
	RunOnGenerated bool
	RunDespiteTypeErrors bool
	Categories []Category
	Fixes []FixMetadata
	Options []OptionMetadata
	Deprecation *Deprecation
	KnownLimitations []string
	Examples []Example
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

// SSAInitializerRule marks an SSA rule that also needs package-level variable
// initializer instructions. The scheduler invokes RunSSA once per physical
// source file with the synthetic package init function and that file's AST.
type SSAInitializerRule interface {
	SSARule
	RunsOnSSAInitializers()
}

// SSADebugRule marks an SSA rule that needs source-expression-to-value
// mappings. The scheduler enables the more expensive x/tools debug mapping
// only when at least one selected SSA rule declares this requirement.
type SSADebugRule interface {
	SSARule
	RequiresSSADebug()
}

// Context is the immutable per-file syntax rule context.
type Context struct {
	file *source.File
	options OptionSet
}

// NewContext creates an immutable per-file rule context.
func NewContext(file *source.File, options OptionSet) *Context {
	return &Context{file: file, options: options}
}

// TypesContext binds one package AST to its exact immutable physical source.
type TypesContext struct {
	file *source.File
	syntax *ast.File
	packageSyntax *PackageSyntax
	fileSet *token.FileSet
	packageID string
	package_ *types.Package
	info *types.Info
	illTyped bool
	options OptionSet
	memoLock sync.Mutex
	memo map[string]any
}

// PackageSyntax is one immutable package-level syntax view shared by every
// per-file and per-rule context in an analysis tier.
type PackageSyntax struct {
	files []*ast.File
	memoLock sync.Mutex
	memo map[string]any
}

// NewPackageSyntax snapshots one package's syntax slice while sharing the
// immutable ASTs themselves.
func NewPackageSyntax(files []*ast.File) *PackageSyntax {
	return &PackageSyntax{files: slices.Clone(files)}
}

// Len returns the number of package syntax trees.
func (s *PackageSyntax) Len() int {
	if s == nil {
		return 0
	}
	return len(s.files)
}

// At returns one immutable syntax tree by canonical package index.
func (s *PackageSyntax) At(index int) *ast.File {
	if s == nil || index < 0 || index >= len(s.files) {
		return nil
	}
	return s.files[index]
}

func (s *PackageSyntax) memoized(key string, build func() any) any {
	if s == nil || build == nil {
		return nil
	}
	s.memoLock.Lock()
	defer s.memoLock.Unlock()
	if s.memo == nil {
		s.memo = make(map[string]any)
	}
	if value, found := s.memo[key]; found {
		return value
	}
	value := build()
	s.memo[key] = value
	return value
}

// PackageFile binds one package AST to its exact immutable physical source and
// records whether the current package invocation owns reporter-visible output
// for that file.
type PackageFile struct {
	file *source.File
	syntax *ast.File
	fileSet *token.FileSet
	target bool
	contextID *packageContextID
}

type packageContextID struct {
	marker byte
}

// PackageContext exposes one shared typed package and its canonical physical
// source mapping to a package-wide native rule.
type PackageContext struct {
	fileSet *token.FileSet
	packageID string
	package_ *types.Package
	info *types.Info
	sizes types.Sizes
	illTyped bool
	files []PackageFile
	dependencies []PackageDependency
	options OptionSet
	contextID *packageContextID
}

// PackageDependency is one immutable dependency package view exposed only to
// package-wide rules that declare dependency syntax in canonical metadata.
type PackageDependency struct {
	fileSet *token.FileSet
	packageID string
	package_ *types.Package
	info *types.Info
	sizes types.Sizes
	illTyped bool
	files []PackageFile
}

// PackageFinding binds one ordinary finding to the owned physical file it
// targets.
type PackageFinding struct {
	File PackageFile
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
		fileSet: fileSet,
		packageID: packageID,
		package_: package_,
		info: info,
		sizes: sizes,
		illTyped: illTyped,
		files: files,
		dependencies: slices.Clone(dependencies),
		options: options,
		contextID: contextID,
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
		fileSet: fileSet,
		packageID: packageID,
		package_: package_,
		info: info,
		sizes: sizes,
		illTyped: illTyped,
		files: files,
	}
}

// Source returns the exact immutable source captured for this package file.
func (f PackageFile) Source() *source.File {
	return f.file
}

// Syntax returns the shared package AST for this physical file.
func (f PackageFile) Syntax() *ast.File {
	return f.syntax
}

// Target reports whether this package invocation owns diagnostics for the file.
func (f PackageFile) Target() bool {
	return f.target
}

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
		return source.Range{}, fmt.Errorf(
			"package range requires source and package positions",
		)
	}
	if !start.IsValid() || !end.IsValid() {
		return source.Range{}, fmt.Errorf("package range positions are invalid")
	}
	physicalStart := f.fileSet.PositionFor(start, false)
	physicalEnd := f.fileSet.PositionFor(end, false)
	if physicalStart.Filename != f.file.Path() || physicalEnd.Filename != f.file.Path() {
		return source.Range{}, fmt.Errorf(
			"package range positions belong to another source file",
		)
	}
	range_ := source.Range{Start: physicalStart.Offset, End: physicalEnd.Offset}
	if _, valid := f.file.Slice(range_); !valid {
		return source.Range{}, fmt.Errorf(
			"package positions map to an invalid physical range",
		)
	}
	return range_, nil
}

// TokenRange maps a package position to this file's exact lexical token.
func (f PackageFile) TokenRange(position token.Pos) (source.Range, error) {
	if f.file == nil || f.fileSet == nil || !position.IsValid() {
		return source.Range{}, fmt.Errorf(
			"package token range requires source and a package position",
		)
	}
	physical := f.fileSet.PositionFor(position, false)
	if physical.Filename != f.file.Path() {
		return source.Range{}, fmt.Errorf(
			"package token position belongs to another source file",
		)
	}
	range_, found := f.file.TokenRangeAtOffset(physical.Offset)
	if !found {
		return source.Range{}, fmt.Errorf(
			"package token position does not identify a physical token",
		)
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
func (d PackageDependency) PackageID() string {
	return d.packageID
}

// Package returns the shared read-only go/types package for this dependency.
func (d PackageDependency) Package() *types.Package {
	return d.package_
}

// Info returns the shared read-only type information for dependency AST nodes.
func (d PackageDependency) Info() *types.Info {
	return d.info
}

// Sizes returns the dependency's architecture-specific type sizes.
func (d PackageDependency) Sizes() types.Sizes {
	return d.sizes
}

// FileSet returns the dependency's shared read-only position mapping.
func (d PackageDependency) FileSet() *token.FileSet {
	return d.fileSet
}

// IllTyped reports whether dependency loading encountered type errors.
func (d PackageDependency) IllTyped() bool {
	return d.illTyped
}

// Files returns independent, non-target dependency file descriptors in
// canonical physical path order.
func (d PackageDependency) Files() []PackageFile {
	return slices.Clone(d.files)
}

// OwnsTarget reports whether a descriptor came from this exact callback and is
// eligible for reporter-visible output.
func (c *PackageContext) OwnsTarget(file PackageFile) bool {
	if c == nil || c.contextID == nil || file.contextID != c.contextID || !file.target {
		return false
	}
	for _, candidate := range c.files {
		if candidate.contextID == file.contextID &&
			candidate.target &&
			candidate.file == file.file &&
			candidate.syntax == file.syntax &&
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
func (c *PackageContext) IllTyped() bool {
	return c != nil && c.illTyped
}

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
	function ast.Node
	body *ast.BlockStmt
	graph *cfg.CFG
	effects EffectFacts
	callMayReturn func(*ast.CallExpr) bool
	callIsTestingSkip func(*ast.CallExpr) bool
	callIsTestingFailure func(*ast.CallExpr) bool
	shared *ControlFlowShared
}

// ControlFlowShared owns lazily constructed, rule-independent state for one
// function graph. The scheduler supplies one instance to every selected CFG
// rule so compatible rules do not repeat the same bounded propagation.
type ControlFlowShared struct {
	lockStateOnce sync.Once
	lockState *lockStateAnalysis
	resourceUseOnce sync.Once
	resourceUse *resourceUseAnalysis
	channelUseOnce sync.Once
	channelUse *channelUseAnalysis
	httpResponseBodyStateOnce sync.Once
	httpResponseBodyState *httpResponseBodyStateAnalysis
	transactionStateOnce sync.Once
	transactionState *sqlTransactionStateAnalysis
	waitGroupCounterOnce sync.Once
	waitGroupCounter *waitGroupCounterAnalysis
}

// NewControlFlowShared constructs one function-local shared analysis cache.
func NewControlFlowShared() *ControlFlowShared {
	return &ControlFlowShared{}
}

// ParameterEffectKind identifies a proven terminal ownership effect applied
// to one function parameter.
type ParameterEffectKind uint8

const (
	ParameterEffectClose ParameterEffectKind = 1 << iota
	ParameterEffectTransactionComplete
	ParameterEffectCancelInvoke
	ParameterEffectTransfer
)

// ParameterEffectSummary describes every normally returning path through one
// statically resolved function parameter. Known distinguishes a proven borrow
// from an unavailable or ambiguous summary. Always is true only when every
// normally returning path reaches one of Kinds before returning.
// GuaranteedKinds contains effects independently present on every path, while
// Kinds also includes effects that occur only on alternative terminal paths.
type ParameterEffectSummary struct {
	Known bool
	Always bool
	Kinds ParameterEffectKind
	GuaranteedKinds ParameterEffectKind
}

// NilState is a proven nilness state for one returned value.
type NilState uint8

const (
	NilStateUnknown NilState = iota
	NilStateNil
	NilStateNonNil
)

// ReturnStateSummary relates one nil-capable result to one error result.
// Each field is independently unknown unless every matching return proves the
// same state.
type ReturnStateSummary struct {
	WhenErrorNil NilState
	WhenErrorNonNil NilState
}

// GuaranteesAny reports whether at least one accepted effect is independently
// guaranteed or every possible terminal effect belongs to the accepted set.
func (s ParameterEffectSummary) GuaranteesAny(accepted ParameterEffectKind) bool {
	return s.Known &&
		s.Always &&
		(s.GuaranteedKinds & accepted != 0 || s.Kinds != 0 && s.Kinds & ^accepted == 0)
}

// EffectFacts exposes immutable, stable cross-load semantic summaries to
// control-flow rules without exposing dependency source as lint targets.
type EffectFacts interface {
	ParameterEffect(*types.Func, int) ParameterEffectSummary
	ReceiverEffect(*types.Func) ParameterEffectSummary
	WriterBorrow(*types.Func, int) bool
	NoOpClose(*types.Func) bool
	ReturnState(*types.Func, int, int) ReturnStateSummary
	ResultState(*types.Func, int) NilState
	MustUseResult(*types.Func, int) bool
	Blocking(*types.Func) bool
	ReturnAliasesArgument(*types.Func, int, int) bool
	CleanupManagedResult(*types.Func, int) bool
}

// SSAContext binds one source function or package initializer to its shared SSA
// program, typed package, and exact immutable physical source.
type SSAContext struct {
	typesContext *TypesContext
	program *ssa.Program
	ssaPackage *ssa.Package
	function *ssa.Function
	syntax ast.Node
	effects EffectFacts
}

// NewSSAContext constructs one read-only SSA rule context.
func NewSSAContext(
	typesContext *TypesContext,
	program *ssa.Program,
	ssaPackage *ssa.Package,
	function *ssa.Function,
	syntax ast.Node,
	effects EffectFacts,
) *SSAContext {
	return &SSAContext{
		typesContext: typesContext,
		program: program,
		ssaPackage: ssaPackage,
		function: function,
		syntax: syntax,
		effects: effects,
	}
}

// ReturnState returns a conservative nil/error relation for a statically
// resolved function result pair.
func (c *SSAContext) ReturnState(
	function *types.Func,
	valueResult int,
	errorResult int,
) ReturnStateSummary {
	if c == nil || c.effects == nil || function == nil {
		return ReturnStateSummary{}
	}
	return c.effects.ReturnState(function, valueResult, errorResult)
}

// ResultState returns the proven unconditional nilness of one function result.
func (c *SSAContext) ResultState(function *types.Func, result int) NilState {
	if c == nil || c.effects == nil || function == nil || result < 0 {
		return NilStateUnknown
	}
	return c.effects.ResultState(function, result)
}

// Program returns the shared read-only SSA program for the current package.
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

// Syntax returns the declaration or literal represented by Function. An
// initializer context returns the physical file AST whose package-level
// initializer expressions are eligible to report.
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

// PackageSyntax returns the package-loaded syntax trees indexed by Info.
// Callers must treat the returned trees as immutable.
func (c *SSAContext) PackageSyntax() *PackageSyntax {
	if c == nil || c.typesContext == nil {
		return nil
	}
	return c.typesContext.PackageSyntax()
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
	effects EffectFacts,
	callMayReturn func(*ast.CallExpr) bool,
	callIsTestingSkip func(*ast.CallExpr) bool,
	callIsTestingFailure func(*ast.CallExpr) bool,
	shared *ControlFlowShared,
) *ControlFlowContext {
	return &ControlFlowContext{
		typesContext: typesContext,
		function: function,
		body: body,
		graph: graph,
		effects: effects,
		callMayReturn: callMayReturn,
		callIsTestingSkip: callIsTestingSkip,
		callIsTestingFailure: callIsTestingFailure,
		shared: shared,
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

// CallMayReturn reports whether the shared no-return analysis found a normal
// continuation for one call. Missing or dynamic call information is
// conservative and therefore returns true.
func (c *ControlFlowContext) CallMayReturn(call *ast.CallExpr) bool {
	if c == nil || c.callMayReturn == nil || call == nil {
		return true
	}
	return c.callMayReturn(call)
}

// CallIsTestingSkip reports whether a statically resolved no-return call is a
// direct testing skip or a selected local-source wrapper whose terminal paths
// are testing skips. Unknown and dynamic calls conservatively return false.
func (c *ControlFlowContext) CallIsTestingSkip(call *ast.CallExpr) bool {
	if c == nil || c.callIsTestingSkip == nil || call == nil {
		return false
	}
	return c.callIsTestingSkip(call)
}

// CallIsTestingFailure reports whether a statically resolved no-return call is
// a direct testing failure or a selected local-source wrapper whose terminal
// paths are testing failures. Unknown and dynamic calls conservatively return
// false.
func (c *ControlFlowContext) CallIsTestingFailure(call *ast.CallExpr) bool {
	if c == nil || c.callIsTestingFailure == nil || call == nil {
		return false
	}
	return c.callIsTestingFailure(call)
}

// ParameterEffect returns the conservative summary for one argument of a
// statically resolved call. Dynamic calls and invalid argument indexes are
// unknown.
func (c *ControlFlowContext) ParameterEffect(
	call *ast.CallExpr,
	argument int,
) ParameterEffectSummary {
	if c == nil ||
		c.typesContext == nil ||
		c.effects == nil ||
		call == nil ||
		argument < 0 ||
		argument >= len(call.Args) {
		return ParameterEffectSummary{}
	}
	callee := typeutil.StaticCallee(c.typesContext.info, call)
	if callee == nil {
		return ParameterEffectSummary{}
	}
	parameter, valid := StaticCallParameter(c.typesContext.info, call, callee, argument)
	if !valid {
		return ParameterEffectSummary{}
	}
	return c.effects.ParameterEffect(callee, parameter)
}

// WriterBorrow reports whether one statically resolved call parameter is
// proven to borrow an io.Writer synchronously without retaining, returning,
// reinitializing, or otherwise exposing it.
func (c *ControlFlowContext) WriterBorrow(call *ast.CallExpr, argument int) bool {
	if c == nil ||
		c.typesContext == nil ||
		c.effects == nil ||
		call == nil ||
		argument < 0 ||
		argument >= len(call.Args) {
		return false
	}
	callee := typeutil.StaticCallee(c.typesContext.info, call)
	if callee == nil {
		return false
	}
	parameter, valid := StaticCallParameter(c.typesContext.info, call, callee, argument)
	return valid && c.effects.WriterBorrow(callee, parameter)
}

// ReceiverEffect returns the conservative summary for the receiver of one
// statically resolved method call. Dynamic calls and functions without a
// receiver are unknown.
func (c *ControlFlowContext) ReceiverEffect(call *ast.CallExpr) ParameterEffectSummary {
	callee := c.staticCallee(call)
	if callee == nil {
		return ParameterEffectSummary{}
	}
	return c.effects.ReceiverEffect(callee)
}

// NoOpCloser reports whether selected local source proves the exact
// conventional Close method satisfies the versioned no-op close contract.
func (c *ControlFlowContext) NoOpCloser(type_ types.Type) bool {
	if c == nil || c.typesContext == nil || c.effects == nil || type_ == nil {
		return false
	}
	object, _, _ := types.LookupFieldOrMethod(type_, true, c.typesContext.package_, "Close")
	method, _ := object.(*types.Func)
	return method != nil && c.effects.NoOpClose(method)
}

// MustUse reports whether a configured contract requires one result from a
// statically resolved call to be consumed.
func (c *ControlFlowContext) MustUse(call *ast.CallExpr, result int) bool {
	callee := c.staticCallee(call)
	return callee != nil && result >= 0 && c.effects.MustUseResult(callee, result)
}

// Blocking reports whether a configured contract marks a statically resolved
// call as a blocking operation.
func (c *ControlFlowContext) Blocking(call *ast.CallExpr) bool {
	callee := c.staticCallee(call)
	return callee != nil && c.effects.Blocking(callee)
}

// ReturnAliasesArgument reports whether a configured contract states that one
// call result aliases the selected call argument.
func (c *ControlFlowContext) ReturnAliasesArgument(
	call *ast.CallExpr,
	result int,
	argument int,
) bool {
	callee := c.staticCallee(call)
	if callee == nil || result < 0 || argument < 0 || argument >= len(call.Args) {
		return false
	}
	parameter, valid := StaticCallParameter(c.typesContext.info, call, callee, argument)
	return valid && c.effects.ReturnAliasesArgument(callee, result, parameter)
}

// CleanupManagedResult reports whether an exact call result is returned from a
// helper only after cleanup has been registered on every normally returning
// path.
func (c *ControlFlowContext) CleanupManagedResult(call *ast.CallExpr, result int) bool {
	if c == nil || c.effects == nil {
		return false
	}
	callee := c.staticCallee(call)
	return callee != nil && result >= 0 && c.effects.CleanupManagedResult(callee, result)
}

// ReturnState returns a configured nil/error relationship for one statically
// resolved call.
func (c *ControlFlowContext) ReturnState(
	call *ast.CallExpr,
	valueResult int,
	errorResult int,
) ReturnStateSummary {
	callee := c.staticCallee(call)
	if callee == nil || valueResult < 0 || errorResult < 0 {
		return ReturnStateSummary{}
	}
	return c.effects.ReturnState(callee, valueResult, errorResult)
}

// ResultState returns the proven unconditional nilness of one statically
// resolved call result.
func (c *ControlFlowContext) ResultState(call *ast.CallExpr, result int) NilState {
	callee := c.staticCallee(call)
	if callee == nil || result < 0 {
		return NilStateUnknown
	}
	return c.effects.ResultState(callee, result)
}

// ResultStateFor returns the proven unconditional nilness of one result from
// an exact statically selected function or method.
func (c *ControlFlowContext) ResultStateFor(function *types.Func, result int) NilState {
	if c == nil || c.effects == nil || function == nil || result < 0 {
		return NilStateUnknown
	}
	return c.effects.ResultState(function, result)
}

func (c *ControlFlowContext) staticCallee(call *ast.CallExpr) *types.Func {
	if c == nil || c.typesContext == nil || c.effects == nil || call == nil {
		return nil
	}
	return typeutil.StaticCallee(c.typesContext.info, call)
}

// StaticCallParameter maps one call argument to the callee parameter index.
// Method-expression calls expose the receiver as argument zero even though the
// receiver is not part of a configured or inferred parameter contract.
func StaticCallParameter(
	info *types.Info,
	call *ast.CallExpr,
	callee *types.Func,
	argument int,
) (int, bool) {
	if info == nil || call == nil || callee == nil || argument < 0 {
		return 0, false
	}
	signature, _ := callee.Type().(*types.Signature)
	if signature == nil || signature.Params() == nil {
		return 0, false
	}
	parameter := argument
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selection := info.Selections[selector];
		selection != nil && selection.Kind() == types.MethodExpr {
		parameter--
	}
	if parameter < 0 {
		return 0, false
	}
	if signature.Variadic() && parameter >= signature.Params().Len() - 1 {
		parameter = signature.Params().Len() - 1
	}
	return parameter, parameter >= 0 && parameter < signature.Params().Len()
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

// PackageSyntax returns the package-loaded syntax trees indexed by Info.
// Callers must treat the returned trees as immutable.
func (c *ControlFlowContext) PackageSyntax() *PackageSyntax {
	if c == nil || c.typesContext == nil {
		return nil
	}
	return c.typesContext.PackageSyntax()
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

// TokenRange maps a package position to the exact physical lexical token.
func (c *ControlFlowContext) TokenRange(position token.Pos) (source.Range, error) {
	if c == nil || c.typesContext == nil {
		return source.Range{}, fmt.Errorf("control-flow token range requires a context")
	}
	return c.typesContext.TokenRange(position)
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
	syntax *ast.File,
	fileSet *token.FileSet,
	packageID string,
	package_ *types.Package,
	info *types.Info,
	illTyped bool,
	options OptionSet,
	packageSyntax *PackageSyntax,
) *TypesContext {
	return &TypesContext{
		file: file,
		syntax: syntax,
		packageSyntax: packageSyntax,
		fileSet: fileSet,
		packageID: packageID,
		package_: package_,
		info: info,
		illTyped: illTyped,
		options: options,
	}
}

// PackageSyntax returns the package-loaded syntax trees indexed by Info.
// Callers must treat the returned trees as immutable.
func (c *TypesContext) PackageSyntax() *PackageSyntax {
	if c == nil {
		return nil
	}
	return c.packageSyntax
}

func (c *TypesContext) memoized(key string, build func() any) any {
	if c == nil || build == nil {
		return nil
	}
	c.memoLock.Lock()
	defer c.memoLock.Unlock()
	if c.memo == nil {
		c.memo = make(map[string]any)
	}
	if value, found := c.memo[key]; found {
		return value
	}
	value := build()
	c.memo[key] = value
	return value
}

// Syntax returns the package-loaded syntax tree whose nodes are indexed by
// Info. Callers must treat it as immutable.
func (c *TypesContext) Syntax() *ast.File {
	if c == nil {
		return nil
	}
	return c.syntax
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
		return source.Range{}, fmt.Errorf(
			"typed range requires source and package positions",
		)
	}
	if !start.IsValid() || !end.IsValid() {
		return source.Range{}, fmt.Errorf("typed range positions are invalid")
	}
	physicalStart := c.fileSet.PositionFor(start, false)
	physicalEnd := c.fileSet.PositionFor(end, false)
	if physicalStart.Filename != c.file.Path() || physicalEnd.Filename != c.file.Path() {
		return source.Range{}, fmt.Errorf(
			"typed range positions belong to another source file",
		)
	}
	range_ := source.Range{Start: physicalStart.Offset, End: physicalEnd.Offset}
	if _, valid := c.file.Slice(range_); !valid {
		return source.Range{}, fmt.Errorf(
			"typed positions map to an invalid physical range",
		)
	}
	return range_, nil
}

// TokenRange maps a package position to the exact physical lexical token.
func (c *TypesContext) TokenRange(position token.Pos) (source.Range, error) {
	if c == nil || c.file == nil || c.fileSet == nil || !position.IsValid() {
		return source.Range{}, fmt.Errorf(
			"typed token range requires source and a package position",
		)
	}
	physical := c.fileSet.PositionFor(position, false)
	if physical.Filename != c.file.Path() {
		return source.Range{}, fmt.Errorf(
			"typed token position belongs to another source file",
		)
	}
	range_, found := c.file.TokenRangeAtOffset(physical.Offset)
	if !found {
		return source.Range{}, fmt.Errorf(
			"typed token position does not identify a physical token",
		)
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
		return source.Range{}, fmt.Errorf(
			"syntax node positions do not map to physical source",
		)
	}
	if _, valid := c.file.Slice(result); !valid {
		return source.Range{}, fmt.Errorf("syntax node maps to an invalid physical range")
	}
	return result, nil
}

// Related identifies a secondary physical source range.
type Related struct {
	Range source.Range
	Message string
}

// Edit is one exact byte replacement proposed by a fix.
type Edit struct {
	Range source.Range
	NewText string
}

// ImportRequirement identifies one exact package binding required by a fix.
type ImportRequirement struct {
	Path string
	Name string
}

// Validate rejects package bindings that cannot be represented as an exact
// ordinary Go import.
func (r ImportRequirement) Validate() error {
	if err := module.CheckImportPath(r.Path); err != nil || r.Path == "C" {
		return fmt.Errorf("required import path %q is invalid", r.Path)
	}
	if !token.IsIdentifier(r.Name) || r.Name == "_" || r.Name == "." {
		return fmt.Errorf("required import name %q is invalid", r.Name)
	}
	return nil
}

// Fix is one named, safety-classified set of source edits.
type Fix struct {
	Name string
	Safety FixSafety
	Edits []Edit
	RequiredImports []ImportRequirement
}

// FixWithholdingReason classifies why a declared fix was not offered for one
// source-specific finding.
type FixWithholdingReason string

const (
	FixWithheldComments FixWithholdingReason = "comments"
)

// WithheldFix records a declared fix that could not be offered safely for one
// source-specific finding.
type WithheldFix struct {
	Name string
	Reason FixWithholdingReason
	Message string
}

// Finding is the rule-owned portion of a diagnostic.
type Finding struct {
	MessageKey string
	Message string
	Range source.Range
	Related []Related
	Notes []string
	Help string
	Fixes []Fix
	WithheldFixes []WithheldFix
}

// Diagnostic is one source-versioned, reporter-ready lint diagnostic.
type Diagnostic struct {
	RuleID string
	Severity Severity
	Targets []string
	MessageKey string
	Message string
	Path string
	Digest source.Digest
	Range source.Range
	Related []Related
	Notes []string
	Help string
	Fixes []Fix
	WithheldFixes []WithheldFix
}

// Selection is one enabled rule with its resolved severity and cost.
type Selection struct {
	ID string
	Severity Severity
	Requirement Requirement
	Options OptionSet
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
