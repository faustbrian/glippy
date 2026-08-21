package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type exportedAPIDocumentationRule struct{}

type exportedDocumentationOptions struct {
	includeTests bool
	includeMain bool
	includeMembers bool
	requireNamePrefix bool
}

// NewExportedAPIDocumentationRule constructs the exported API documentation
// policy rule.
func NewExportedAPIDocumentationRule() Rule {
	return exportedAPIDocumentationRule{}
}

func (exportedAPIDocumentationRule) Metadata() Metadata {
	falseDefault := BooleanOption(false)
	trueDefault := BooleanOption(true)
	return Metadata{
		ID: "exported-api-documentation",
		Summary: "requires documentation for exported API declarations",
		Documentation: "Exported declarations form a package's user-facing contract. This restriction rule requires documentation on exported functions, methods, types, constants, and variables, with optional exported struct-field and interface-method coverage. It can also require declaration-specific comments to begin with the documented name, following established Go documentation conventions.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetRestriction},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeFile},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		Options: []OptionMetadata{
			{
				Name: "include-tests",
				Summary: "require exported API documentation in files whose base name ends in _test.go",
				Kind: OptionBoolean,
				Default: &falseDefault,
			},
			{
				Name: "include-main",
				Summary: "require exported API documentation in package main",
				Kind: OptionBoolean,
				Default: &falseDefault,
			},
			{
				Name: "include-members",
				Summary: "require documentation for exported named struct fields and interface methods",
				Kind: OptionBoolean,
				Default: &trueDefault,
			},
			{
				Name: "require-name-prefix",
				Summary: "require declaration-specific documentation to begin with the exported name",
				Kind: OptionBoolean,
				Default: &trueDefault,
			},
		},
		KnownLimitations: []string{
			"Package-clause documentation is not checked because the syntax-tier scheduler analyzes files independently and cannot prove that another file does not document the package.",
			"Grouped declaration comments are accepted as documentation for the complete group and are not required to begin with every declared name.",
			"Exported embedded fields are excluded because one embedded declaration can expose a promoted API whose documentation ownership requires package-level type information.",
			"Named exported fields and interface methods are checked by default; include-members disables both member policies.",
			"Package main and test files are excluded by default; include-main and include-tests enable those policies independently.",
			"Generated files are excluded.",
			"No fix is offered because useful API documentation cannot be synthesized from syntax.",
		},
		Examples: []Example{
			{
				Title: "Document the exported contract",
				Incorrect: "func Open(path string) (*File, error) { return nil, nil }",
				Correct: "// Open opens path for reading.\nfunc Open(path string) (*File, error) { return nil, nil }",
			},
		},
	}
}

func (exportedAPIDocumentationRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	file, ok := node.(*ast.File)
	if !ok || ctx == nil || ctx.File() == nil {
		return nil, fmt.Errorf(
			"exported-api-documentation requires a file and source context",
		)
	}
	options, err := exportedDocumentationPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if (!options.includeTests &&
		strings.HasSuffix(filepath.Base(ctx.File().Path()), "_test.go")) ||
		(!options.includeMain && file.Name != nil && file.Name.Name == "main") {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			finding, found, findingErr := exportedFunctionDocumentationFinding(
				ctx,
				declaration,
				options,
			)
			if findingErr != nil {
				return nil, findingErr
			}
			if found {
				findings = append(findings, finding)
			}
		case *ast.GenDecl:
			declarationFindings, findingErr := exportedGeneralDocumentationFindings(
				ctx,
				declaration,
				options,
			)
			if findingErr != nil {
				return nil, findingErr
			}
			findings = append(findings, declarationFindings...)
		}
	}
	return findings, nil
}

func exportedDocumentationPolicy(ctx *Context) (exportedDocumentationOptions, error) {
	var result exportedDocumentationOptions
	options := []struct {
		name string
		target *bool
	}{
		{name: "include-tests", target: &result.includeTests},
		{name: "include-main", target: &result.includeMain},
		{name: "include-members", target: &result.includeMembers},
		{name: "require-name-prefix", target: &result.requireNamePrefix},
	}
	for _, option := range options {
		value, found := ctx.BooleanOption(option.name)
		if !found {
			return exportedDocumentationOptions{}, fmt.Errorf(
				"exported-api-documentation requires the %s option",
				option.name,
			)
		}
		*option.target = value
	}
	return result, nil
}

func exportedFunctionDocumentationFinding(
	ctx *Context,
	declaration *ast.FuncDecl,
	options exportedDocumentationOptions,
) (Finding, bool, error) {
	if declaration == nil || declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
		return Finding{}, false, nil
	}
	kind := "function"
	if declaration.Recv != nil {
		kind = "method"
	}
	return exportedDocumentationFinding(
		ctx,
		declaration.Name,
		declaration.Doc,
		kind,
		options.requireNamePrefix,
		false,
	)
}

func exportedGeneralDocumentationFindings(
	ctx *Context,
	declaration *ast.GenDecl,
	options exportedDocumentationOptions,
) ([]Finding, error) {
	if declaration == nil ||
		(declaration.Tok != token.TYPE &&
			declaration.Tok != token.CONST &&
			declaration.Tok != token.VAR) {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for _, specification := range declaration.Specs {
		switch specification := specification.(type) {
		case *ast.TypeSpec:
			if specification.Name == nil {
				continue
			}
			if ast.IsExported(specification.Name.Name) {
				documentation, grouped := declarationDocumentation(
					specification.Doc,
					declaration.Doc,
					declaration.Lparen.IsValid(),
				)
				finding, found, err := exportedDocumentationFinding(
					ctx,
					specification.Name,
					documentation,
					"type",
					options.requireNamePrefix,
					grouped,
				)
				if err != nil {
					return nil, err
				}
				if found {
					findings = append(findings, finding)
				}
			}
			if options.includeMembers {
				memberFindings, memberErr := exportedMemberDocumentationFindings(
					ctx,
					specification,
					options.requireNamePrefix,
				)
				if memberErr != nil {
					return nil, memberErr
				}
				findings = append(findings, memberFindings...)
			}
		case *ast.ValueSpec:
			exportedNames := exportedIdentifiers(specification.Names)
			if len(exportedNames) == 0 {
				continue
			}
			documentation, grouped := declarationDocumentation(
				specification.Doc,
				declaration.Doc,
				declaration.Lparen.IsValid(),
			)
			grouped = grouped || len(exportedNames) > 1
			kind := "variable"
			if declaration.Tok == token.CONST {
				kind = "constant"
			}
			for _, name := range exportedNames {
				finding, found, err := exportedDocumentationFinding(
					ctx,
					name,
					documentation,
					kind,
					options.requireNamePrefix,
					grouped,
				)
				if err != nil {
					return nil, err
				}
				if found {
					findings = append(findings, finding)
				}
			}
		}
	}
	return findings, nil
}

func declarationDocumentation(
	specific *ast.CommentGroup,
	group *ast.CommentGroup,
	groupedDeclaration bool,
) (*ast.CommentGroup, bool) {
	if substantiveDocumentation(specific) {
		return specific, false
	}
	if substantiveDocumentation(group) {
		return group, groupedDeclaration
	}
	return nil, false
}

func exportedMemberDocumentationFindings(
	ctx *Context,
	specification *ast.TypeSpec,
	requireNamePrefix bool,
) ([]Finding, error) {
	var fields *ast.FieldList
	kind := "field"
	switch type_ := specification.Type.(type) {
	case *ast.StructType:
		fields = type_.Fields
	case *ast.InterfaceType:
		fields = type_.Methods
		kind = "interface method"
	default:
		return nil, nil
	}
	if fields == nil {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for _, field := range fields.List {
		names := exportedIdentifiers(field.Names)
		if len(names) == 0 {
			continue
		}
		documentation := field.Doc
		if !substantiveDocumentation(documentation) {
			documentation = field.Comment
		}
		grouped := len(names) > 1
		for _, name := range names {
			finding, found, err := exportedDocumentationFinding(
				ctx,
				name,
				documentation,
				kind,
				requireNamePrefix,
				grouped,
			)
			if err != nil {
				return nil, err
			}
			if found {
				findings = append(findings, finding)
			}
		}
	}
	return findings, nil
}

func exportedIdentifiers(names []*ast.Ident) []*ast.Ident {
	result := make([]*ast.Ident, 0, len(names))
	for _, name := range names {
		if name != nil && ast.IsExported(name.Name) {
			result = append(result, name)
		}
	}
	return result
}

func exportedDocumentationFinding(
	ctx *Context,
	name *ast.Ident,
	documentation *ast.CommentGroup,
	kind string,
	requireNamePrefix bool,
	grouped bool,
) (Finding, bool, error) {
	if name == nil {
		return Finding{}, false, nil
	}
	range_, err := ctx.Range(name)
	if err != nil {
		return Finding{}, false, err
	}
	text := documentationText(documentation)
	messageKind := strings.ReplaceAll(kind, " ", "-")
	if text == "" {
		return Finding{
			MessageKey: "missing-" + messageKind + "-documentation",
			Message: fmt.Sprintf(
				"exported %s %s has no documentation",
				kind,
				name.Name,
			),
			Range: range_,
			Help: "add a doc comment describing the exported contract",
		}, true, nil
	}
	if !requireNamePrefix ||
		grouped ||
		documentationBeginsWithName(text, name.Name, kind == "type") {
		return Finding{}, false, nil
	}
	commentRange, err := ctx.Range(documentation)
	if err != nil {
		return Finding{}, false, err
	}
	return Finding{
		MessageKey: "noncanonical-" + messageKind + "-documentation",
		Message: fmt.Sprintf(
			"documentation for exported %s %s does not begin with its name",
			kind,
			name.Name,
		),
		Range: range_,
		Related: []Related{{Range: commentRange, Message: "documentation begins here"}},
		Help: fmt.Sprintf("begin the doc comment with %s", name.Name),
	}, true, nil
}

func substantiveDocumentation(group *ast.CommentGroup) bool {
	return documentationText(group) != ""
}

func documentationText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}

func documentationBeginsWithName(text string, name string, allowArticle bool) bool {
	text = strings.TrimSpace(text)
	candidates := []string{name}
	if allowArticle {
		candidates = append(candidates, "A " + name, "An " + name, "The " + name)
	}
	for _, candidate := range candidates {
		if !strings.HasPrefix(text, candidate) {
			continue
		}
		remainder := strings.TrimPrefix(text, candidate)
		if remainder == "" {
			return true
		}
		next, _ := utf8.DecodeRuneInString(remainder)
		if unicode.IsSpace(next) || unicode.IsPunct(next) {
			return true
		}
	}
	return false
}
