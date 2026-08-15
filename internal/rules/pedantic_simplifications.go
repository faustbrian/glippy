package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/faustbrian/glippy/internal/source"
)

const (
	removeBlankIdentifierFix = "remove-blank-identifier"
	removeRedundantNilCheckFix = "remove-redundant-nil-check"
	useFormatOperandFix = "use-format-operand"
	useTimeSinceFix = "use-time-since"
	useTimeUntilFix = "use-time-until"
)

type needlessBlankIdentifierRule struct{}

type redundantClosureRule struct{}

type redundantNilCheckRule struct{}

type timeSinceRule struct{}

type timeUntilRule struct{}

type bufferStringConversionRule struct{}

type unnecessaryFormatRule struct{}

type inefficientStringComparisonRule struct{}

// NewNeedlessBlankIdentifierRule constructs the discard simplification rule.
func NewNeedlessBlankIdentifierRule() Rule {
	return needlessBlankIdentifierRule{}
}

// NewRedundantClosureRule constructs the direct delegation rule.
func NewRedundantClosureRule() Rule {
	return redundantClosureRule{}
}

// NewRedundantNilCheckRule constructs the nil-and-len simplification rule.
func NewRedundantNilCheckRule() Rule {
	return redundantNilCheckRule{}
}

// NewTimeSinceRule constructs the time.Since convenience rule.
func NewTimeSinceRule() Rule {
	return timeSinceRule{}
}

// NewTimeUntilRule constructs the time.Until convenience rule.
func NewTimeUntilRule() Rule {
	return timeUntilRule{}
}

// NewBufferStringConversionRule constructs the bytes.Buffer conversion rule.
func NewBufferStringConversionRule() Rule {
	return bufferStringConversionRule{}
}

// NewUnnecessaryFormatRule constructs the constant-format rule.
func NewUnnecessaryFormatRule() Rule {
	return unnecessaryFormatRule{}
}

// NewInefficientStringComparisonRule constructs the case-fold comparison rule.
func NewInefficientStringComparisonRule() Rule {
	return inefficientStringComparisonRule{}
}

func (needlessBlankIdentifierRule) Metadata() Metadata {
	return Metadata{
		ID: "needless-blank-identifier",
		Summary: "detects blank identifiers that discard values implicitly discarded by Go",
		Documentation: "Range clauses and channel receives can discard values without assigning them to the blank identifier. Removing the redundant assignment makes the operation's intent direct without changing which iteration or receive occurs.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeAssignStmt, NodeRangeStmt},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		Fixes: []FixMetadata{
			{
				Name: removeBlankIdentifierFix,
				Description: "remove the unnecessary blank-identifier assignment",
				Safety: FixSuggestion,
			},
		},
		KnownLimitations: []string{
			"Map lookup blank identifiers remain visible because they can document intentional presence handling.",
			"Range-over-function variables are required by the language and are not reported.",
			"A suggestion is withheld when removing the assignment would also remove a comment.",
		},
		Examples: []Example{
			{
				Title: "Discard range values implicitly",
				Incorrect: "for _, _ = range values {}",
				Correct: "for range values {}",
			},
		},
	}
}

func (needlessBlankIdentifierRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"needless-blank-identifier requires complete type information",
		)
	}
	switch current := node.(type) {
	case *ast.RangeStmt:
		return needlessRangeBlank(ctx, current)
	case *ast.AssignStmt:
		return needlessReceiveBlank(ctx, current)
	default:
		return nil, fmt.Errorf(
			"needless-blank-identifier requires an assignment or range statement",
		)
	}
}

func needlessRangeBlank(ctx *TypesContext, statement *ast.RangeStmt) ([]Finding, error) {
	if statement == nil || statement.Key == nil {
		return nil, nil
	}
	if _, rangeFunction := types.Unalias(ctx.Info().TypeOf(
				statement.X,
			)).Underlying().(*types.Signature);
		rangeFunction {
		return nil, nil
	}
	keyBlank := blankIdentifier(statement.Key)
	valueBlank := blankIdentifier(statement.Value)
	if !keyBlank && !valueBlank {
		return nil, nil
	}
	if keyBlank && statement.Value != nil && !valueBlank {
		return nil, nil
	}
	if statement.Value == nil && !keyBlank {
		return nil, nil
	}
	start := statement.Key.Pos()
	end := statement.X.End()
	range_, err := ctx.PositionRange(start, end)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "omit-blank-identifier",
		Message: "assignment to the blank identifier is unnecessary",
		Range: range_,
		Help: "omit the discarded range variable",
	}
	var editRange source.Range
	if keyBlank {
		editRange, err = ctx.PositionRange(
			statement.Key.Pos(),
			statement.TokPos + token.Pos(len(statement.Tok.String())),
		)
	} else {
		editRange, err = ctx.PositionRange(statement.Key.End(), statement.Value.End())
	}
	if err != nil {
		return nil, err
	}
	if !rangeContainsComment(ctx.File().Comments(), editRange) {
		finding.Fixes = []Fix{
			{
				Name: removeBlankIdentifierFix,
				Safety: FixSuggestion,
				Edits: []Edit{{Range: editRange}},
			},
		}
	} else {
		finding.WithheldFixes = commentWithheldFix(
			removeBlankIdentifierFix,
			"removing this blank-identifier assignment would remove comments",
		)
	}
	return []Finding{finding}, nil
}

func needlessReceiveBlank(ctx *TypesContext, statement *ast.AssignStmt) ([]Finding, error) {
	if statement == nil || statement.Tok != token.ASSIGN || len(statement.Rhs) != 1 {
		return nil, nil
	}
	receive, ok := ast.Unparen(statement.Rhs[0]).(*ast.UnaryExpr)
	if !ok || receive.Op != token.ARROW {
		return nil, nil
	}
	firstBlank := len(statement.Lhs) >= 1 && blankIdentifier(statement.Lhs[0])
	secondBlank := len(statement.Lhs) == 2 && blankIdentifier(statement.Lhs[1])
	if !(len(statement.Lhs) == 1 && firstBlank) && !secondBlank {
		return nil, nil
	}
	range_, err := ctx.Range(statement)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "omit-blank-identifier",
		Message: "assignment to the blank identifier is unnecessary",
		Range: range_,
		Help: "receive directly or retain only the used value",
	}
	if firstBlank {
		receiveRange, rangeErr := ctx.Range(statement.Rhs[0])
		if rangeErr != nil {
			return nil, rangeErr
		}
		if !commentsOutsideRetainedRange(ctx.File().Comments(), range_, receiveRange) {
			replacement, found := ctx.File().Slice(receiveRange)
			if !found {
				return nil, fmt.Errorf(
					"needless blank receive has an invalid source range",
				)
			}
			finding.Fixes = []Fix{
				{
					Name: removeBlankIdentifierFix,
					Safety: FixSuggestion,
					Edits: []Edit{{Range: range_, NewText: replacement}},
				},
			}
		} else {
			finding.WithheldFixes = commentWithheldFix(
				removeBlankIdentifierFix,
				"removing this blank-identifier assignment would remove comments",
			)
		}
	} else {
		editRange, rangeErr := ctx.PositionRange(
			statement.Lhs[0].End(),
			statement.Lhs[1].End(),
		)
		if rangeErr != nil {
			return nil, rangeErr
		}
		if !rangeContainsComment(ctx.File().Comments(), editRange) {
			finding.Fixes = []Fix{
				{
					Name: removeBlankIdentifierFix,
					Safety: FixSuggestion,
					Edits: []Edit{{Range: editRange}},
				},
			}
		} else {
			finding.WithheldFixes = commentWithheldFix(
				removeBlankIdentifierFix,
				"removing this blank-identifier assignment would remove comments",
			)
		}
	}
	return []Finding{finding}, nil
}

func blankIdentifier(expression ast.Expr) bool {
	identifier, _ := ast.Unparen(expression).(*ast.Ident)
	return identifier != nil && identifier.Name == "_"
}

func (redundantClosureRule) Metadata() Metadata {
	return Metadata{
		ID: "redundant-closure",
		Summary: "detects function literals that only delegate their parameters",
		Documentation: "A function literal that forwards every parameter unchanged to an identically typed function adds a wrapper without adding behavior. Passing the delegated function directly is clearer, while wrappers with captures, method values, conversions, or additional statements remain explicit.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeFuncLit},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		KnownLimitations: []string{
			"Only one-statement delegation to a declared function or imported package function is reported.",
			"Method values, captures, argument transformations, comments, and differing function types are excluded.",
			"No fix is offered because removing a wrapper can affect stack inspection and panic traces even when ordinary call results are identical.",
		},
		Examples: []Example{
			{
				Title: "Pass the delegated function directly",
				Incorrect: "apply(func(value string) string { return strings.TrimSpace(value) })",
				Correct: "apply(strings.TrimSpace)",
			},
		},
	}
}

func (redundantClosureRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	literal, ok := node.(*ast.FuncLit)
	if !ok {
		return nil, fmt.Errorf("redundant-closure requires a function literal")
	}
	if ctx == nil || ctx.Info() == nil || literal.Body == nil || len(literal.Body.List) != 1 {
		return nil, nil
	}
	call := delegatedCall(literal.Body.List[0])
	if call == nil ||
		!directDeclaredFunction(ctx.Info(), call.Fun) ||
		!types.Identical(ctx.Info().TypeOf(literal), ctx.Info().TypeOf(call.Fun)) ||
		!forwardsLiteralParameters(ctx.Info(), literal, call) {
		return nil, nil
	}
	range_, err := ctx.Range(literal)
	if err != nil {
		return nil, err
	}
	if rangeContainsComment(ctx.File().Comments(), range_) {
		return nil, nil
	}
	return []Finding{
		{
			MessageKey: "direct-delegation",
			Message: "function literal only delegates its parameters",
			Range: range_,
			Help: "use the delegated function directly",
		},
	}, nil
}

func delegatedCall(statement ast.Stmt) *ast.CallExpr {
	switch current := statement.(type) {
	case *ast.ReturnStmt:
		if len(current.Results) == 1 {
			call, _ := ast.Unparen(current.Results[0]).(*ast.CallExpr)
			return call
		}
	case *ast.ExprStmt:
		call, _ := ast.Unparen(current.X).(*ast.CallExpr)
		return call
	}
	return nil
}

func directDeclaredFunction(info *types.Info, expression ast.Expr) bool {
	switch current := ast.Unparen(expression).(type) {
	case *ast.Ident:
		_, ok := info.ObjectOf(current).(*types.Func)
		return ok
	case *ast.SelectorExpr:
		if info.Selections[current] != nil {
			return false
		}
		_, ok := info.ObjectOf(current.Sel).(*types.Func)
		return ok
	default:
		return false
	}
}

func forwardsLiteralParameters(info *types.Info, literal *ast.FuncLit, call *ast.CallExpr) bool {
	parameters := make([]*ast.Ident, 0)
	if literal.Type.Params != nil {
		for _, field := range literal.Type.Params.List {
			if len(field.Names) == 0 {
				return false
			}
			parameters = append(parameters, field.Names...)
		}
	}
	if len(parameters) != len(call.Args) {
		return false
	}
	signature, _ := types.Unalias(info.TypeOf(literal)).(*types.Signature)
	if signature == nil || (signature.Variadic() != call.Ellipsis.IsValid()) {
		return false
	}
	for index, parameter := range parameters {
		argument, _ := ast.Unparen(call.Args[index]).(*ast.Ident)
		if argument == nil || info.ObjectOf(argument) != info.ObjectOf(parameter) {
			return false
		}
	}
	return true
}

func (redundantNilCheckRule) Metadata() Metadata {
	return Metadata{
		ID: "redundant-nil-check",
		Summary: "detects nil checks already implied by len comparisons",
		Documentation: "The built-in len function returns zero for nil slices, maps, and channels. In supported comparisons, a preceding nil check therefore cannot change the condition and only duplicates the length test.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBinaryExpr},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		Fixes: []FixMetadata{
			{
				Name: removeRedundantNilCheckFix,
				Description: "replace the condition with its equivalent length comparison",
				Safety: FixSafe,
			},
		},
		KnownLimitations: []string{
			"Only identifier and selector expressions repeated without calls or indexing are matched.",
			"Pointers to arrays and type parameters are excluded.",
			"The safe fix is withheld when removing the nil check would also remove a comment.",
		},
		Examples: []Example{
			{
				Title: "Rely on len for nil slices",
				Incorrect: "if values != nil && len(values) > 0 {}",
				Correct: "if len(values) > 0 {}",
			},
		},
	}
}

func (redundantNilCheckRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	outer, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, fmt.Errorf("redundant-nil-check requires a binary expression")
	}
	if ctx == nil || ctx.Info() == nil || (outer.Op != token.LAND && outer.Op != token.LOR) {
		return nil, nil
	}
	nilCheck, ok := ast.Unparen(outer.X).(*ast.BinaryExpr)
	if !ok {
		return nil, nil
	}
	lengthCheck, ok := ast.Unparen(outer.Y).(*ast.BinaryExpr)
	if !ok {
		return nil, nil
	}
	value, nilExpression, ok := nilComparison(nilCheck)
	if !ok || !isNilExpression(ctx.Info(), nilExpression) {
		return nil, nil
	}
	lengthValue, threshold, ok := lengthComparison(ctx.Info(), lengthCheck)
	if !ok ||
		!sameSimpleExpression(ctx.Info(), value, lengthValue) ||
		!nilLengthType(ctx.Info().TypeOf(value)) ||
		!redundantNilOperators(outer.Op, nilCheck.Op, lengthCheck.Op, threshold) {
		return nil, nil
	}
	range_, err := ctx.Range(outer)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "omit-nil-check",
		Message: "nil check is redundant because len is zero for nil values",
		Range: range_,
		Help: "use the length comparison directly",
	}
	retainedRange, err := ctx.Range(outer.Y)
	if err != nil {
		return nil, err
	}
	if commentsOutsideRetainedRange(ctx.File().Comments(), range_, retainedRange) {
		finding.WithheldFixes = commentWithheldFix(
			removeRedundantNilCheckFix,
			"removing this nil check would remove comments",
		)
		return []Finding{finding}, nil
	}
	replacement, found := ctx.File().Slice(retainedRange)
	if !found {
		return nil, fmt.Errorf("redundant nil check has an invalid retained range")
	}
	finding.Fixes = []Fix{
		{
			Name: removeRedundantNilCheckFix,
			Safety: FixSafe,
			Edits: []Edit{{Range: range_, NewText: replacement}},
		},
	}
	return []Finding{finding}, nil
}

func nilComparison(expression *ast.BinaryExpr) (ast.Expr, ast.Expr, bool) {
	if expression == nil || (expression.Op != token.EQL && expression.Op != token.NEQ) {
		return nil, nil, false
	}
	return expression.X, expression.Y, true
}

func isNilExpression(info *types.Info, expression ast.Expr) bool {
	identifier, _ := ast.Unparen(expression).(*ast.Ident)
	if identifier == nil || identifier.Name != "nil" {
		return false
	}
	object := info.ObjectOf(identifier)
	_, ok := object.(*types.Nil)
	return ok
}

func lengthComparison(info *types.Info, expression *ast.BinaryExpr) (ast.Expr, int64, bool) {
	call, _ := ast.Unparen(expression.X).(*ast.CallExpr)
	if call == nil || len(call.Args) != 1 {
		return nil, 0, false
	}
	builtin, _ := ast.Unparen(call.Fun).(*ast.Ident)
	if builtin == nil || builtin.Name != "len" {
		return nil, 0, false
	}
	object, _ := info.ObjectOf(builtin).(*types.Builtin)
	if object == nil || object.Name() != "len" {
		return nil, 0, false
	}
	value := info.Types[expression.Y].Value
	if value == nil || value.Kind() != constant.Int {
		return nil, 0, false
	}
	threshold, exact := constant.Int64Val(value)
	return call.Args[0], threshold, exact
}

func redundantNilOperators(outer, nilOperator, lengthOperator token.Token, threshold int64) bool {
	if outer == token.LOR && nilOperator == token.EQL {
		switch lengthOperator {
		case token.EQL:
			return threshold == 0
		case token.LEQ:
			return threshold >= 0
		case token.LSS:
			return threshold > 0
		}
	}
	if outer == token.LAND && nilOperator == token.NEQ {
		switch lengthOperator {
		case token.EQL:
			return threshold != 0
		case token.GEQ:
			return threshold > 0
		case token.NEQ:
			return threshold == 0
		case token.GTR:
			return threshold >= 0
		}
	}
	return false
}

func nilLengthType(type_ types.Type) bool {
	switch types.Unalias(type_).Underlying().(type) {
	case *types.Slice, *types.Map, *types.Chan:
		return true
	default:
		return false
	}
}

func sameSimpleExpression(info *types.Info, first, second ast.Expr) bool {
	first = ast.Unparen(first)
	second = ast.Unparen(second)
	switch left := first.(type) {
	case *ast.Ident:
		right, _ := second.(*ast.Ident)
		return right != nil && info.ObjectOf(left) == info.ObjectOf(right)
	case *ast.SelectorExpr:
		right, _ := second.(*ast.SelectorExpr)
		return right != nil &&
			info.ObjectOf(left.Sel) == info.ObjectOf(right.Sel) &&
			sameSimpleExpression(info, left.X, right.X)
	default:
		return false
	}
}

func (timeSinceRule) Metadata() Metadata {
	return timeConvenienceMetadata(
		"time-since",
		"detects time.Now().Sub calls that can use time.Since",
		"time.Since expresses elapsed time directly and has the same monotonic-clock behavior as time.Now().Sub for a time.Time argument.",
		useTimeSinceFix,
		"replace time.Now().Sub with time.Since",
		"time.Now().Sub(start)",
		"time.Since(start)",
	)
}

func (timeUntilRule) Metadata() Metadata {
	return timeConvenienceMetadata(
		"time-until",
		"detects Time.Sub(time.Now()) calls that can use time.Until",
		"time.Until expresses duration until a deadline directly and has the same monotonic-clock behavior as deadline.Sub(time.Now()).",
		useTimeUntilFix,
		"replace Time.Sub(time.Now()) with time.Until",
		"deadline.Sub(time.Now())",
		"time.Until(deadline)",
	)
}

func timeConvenienceMetadata(
	id, summary, documentation, fix, description, incorrect, correct string,
) Metadata {
	return Metadata{
		ID: id,
		Summary: summary,
		Documentation: documentation,
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		Fixes: []FixMetadata{{Name: fix, Description: description, Safety: FixSuggestion}},
		KnownLimitations: []string{
			"Only the standard library time.Time.Sub and time.Now functions are recognized through go/types.",
			"A suggestion is withheld when the rewrite would discard comments inside the original call.",
		},
		Examples: []Example{
			{
				Title: "Use the time convenience helper",
				Incorrect: incorrect,
				Correct: correct,
			},
		},
	}
}

func (timeSinceRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("time-since requires a call expression")
	}
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || len(call.Args) != 1 || !isTimeMethod(ctx.Info(), selector, "Sub") {
		return nil, nil
	}
	now, _ := ast.Unparen(selector.X).(*ast.CallExpr)
	qualifier, ok := timeNowQualifier(ctx.Info(), now)
	if !ok {
		return nil, nil
	}
	return timeConvenienceFinding(
		ctx,
		call,
		call.Args[0],
		qualifier,
		"Since",
		"use-time-since",
		useTimeSinceFix,
	)
}

func (timeUntilRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("time-until requires a call expression")
	}
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || len(call.Args) != 1 || !isTimeMethod(ctx.Info(), selector, "Sub") {
		return nil, nil
	}
	now, _ := ast.Unparen(call.Args[0]).(*ast.CallExpr)
	qualifier, ok := timeNowQualifier(ctx.Info(), now)
	if !ok {
		return nil, nil
	}
	return timeConvenienceFinding(
		ctx,
		call,
		selector.X,
		qualifier,
		"Until",
		"use-time-until",
		useTimeUntilFix,
	)
}

func isTimeMethod(info *types.Info, selector *ast.SelectorExpr, name string) bool {
	if info == nil || selector == nil {
		return false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	return function != nil &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "time" &&
		function.Name() == name
}

func timeNowQualifier(info *types.Info, call *ast.CallExpr) (ast.Expr, bool) {
	if call == nil ||
		len(call.Args) != 0 ||
		!isStandardFunction(info, call.Fun, "time", "Now") {
		return nil, false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil, false
	}
	return selector.X, true
}

func timeConvenienceFinding(
	ctx *TypesContext,
	call *ast.CallExpr,
	argument ast.Expr,
	qualifier ast.Expr,
	helper string,
	messageKey string,
	fixName string,
) ([]Finding, error) {
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: messageKey,
		Message: "use time." + helper + " for this duration calculation",
		Range: range_,
		Help: "use the standard time convenience helper",
	}
	argumentRange, err := ctx.Range(argument)
	if err != nil {
		return nil, err
	}
	qualifierRange, err := ctx.Range(qualifier)
	if err != nil {
		return nil, err
	}
	if commentsOutsideRetainedRanges(
		ctx.File().Comments(),
		range_,
		argumentRange,
		qualifierRange,
	) {
		finding.WithheldFixes = commentWithheldFix(
			fixName,
			"rewriting this duration calculation would remove comments",
		)
		return []Finding{finding}, nil
	}
	argumentSource, found := ctx.File().Slice(argumentRange)
	if !found {
		return nil, fmt.Errorf("time convenience argument has an invalid source range")
	}
	qualifierSource, found := ctx.File().Slice(qualifierRange)
	if !found {
		return nil, fmt.Errorf("time convenience qualifier has an invalid source range")
	}
	finding.Fixes = []Fix{
		{
			Name: fixName,
			Safety: FixSuggestion,
			Edits: []Edit{
				{
					Range: range_,
					NewText: qualifierSource +
						"." +
						helper +
						"(" +
						argumentSource +
						")",
				},
			},
		},
	}
	return []Finding{finding}, nil
}

func commentsOutsideRetainedRanges(
	comments []source.Comment,
	outer source.Range,
	retained ...source.Range,
) bool {
	for _, comment := range comments {
		if comment.Range.Start < outer.Start || comment.Range.End > outer.End {
			continue
		}
		kept := false
		for _, range_ := range retained {
			if comment.Range.Start >= range_.Start && comment.Range.End <= range_.End {
				kept = true
				break
			}
		}
		if !kept {
			return true
		}
	}
	return false
}

func (bufferStringConversionRule) Metadata() Metadata {
	return Metadata{
		ID: "buffer-string-conversion",
		Summary: "detects conversions that duplicate bytes.Buffer accessors",
		Documentation: "bytes.Buffer already exposes both String and Bytes. Converting the result of one accessor to the other representation adds an allocation or conversion that the matching accessor avoids.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeFile},
		Categories: []Category{CategoryPerformance, CategoryStyle},
		KnownLimitations: []string{
			"Map lookups using string(buffer.Bytes()) are excluded because the compiler can avoid an allocation for that form.",
			"Only direct bytes.Buffer receivers and the predeclared string and []byte target types are reported.",
			"No fix is offered because Buffer.Bytes aliases mutable storage while a converted string does not.",
		},
		Examples: []Example{
			{
				Title: "Use the matching buffer accessor",
				Incorrect: "text := string(buffer.Bytes())",
				Correct: "text := buffer.String()",
			},
		},
	}
}

func (bufferStringConversionRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	file, ok := node.(*ast.File)
	if !ok {
		return nil, fmt.Errorf("buffer-string-conversion requires a file")
	}
	if ctx == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil ||
		ctx.Package().Path() == "bytes" ||
		ctx.Package().Path() == "bytes_test" {
		return nil, nil
	}
	findings := make([]Finding, 0)
	var inspect func(ast.Node, ast.Node) error
	inspect = func(current ast.Node, parent ast.Node) error {
		if current == nil {
			return nil
		}
		if call, callOK := current.(*ast.CallExpr); callOK {
			finding, found, err := bufferConversionFinding(ctx, call, parent)
			if err != nil {
				return err
			}
			if found {
				findings = append(findings, finding)
			}
		}
		var childErr error
		ast.Inspect(
			current,
			func(child ast.Node) bool {
				if child == nil || child == current || childErr != nil {
					return childErr == nil
				}
				childErr = inspect(child, current)
				return false
			},
		)
		return childErr
	}
	if err := inspect(file, nil); err != nil {
		return nil, err
	}
	return findings, nil
}

func bufferConversionFinding(
	ctx *TypesContext,
	call *ast.CallExpr,
	parent ast.Node,
) (Finding, bool, error) {
	if call == nil || len(call.Args) != 1 || !ctx.Info().Types[call.Fun].IsType() {
		return Finding{}, false, nil
	}
	inner, _ := ast.Unparen(call.Args[0]).(*ast.CallExpr)
	selector, _ := func() (*ast.SelectorExpr, bool) {
		if inner == nil || len(inner.Args) != 0 {
			return nil, false
		}
		value, ok := ast.Unparen(inner.Fun).(*ast.SelectorExpr)
		return value, ok
	}()
	if selector == nil || !directBytesBuffer(ctx.Info().TypeOf(selector.X)) {
		return Finding{}, false, nil
	}
	target := types.Unalias(ctx.Info().TypeOf(call.Fun))
	method := ""
	if types.Identical(target, types.Typ[types.String]) &&
		isBytesBufferMethod(ctx.Info(), selector, "Bytes") {
		if index, ok := parent.(*ast.IndexExpr); ok && index.Index == call {
			return Finding{}, false, nil
		}
		method = "String"
	} else if slice, ok := target.(*types.Slice);
		ok &&
			types.Identical(slice.Elem(), types.Typ[types.Uint8]) &&
			isBytesBufferMethod(ctx.Info(), selector, "String") {
		method = "Bytes"
	} else {
		return Finding{}, false, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return Finding{}, false, err
	}
	return Finding{
		MessageKey: "use-buffer-method",
		Message: "use bytes.Buffer." + method + " instead of converting the other accessor",
		Range: range_,
		Help: "call the buffer accessor for the required representation",
	}, true, nil
}

func directBytesBuffer(type_ types.Type) bool {
	type_ = types.Unalias(type_)
	if pointer, ok := type_.(*types.Pointer); ok {
		type_ = types.Unalias(pointer.Elem())
	}
	named, _ := type_.(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "bytes" &&
		named.Obj().Name() == "Buffer"
}

func isBytesBufferMethod(info *types.Info, selector *ast.SelectorExpr, name string) bool {
	selection := info.Selections[selector]
	if selection == nil {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	return function != nil &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "bytes" &&
		function.Name() == name
}

func (unnecessaryFormatRule) Metadata() Metadata {
	return Metadata{
		ID: "unnecessary-format",
		Summary: "detects formatting calls whose constant format contains no directives",
		Documentation: "fmt.Sprintf adds parsing and formatting machinery only when its format contains directives. A compile-time constant format with no percent sign and no formatting arguments is already the complete result string.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryPerformance, CategoryStyle},
		Fixes: []FixMetadata{
			{
				Name: useFormatOperandFix,
				Description: "replace fmt.Sprintf with its constant format operand",
				Safety: FixSuggestion,
			},
		},
		KnownLimitations: []string{
			"Only the exact standard fmt.Sprintf call is recognized through go/types; logging, testing, error, print, and scan APIs are excluded to keep pedantic noise bounded.",
			"Calls with arguments, dynamic formats, and percent escapes are excluded.",
			"The suggestion is withheld when replacing the call would remove a comment.",
			"When the accepted fix makes its qualified fmt import unused, the fix coordinator removes only that fix-owned import; unrelated import organization remains out of scope.",
		},
		Examples: []Example{
			{
				Title: "Use a literal directly",
				Incorrect: "text := fmt.Sprintf(\"ready\")",
				Correct: "text := \"ready\"",
			},
		},
	}
}

func (unnecessaryFormatRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("unnecessary-format requires a call expression")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, nil
	}
	if len(call.Args) != 1 || !isStandardFunction(ctx.Info(), call.Fun, "fmt", "Sprintf") {
		return nil, nil
	}
	format := ctx.Info().Types[call.Args[0]].Value
	if format == nil ||
		format.Kind() != constant.String ||
		strings.Contains(constant.StringVal(format), "%") {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "omit-formatting",
		Message: "formatting call has no formatting directives",
		Range: range_,
		Help: "use the constant format operand directly",
	}
	operandRange, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	if commentsOutsideRetainedRange(ctx.File().Comments(), range_, operandRange) {
		finding.WithheldFixes = commentWithheldFix(
			useFormatOperandFix,
			"replacing this formatting call would remove comments",
		)
		return []Finding{finding}, nil
	}
	replacement, found := ctx.File().Slice(operandRange)
	if !found {
		return nil, fmt.Errorf("unnecessary format has an invalid operand range")
	}
	finding.Fixes = []Fix{
		{
			Name: useFormatOperandFix,
			Safety: FixSuggestion,
			Edits: []Edit{{Range: range_, NewText: replacement}},
		},
	}
	return []Finding{finding}, nil
}

func commentWithheldFix(name, message string) []WithheldFix {
	return []WithheldFix{{Name: name, Reason: FixWithheldComments, Message: message}}
}

func (inefficientStringComparisonRule) Metadata() Metadata {
	return Metadata{
		ID: "inefficient-string-comparison",
		Summary: "detects case conversion used only for string comparison",
		Documentation: "Converting both strings to lower or upper case allocates normalized strings solely to compare them. strings.EqualFold performs a Unicode-aware case-insensitive comparison directly.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBinaryExpr},
		Categories: []Category{CategoryPerformance, CategoryStyle},
		KnownLimitations: []string{
			"Only two matching strings.ToLower or strings.ToUpper calls with simple distinct arguments are reported.",
			"Mixed normalization, one-sided normalization, byte slices, and expressions with calls or indexing are excluded.",
			"No fix is offered because Unicode case folding is not identical to every normalization-based comparison for all inputs.",
		},
		Examples: []Example{
			{
				Title: "Compare strings with EqualFold",
				Incorrect: "strings.ToLower(left) == strings.ToLower(right)",
				Correct: "strings.EqualFold(left, right)",
			},
		},
	}
}

func (inefficientStringComparisonRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	comparison, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, fmt.Errorf("inefficient-string-comparison requires a binary expression")
	}
	if ctx == nil ||
		ctx.Info() == nil ||
		(comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return nil, nil
	}
	leftName, leftArgument, leftOK := stringCaseConversion(ctx.Info(), comparison.X)
	rightName, rightArgument, rightOK := stringCaseConversion(ctx.Info(), comparison.Y)
	if !leftOK ||
		!rightOK ||
		leftName != rightName ||
		!simpleExpression(leftArgument) ||
		!simpleExpression(rightArgument) ||
		sameSimpleExpression(ctx.Info(), leftArgument, rightArgument) {
		return nil, nil
	}
	range_, err := ctx.Range(comparison)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "use-equal-fold",
			Message: "case conversion allocates strings only for comparison",
			Range: range_,
			Help: "consider strings.EqualFold for case-insensitive comparison",
		},
	}, nil
}

func stringCaseConversion(info *types.Info, expression ast.Expr) (string, ast.Expr, bool) {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil || len(call.Args) != 1 {
		return "", nil, false
	}
	for _, name := range []string{"ToLower", "ToUpper"} {
		if isStandardFunction(info, call.Fun, "strings", name) {
			return name, call.Args[0], true
		}
	}
	return "", nil, false
}

func simpleExpression(expression ast.Expr) bool {
	switch current := ast.Unparen(expression).(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return simpleExpression(current.X)
	default:
		return false
	}
}
