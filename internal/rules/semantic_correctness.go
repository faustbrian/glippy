package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
)

type ignoredAppendResultRule struct{}

type nilMapWriteRule struct{}

type ineffectiveValueReceiverAssignmentRule struct{}

type nanComparisonRule struct{}

type integerDivisionBeforeConversionRule struct{}

type ineffectiveURLQueryMutationRule struct{}

type deferredLockRule struct{}

type deferBeforeErrorCheckRule struct{}

type infiniteRecursionRule struct{}

type impossibleInterfaceNilComparisonRule struct{}

func NewIgnoredAppendResultRule() Rule {
	return ignoredAppendResultRule{}
}

func NewNilMapWriteRule() Rule {
	return nilMapWriteRule{}
}

func NewIneffectiveValueReceiverAssignmentRule() Rule {
	return ineffectiveValueReceiverAssignmentRule{}
}

func NewNaNComparisonRule() Rule {
	return nanComparisonRule{}
}

func NewIntegerDivisionBeforeConversionRule() Rule {
	return integerDivisionBeforeConversionRule{}
}

func NewIneffectiveURLQueryMutationRule() Rule {
	return ineffectiveURLQueryMutationRule{}
}

func NewDeferredLockRule() Rule {
	return deferredLockRule{}
}

func NewDeferBeforeErrorCheckRule() Rule {
	return deferBeforeErrorCheckRule{}
}

func NewInfiniteRecursionRule() Rule {
	return infiniteRecursionRule{}
}

func NewImpossibleInterfaceNilComparisonRule() Rule {
	return impossibleInterfaceNilComparisonRule{}
}

func (ignoredAppendResultRule) Metadata() Metadata {
	metadata := semanticMetadata(
		"ignored-append-result",
		"detects append results that are discarded",
		"append may allocate a new backing array and always returns the slice header that owns the updated length. Discarding that result leaves the caller's slice length unchanged and may lose the appended values entirely.",
		PresetSuspicious,
		NodeAssignStmt,
		"_ = append(items, value)",
		"items = append(items, value)",
	)
	metadata.KnownLimitations = []string{
		"Only an explicit blank-identifier assignment is reported; a bare append call is already rejected by the Go type checker as an unused value.",
		"Generated files and packages with type errors are excluded.",
	}
	return metadata
}

func (ignoredAppendResultRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.AssignStmt)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"ignored-append-result requires an assignment and type information",
		)
	}
	if len(statement.Lhs) != 1 || len(statement.Rhs) != 1 {
		return nil, nil
	}
	blank, _ := ast.Unparen(statement.Lhs[0]).(*ast.Ident)
	if blank == nil || blank.Name != "_" {
		return nil, nil
	}
	call, _ := ast.Unparen(statement.Rhs[0]).(*ast.CallExpr)
	identifier, _ := ast.Unparen(callFun(call)).(*ast.Ident)
	if identifier == nil {
		return nil, nil
	}
	builtin, _ := ctx.Info().ObjectOf(identifier).(*types.Builtin)
	if builtin == nil || builtin.Name() != "append" {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "ignored-append-result",
			Message: "append result is discarded, so the updated slice header is lost",
			Range: range_,
			Help: "assign the result back to the destination slice",
		},
	}, nil
}

func (nilMapWriteRule) Metadata() Metadata {
	return semanticMetadata(
		"nil-map-write",
		"detects writes to maps proven to remain nil",
		"A declared map has a nil value until it is initialized. Assigning an entry through that nil map panics at runtime. This rule follows local straight-line declarations and assignments and reports only writes whose map is still proven nil.",
		PresetCorrectness,
		NodeFile,
		"var entries map[string]int\nentries[\"key\"] = 1",
		"entries := make(map[string]int)\nentries[\"key\"] = 1",
	)
}

func (nilMapWriteRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	file, ok := node.(*ast.File)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("nil-map-write requires a file and type information")
	}
	findings := make([]Finding, 0)
	for _, declaration := range file.Decls {
		function, _ := declaration.(*ast.FuncDecl)
		if function == nil || function.Body == nil {
			continue
		}
		produced, err := nilMapWritesInBlock(ctx, function.Body, make(map[*types.Var]bool))
		if err != nil {
			return nil, err
		}
		findings = append(findings, produced...)
	}
	return findings, nil
}

func nilMapWritesInBlock(
	ctx *TypesContext,
	block *ast.BlockStmt,
	nilMaps map[*types.Var]bool,
) ([]Finding, error) {
	findings := make([]Finding, 0)
	for _, statement := range block.List {
		switch statement := statement.(type) {
		case *ast.DeclStmt:
			invalidateNilMapFactsForStatement(ctx.Info(), statement, nilMaps)
			declaration, _ := statement.Decl.(*ast.GenDecl)
			if declaration == nil || declaration.Tok != token.VAR {
				continue
			}
			for _, specification := range declaration.Specs {
				values, _ := specification.(*ast.ValueSpec)
				if values == nil {
					continue
				}
				for index, name := range values.Names {
					variable, _ := ctx.Info().Defs[name].(*types.Var)
					if variable == nil || !isMapType(variable.Type()) {
						continue
					}
					nilMaps[variable] = index >= len(values.Values) ||
						isNilExpression(ctx.Info(), values.Values[index])
				}
			}
		case *ast.AssignStmt:
			invalidateNilMapFactsForStatement(ctx.Info(), statement, nilMaps)
			for _, target := range statement.Lhs {
				index, _ := ast.Unparen(target).(*ast.IndexExpr)
				if index == nil {
					continue
				}
				identifier, _ := ast.Unparen(index.X).(*ast.Ident)
				variable, _ := ctx.Info().ObjectOf(identifier).(*types.Var)
				if variable == nil || !nilMaps[variable] {
					continue
				}
				range_, err := ctx.Range(index)
				if err != nil {
					return nil, err
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "nil-map-write",
						Message: "assignment to this nil map will panic",
						Range: range_,
						Help: "initialize the map with make or a map literal before writing entries",
					},
				)
			}
			for index, target := range statement.Lhs {
				identifier, _ := ast.Unparen(target).(*ast.Ident)
				if identifier == nil {
					continue
				}
				variable, _ := ctx.Info().ObjectOf(identifier).(*types.Var)
				if statement.Tok == token.DEFINE {
					variable, _ = ctx.Info().Defs[identifier].(*types.Var)
				}
				if variable == nil || !isMapType(variable.Type()) {
					continue
				}
				nilMaps[variable] = index < len(statement.Rhs) &&
					isNilExpression(ctx.Info(), statement.Rhs[index])
			}
		case *ast.BlockStmt:
			produced, err := nilMapWritesInBlock(ctx, statement, nilMaps)
			if err != nil {
				return nil, err
			}
			findings = append(findings, produced...)
		case *ast.IfStmt:
			if statement.Init != nil {
				produced, err := nilMapWritesInBlock(
					ctx,
					&ast.BlockStmt{List: []ast.Stmt{statement.Init}},
					nilMaps,
				)
				if err != nil {
					return nil, err
				}
				findings = append(findings, produced...)
			}
			thenState := cloneNilMapState(nilMaps)
			produced, err := nilMapWritesInBlock(ctx, statement.Body, thenState)
			if err != nil {
				return nil, err
			}
			findings = append(findings, produced...)
			elseState := cloneNilMapState(nilMaps)
			if statement.Else != nil {
				produced, err = nilMapWritesInBlock(
					ctx,
					&ast.BlockStmt{List: []ast.Stmt{statement.Else}},
					elseState,
				)
				if err != nil {
					return nil, err
				}
				findings = append(findings, produced...)
			}
			mergeNilMapStates(nilMaps, thenState, elseState)
		default:
			invalidateNilMapFactsForStatement(ctx.Info(), statement, nilMaps)
		}
	}
	return findings, nil
}

func invalidateNilMapFactsForStatement(
	info *types.Info,
	statement ast.Stmt,
	nilMaps map[*types.Var]bool,
) {
	invalidate := func(expression ast.Expr) {
		identifier, _ := ast.Unparen(expression).(*ast.Ident)
		variable, _ := info.ObjectOf(identifier).(*types.Var)
		if variable != nil && nilMaps[variable] {
			nilMaps[variable] = false
		}
	}
	ast.Inspect(
		statement,
		func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				for _, target := range node.Lhs {
					invalidate(target)
				}
			case *ast.RangeStmt:
				invalidate(node.Key)
				invalidate(node.Value)
			case *ast.UnaryExpr:
				if node.Op != token.AND {
					return true
				}
				invalidate(node.X)
			}
			return true
		},
	)
}

func mergeNilMapStates(target, left, right map[*types.Var]bool) {
	for variable := range target {
		target[variable] = left[variable] && right[variable]
	}
}

func cloneNilMapState(input map[*types.Var]bool) map[*types.Var]bool {
	result := make(map[*types.Var]bool, len(input))
	for variable, nil_ := range input {
		result[variable] = nil_
	}
	return result
}

func isMapType(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	_, ok := types.Unalias(type_).Underlying().(*types.Map)
	return ok
}

func (ineffectiveValueReceiverAssignmentRule) Metadata() Metadata {
	metadata := semanticMetadata(
		"ineffective-value-receiver-assignment",
		"detects field assignments that are lost on value receivers",
		"A method with a value receiver operates on a copy. Assigning one of that receiver's fields cannot update the caller's value and is commonly an unintended pointer-receiver omission.",
		PresetSuspicious,
		NodeFuncDecl,
		"func (value counter) set() { value.count = 1 }",
		"func (value *counter) set() { value.count = 1 }",
	)
	metadata.KnownLimitations = []string{
		"Only direct, non-promoted receiver fields in result-less methods are considered.",
		"A mutation is excluded when the receiver is referenced later, captured by a function literal, or has its address taken, because the copied value may still be intentionally observed within the method.",
		"Generated files and packages with type errors are excluded.",
	}
	return metadata
}

func (ineffectiveValueReceiverAssignmentRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	function, ok := node.(*ast.FuncDecl)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"ineffective-value-receiver-assignment requires a function and type information",
		)
	}
	if function.Recv == nil ||
		len(function.Recv.List) != 1 ||
		len(function.Recv.List[0].Names) != 1 ||
		function.Body == nil {
		return nil, nil
	}
	if function.Type.Results != nil && len(function.Type.Results.List) != 0 {
		return nil, nil
	}
	if _, pointer := ast.Unparen(function.Recv.List[0].Type).(*ast.StarExpr); pointer {
		return nil, nil
	}
	receiver := ctx.Info().Defs[function.Recv.List[0].Names[0]]
	receiverUses, receiverCaptured, receiverAddressed := receiverUsePositions(
		ctx.Info(),
		function.Body,
		receiver,
	)
	mutationTargets := make([]ast.Expr, 0)
	ast.Inspect(
		function.Body,
		func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			assignment, assignmentFound := node.(*ast.AssignStmt)
			increment, incrementFound := node.(*ast.IncDecStmt)
			var candidateTargets []ast.Expr
			if assignmentFound {
				candidateTargets = assignment.Lhs
			} else if incrementFound {
				candidateTargets = []ast.Expr{increment.X}
			} else {
				return true
			}
			for _, target := range candidateTargets {
				selector, _ := ast.Unparen(target).(*ast.SelectorExpr)
				identifier, _ := ast.Unparen(
					selectorExpression(selector),
				).(*ast.Ident)
				selection := ctx.Info().Selections[selector]
				if identifier == nil ||
					ctx.Info().ObjectOf(identifier) != receiver ||
					selection == nil ||
					selection.Kind() != types.FieldVal ||
					len(selection.Index()) != 1 ||
					selection.Indirect() ||
					receiverCaptured ||
					receiverAddressed ||
					receiverReferencedAtOrAfter(receiverUses, target.End()) {
					continue
				}
				mutationTargets = append(mutationTargets, target)
			}
			return false
		},
	)
	findings := make([]Finding, 0, len(mutationTargets))
	for _, target := range mutationTargets {
		range_, err := ctx.Range(target)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "ineffective-value-receiver-assignment",
				Message: "assignment mutates only the method's value-receiver copy",
				Range: range_,
				Help: "use a pointer receiver when the mutation must persist",
			},
		)
	}
	return findings, nil
}

func selectorExpression(selector *ast.SelectorExpr) ast.Expr {
	if selector == nil {
		return nil
	}
	return selector.X
}

func receiverUsePositions(
	info *types.Info,
	body *ast.BlockStmt,
	receiver types.Object,
) ([]token.Pos, bool, bool) {
	positions := make([]token.Pos, 0)
	captured := false
	addressed := false
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if node == nil {
				return false
			}
			if literal, ok := node.(*ast.FuncLit);
				ok && objectReferenced(info, literal.Body, receiver) {
				captured = true
				return false
			}
			if unary, ok := node.(*ast.UnaryExpr);
				ok &&
					unary.Op == token.AND &&
					objectReferenced(info, unary.X, receiver) {
				addressed = true
			}
			identifier, ok := node.(*ast.Ident)
			if ok && info.ObjectOf(identifier) == receiver {
				positions = append(positions, identifier.Pos())
			}
			return true
		},
	)
	sort.Slice(
		positions,
		func(left, right int) bool {
			return positions[left] < positions[right]
		},
	)
	return positions, captured, addressed
}

func receiverReferencedAtOrAfter(positions []token.Pos, position token.Pos) bool {
	index := sort.Search(
		len(positions),
		func(index int) bool {
			return positions[index] >= position
		},
	)
	return index < len(positions)
}

func objectReferenced(info *types.Info, node ast.Node, object types.Object) bool {
	referenced := false
	ast.Inspect(
		node,
		func(node ast.Node) bool {
			if referenced || node == nil {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if ok && info.ObjectOf(identifier) == object {
				referenced = true
				return false
			}
			return true
		},
	)
	return referenced
}

func (nanComparisonRule) Metadata() Metadata {
	return semanticMetadata(
		"nan-comparison",
		"detects equality comparisons involving NaN",
		"IEEE floating-point NaN is unequal to every value, including itself. Comparing against math.NaN() with == or != therefore produces a constant result instead of testing whether a value is NaN.",
		PresetCorrectness,
		NodeBinaryExpr,
		"value == math.NaN()",
		"math.IsNaN(value)",
	)
}

func (nanComparisonRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	comparison, ok := node.(*ast.BinaryExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"nan-comparison requires a binary expression and type information",
		)
	}
	if comparison.Op != token.EQL && comparison.Op != token.NEQ ||
		(!isMathNaNCall(ctx.Info(), comparison.X) &&
			!isMathNaNCall(ctx.Info(), comparison.Y)) {
		return nil, nil
	}
	range_, err := ctx.Range(comparison)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "nan-comparison",
			Message: "NaN comparison has a constant result",
			Range: range_,
			Help: "use math.IsNaN to test for NaN",
		},
	}, nil
}

func isMathNaNCall(info *types.Info, expression ast.Expr) bool {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil || len(call.Args) != 0 {
		return false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	identifier := selectorIdent(selector)
	if identifier == nil {
		return false
	}
	function, _ := info.ObjectOf(identifier).(*types.Func)
	return function != nil &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "math" &&
		function.Name() == "NaN"
}

func selectorIdent(selector *ast.SelectorExpr) *ast.Ident {
	if selector == nil {
		return nil
	}
	return selector.Sel
}

func (integerDivisionBeforeConversionRule) Metadata() Metadata {
	metadata := semanticMetadata(
		"integer-division-before-conversion",
		"detects integer division converted to floating point afterward",
		"Integer division truncates before its result is converted to a floating-point type. When a fractional result is intended, at least one operand must be converted before the division.",
		PresetSuspicious,
		NodeCallExpr,
		"float64(total / count)",
		"float64(total) / float64(count)",
	)
	metadata.KnownLimitations = []string{
		"Exact constant integer divisions are excluded because conversion cannot recover a missing fraction.",
		"Nonconstant integer ratios deliberately converted after truncation require suppression.",
		"Generated files and packages with type errors are excluded.",
	}
	return metadata
}

func (integerDivisionBeforeConversionRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"integer-division-before-conversion requires a call and type information",
		)
	}
	if len(call.Args) != 1 ||
		!ctx.Info().Types[call.Fun].IsType() ||
		!isFloatType(ctx.Info().TypeOf(call)) {
		return nil, nil
	}
	division, _ := ast.Unparen(call.Args[0]).(*ast.BinaryExpr)
	if division == nil ||
		division.Op != token.QUO ||
		!isIntegerBasicType(ctx.Info().TypeOf(division.X)) ||
		!isIntegerBasicType(ctx.Info().TypeOf(division.Y)) ||
		exactConstantIntegerDivision(ctx.Info(), division) {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "integer-division-before-conversion",
			Message: "integer division truncates before this floating-point conversion",
			Range: range_,
			Help: "convert an operand before division when a fractional result is intended",
		},
	}, nil
}

func exactConstantIntegerDivision(info *types.Info, division *ast.BinaryExpr) bool {
	if info == nil || division == nil {
		return false
	}
	left := info.Types[division.X].Value
	right := info.Types[division.Y].Value
	if left == nil ||
		right == nil ||
		constant.Sign(right) == 0 ||
		left.Kind() != constant.Int ||
		right.Kind() != constant.Int {
		return false
	}
	return constant.Sign(constant.BinaryOp(left, token.REM, right)) == 0
}

func isIntegerBasicType(type_ types.Type) bool {
	basic, _ := types.Unalias(type_).Underlying().(*types.Basic)
	return basic != nil && basic.Info() & types.IsInteger != 0
}

func isFloatType(type_ types.Type) bool {
	basic, _ := types.Unalias(type_).Underlying().(*types.Basic)
	return basic != nil && basic.Info() & types.IsFloat != 0
}

func (ineffectiveURLQueryMutationRule) Metadata() Metadata {
	metadata := semanticMetadata(
		"ineffective-url-query-mutation",
		"detects mutations of temporary URL query values",
		"URL.Query returns a newly parsed url.Values map. Mutating that temporary map does not update URL.RawQuery, so the request URL remains unchanged unless the encoded values are assigned back.",
		PresetCorrectness,
		NodeExprStmt,
		"request.URL.Query().Set(\"page\", \"2\")",
		"query := request.URL.Query()\nquery.Set(\"page\", \"2\")\nrequest.URL.RawQuery = query.Encode()",
	)
	metadata.NodeInterests = []NodeKind{NodeExprStmt, NodeAssignStmt}
	metadata.KnownLimitations = []string{
		"Direct Set, Add, Del, delete, clear, and index assignments on a temporary URL.Query result are reported.",
		"Mutations routed through helper functions or aliases require value-flow analysis and are excluded.",
		"Generated files and packages with type errors are excluded.",
	}
	return metadata
}

func (ineffectiveURLQueryMutationRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"ineffective-url-query-mutation requires a mutation and type information",
		)
	}
	mutation := temporaryQueryMutation(ctx.Info(), node)
	if mutation == nil {
		return nil, nil
	}
	range_, err := ctx.Range(mutation)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "ineffective-url-query-mutation",
			Message: "mutation affects only the temporary result of URL.Query",
			Range: range_,
			Help: "store the query values and assign query.Encode() to URL.RawQuery",
		},
	}, nil
}

func temporaryQueryMutation(info *types.Info, node ast.Node) ast.Node {
	switch node := node.(type) {
	case *ast.AssignStmt:
		for _, target := range node.Lhs {
			index, _ := ast.Unparen(target).(*ast.IndexExpr)
			if index != nil && isURLQueryCall(info, index.X) {
				return index
			}
		}
		return nil
	case *ast.ExprStmt:
		call, _ := ast.Unparen(node.X).(*ast.CallExpr)
		if call == nil {
			return nil
		}
		if isURLQueryMethodMutation(info, call) || isURLQueryBuiltinMutation(info, call) {
			return call
		}
		return nil
	default:
		return nil
	}
}

func isURLQueryMethodMutation(info *types.Info, mutation *ast.CallExpr) bool {
	selector, _ := ast.Unparen(callFun(mutation)).(*ast.SelectorExpr)
	if selector == nil ||
		(selector.Sel.Name != "Set" &&
			selector.Sel.Name != "Add" &&
			selector.Sel.Name != "Del") {
		return false
	}
	return isURLQueryCall(info, selector.X)
}

func isURLQueryBuiltinMutation(info *types.Info, mutation *ast.CallExpr) bool {
	identifier, _ := ast.Unparen(mutation.Fun).(*ast.Ident)
	builtin, _ := info.ObjectOf(identifier).(*types.Builtin)
	if builtin == nil ||
		(builtin.Name() != "delete" && builtin.Name() != "clear") ||
		len(mutation.Args) == 0 {
		return false
	}
	return isURLQueryCall(info, mutation.Args[0])
}

func isURLQueryCall(info *types.Info, expression ast.Expr) bool {
	query, _ := ast.Unparen(expression).(*ast.CallExpr)
	querySelector, _ := ast.Unparen(callFun(query)).(*ast.SelectorExpr)
	selection := info.Selections[querySelector]
	function, _ := selectionObject(selection).(*types.Func)
	return function != nil &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "net/url" &&
		function.Name() == "Query"
}

func selectionObject(selection *types.Selection) types.Object {
	if selection == nil {
		return nil
	}
	return selection.Obj()
}

func (deferredLockRule) Metadata() Metadata {
	metadata := semanticMetadata(
		"deferred-lock",
		"detects a deferred Lock immediately after locking",
		"Calling Mutex.Lock or RWMutex.RLock and immediately deferring the same lock operation on the same receiver is highly likely to be a transposition of the corresponding deferred unlock.",
		PresetCorrectness,
		NodeBlockStmt,
		"lock.Lock()\ndefer lock.Lock()",
		"lock.Lock()\ndefer lock.Unlock()",
	)
	metadata.KnownLimitations = []string{
		"Only adjacent calls on the same simple identifier or selector receiver are reported; standalone deferred locks may intentionally restore a caller-owned lock before return.",
		"Generated files and packages with type errors are excluded.",
	}
	return metadata
}

func (deferredLockRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	block, ok := node.(*ast.BlockStmt)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("deferred-lock requires a block and type information")
	}
	findings := make([]Finding, 0)
	for index := 1; index < len(block.List); index++ {
		previous, _ := block.List[index - 1].(*ast.ExprStmt)
		deferred, _ := block.List[index].(*ast.DeferStmt)
		previousCall, _ := ast.Unparen(expressionStatement(previous)).(*ast.CallExpr)
		if previousCall == nil || deferred == nil {
			continue
		}
		previousReceiver, previousName, previousOK := syncLockCall(ctx.Info(), previousCall)
		deferredReceiver, deferredName, deferredOK := syncLockCall(
			ctx.Info(),
			deferred.Call,
		)
		if !previousOK ||
			!deferredOK ||
			previousName != deferredName ||
			!sameSimpleExpression(ctx.Info(), previousReceiver, deferredReceiver) {
			continue
		}
		range_, err := ctx.Range(deferred.Call)
		if err != nil {
			return nil, err
		}
		unlock := "Unlock"
		if deferredName == "RLock" {
			unlock = "RUnlock"
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "deferred-lock",
				Message: "the same lock operation is deferred immediately after locking",
				Range: range_,
				Help: "defer " + unlock + " on the same receiver instead",
			},
		)
	}
	return findings, nil
}

func expressionStatement(statement *ast.ExprStmt) ast.Expr {
	if statement == nil {
		return nil
	}
	return statement.X
}

func syncLockCall(info *types.Info, call *ast.CallExpr) (ast.Expr, string, bool) {
	if info == nil || call == nil || len(call.Args) != 0 {
		return nil, "", false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	selection := info.Selections[selector]
	function, _ := selectionObject(selection).(*types.Func)
	if selector == nil ||
		function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "sync" ||
		(function.Name() != "Lock" && function.Name() != "RLock") {
		return nil, "", false
	}
	return selector.X, function.Name(), true
}

func (deferBeforeErrorCheckRule) Metadata() Metadata {
	metadata := semanticMetadata(
		"defer-before-error-check",
		"detects cleanup deferred before acquisition errors are checked",
		"Resource acquisition commonly returns a resource and an error. Deferring a method on that resource before checking the paired error can dereference an invalid resource and hide the original failure.",
		PresetSuspicious,
		NodeBlockStmt,
		"resource, err := acquire()\ndefer resource.Close()\nif err != nil { return err }",
		"resource, err := acquire()\nif err != nil { return err }\ndefer resource.Close()",
	)
	metadata.KnownLimitations = []string{
		"Only a direct acquisition assignment, deferred Close call, and later err != nil comparison in the same block are reported.",
		"Generated files and packages with type errors are excluded.",
	}
	return metadata
}

func (deferBeforeErrorCheckRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	block, ok := node.(*ast.BlockStmt)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"defer-before-error-check requires a block and type information",
		)
	}
	findings := make([]Finding, 0)
	for index, raw := range block.List {
		assignment, _ := raw.(*ast.AssignStmt)
		if assignment == nil || len(assignment.Lhs) < 2 || len(assignment.Rhs) != 1 {
			continue
		}
		call, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		tuple, _ := ctx.Info().TypeOf(call).(*types.Tuple)
		if call == nil ||
			tuple == nil ||
			tuple.Len() != len(assignment.Lhs) ||
			tuple.Len() < 2 ||
			!isErrorType(tuple.At(tuple.Len() - 1).Type()) {
			continue
		}
		resourceIdent, _ := ast.Unparen(assignment.Lhs[0]).(*ast.Ident)
		errorIdent, _ := ast.Unparen(assignment.Lhs[tuple.Len() - 1]).(*ast.Ident)
		resource := assignedObject(ctx.Info(), assignment, resourceIdent)
		errorObject := assignedObject(ctx.Info(), assignment, errorIdent)
		if resource == nil || errorObject == nil || !typeHasCloseMethod(resource.Type()) {
			continue
		}
		var deferred *ast.DeferStmt
		for _, following := range block.List[index + 1:] {
			if statementAssignsObject(ctx.Info(), following, resource) ||
				statementAssignsObject(ctx.Info(), following, errorObject) {
				break
			}
			if branch, isIf := following.(*ast.IfStmt);
				isIf &&
					expressionComparesObjectNotEqualNil(
						ctx.Info(),
						branch.Cond,
						errorObject,
					) {
				if deferred != nil {
					range_, err := ctx.Range(deferred.Call)
					if err != nil {
						return nil, err
					}
					findings = append(
						findings,
						Finding{
							MessageKey: "defer-before-error-check",
							Message: "cleanup is deferred before the acquisition error is checked",
							Range: range_,
							Help: "check the error before using or deferring cleanup of the resource",
						},
					)
				}
				break
			}
			candidate, _ := following.(*ast.DeferStmt)
			if candidate != nil &&
				deferredCloseReceiverObject(ctx.Info(), candidate.Call) ==
					resource {
				deferred = candidate
			}
		}
	}
	return findings, nil
}

func statementAssignsObject(info *types.Info, statement ast.Stmt, object types.Object) bool {
	assigned := false
	ast.Inspect(
		statement,
		func(node ast.Node) bool {
			if assigned || node == nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, target := range assignment.Lhs {
				identifier, _ := ast.Unparen(target).(*ast.Ident)
				if identifier != nil && info.ObjectOf(identifier) == object {
					assigned = true
					return false
				}
			}
			return false
		},
	)
	return assigned
}

func assignedObject(
	info *types.Info,
	assignment *ast.AssignStmt,
	identifier *ast.Ident,
) types.Object {
	if info == nil || assignment == nil || identifier == nil {
		return nil
	}
	if assignment.Tok == token.DEFINE && info.Defs[identifier] != nil {
		return info.Defs[identifier]
	}
	return info.ObjectOf(identifier)
}

func typeHasCloseMethod(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	method, _, _ := types.LookupFieldOrMethod(type_, true, nil, "Close")
	function, _ := method.(*types.Func)
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && signature.Params().Len() == 0
}

func expressionComparesObjectNotEqualNil(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
) bool {
	comparison, _ := ast.Unparen(expression).(*ast.BinaryExpr)
	if comparison == nil || comparison.Op != token.NEQ {
		return false
	}
	return objectAndNil(info, comparison.X, comparison.Y, object) ||
		objectAndNil(info, comparison.Y, comparison.X, object)
}

func objectAndNil(
	info *types.Info,
	objectExpression, nilExpression ast.Expr,
	object types.Object,
) bool {
	identifier, _ := ast.Unparen(objectExpression).(*ast.Ident)
	return identifier != nil &&
		info.ObjectOf(identifier) == object &&
		isNilExpression(info, nilExpression)
}

func callReceiverObject(info *types.Info, call *ast.CallExpr) types.Object {
	if info == nil || call == nil {
		return nil
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	identifier, _ := ast.Unparen(selectorExpression(selector)).(*ast.Ident)
	if identifier == nil {
		return nil
	}
	return info.ObjectOf(identifier)
}

func deferredCloseReceiverObject(info *types.Info, call *ast.CallExpr) types.Object {
	if call == nil {
		return nil
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || selector.Sel.Name != "Close" {
		return nil
	}
	return callReceiverObject(info, call)
}

func (infiniteRecursionRule) Metadata() Metadata {
	return semanticMetadata(
		"infinite-recursion",
		"detects functions whose only statement directly calls themselves",
		"A function whose complete body directly returns or invokes the same function has no terminating path. Every invocation recurses until stack exhaustion without producing a result.",
		PresetCorrectness,
		NodeFuncDecl,
		"func recurse(value int) int { return recurse(value) }",
		"func recurse(value int) int { if value == 0 { return 0 }; return recurse(value - 1) }",
	)
}

func (infiniteRecursionRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	function, ok := node.(*ast.FuncDecl)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"infinite-recursion requires a function and type information",
		)
	}
	if function.Body == nil || len(function.Body.List) != 1 {
		return nil, nil
	}
	call := soleRecursiveCall(function.Body.List[0])
	if call == nil || calledFunctionObject(ctx.Info(), call) != ctx.Info().Defs[function.Name] {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "infinite-recursion",
			Message: "this function can only recurse and has no terminating path",
			Range: range_,
			Help: "add a terminating path or call the intended function",
		},
	}, nil
}

func soleRecursiveCall(statement ast.Stmt) *ast.CallExpr {
	switch statement := statement.(type) {
	case *ast.ReturnStmt:
		if len(statement.Results) != 1 {
			return nil
		}
		call, _ := ast.Unparen(statement.Results[0]).(*ast.CallExpr)
		return call
	case *ast.ExprStmt:
		call, _ := ast.Unparen(statement.X).(*ast.CallExpr)
		return call
	default:
		return nil
	}
}

func calledFunctionObject(info *types.Info, call *ast.CallExpr) types.Object {
	if info == nil || call == nil {
		return nil
	}
	switch function := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		return info.ObjectOf(function)
	case *ast.SelectorExpr:
		return selectionObject(info.Selections[function])
	default:
		return nil
	}
}

func (impossibleInterfaceNilComparisonRule) Metadata() Metadata {
	metadata := semanticMetadata(
		"impossible-interface-nil-comparison",
		"detects nil comparisons against interfaces holding concrete values",
		"Converting a concrete value to an interface produces a non-nil interface because its dynamic type is present, even when the concrete value is a typed nil pointer, map, slice, function, or channel. Comparing that conversion to nil therefore has a constant result.",
		PresetCorrectness,
		NodeBinaryExpr,
		"any(42) == nil",
		"value == nil",
	)
	metadata.KnownLimitations = []string{
		"Direct conversions are reported; interface-valued operands and type parameters are excluded because their dynamic value may be nil.",
		"Generated files and packages with type errors are excluded.",
	}
	return metadata
}

func (impossibleInterfaceNilComparisonRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	comparison, ok := node.(*ast.BinaryExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"impossible-interface-nil-comparison requires a comparison and type information",
		)
	}
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil, nil
	}
	conversion := interfaceConversionComparedToNil(ctx.Info(), comparison.X, comparison.Y)
	if conversion == nil {
		conversion = interfaceConversionComparedToNil(
			ctx.Info(),
			comparison.Y,
			comparison.X,
		)
	}
	if conversion == nil {
		return nil, nil
	}
	range_, err := ctx.Range(comparison)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "impossible-interface-nil-comparison",
			Message: "interface conversion of this concrete value cannot be nil",
			Range: range_,
			Help: "compare the concrete value directly or remove the constant condition",
		},
	}, nil
}

func interfaceConversionComparedToNil(info *types.Info, value, nil_ ast.Expr) *ast.CallExpr {
	if !isNilExpression(info, nil_) {
		return nil
	}
	call, _ := ast.Unparen(value).(*ast.CallExpr)
	if call == nil || len(call.Args) != 1 || !info.Types[call.Fun].IsType() {
		return nil
	}
	convertedType := info.TypeOf(call)
	if convertedType == nil {
		return nil
	}
	if _, ok := types.Unalias(convertedType).Underlying().(*types.Interface); !ok {
		return nil
	}
	if isNilExpression(info, call.Args[0]) || typeMayBeNilInterface(info.TypeOf(call.Args[0])) {
		return nil
	}
	return call
}

func typeMayBeNilInterface(type_ types.Type) bool {
	if type_ == nil {
		return true
	}
	_, interface_ := types.Unalias(type_).Underlying().(*types.Interface)
	return interface_
}

func semanticMetadata(
	id string,
	summary string,
	documentation string,
	preset Preset,
	interest NodeKind,
	incorrect string,
	correct string,
) Metadata {
	categories := []Category{CategoryCorrectness}
	if preset == PresetSuspicious {
		categories = append(categories, CategorySuspicious)
	}
	return Metadata{
		ID: id,
		Summary: summary,
		Documentation: documentation,
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{preset},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{interest},
		Categories: categories,
		KnownLimitations: []string{
			"The rule reports only the directly proven syntax and type pattern documented above.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Preserve the intended behavior",
				Incorrect: incorrect,
				Correct: correct,
			},
		},
	}
}

func callFun(call *ast.CallExpr) ast.Expr {
	if call == nil {
		return nil
	}
	return call.Fun
}
