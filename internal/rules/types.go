// Package rules defines native lint-rule contracts and canonical metadata.
package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/faustbrian/gox/internal/source"
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
	ID                   string
	Summary              string
	Documentation        string
	DefaultSeverity      Severity
	Presets              []Preset
	MinimumGoVersion     string
	Requirement          Requirement
	NodeInterests        []NodeKind
	RunOnGenerated       bool
	RunDespiteTypeErrors bool
	Categories           []Category
	Fixes                []FixMetadata
	Options              []OptionMetadata
	Deprecation          *Deprecation
	KnownLimitations     []string
	Examples             []Example
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
	RunSyntaxFile(*source.File) ([]Finding, error)
}

// TypesRule receives package AST nodes with shared type information and exact
// physical source identity.
type TypesRule interface {
	Rule
	RunTypes(*TypesContext, ast.Node) ([]Finding, error)
}

// Context is the immutable per-file syntax rule context.
type Context struct {
	file *source.File
}

// NewContext creates an immutable per-file rule context.
func NewContext(file *source.File) *Context {
	return &Context{file: file}
}

// TypesContext binds one package AST to its exact immutable physical source.
type TypesContext struct {
	file      *source.File
	fileSet   *token.FileSet
	packageID string
	package_  *types.Package
	info      *types.Info
	illTyped  bool
}

// NewTypesContext constructs one read-only typed rule context.
func NewTypesContext(
	file *source.File,
	fileSet *token.FileSet,
	packageID string,
	package_ *types.Package,
	info *types.Info,
	illTyped bool,
) *TypesContext {
	return &TypesContext{
		file:      file,
		fileSet:   fileSet,
		packageID: packageID,
		package_:  package_,
		info:      info,
		illTyped:  illTyped,
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
