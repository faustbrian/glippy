package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

type suspiciousRangeRule struct{}

const suspiciousRangeFileWorkBudget uint64 = 4_000_000

type suspiciousRangeWorkBudget struct {
	remaining uint64
}

// NewSuspiciousRangeRule constructs the copied-range-value mutation rule for
// product registry composition.
func NewSuspiciousRangeRule() Rule {
	return suspiciousRangeRule{}
}

func (suspiciousRangeRule) Metadata() Metadata {
	return Metadata{
		ID: "suspicious-range",
		Summary: "detects mutations made only to a copied range value",
		Documentation: "Range values are copies. Mutating a field or element reached only through a non-pointer struct or array range variable does not update the slice, array, or map element that produced it and is usually an ineffective update.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeRangeStmt},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"The rule reports assignments and increments rooted in the exact range value object and ignores nested function literals.",
			"Paths that cross a pointer, slice, map, interface, or channel are excluded because mutation can reach shared state.",
			"A mutation followed by any later direct use of the range value, or by use of a local function that captured it before the mutation, is not reported because it can be intentional local computation, projection, or write-back.",
			"Adversarial loops whose closure-state proof exceeds the fixed per-file work budget are conservatively left unreported.",
		},
		Examples: []Example{
			{
				Title: "Mutate a slice element through its index",
				Incorrect: "for _, value := range values { value.ready = true }",
				Correct: "for index := range values { values[index].ready = true }",
			},
		},
	}
}

func (suspiciousRangeRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	loop, ok := node.(*ast.RangeStmt)
	if !ok {
		return nil, fmt.Errorf("suspicious-range requires a range statement")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("suspicious-range requires complete type information")
	}
	identifier, _ := loop.Value.(*ast.Ident)
	if identifier == nil || identifier.Name == "_" {
		return nil, nil
	}
	object := ctx.Info().ObjectOf(identifier)
	if object == nil || !copiedAggregateType(object.Type()) {
		return nil, nil
	}
	if !reserveSuspiciousRangeWork(ctx, loop.Body) {
		return nil, nil
	}
	findings := make([]Finding, 0)
	var rangeErr error
	ast.Inspect(
		loop.Body,
		func(current ast.Node) bool {
			if rangeErr != nil {
				return false
			}
			if current == nil {
				return true
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			var targets []ast.Expr
			var mutationStart token.Pos
			var mutationEnd token.Pos
			switch statement := current.(type) {
			case *ast.AssignStmt:
				targets = statement.Lhs
				mutationStart = statement.Pos()
				mutationEnd = statement.End()
			case *ast.IncDecStmt:
				targets = []ast.Expr{statement.X}
				mutationStart = statement.Pos()
				mutationEnd = statement.End()
			default:
				return true
			}
			for _, target := range targets {
				if !mutationStaysOnRangeCopy(ctx.Info(), target, object) {
					continue
				}
				if rangeValueUsedAfter(
					ctx.Info(),
					loop.Body,
					object,
					ctx.PackageSyntax(),
					mutationStart,
					mutationEnd,
				) {
					continue
				}
				range_, err := ctx.Range(target)
				if err != nil {
					rangeErr = err
					return false
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "range-value-copy-mutation",
						Message: "this mutation changes only the range value copy",
						Range: range_,
						Help: "range over indexes or store the modified value back into the collection",
					},
				)
			}
			return true
		},
	)
	return findings, rangeErr
}

func reserveSuspiciousRangeWork(ctx *TypesContext, body *ast.BlockStmt) bool {
	if ctx == nil || body == nil {
		return false
	}
	var nodes uint64
	var mutations uint64
	var closureUpdates uint64
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if node == nil {
				return false
			}
			nodes++
			switch node.(type) {
			case *ast.AssignStmt:
				mutations++
				closureUpdates++
			case *ast.IncDecStmt:
				mutations++
			case *ast.ValueSpec:
				closureUpdates++
			}
			return true
		},
	)
	cost := suspiciousRangeWorkCost(nodes, mutations, closureUpdates)
	value := ctx.memoized(
		"suspicious-range/work-budget-v1",
		func() any {
			return &suspiciousRangeWorkBudget{remaining: suspiciousRangeFileWorkBudget}
		},
	)
	budget, _ := value.(*suspiciousRangeWorkBudget)
	if budget == nil || cost > budget.remaining {
		return false
	}
	budget.remaining -= cost
	return true
}

func suspiciousRangeWorkCost(factors ...uint64) uint64 {
	cost := uint64(1)
	for _, factor := range factors {
		if factor == 0 {
			factor = 1
		}
		if cost > suspiciousRangeFileWorkBudget / factor {
			return suspiciousRangeFileWorkBudget + 1
		}
		cost *= factor
	}
	if len(factors) >= 3 {
		factor := factors[2]
		if factor == 0 {
			factor = 1
		}
		if cost > suspiciousRangeFileWorkBudget / factor {
			return suspiciousRangeFileWorkBudget + 1
		}
		cost *= factor
	}
	return cost
}

func rangeValueUsedAfter(
	info *types.Info,
	body *ast.BlockStmt,
	object types.Object,
	files *PackageSyntax,
	mutationStart token.Pos,
	mutationEnd token.Pos,
) bool {
	if info == nil ||
		body == nil ||
		object == nil ||
		!mutationStart.IsValid() ||
		!mutationEnd.IsValid() {
		return false
	}
	if objectUsedInExecutedNodeAfter(info, body, object, mutationEnd) {
		return true
	}
	if objectReadOnLoopBackedge(info, body, object, mutationStart) {
		return true
	}
	return capturedRangeValueUsedAfter(info, body, object, files, mutationEnd)
}

func objectReadOnLoopBackedge(
	info *types.Info,
	body *ast.BlockStmt,
	object types.Object,
	mutationStart token.Pos,
) bool {
	if info == nil || body == nil || object == nil || !mutationStart.IsValid() {
		return false
	}
	found := false
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil || found {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			var loopBody *ast.BlockStmt
			switch loop := current.(type) {
			case *ast.ForStmt:
				loopBody = loop.Body
			case *ast.RangeStmt:
				loopBody = loop.Body
			default:
				return true
			}
			if loopBody == nil ||
				mutationStart < loopBody.Pos() ||
				mutationStart >= loopBody.End() {
				return true
			}
			if !loopBackedgeStructurallyReachable(loopBody, mutationStart) {
				return true
			}
			writeOnly := writeOnlyObjectIdentifiers(info, loopBody, object)
			found = objectReadInMutationStatement(
				info,
				loopBody,
				object,
				mutationStart,
				writeOnly,
			) ||
				objectReadBeforePosition(
					info,
					loopBody,
					object,
					mutationStart,
					writeOnly,
				)
			if loop, _ := current.(*ast.ForStmt); loop != nil {
				for _, repeated := range []ast.Node{loop.Post, loop.Cond} {
					found = found ||
						objectReadBeforePosition(
							info,
							repeated,
							object,
							mutationStart,
							writeOnly,
						)
				}
			}
			return !found
		},
	)
	return found
}

func objectReadInMutationStatement(
	info *types.Info,
	node ast.Node,
	object types.Object,
	position token.Pos,
	writeOnly map[*ast.Ident]struct{},
) bool {
	if info == nil || node == nil || object == nil || !position.IsValid() {
		return false
	}
	read := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if current == nil || read {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			switch current := current.(type) {
			case *ast.AssignStmt:
				if current.Pos() != position {
					return true
				}
				for _, expression := range current.Rhs {
					if expressionUsesObject(info, expression, object) {
						read = true
						return false
					}
				}
				for _, target := range current.Lhs {
					ast.Inspect(
						target,
						func(candidate ast.Node) bool {
							identifier, _ := candidate.(*ast.Ident)
							if identifier == nil ||
								info.ObjectOf(identifier) !=
									object {
								return true
							}
							_, onlyWritten := writeOnly[identifier]
							if current.Tok != token.ASSIGN ||
								!onlyWritten {
								read = true
							}
							return !read
						},
					)
					if read {
						return false
					}
				}
				return false
			case *ast.IncDecStmt:
				if current.Pos() == position &&
					expressionUsesObject(info, current.X, object) {
					read = true
				}
				return false
			}
			return true
		},
	)
	return read
}

func loopBackedgeStructurallyReachable(body *ast.BlockStmt, position token.Pos) bool {
	if body == nil || !position.IsValid() {
		return true
	}
	parents := make(map[ast.Node]ast.Node)
	var parent ast.Node
	var containing ast.Stmt
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				parent = parents[parent]
				return false
			}
			if parent != nil {
				parents[current] = parent
			}
			parent = current
			if literal, _ := current.(*ast.FuncLit); literal != nil {
				return false
			}
			statement, _ := current.(ast.Stmt)
			if statement != nil &&
				statement.Pos() <= position &&
				position < statement.End() &&
				(containing == nil ||
					statement.End() - statement.Pos() <
						containing.End() - containing.Pos()) {
				containing = statement
			}
			return true
		},
	)
	if containing == nil {
		return true
	}
	for child := ast.Node(containing); child != nil && child != body; {
		parent := parents[child]
		switch parent := parent.(type) {
		case *ast.BlockStmt:
			if statement, _ := child.(ast.Stmt);
				statement != nil &&
					!clauseBlock(parent, parents) &&
					statementListTerminatesLoop(
						parent.List,
						statement,
						body,
						parents,
					) {
				return false
			}
		case *ast.CaseClause:
			if statement, _ := child.(ast.Stmt);
				statement != nil &&
					statementListTerminatesLoop(
						parent.Body,
						statement,
						body,
						parents,
					) {
				return false
			}
		case *ast.CommClause:
			if statement, _ := child.(ast.Stmt);
				statement != nil &&
					statementListTerminatesLoop(
						parent.Body,
						statement,
						body,
						parents,
					) {
				return false
			}
		}
		child = parent
	}
	return true
}

func clauseBlock(block *ast.BlockStmt, parents map[ast.Node]ast.Node) bool {
	if block == nil {
		return false
	}
	switch parents[block].(type) {
	case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return true
	default:
		return false
	}
}

func statementListTerminatesLoop(
	statements []ast.Stmt,
	current ast.Stmt,
	body *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
) bool {
	found := false
	for _, statement := range statements {
		if statement == current {
			found = true
			continue
		}
		if !found || statement == nil {
			continue
		}
		if statementTerminatesLoop(statement, body, parents) {
			return true
		}
	}
	return false
}

func statementTerminatesLoop(
	statement ast.Stmt,
	body *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
) bool {
	switch statement := statement.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return statement.Tok == token.BREAK &&
			statement.Label == nil &&
			breakTargetsLoopBody(statement, body, parents)
	case *ast.BlockStmt:
		for _, nested := range statement.List {
			if statementTerminatesLoop(nested, body, parents) {
				return true
			}
		}
		return false
	case *ast.IfStmt:
		if statement.Else == nil ||
			!statementTerminatesLoop(statement.Body, body, parents) {
			return false
		}
		switch alternative := statement.Else.(type) {
		case *ast.BlockStmt:
			return statementTerminatesLoop(alternative, body, parents)
		case *ast.IfStmt:
			return statementTerminatesLoop(alternative, body, parents)
		default:
			return false
		}
	default:
		return false
	}
}

func breakTargetsLoopBody(
	branch *ast.BranchStmt,
	body *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
) bool {
	for current := ast.Node(branch); current != nil; current = parents[current] {
		if current == body {
			return true
		}
		switch current.(type) {
		case *ast.ForStmt,
			*ast.RangeStmt,
			*ast.SwitchStmt,
			*ast.TypeSwitchStmt,
			*ast.SelectStmt:
			return false
		}
	}
	return false
}

func objectReadBeforePosition(
	info *types.Info,
	node ast.Node,
	object types.Object,
	position token.Pos,
	writeOnly map[*ast.Ident]struct{},
) bool {
	if info == nil || node == nil || object == nil {
		return false
	}
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if current == nil || found {
				return false
			}
			if identifier, _ := current.(*ast.Ident);
				identifier != nil &&
					identifier.Pos() < position &&
					info.ObjectOf(identifier) == object {
				if _, onlyWritten := writeOnly[identifier]; !onlyWritten {
					found = true
					return false
				}
			}
			if call, _ := current.(*ast.CallExpr); call != nil {
				if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit);
					literal != nil {
					found = objectReadBeforePosition(
						info,
						literal.Body,
						object,
						position,
						writeOnly,
					)
					for _, argument := range call.Args {
						found = found ||
							objectReadBeforePosition(
								info,
								argument,
								object,
								position,
								writeOnly,
							)
					}
					return false
				}
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			return true
		},
	)
	return found
}

func writeOnlyObjectIdentifiers(
	info *types.Info,
	node ast.Node,
	object types.Object,
) map[*ast.Ident]struct{} {
	writeOnly := make(map[*ast.Ident]struct{})
	if info == nil || node == nil || object == nil {
		return writeOnly
	}
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			assignment, _ := current.(*ast.AssignStmt)
			if assignment == nil || assignment.Tok != token.ASSIGN {
				return true
			}
			for _, target := range assignment.Lhs {
				if directObject(info, target) != object &&
					!mutationStaysOnRangeCopy(info, target, object) {
					continue
				}
				collectWriteOnlyMutationPath(info, target, object, writeOnly)
			}
			return true
		},
	)
	return writeOnly
}

func collectWriteOnlyMutationPath(
	info *types.Info,
	target ast.Expr,
	object types.Object,
	writeOnly map[*ast.Ident]struct{},
) {
	if info == nil || target == nil || object == nil {
		return
	}
	switch target := ast.Unparen(target).(type) {
	case *ast.Ident:
		if info.ObjectOf(target) == object {
			writeOnly[target] = struct{}{}
		}
	case *ast.SelectorExpr:
		collectWriteOnlyMutationPath(info, target.X, object, writeOnly)
	case *ast.IndexExpr:
		collectWriteOnlyMutationPath(info, target.X, object, writeOnly)
	case *ast.IndexListExpr:
		collectWriteOnlyMutationPath(info, target.X, object, writeOnly)
	}
}

func objectUsedInExecutedNodeAfter(
	info *types.Info,
	node ast.Node,
	object types.Object,
	position token.Pos,
) bool {
	if info == nil || node == nil || object == nil {
		return false
	}
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if current == nil || found {
				return false
			}
			if identifier, _ := current.(*ast.Ident);
				identifier != nil &&
					(!position.IsValid() || identifier.Pos() > position) &&
					info.ObjectOf(identifier) == object {
				found = true
				return false
			}
			if call, _ := current.(*ast.CallExpr); call != nil {
				if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit);
					literal != nil {
					found = objectUsedInExecutedNodeAfter(
						info,
						literal.Body,
						object,
						position,
					)
					for _, argument := range call.Args {
						found = found ||
							objectUsedInExecutedNodeAfter(
								info,
								argument,
								object,
								position,
							)
					}
					return false
				}
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			return true
		},
	)
	return found
}

type deferredRangeCall struct {
	capturedAtRegistration bool
	bodies []*ast.BlockStmt
}

type closureStateKey struct {
	object types.Object
	receiver types.Object
	field types.Object
	path string
}

type closureStateResolver struct {
	info *types.Info
	receiverAliases map[types.Object]types.Object
}

type closureStateReachability uint8

const (
	closureStateNever closureStateReachability = iota
	closureStateMaybe
	closureStateAlways
)

func suspiciousRangeParents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var parent ast.Node
	ast.Inspect(
		root,
		func(current ast.Node) bool {
			if current == nil {
				parent = parents[parent]
				return false
			}
			if parent != nil {
				parents[current] = parent
			}
			parent = current
			return true
		},
	)
	return parents
}

func closureStateReachabilityAt(
	node ast.Node,
	position token.Pos,
	parents map[ast.Node]ast.Node,
) closureStateReachability {
	if node == nil || !position.IsValid() {
		return closureStateMaybe
	}
	reachability := closureStateAlways
	child := node
	for parent := parents[child]; parent != nil; parent = parents[parent] {
		var branch ast.Node
		var control ast.Node
		switch parent := parent.(type) {
		case *ast.IfStmt:
			if child == parent.Body || child == parent.Else {
				branch, control = child, parent
			}
		case *ast.ForStmt:
			if child == parent.Body {
				branch, control = child, parent
			}
		case *ast.RangeStmt:
			if child == parent.Body {
				branch, control = child, parent
			}
		case *ast.CaseClause:
			branch = parent
			control = closureClauseControl(parents, parent)
		case *ast.CommClause:
			branch = parent
			control = closureClauseControl(parents, parent)
		}
		if branch != nil && !(branch.Pos() <= position && position < branch.End()) {
			if control != nil && control.Pos() <= position && position < control.End() {
				return closureStateNever
			}
			reachability = closureStateMaybe
		}
		child = parent
	}
	if reachability == closureStateAlways &&
		closureStatePathMayBypass(node, position, parents) {
		return closureStateMaybe
	}
	return reachability
}

func closureStatePathMayBypass(
	node ast.Node,
	position token.Pos,
	parents map[ast.Node]ast.Node,
) bool {
	if node == nil || !position.IsValid() || node.Pos() <= position {
		return false
	}
	for owner := closureStateAncestorStatementOwner(node, parents); owner != nil; {
		statements := closureStateStatements(owner)
		var earlier ast.Stmt
		var later ast.Stmt
		for _, statement := range statements {
			if statement == nil {
				continue
			}
			if statement.Pos() <= position && position <= statement.End() {
				earlier = statement
			}
			if statement.Pos() <= node.Pos() && node.Pos() < statement.End() {
				later = statement
			}
		}
		if earlier != nil && later != nil {
			for _, statement := range statements {
				if statement == nil || statement.End() <= position {
					continue
				}
				if statement.Pos() >= later.Pos() {
					break
				}
				if closureStateStatementMayBypass(statement, owner, node, parents) {
					return true
				}
			}
			return false
		}
		owner = closureStateAncestorStatementOwner(parents[owner], parents)
	}
	return false
}

func closureStateAncestorStatementOwner(node ast.Node, parents map[ast.Node]ast.Node) ast.Node {
	for current := node; current != nil; current = parents[current] {
		switch current.(type) {
		case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
			return current
		}
	}
	return nil
}

func closureStateStatements(owner ast.Node) []ast.Stmt {
	switch owner := owner.(type) {
	case *ast.BlockStmt:
		return owner.List
	case *ast.CaseClause:
		return owner.Body
	case *ast.CommClause:
		return owner.Body
	default:
		return nil
	}
}

func closureStateStatementMayBypass(
	statement ast.Stmt,
	owner ast.Node,
	candidate ast.Node,
	parents map[ast.Node]ast.Node,
) bool {
	if statement == nil || owner == nil || candidate == nil {
		return false
	}
	bypasses := false
	ast.Inspect(
		statement,
		func(current ast.Node) bool {
			if current == nil || bypasses {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			if branch, _ := current.(*ast.BranchStmt); branch != nil {
				bypasses = closureStateBranchMayBypass(
					branch,
					owner,
					candidate,
					parents,
				)
			}
			return !bypasses
		},
	)
	return bypasses
}

func closureStateBranchMayBypass(
	branch *ast.BranchStmt,
	owner ast.Node,
	candidate ast.Node,
	parents map[ast.Node]ast.Node,
) bool {
	if branch == nil || owner == nil || candidate == nil {
		return false
	}
	switch branch.Tok {
	case token.CONTINUE:
		return closureStateOwnerWithinControl(
			owner,
			closureStateContinueControl(branch, parents),
			parents,
		)
	case token.BREAK:
		return closureStateOwnerWithinControl(
			owner,
			closureStateBreakControl(branch, parents),
			parents,
		)
	case token.GOTO:
		target := closureStateGotoTarget(branch, owner, parents)
		return target != nil && candidate.End() <= target.Pos()
	default:
		return false
	}
}

func closureStateControlBody(control ast.Node) *ast.BlockStmt {
	switch control := control.(type) {
	case *ast.ForStmt:
		return control.Body
	case *ast.RangeStmt:
		return control.Body
	case *ast.SwitchStmt:
		return control.Body
	case *ast.TypeSwitchStmt:
		return control.Body
	case *ast.SelectStmt:
		return control.Body
	}
	return nil
}

func closureStateOwnerWithinControl(
	owner ast.Node,
	control ast.Node,
	parents map[ast.Node]ast.Node,
) bool {
	body := closureStateControlBody(control)
	if owner == nil || body == nil {
		return false
	}
	for current := owner; current != nil; current = parents[current] {
		if current == body {
			return true
		}
		switch current.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		}
	}
	return false
}

func closureStateContinueControl(branch *ast.BranchStmt, parents map[ast.Node]ast.Node) ast.Node {
	if branch == nil {
		return nil
	}
	if branch.Label != nil {
		return closureStateLabeledControl(branch, parents)
	}
	for current := parents[branch]; current != nil; current = parents[current] {
		switch current.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return current
		case *ast.FuncDecl, *ast.FuncLit:
			return nil
		}
	}
	return nil
}

func closureStateBreakControl(branch *ast.BranchStmt, parents map[ast.Node]ast.Node) ast.Node {
	if branch == nil {
		return nil
	}
	if branch.Label != nil {
		return closureStateLabeledControl(branch, parents)
	}
	for current := parents[branch]; current != nil; current = parents[current] {
		switch current.(type) {
		case *ast.ForStmt,
			*ast.RangeStmt,
			*ast.SwitchStmt,
			*ast.TypeSwitchStmt,
			*ast.SelectStmt:
			return current
		case *ast.FuncDecl, *ast.FuncLit:
			return nil
		}
	}
	return nil
}

func closureStateLabeledControl(branch *ast.BranchStmt, parents map[ast.Node]ast.Node) ast.Node {
	if branch == nil || branch.Label == nil {
		return nil
	}
	for current := parents[branch]; current != nil; current = parents[current] {
		if labeled, _ := current.(*ast.LabeledStmt);
			labeled != nil && labeled.Label.Name == branch.Label.Name {
			return labeled.Stmt
		}
		switch current.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return nil
		}
	}
	return nil
}

func closureStateGotoTarget(
	branch *ast.BranchStmt,
	owner ast.Node,
	parents map[ast.Node]ast.Node,
) *ast.LabeledStmt {
	if branch == nil || branch.Label == nil || owner == nil {
		return nil
	}
	root := owner
	for current := parents[root]; current != nil; current = parents[current] {
		root = current
	}
	var target *ast.LabeledStmt
	ast.Inspect(
		root,
		func(current ast.Node) bool {
			if current == nil || target != nil {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			labeled, _ := current.(*ast.LabeledStmt)
			if labeled != nil && labeled.Label.Name == branch.Label.Name {
				target = labeled
				return false
			}
			return true
		},
	)
	return target
}

func closureClauseControl(parents map[ast.Node]ast.Node, clause ast.Node) ast.Node {
	block, _ := parents[clause].(*ast.BlockStmt)
	if block == nil {
		return nil
	}
	return parents[block]
}

func newClosureStateResolver(info *types.Info, body *ast.BlockStmt) *closureStateResolver {
	resolver := &closureStateResolver{
		info: info,
		receiverAliases: make(map[types.Object]types.Object),
	}
	if info == nil || body == nil {
		return resolver
	}
	type aliasCandidate struct {
		target types.Object
		source types.Object
	}
	writes := make(map[types.Object]int)
	candidates := make([]aliasCandidate, 0)
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			var left []ast.Expr
			var right []ast.Expr
			directDefinition := false
			switch current := current.(type) {
			case *ast.AssignStmt:
				left, right = current.Lhs, current.Rhs
				directDefinition = current.Tok == token.DEFINE
			case *ast.ValueSpec:
				left = make([]ast.Expr, len(current.Names))
				for index, name := range current.Names {
					left[index] = name
				}
				right = current.Values
				directDefinition = true
			default:
				return true
			}
			for index, target := range left {
				object := directObject(info, target)
				if object == nil {
					continue
				}
				writes[object]++
				if !directDefinition ||
					index >= len(right) ||
					!pointerLikeType(object.Type()) {
					continue
				}
				source := directObject(info, right[index])
				if source != nil && types.Identical(object.Type(), source.Type()) {
					candidates = append(
						candidates,
						aliasCandidate{target: object, source: source},
					)
				}
			}
			return true
		},
	)
	for _, candidate := range candidates {
		if writes[candidate.target] == 1 {
			resolver.receiverAliases[candidate.target] = candidate.source
		}
	}
	return resolver
}

func pointerLikeType(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	switch types.Unalias(type_).Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Map, *types.Slice, *types.Chan:
		return true
	default:
		return false
	}
}

func pointerMethodValueCapturesObject(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
) bool {
	if info == nil || expression == nil || object == nil {
		return false
	}
	selector, _ := ast.Unparen(expression).(*ast.SelectorExpr)
	if selector == nil || directObject(info, selector.X) != object {
		return false
	}
	selection := info.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	_, pointer := types.Unalias(signature.Recv().Type()).(*types.Pointer)
	return pointer
}

func hasClosureStateKey(set map[closureStateKey]struct{}, key closureStateKey) bool {
	_, found := set[key]
	return found
}

func capturedRangeValueUsedAfter(
	info *types.Info,
	body *ast.BlockStmt,
	rangeObject types.Object,
	files *PackageSyntax,
	position token.Pos,
) bool {
	capturedFunctions := make(map[closureStateKey]struct{})
	directCapturedFunctions := make(map[closureStateKey]struct{})
	wrapperBodies := make(map[closureStateKey][]*ast.BlockStmt)
	resolver := newClosureStateResolver(info, body)
	parents := suspiciousRangeParents(body)
	deferredCalls := make([]deferredRangeCall, 0)
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil || current.Pos() >= position {
				return current == nil
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			var left []ast.Expr
			var right []ast.Expr
			unconditional := false
			switch current := current.(type) {
			case *ast.AssignStmt:
				left = current.Lhs
				right = current.Rhs
				reachability := closureStateReachabilityAt(
					current,
					position,
					parents,
				)
				if reachability == closureStateNever {
					return true
				}
				unconditional = reachability == closureStateAlways
			case *ast.ValueSpec:
				left = make([]ast.Expr, len(current.Names))
				for index, name := range current.Names {
					left[index] = name
				}
				right = current.Values
				reachability := closureStateReachabilityAt(
					current,
					position,
					parents,
				)
				if reachability == closureStateNever {
					return true
				}
				unconditional = reachability == closureStateAlways
			case *ast.DeferStmt:
				deferredCalls = append(
					deferredCalls,
					snapshotDeferredRangeCall(
						info,
						resolver,
						current.Call,
						rangeObject,
						capturedFunctions,
						wrapperBodies,
					),
				)
				return false
			case *ast.GoStmt:
				asynchronous := snapshotDeferredRangeCall(
					info,
					resolver,
					current.Call,
					rangeObject,
					capturedFunctions,
					wrapperBodies,
				)
				if asynchronous.capturedAtRegistration ||
					len(asynchronous.bodies) != 0 {
					deferredCalls = append(deferredCalls, asynchronous)
				}
				return false
			default:
				return true
			}
			if len(left) != len(right) {
				updateTupleClosureState(
					info,
					resolver,
					left,
					right,
					rangeObject,
					capturedFunctions,
					directCapturedFunctions,
					wrapperBodies,
					unconditional,
					files,
				)
				return true
			}
			type wrapperUpdate struct {
				object closureStateKey
				bodies []*ast.BlockStmt
				directCapture bool
			}
			updates := make([]wrapperUpdate, 0, len(right))
			for index, expression := range right {
				functionObject, found := resolver.key(left[index])
				if !found {
					continue
				}
				var bodies []*ast.BlockStmt
				if literal, _ := ast.Unparen(expression).(*ast.FuncLit);
					literal != nil {
					bodies = append(bodies, literal.Body)
				} else {
					bodies = append(
						bodies,
						wrapperBodies[resolver.mustKey(expression)]...,
					)
				}
				updates = append(
					updates,
					wrapperUpdate{
						object: functionObject,
						bodies: bodies,
						directCapture: pointerMethodValueCapturesObject(
							info,
							expression,
							rangeObject,
						) ||
							hasClosureStateKey(
								directCapturedFunctions,
								resolver.mustKey(expression),
							),
					},
				)
			}
			for _, update := range updates {
				if unconditional {
					delete(wrapperBodies, update.object)
					delete(directCapturedFunctions, update.object)
				}
				wrapperBodies[update.object] = appendUniqueWrapperBodies(
					wrapperBodies[update.object],
					update.bodies,
				)
				if update.directCapture {
					directCapturedFunctions[update.object] = struct{}{}
				}
			}
			recomputeCapturedFunctions(
				info,
				resolver,
				wrapperBodies,
				rangeObject,
				capturedFunctions,
				directCapturedFunctions,
			)
			return true
		},
	)
	for _, call := range deferredCalls {
		if call.capturedAtRegistration {
			return true
		}
	}
	if capturedFunctionInvokedAfter(
		info,
		resolver,
		body,
		rangeObject,
		position,
		capturedFunctions,
		directCapturedFunctions,
		wrapperBodies,
		parents,
		files,
	) {
		return true
	}
	if capturedFunctionInvokedOnLoopBackedge(
		info,
		resolver,
		body,
		position,
		capturedFunctions,
	) {
		return true
	}
	for _, call := range deferredCalls {
		for _, wrapperBody := range call.bodies {
			if executedClosureBodyUsesObject(
				info,
				resolver,
				wrapperBody,
				rangeObject,
				make(map[*ast.BlockStmt]struct{}),
			) ||
				expressionInvokesCapturedFunction(
					info,
					resolver,
					wrapperBody,
					capturedFunctions,
				) {
				return true
			}
		}
	}
	return false
}

func snapshotDeferredRangeCall(
	info *types.Info,
	resolver *closureStateResolver,
	call *ast.CallExpr,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
	wrapperBodies map[closureStateKey][]*ast.BlockStmt,
) deferredRangeCall {
	state := deferredRangeCall{}
	if info == nil || call == nil || rangeObject == nil {
		return state
	}
	if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit); literal != nil {
		state.bodies = append(state.bodies, literal.Body)
	} else {
		functionObject := resolver.mustKey(call.Fun)
		state.bodies = append(state.bodies, wrapperBodies[functionObject]...)
		if len(state.bodies) == 0 {
			_, state.capturedAtRegistration = capturedFunctions[functionObject]
		}
	}
	for _, argument := range call.Args {
		if closureExpressionCapturesRange(
			info,
			resolver,
			argument,
			rangeObject,
			capturedFunctions,
		) {
			state.capturedAtRegistration = true
			break
		}
	}
	return state
}

func recomputeCapturedFunctions(
	info *types.Info,
	resolver *closureStateResolver,
	wrapperBodies map[closureStateKey][]*ast.BlockStmt,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
	directCapturedFunctions map[closureStateKey]struct{},
) {
	clear(capturedFunctions)
	for functionObject := range directCapturedFunctions {
		capturedFunctions[functionObject] = struct{}{}
	}
	for functionObject, bodies := range wrapperBodies {
		for _, body := range bodies {
			if executedClosureBodyUsesObject(
				info,
				resolver,
				body,
				rangeObject,
				make(map[*ast.BlockStmt]struct{}),
			) {
				capturedFunctions[functionObject] = struct{}{}
				break
			}
		}
	}
	propagateCapturedFunctionWrappers(info, resolver, wrapperBodies, capturedFunctions)
}

func propagateCapturedFunctionWrappers(
	info *types.Info,
	resolver *closureStateResolver,
	wrapperBodies map[closureStateKey][]*ast.BlockStmt,
	capturedFunctions map[closureStateKey]struct{},
) {
	for changed := true; changed; {
		changed = false
		for functionObject, bodies := range wrapperBodies {
			if _, captured := capturedFunctions[functionObject]; captured {
				continue
			}
			for _, wrapperBody := range bodies {
				if !expressionInvokesCapturedFunction(
					info,
					resolver,
					wrapperBody,
					capturedFunctions,
				) {
					continue
				}
				capturedFunctions[functionObject] = struct{}{}
				changed = true
				break
			}
		}
	}
}

func capturedFunctionInvokedAfter(
	info *types.Info,
	resolver *closureStateResolver,
	body *ast.BlockStmt,
	rangeObject types.Object,
	position token.Pos,
	capturedFunctions map[closureStateKey]struct{},
	directCapturedFunctions map[closureStateKey]struct{},
	wrapperBodies map[closureStateKey][]*ast.BlockStmt,
	parents map[ast.Node]ast.Node,
	files *PackageSyntax,
) bool {
	invoked := false
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil || invoked {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			var left []ast.Expr
			var right []ast.Expr
			unconditional := false
			switch current := current.(type) {
			case *ast.AssignStmt:
				if current.End() <= position {
					return true
				}
				left = current.Lhs
				right = current.Rhs
				reachability := closureStateReachabilityAt(
					current,
					position,
					parents,
				)
				if reachability == closureStateNever {
					return true
				}
				unconditional = reachability == closureStateAlways
			case *ast.ValueSpec:
				if current.End() <= position {
					return true
				}
				left = make([]ast.Expr, len(current.Names))
				for index, name := range current.Names {
					left[index] = name
				}
				right = current.Values
				reachability := closureStateReachabilityAt(
					current,
					position,
					parents,
				)
				if reachability == closureStateNever {
					return true
				}
				unconditional = reachability == closureStateAlways
			case *ast.ReturnStmt:
				if current.Pos() <= position {
					return true
				}
				for _, result := range current.Results {
					invoked = invoked ||
						capturedClosureValue(
							info,
							resolver,
							result,
							rangeObject,
							capturedFunctions,
						)
				}
				return false
			case *ast.SendStmt:
				if current.Pos() <= position {
					return true
				}
				invoked = capturedClosureValue(
					info,
					resolver,
					current.Value,
					rangeObject,
					capturedFunctions,
				)
				return false
			case *ast.CallExpr:
				if current.Pos() <= position || current.Pos() >= body.End() {
					return true
				}
				if literal, _ := ast.Unparen(current.Fun).(*ast.FuncLit);
					literal != nil {
					invoked = expressionInvokesCapturedFunction(
						info,
						resolver,
						literal.Body,
						capturedFunctions,
					)
					for _, argument := range current.Args {
						invoked = invoked ||
							capturedClosureValue(
								info,
								resolver,
								argument,
								rangeObject,
								capturedFunctions,
							)
					}
					return false
				}
				_, invoked = capturedFunctions[resolver.mustKey(current.Fun)]
				for _, argument := range current.Args {
					invoked = invoked ||
						capturedClosureValue(
							info,
							resolver,
							argument,
							rangeObject,
							capturedFunctions,
						)
				}
				return !invoked
			default:
				return true
			}
			for index, expression := range right {
				if callReceivesCapturedClosure(
					info,
					resolver,
					expression,
					rangeObject,
					capturedFunctions,
				) {
					invoked = true
					return false
				}
				if expressionInvokesCapturedFunction(
					info,
					resolver,
					expression,
					capturedFunctions,
				) {
					invoked = true
					return false
				}
				if index < len(left) &&
					closureAssignmentEscapes(
						info,
						resolver,
						left[index],
						expression,
						rangeObject,
						capturedFunctions,
					) {
					invoked = true
					return false
				}
			}
			updateCapturedFunctions(
				info,
				resolver,
				left,
				right,
				rangeObject,
				capturedFunctions,
				directCapturedFunctions,
				wrapperBodies,
				unconditional,
				files,
			)
			return false
		},
	)
	return invoked
}

func callReceivesCapturedClosure(
	info *types.Info,
	resolver *closureStateResolver,
	expression ast.Expr,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
) bool {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil {
		return false
	}
	for _, argument := range call.Args {
		if capturedClosureValue(info, resolver, argument, rangeObject, capturedFunctions) {
			return true
		}
	}
	return false
}

func closureAssignmentEscapes(
	info *types.Info,
	resolver *closureStateResolver,
	target ast.Expr,
	value ast.Expr,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
) bool {
	if !capturedClosureValue(info, resolver, value, rangeObject, capturedFunctions) {
		return false
	}
	if blankIdentifier(target) {
		return false
	}
	object := directObject(info, target)
	if object == nil || packageScopeObject(object) {
		return true
	}
	_, localFunction := types.Unalias(object.Type()).Underlying().(*types.Signature)
	return !localFunction
}

func capturedClosureValue(
	info *types.Info,
	resolver *closureStateResolver,
	expression ast.Expr,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
) bool {
	if info == nil || expression == nil {
		return false
	}
	if closureExpressionCapturesRange(
		info,
		resolver,
		expression,
		rangeObject,
		capturedFunctions,
	) {
		return true
	}
	found := false
	ast.Inspect(
		expression,
		func(current ast.Node) bool {
			if current == nil || found {
				return false
			}
			identifier, _ := current.(*ast.Ident)
			if identifier == nil {
				return true
			}
			key, keyed := resolver.key(identifier)
			if keyed {
				_, found = capturedFunctions[key]
			}
			return !found
		},
	)
	return found
}

func updateCapturedFunctions(
	info *types.Info,
	resolver *closureStateResolver,
	left []ast.Expr,
	right []ast.Expr,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
	directCapturedFunctions map[closureStateKey]struct{},
	wrapperBodies map[closureStateKey][]*ast.BlockStmt,
	unconditional bool,
	files *PackageSyntax,
) {
	if len(left) != len(right) {
		updateTupleClosureState(
			info,
			resolver,
			left,
			right,
			rangeObject,
			capturedFunctions,
			directCapturedFunctions,
			wrapperBodies,
			unconditional,
			files,
		)
		return
	}
	type wrapperUpdate struct {
		object closureStateKey
		bodies []*ast.BlockStmt
		directCapture bool
	}
	updates := make([]wrapperUpdate, 0, len(right))
	for index, expression := range right {
		functionObject, found := resolver.key(left[index])
		if !found {
			continue
		}
		var bodies []*ast.BlockStmt
		if literal, _ := ast.Unparen(expression).(*ast.FuncLit); literal != nil {
			bodies = append(bodies, literal.Body)
		} else {
			bodies = append(bodies, wrapperBodies[resolver.mustKey(expression)]...)
		}
		updates = append(
			updates,
			wrapperUpdate{
				object: functionObject,
				bodies: bodies,
				directCapture: pointerMethodValueCapturesObject(
					info,
					expression,
					rangeObject,
				) ||
					hasClosureStateKey(
						directCapturedFunctions,
						resolver.mustKey(expression),
					),
			},
		)
	}
	for _, update := range updates {
		if unconditional {
			delete(wrapperBodies, update.object)
			delete(directCapturedFunctions, update.object)
		}
		wrapperBodies[update.object] = appendUniqueWrapperBodies(
			wrapperBodies[update.object],
			update.bodies,
		)
		if update.directCapture {
			directCapturedFunctions[update.object] = struct{}{}
		}
	}
	recomputeCapturedFunctions(
		info,
		resolver,
		wrapperBodies,
		rangeObject,
		capturedFunctions,
		directCapturedFunctions,
	)
}

func updateTupleClosureState(
	info *types.Info,
	resolver *closureStateResolver,
	left []ast.Expr,
	right []ast.Expr,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
	directCapturedFunctions map[closureStateKey]struct{},
	wrapperBodies map[closureStateKey][]*ast.BlockStmt,
	unconditional bool,
	files *PackageSyntax,
) {
	if info == nil || files == nil || len(left) == 0 || len(right) != 1 {
		return
	}
	call, _ := ast.Unparen(right[0]).(*ast.CallExpr)
	if call == nil {
		return
	}
	type tupleUpdate struct {
		key closureStateKey
		bodies []*ast.BlockStmt
		directCapture bool
		exact bool
	}
	updates := make([]tupleUpdate, 0, len(left))
	for resultIndex, expression := range left {
		key, found := resolver.key(expression)
		if !found {
			continue
		}
		if _, function := types.Unalias(info.TypeOf(expression)).Underlying().(*types.Signature);
			!function {
			continue
		}
		argumentIndex, exact := exactTupleReturnedArgument(info, files, call, resultIndex)
		if !exact || argumentIndex < 0 || argumentIndex >= len(call.Args) {
			updates = append(updates, tupleUpdate{key: key})
			continue
		}
		argument := call.Args[argumentIndex]
		directCapture := pointerMethodValueCapturesObject(info, argument, rangeObject) ||
			hasClosureStateKey(directCapturedFunctions, resolver.mustKey(argument))
		bodies := make([]*ast.BlockStmt, 0)
		if literal, _ := ast.Unparen(argument).(*ast.FuncLit); literal != nil {
			bodies = append(bodies, literal.Body)
		} else if source, sourceFound := resolver.key(argument); sourceFound {
			bodies = appendUniqueWrapperBodies(bodies, wrapperBodies[source])
		}
		updates = append(
			updates,
			tupleUpdate{
				key: key,
				bodies: bodies,
				directCapture: directCapture,
				exact: true,
			},
		)
	}
	for _, update := range updates {
		if unconditional {
			delete(wrapperBodies, update.key)
			delete(directCapturedFunctions, update.key)
		}
		if !update.exact {
			continue
		}
		wrapperBodies[update.key] = appendUniqueWrapperBodies(
			wrapperBodies[update.key],
			update.bodies,
		)
		if update.directCapture {
			directCapturedFunctions[update.key] = struct{}{}
		}
	}
	recomputeCapturedFunctions(
		info,
		resolver,
		wrapperBodies,
		rangeObject,
		capturedFunctions,
		directCapturedFunctions,
	)
}

func exactTupleReturnedArgument(
	info *types.Info,
	files *PackageSyntax,
	call *ast.CallExpr,
	resultIndex int,
) (int, bool) {
	if info == nil || files == nil || call == nil || resultIndex < 0 {
		return 0, false
	}
	callee := typeutil.StaticCallee(info, call)
	if callee == nil {
		return 0, false
	}
	declaration := functionDeclaration(info, files, callee)
	if declaration == nil || declaration.Body == nil || declaration.Type == nil {
		return 0, false
	}
	parameters := make(map[types.Object]int)
	argumentIndex := 0
	if declaration.Type.Params != nil {
		for _, field := range declaration.Type.Params.List {
			for _, name := range field.Names {
				parameters[info.ObjectOf(name)] = argumentIndex
				argumentIndex++
			}
			if len(field.Names) == 0 {
				argumentIndex++
			}
		}
	}
	returnedArgument := -1
	valid := true
	seenReturn := false
	modifiedParameters := make(map[types.Object]struct{})
	immediatelyInvoked := immediatelyInvokedFunctionLiterals(declaration.Body)
	ast.Inspect(
		declaration.Body,
		func(current ast.Node) bool {
			if current == nil || !valid {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				_, invoked := immediatelyInvoked[literal]
				return invoked
			}
			switch current := current.(type) {
			case *ast.AssignStmt:
				for _, target := range current.Lhs {
					if object := directObject(info, target); object != nil {
						if _, parameter := parameters[object]; parameter {
							modifiedParameters[object] = struct{}{}
						}
					}
				}
			case *ast.IncDecStmt:
				if object := directObject(info, current.X); object != nil {
					if _, parameter := parameters[object]; parameter {
						modifiedParameters[object] = struct{}{}
					}
				}
			case *ast.RangeStmt:
				for _, target := range []ast.Expr{current.Key, current.Value} {
					if object := directObject(info, target); object != nil {
						if _, parameter := parameters[object]; parameter {
							modifiedParameters[object] = struct{}{}
						}
					}
				}
			case *ast.UnaryExpr:
				if current.Op == token.AND {
					if object := directObject(info, current.X); object != nil {
						if _, parameter := parameters[object]; parameter {
							modifiedParameters[object] = struct{}{}
						}
					}
				}
			}
			returned, _ := current.(*ast.ReturnStmt)
			if returned == nil {
				return true
			}
			seenReturn = true
			if resultIndex >= len(returned.Results) {
				valid = false
				return false
			}
			identifier, _ := ast.Unparen(returned.Results[resultIndex]).(*ast.Ident)
			object := info.ObjectOf(identifier)
			candidate, found := parameters[object]
			_, modified := modifiedParameters[object]
			if identifier == nil ||
				!found ||
				modified ||
				(returnedArgument >= 0 && returnedArgument != candidate) {
				valid = false
				return false
			}
			returnedArgument = candidate
			return true
		},
	)
	if !valid || !seenReturn || returnedArgument < 0 {
		return returnedArgument, false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selection := info.Selections[selector];
		selection != nil && selection.Kind() == types.MethodExpr {
		returnedArgument++
	}
	return returnedArgument, true
}

func immediatelyInvokedFunctionLiterals(node ast.Node) map[*ast.FuncLit]struct{} {
	invoked := make(map[*ast.FuncLit]struct{})
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				_, execute := invoked[literal]
				return execute
			}
			call, _ := current.(*ast.CallExpr)
			if call == nil {
				return true
			}
			literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit)
			if literal != nil {
				invoked[literal] = struct{}{}
			}
			return true
		},
	)
	return invoked
}

func functionDeclaration(
	info *types.Info,
	files *PackageSyntax,
	function *types.Func,
) *ast.FuncDecl {
	if info == nil || files == nil || function == nil {
		return nil
	}
	value := files.memoized(
		"suspicious-range/function-declarations-v1",
		func() any {
			declarations := make(map[*types.Func]*ast.FuncDecl)
			for index := 0; index < files.Len(); index++ {
				file := files.At(index)
				if file == nil {
					continue
				}
				for _, declaration := range file.Decls {
					candidate, _ := declaration.(*ast.FuncDecl)
					if candidate == nil {
						continue
					}
					if object, _ := info.ObjectOf(candidate.Name).(*types.Func);
						object != nil {
						declarations[object] = candidate
					}
				}
			}
			return declarations
		},
	)
	declarations, _ := value.(map[*types.Func]*ast.FuncDecl)
	return declarations[function]
}

func appendUniqueWrapperBodies(
	existing []*ast.BlockStmt,
	additional []*ast.BlockStmt,
) []*ast.BlockStmt {
	seen := make(map[*ast.BlockStmt]struct{}, len(existing) + len(additional))
	for _, body := range existing {
		if body != nil {
			seen[body] = struct{}{}
		}
	}
	for _, body := range additional {
		if body == nil {
			continue
		}
		if _, duplicate := seen[body]; duplicate {
			continue
		}
		existing = append(existing, body)
		seen[body] = struct{}{}
	}
	return existing
}

func closureExpressionCapturesRange(
	info *types.Info,
	resolver *closureStateResolver,
	expression ast.Expr,
	rangeObject types.Object,
	capturedFunctions map[closureStateKey]struct{},
) bool {
	if literal, _ := ast.Unparen(expression).(*ast.FuncLit); literal != nil {
		return executedClosureBodyUsesObject(
			info,
			resolver,
			literal.Body,
			rangeObject,
			make(map[*ast.BlockStmt]struct{}),
		) ||
			expressionInvokesCapturedFunction(
				info,
				resolver,
				literal.Body,
				capturedFunctions,
			)
	}
	_, captured := capturedFunctions[resolver.mustKey(expression)]
	return captured
}

func executedClosureBodyUsesObject(
	info *types.Info,
	resolver *closureStateResolver,
	body *ast.BlockStmt,
	object types.Object,
	active map[*ast.BlockStmt]struct{},
) bool {
	if info == nil || body == nil || object == nil {
		return false
	}
	if _, recursive := active[body]; recursive {
		return false
	}
	active[body] = struct{}{}
	defer delete(active, body)
	if objectUsedInExecutedNodeAfter(info, body, object, token.NoPos) {
		return true
	}
	localBodies := make(map[closureStateKey][]*ast.BlockStmt)
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			var left []ast.Expr
			var right []ast.Expr
			switch current := current.(type) {
			case *ast.AssignStmt:
				left, right = current.Lhs, current.Rhs
			case *ast.ValueSpec:
				left = make([]ast.Expr, len(current.Names))
				for index, name := range current.Names {
					left[index] = name
				}
				right = current.Values
			default:
				return true
			}
			if len(left) != len(right) {
				return true
			}
			for index, expression := range right {
				literal, _ := ast.Unparen(expression).(*ast.FuncLit)
				key, found := resolver.key(left[index])
				if literal != nil && found {
					localBodies[key] = appendUniqueWrapperBodies(
						localBodies[key],
						[]*ast.BlockStmt{literal.Body},
					)
				}
			}
			return true
		},
	)
	found := false
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil || found {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			call, _ := current.(*ast.CallExpr)
			if call == nil {
				return true
			}
			if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit); literal != nil {
				found = executedClosureBodyUsesObject(
					info,
					resolver,
					literal.Body,
					object,
					active,
				)
				return false
			}
			key, keyed := resolver.key(call.Fun)
			if !keyed {
				return true
			}
			for _, nested := range localBodies[key] {
				if executedClosureBodyUsesObject(
					info,
					resolver,
					nested,
					object,
					active,
				) {
					found = true
					break
				}
			}
			return !found
		},
	)
	return found
}

func expressionInvokesCapturedFunction(
	info *types.Info,
	resolver *closureStateResolver,
	node ast.Node,
	capturedFunctions map[closureStateKey]struct{},
) bool {
	if info == nil || node == nil || len(capturedFunctions) == 0 {
		return false
	}
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if found {
				return false
			}
			call, _ := current.(*ast.CallExpr)
			if call == nil {
				if _, literal := current.(*ast.FuncLit); literal {
					return false
				}
				return true
			}
			if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit); literal != nil {
				found = expressionInvokesCapturedFunction(
					info,
					resolver,
					literal.Body,
					capturedFunctions,
				)
				for _, argument := range call.Args {
					found = found ||
						expressionInvokesCapturedFunction(
							info,
							resolver,
							argument,
							capturedFunctions,
						)
				}
				return false
			}
			_, found = capturedFunctions[resolver.mustKey(call.Fun)]
			return !found
		},
	)
	return found
}

func (r *closureStateResolver) key(expression ast.Expr) (closureStateKey, bool) {
	if r == nil || r.info == nil || expression == nil {
		return closureStateKey{}, false
	}
	if object := directObject(r.info, expression); object != nil {
		return closureStateKey{object: object}, true
	}
	selector, _ := ast.Unparen(expression).(*ast.SelectorExpr)
	if selector == nil {
		return closureStateKey{}, false
	}
	selection := r.info.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return closureStateKey{}, false
	}
	receiver, path, found := r.receiverPath(selector.X)
	if !found {
		return closureStateKey{}, false
	}
	field := selection.Obj()
	path += fmt.Sprintf("/%d", field.Pos())
	return closureStateKey{receiver: receiver, field: field, path: path}, true
}

func (r *closureStateResolver) receiverPath(expression ast.Expr) (types.Object, string, bool) {
	expression = ast.Unparen(expression)
	if unary, _ := expression.(*ast.UnaryExpr);
		unary != nil && (unary.Op == token.AND || unary.Op == token.MUL) {
		return r.receiverPath(unary.X)
	}
	if object := directObject(r.info, expression); object != nil {
		seen := make(map[types.Object]struct{})
		for {
			if _, active := seen[object]; active {
				return nil, "", false
			}
			seen[object] = struct{}{}
			source := r.receiverAliases[object]
			if source == nil {
				return object, "", true
			}
			object = source
		}
	}
	selector, _ := expression.(*ast.SelectorExpr)
	if selector == nil {
		return nil, "", false
	}
	selection := r.info.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return nil, "", false
	}
	receiver, path, found := r.receiverPath(selector.X)
	if !found {
		return nil, "", false
	}
	return receiver, path + fmt.Sprintf("/%d", selection.Obj().Pos()), true
}

func (r *closureStateResolver) mustKey(expression ast.Expr) closureStateKey {
	key, _ := r.key(expression)
	return key
}

func capturedFunctionInvokedOnLoopBackedge(
	info *types.Info,
	resolver *closureStateResolver,
	body *ast.BlockStmt,
	position token.Pos,
	capturedFunctions map[closureStateKey]struct{},
) bool {
	if info == nil || body == nil || len(capturedFunctions) == 0 {
		return false
	}
	found := false
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil || found {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			var loopBody *ast.BlockStmt
			var repeated []ast.Node
			switch loop := current.(type) {
			case *ast.ForStmt:
				loopBody = loop.Body
				repeated = []ast.Node{loop.Body, loop.Post, loop.Cond}
			case *ast.RangeStmt:
				loopBody = loop.Body
				repeated = []ast.Node{loop.Body}
			default:
				return true
			}
			if loopBody == nil || loopBody.End() <= position {
				return true
			}
			if !loopBackedgeStructurallyReachable(loopBody, position) {
				return true
			}
			for _, candidate := range repeated {
				found = found ||
					expressionInvokesCapturedFunction(
						info,
						resolver,
						candidate,
						capturedFunctions,
					)
			}
			return !found
		},
	)
	return found
}

func copiedAggregateType(type_ types.Type) bool {
	switch types.Unalias(type_).Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	default:
		return false
	}
}

func mutationStaysOnRangeCopy(info *types.Info, expression ast.Expr, object types.Object) bool {
	expression = ast.Unparen(expression)
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		return copyMutationPath(info, expression.X, object)
	case *ast.IndexExpr:
		return copyMutationPath(info, expression.X, object)
	default:
		return false
	}
}

func copyMutationPath(info *types.Info, expression ast.Expr, object types.Object) bool {
	expression = ast.Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		return info.ObjectOf(identifier) == object
	}
	switch types.Unalias(info.TypeOf(expression)).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Chan:
		return false
	}
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		return copyMutationPath(info, expression.X, object)
	case *ast.IndexExpr:
		return copyMutationPath(info, expression.X, object)
	default:
		return false
	}
}
