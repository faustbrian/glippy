package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

const removeRedundantTypeFix = "remove-redundant-type"

type emptyBranchRule struct{}

type manualMinMaxRule struct{}

type redundantTypeDeclarationRule struct{}

// NewEmptyBranchRule constructs the empty conditional branch rule.
func NewEmptyBranchRule() Rule {
	return emptyBranchRule{}
}

// NewManualMinMaxRule constructs the manual integer min/max rule.
func NewManualMinMaxRule() Rule {
	return manualMinMaxRule{}
}

// NewRedundantTypeDeclarationRule constructs the redundant explicit type rule.
func NewRedundantTypeDeclarationRule() Rule {
	return redundantTypeDeclarationRule{}
}

func (emptyBranchRule) Metadata() Metadata {
	return Metadata{
		ID: "empty-branch",
		Summary: "detects uncommented empty if and else branches",
		Documentation: "An empty conditional branch is easy to overlook and often survives after logic is removed or moved. Glippy reports direct if and else blocks with no statements or comments so deliberate placeholders can remain documented without noise.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeIfStmt},
		Categories: []Category{CategoryStyle, CategoryMaintainability, CategorySuspicious},
		KnownLimitations: []string{
			"Only direct if and else blocks are considered; empty switch, select, and loop bodies have different control-flow meanings.",
			"A comment anywhere inside an otherwise empty block is treated as evidence of deliberate intent.",
			"No fix is offered because removing a branch may also remove condition evaluation or change the surrounding alternative.",
		},
		Examples: []Example{
			{
				Title: "Remove or explain an empty branch",
				Incorrect: "if ready {}",
				Correct: "if ready { run() }",
			},
		},
	}
}

func (emptyBranchRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.IfStmt)
	if !ok || ctx == nil || ctx.File() == nil {
		return nil, fmt.Errorf("empty-branch requires an if statement and source file")
	}
	findings := make([]Finding, 0, 2)
	bodyFinding, found, err := emptyIfBlockFinding(ctx, statement.Body)
	if err != nil {
		return nil, err
	}
	if found {
		findings = append(findings, bodyFinding)
	}
	if alternative, directBlock := statement.Else.(*ast.BlockStmt); directBlock {
		elseFinding, found, err := emptyIfBlockFinding(ctx, alternative)
		if err != nil {
			return nil, err
		}
		if found {
			findings = append(findings, elseFinding)
		}
	}
	return findings, nil
}

func emptyIfBlockFinding(ctx *Context, block *ast.BlockStmt) (Finding, bool, error) {
	if !effectivelyEmptyBlock(block) {
		return Finding{}, false, nil
	}
	range_, err := ctx.Range(block)
	if err != nil {
		return Finding{}, false, err
	}
	if rangeContainsComment(ctx.File().Comments(), range_) {
		return Finding{}, false, nil
	}
	return Finding{
		MessageKey: "empty-if-branch",
		Message: "conditional branch is empty and has no explanatory comment",
		Range: range_,
		Help: "remove the branch, implement it, or document why it is intentionally empty",
	}, true, nil
}

func effectivelyEmptyBlock(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	for _, statement := range block.List {
		if _, empty := statement.(*ast.EmptyStmt); !empty {
			return false
		}
	}
	return true
}

func (manualMinMaxRule) Metadata() Metadata {
	return Metadata{
		ID: "manual-min-max",
		Summary: "detects integer assignments that manually implement min or max",
		Documentation: "Go's min and max built-ins express an integer bound update directly. A one-statement conditional assignment over the same two integer variables can use the built-in operation instead of repeating comparison and assignment structure.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeIfStmt},
		Categories: []Category{CategoryStyle, CategoryComplexity, CategoryMaintainability},
		KnownLimitations: []string{
			"The rule requires two distinct plain identifiers of one identical integer type and one direct assignment with no initializer, else branch, or additional statement.",
			"Floating-point forms are excluded because NaN behavior differs between comparisons and the min and max built-ins.",
			"No fix is offered because preserving comments and choosing the preferred assignment spelling remains a readability decision.",
		},
		Examples: []Example{
			{
				Title: "Use the max built-in",
				Incorrect: "if current < candidate { current = candidate }",
				Correct: "current = max(current, candidate)",
			},
		},
	}
}

func (manualMinMaxRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.IfStmt)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"manual-min-max requires an if statement and type information",
		)
	}
	operation, condition, found := manualIntegerMinMax(ctx.Info(), statement)
	if !found {
		return nil, nil
	}
	range_, err := ctx.Range(condition)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "manual-min-max",
			Message: fmt.Sprintf(
				"conditional assignment manually implements %s",
				operation,
			),
			Range: range_,
			Help: fmt.Sprintf(
				"assign the result of the %s built-in directly",
				operation,
			),
		},
	}, nil
}

func manualIntegerMinMax(info *types.Info, statement *ast.IfStmt) (string, *ast.BinaryExpr, bool) {
	if statement == nil ||
		statement.Init != nil ||
		statement.Else != nil ||
		statement.Body == nil ||
		len(statement.Body.List) != 1 {
		return "", nil, false
	}
	condition, _ := ast.Unparen(statement.Cond).(*ast.BinaryExpr)
	if condition == nil || (condition.Op != token.LSS && condition.Op != token.GTR) {
		return "", nil, false
	}
	left, _ := ast.Unparen(condition.X).(*ast.Ident)
	right, _ := ast.Unparen(condition.Y).(*ast.Ident)
	if left == nil || right == nil {
		return "", nil, false
	}
	leftObject := info.ObjectOf(left)
	rightObject := info.ObjectOf(right)
	if leftObject == nil ||
		rightObject == nil ||
		leftObject == rightObject ||
		!types.Identical(leftObject.Type(), rightObject.Type()) ||
		!isIntegerType(leftObject.Type()) {
		return "", nil, false
	}
	assignment, _ := statement.Body.List[0].(*ast.AssignStmt)
	if assignment == nil ||
		assignment.Tok != token.ASSIGN ||
		len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return "", nil, false
	}
	target := directObject(info, assignment.Lhs[0])
	value := directObject(info, assignment.Rhs[0])
	if target == leftObject && value == rightObject {
		if condition.Op == token.LSS {
			return "max", condition, true
		}
		return "min", condition, true
	}
	if target == rightObject && value == leftObject {
		if condition.Op == token.LSS {
			return "min", condition, true
		}
		return "max", condition, true
	}
	return "", nil, false
}

func (redundantTypeDeclarationRule) Metadata() Metadata {
	return Metadata{
		ID: "redundant-type-declaration",
		Summary: "detects variable types already inferred from their initializer",
		Documentation: "A variable declaration does not need an explicit type when Go infers the identical static type from its initializer. Removing that duplicate spelling keeps the initializer as the single source of type information without changing the variable's type.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeValueSpec},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		Fixes: []FixMetadata{
			{
				Name: removeRedundantTypeFix,
				Description: "remove the explicit type already inferred from the initializer",
				Safety: FixSafe,
			},
		},
		KnownLimitations: []string{
			"The rule requires exactly one variable and one initializer; tuple-producing calls and grouped declarations remain unchanged.",
			"Untyped nil and initializers whose default type differs from the declared type are excluded.",
			"The safe fix is withheld when the removed type region contains a comment.",
		},
		Examples: []Example{
			{
				Title: "Infer the initializer type",
				Incorrect: "var retries int = configuredRetries()",
				Correct: "var retries = configuredRetries()",
			},
		},
	}
}

func (redundantTypeDeclarationRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	specification, ok := node.(*ast.ValueSpec)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"redundant-type-declaration requires a value specification and type information",
		)
	}
	if specification.Type == nil ||
		len(specification.Names) != 1 ||
		len(specification.Values) != 1 {
		return nil, nil
	}
	declared, variable := ctx.Info().Defs[specification.Names[0]].(*types.Var)
	inferred := inferredInitializerType(ctx.Info(), specification.Values[0])
	if !variable || inferred == nil || !types.Identical(declared.Type(), inferred) {
		return nil, nil
	}
	range_, err := ctx.Range(specification)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "redundant-type",
		Message: "explicit variable type is identical to the type inferred from its initializer",
		Range: range_,
		Help: "omit the explicit type and retain the initializer",
	}
	editRange, err := ctx.PositionRange(
		specification.Names[0].End(),
		specification.Values[0].Pos(),
	)
	if err != nil {
		return nil, err
	}
	if !rangeContainsComment(ctx.File().Comments(), editRange) {
		finding.Fixes = []Fix{
			{
				Name: removeRedundantTypeFix,
				Safety: FixSafe,
				Edits: []Edit{{Range: editRange, NewText: " = "}},
			},
		}
	}
	return []Finding{finding}, nil
}

func inferredInitializerType(info *types.Info, expression ast.Expr) types.Type {
	if info == nil || expression == nil {
		return nil
	}
	typeAndValue, found := info.Types[expression]
	if !found || typeAndValue.Type == nil {
		return nil
	}
	if typeAndValue.Value == nil {
		return typeAndValue.Type
	}
	current := ast.Unparen(expression)
	if call, ok := current.(*ast.CallExpr); ok && info.Types[call.Fun].IsType() {
		return typeAndValue.Type
	}
	if identifier, ok := current.(*ast.Ident); ok {
		constant_, _ := info.ObjectOf(identifier).(*types.Const)
		if constant_ == nil {
			return nil
		}
		return types.Default(constant_.Type())
	}
	literal, ok := current.(*ast.BasicLit)
	if !ok {
		basic, _ := types.Unalias(typeAndValue.Type).(*types.Basic)
		if basic == nil || basic.Info() & types.IsUntyped == 0 {
			return nil
		}
		return types.Default(basic)
	}
	switch literal.Kind {
	case token.INT:
		return types.Typ[types.Int]
	case token.FLOAT:
		return types.Typ[types.Float64]
	case token.IMAG:
		return types.Typ[types.Complex128]
	case token.CHAR:
		return types.Typ[types.Rune]
	case token.STRING:
		return types.Typ[types.String]
	default:
		return nil
	}
}
