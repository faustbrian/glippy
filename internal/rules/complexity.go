package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/faustbrian/glippy/internal/source"
)

const complexityMaximumOption = "maximum"

type excessiveNestingRule struct{}

type tooManyLinesRule struct{}

type tooManyParametersRule struct{}

type tooManyResultsRule struct{}

// NewExcessiveNestingRule constructs the configurable control-flow nesting rule.
func NewExcessiveNestingRule() Rule {
	return excessiveNestingRule{}
}

// NewTooManyLinesRule constructs the configurable logical function-length rule.
func NewTooManyLinesRule() Rule {
	return tooManyLinesRule{}
}

// NewTooManyParametersRule constructs the configurable parameter-count rule.
func NewTooManyParametersRule() Rule {
	return tooManyParametersRule{}
}

// NewTooManyResultsRule constructs the configurable result-count rule.
func NewTooManyResultsRule() Rule {
	return tooManyResultsRule{}
}

func (excessiveNestingRule) Metadata() Metadata {
	return Metadata{
		ID: "excessive-nesting",
		Summary: "detects functions with deeply nested control flow",
		Documentation: "Deeply nested branches and loops make the active conditions harder to track. This rule measures structural Go control-flow nesting, treats an else-if chain as one level per branch rather than indentation, and excludes nested function literals from their enclosing function.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetComplexity},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeFuncDecl, NodeFuncLit},
		Categories: []Category{CategoryComplexity, CategoryMaintainability},
		Options: complexityOptions(5, 100, "maximum permitted control-flow nesting depth"),
		KnownLimitations: []string{
			"The metric is structural and does not claim to measure cognitive complexity.",
			"Closures are measured independently and do not increase their enclosing function's depth.",
		},
		Examples: []Example{
			{
				Title: "Extract deeply nested work",
				Incorrect: "func run() { if ready { for next() { if valid() { work() } } } }",
				Correct: "func run() { if !ready { return }; processValid() }",
			},
		},
	}
}

func (tooManyLinesRule) Metadata() Metadata {
	return Metadata{
		ID: "too-many-lines",
		Summary: "detects functions with too many logical source lines",
		Documentation: "Large functions are harder to review and isolate. Glippy counts physical lines containing lexical Go tokens inside the function body while excluding blank lines, comment-only lines, automatically inserted semicolons, and the enclosing braces.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetComplexity},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeFuncDecl, NodeFuncLit},
		Categories: []Category{CategoryComplexity, CategoryMaintainability},
		Options: complexityOptions(100, 10_000, "maximum permitted logical source lines"),
		KnownLimitations: []string{
			"A multiline literal counts as one lexical line because its interior is data rather than Go structure.",
			"Closures are measured independently and their bodies do not add token lines to the enclosing function.",
		},
		Examples: []Example{
			{
				Title: "Extract one coherent operation",
				Incorrect: "func run() { stepOne(); stepTwo(); stepThree(); stepFour() }",
				Correct: "func run() { prepare(); execute() }",
			},
		},
	}
}

func (tooManyParametersRule) Metadata() Metadata {
	return Metadata{
		ID: "too-many-parameters",
		Summary: "detects function signatures with too many parameters",
		Documentation: "A long parameter list makes call sites difficult to read and often indicates that related inputs need a named value. The method receiver is not counted; each declared parameter name and each unnamed parameter field counts once.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetComplexity},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeFuncType},
		Categories: []Category{CategoryComplexity, CategoryMaintainability},
		Options: complexityOptions(7, 100, "maximum permitted parameters"),
		KnownLimitations: []string{
			"Generated files are excluded through the shared generated-file policy.",
			"The rule does not infer whether replacing parameters with an options struct would improve a particular API.",
		},
		Examples: []Example{
			{
				Title: "Group related inputs",
				Incorrect: "func send(host string, port int, user, password, path string, timeout int, retries int, secure bool)",
				Correct: "func send(target Target, credentials Credentials, policy Policy)",
			},
		},
	}
}

func (tooManyResultsRule) Metadata() Metadata {
	return Metadata{
		ID: "too-many-results",
		Summary: "detects function signatures with too many results",
		Documentation: "Returning many independent values makes ordering and ownership difficult to understand. Each named result and each unnamed result field counts once; a trailing error is counted because it remains part of the call contract.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetComplexity},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeFuncType},
		Categories: []Category{CategoryComplexity, CategoryMaintainability},
		Options: complexityOptions(3, 100, "maximum permitted results"),
		KnownLimitations: []string{
			"The rule does not infer whether a result struct would improve a particular API.",
			"Named and unnamed results use the same threshold because both affect call-site arity.",
		},
		Examples: []Example{
			{
				Title: "Return a named aggregate",
				Incorrect: "func inspect() (string, int, bool, error)",
				Correct: "func inspect() (Inspection, error)",
			},
		},
	}
}

func boundedMaximumOption(defaultValue, minimum, maximum int64, summary string) OptionMetadata {
	defaultOption := IntegerOption(defaultValue)
	minimumValue := minimum
	maximumValue := maximum
	return OptionMetadata{
		Name: complexityMaximumOption,
		Summary: summary,
		Kind: OptionInteger,
		Default: &defaultOption,
		Minimum: &minimumValue,
		Maximum: &maximumValue,
	}
}

func complexityOptions(defaultMaximum, upperBound int64, summary string) []OptionMetadata {
	includeTests := BooleanOption(false)
	return []OptionMetadata{
		boundedMaximumOption(defaultMaximum, 1, upperBound, summary),
		{
			Name: "include-tests",
			Summary: "analyze functions declared in _test.go files",
			Kind: OptionBoolean,
			Default: &includeTests,
		},
	}
}

func complexityFileEligible(ctx *Context) (bool, error) {
	includeTests, found := ctx.BooleanOption("include-tests")
	if !found {
		return false, fmt.Errorf("complexity rule requires the include-tests option")
	}
	return includeTests || !strings.HasSuffix(ctx.File().Path(), "_test.go"), nil
}

func (excessiveNestingRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	eligible, err := complexityFileEligible(ctx)
	if err != nil || !eligible {
		return nil, err
	}
	body, namePosition, nameEnd, err := complexityFunction(node)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}
	maximum, found := ctx.IntegerOption(complexityMaximumOption)
	if !found {
		return nil, fmt.Errorf("excessive-nesting requires the maximum option")
	}
	depth := maximumBlockNesting(body, 0)
	if int64(depth) <= maximum {
		return nil, nil
	}
	range_, err := ctx.PositionRange(namePosition, nameEnd)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "excessive-nesting",
			Message: fmt.Sprintf(
				"function nesting is too deep (%d/%d)",
				depth,
				maximum,
			),
			Range: range_,
			Help: "extract nested work or use early exits to reduce nesting",
		},
	}, nil
}

func (tooManyLinesRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	eligible, err := complexityFileEligible(ctx)
	if err != nil || !eligible {
		return nil, err
	}
	body, namePosition, nameEnd, err := complexityFunction(node)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}
	maximum, found := ctx.IntegerOption(complexityMaximumOption)
	if !found {
		return nil, fmt.Errorf("too-many-lines requires the maximum option")
	}
	lines, err := logicalFunctionLines(ctx, body)
	if err != nil {
		return nil, err
	}
	if int64(lines) <= maximum {
		return nil, nil
	}
	range_, err := ctx.PositionRange(namePosition, nameEnd)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "too-many-lines",
			Message: fmt.Sprintf(
				"function has too many logical lines (%d/%d)",
				lines,
				maximum,
			),
			Range: range_,
			Help: "extract one or more coherent operations into smaller functions",
		},
	}, nil
}

func (tooManyParametersRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	eligible, err := complexityFileEligible(ctx)
	if err != nil || !eligible {
		return nil, err
	}
	function, ok := node.(*ast.FuncType)
	if !ok {
		return nil, fmt.Errorf("too-many-parameters requires a function type")
	}
	return reportFieldListLimit(ctx, "too-many-parameters", "parameters", function.Params)
}

func (tooManyResultsRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	eligible, err := complexityFileEligible(ctx)
	if err != nil || !eligible {
		return nil, err
	}
	function, ok := node.(*ast.FuncType)
	if !ok {
		return nil, fmt.Errorf("too-many-results requires a function type")
	}
	return reportFieldListLimit(ctx, "too-many-results", "results", function.Results)
}

func reportFieldListLimit(
	ctx *Context,
	ruleID string,
	label string,
	fields *ast.FieldList,
) ([]Finding, error) {
	maximum, found := ctx.IntegerOption(complexityMaximumOption)
	if !found {
		return nil, fmt.Errorf("%s requires the maximum option", ruleID)
	}
	count := fieldListArity(fields)
	if int64(count) <= maximum {
		return nil, nil
	}
	range_, err := fieldListInteriorRange(ctx, fields)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: ruleID,
			Message: fmt.Sprintf(
				"function has too many %s (%d/%d)",
				label,
				count,
				maximum,
			),
			Range: range_,
			Help: "group related values into a named type when that clarifies the API",
		},
	}, nil
}

func complexityFunction(node ast.Node) (*ast.BlockStmt, token.Pos, token.Pos, error) {
	switch function := node.(type) {
	case *ast.FuncDecl:
		if function.Name == nil {
			return nil, token.NoPos, token.NoPos, fmt.Errorf(
				"function declaration has no name",
			)
		}
		return function.Body, function.Name.Pos(), function.Name.End(), nil
	case *ast.FuncLit:
		if function.Body == nil || function.Type == nil {
			return nil, token.NoPos, token.NoPos, fmt.Errorf(
				"function literal has no body or type",
			)
		}
		return function.Body, function.
			Type.
			Func, function.Type.Func + token.Pos(len("func")), nil
	default:
		return nil, token.NoPos, token.NoPos, fmt.Errorf(
			"complexity function rule requires a declaration or literal",
		)
	}
}

func fieldListArity(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
}

func fieldListInteriorRange(ctx *Context, fields *ast.FieldList) (source.Range, error) {
	if fields == nil || !fields.Opening.IsValid() || !fields.Closing.IsValid() {
		return source.Range{}, fmt.Errorf("complexity field list has no delimited range")
	}
	return ctx.PositionRange(fields.Opening + 1, fields.Closing)
}

func logicalFunctionLines(ctx *Context, body *ast.BlockStmt) (int, error) {
	bodyRange, err := ctx.Range(body)
	if err != nil {
		return 0, err
	}
	nested, err := nestedFunctionBodyRanges(ctx, body)
	if err != nil {
		return 0, err
	}
	tokens, valid := ctx.File().TokensInRange(bodyRange)
	if !valid {
		return 0, fmt.Errorf("function body does not map to a valid token range")
	}
	lines := make(map[int]struct{})
	for _, item := range tokens {
		if item.Range.Start <= bodyRange.Start || item.Range.End >= bodyRange.End {
			continue
		}
		if item.Kind == token.COMMENT || item.Semicolon == source.SemicolonInserted {
			continue
		}
		if rangeContainsToken(nested, item.Range) {
			continue
		}
		position, found := ctx.File().Position(item.Range.Start)
		if !found {
			return 0, fmt.Errorf("function token does not map to a physical line")
		}
		lines[position.Line] = struct{}{}
	}
	return len(lines), nil
}

func nestedFunctionBodyRanges(ctx *Context, body *ast.BlockStmt) ([]source.Range, error) {
	ranges := make([]source.Range, 0)
	var rangeErr error
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if rangeErr != nil {
				return false
			}
			literal, nested := node.(*ast.FuncLit)
			if !nested {
				return true
			}
			range_, err := ctx.Range(literal.Body)
			if err != nil {
				rangeErr = err
				return false
			}
			ranges = append(ranges, range_)
			return false
		},
	)
	return ranges, rangeErr
}

func rangeContainsToken(ranges []source.Range, tokenRange source.Range) bool {
	for _, range_ := range ranges {
		if tokenRange.Start >= range_.Start && tokenRange.End <= range_.End {
			return true
		}
	}
	return false
}

func maximumBlockNesting(block *ast.BlockStmt, depth int) int {
	if block == nil {
		return depth
	}
	maximum := depth
	for _, statement := range block.List {
		maximum = max(maximum, maximumStatementNesting(statement, depth))
	}
	return maximum
}

func maximumStatementNesting(statement ast.Stmt, depth int) int {
	switch current := statement.(type) {
	case *ast.IfStmt:
		level := depth + 1
		maximum := max(level, maximumBlockNesting(current.Body, level))
		switch alternative := current.Else.(type) {
		case *ast.IfStmt:
			maximum = max(maximum, maximumStatementNesting(alternative, depth))
		case *ast.BlockStmt:
			maximum = max(maximum, maximumBlockNesting(alternative, level))
		}
		return maximum
	case *ast.ForStmt:
		level := depth + 1
		return max(level, maximumBlockNesting(current.Body, level))
	case *ast.RangeStmt:
		level := depth + 1
		return max(level, maximumBlockNesting(current.Body, level))
	case *ast.SwitchStmt:
		return maximumClauseNesting(current.Body, depth + 1)
	case *ast.TypeSwitchStmt:
		return maximumClauseNesting(current.Body, depth + 1)
	case *ast.SelectStmt:
		return maximumClauseNesting(current.Body, depth + 1)
	case *ast.BlockStmt:
		level := depth + 1
		return max(level, maximumBlockNesting(current, level))
	case *ast.LabeledStmt:
		return maximumStatementNesting(current.Stmt, depth)
	default:
		return depth
	}
}

func maximumClauseNesting(body *ast.BlockStmt, level int) int {
	maximum := level
	if body == nil {
		return maximum
	}
	for _, statement := range body.List {
		switch clause := statement.(type) {
		case *ast.CaseClause:
			for _, child := range clause.Body {
				maximum = max(maximum, maximumStatementNesting(child, level))
			}
		case *ast.CommClause:
			for _, child := range clause.Body {
				maximum = max(maximum, maximumStatementNesting(child, level))
			}
		}
	}
	return maximum
}
