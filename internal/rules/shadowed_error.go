package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

type shadowedErrorRule struct{}

// NewShadowedErrorRule constructs the stale error-flow rule for product
// registry composition.
func NewShadowedErrorRule() Rule {
	return shadowedErrorRule{}
}

func (shadowedErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "shadowed-error",
		Summary: "detects shadowed errors that leave stale error state",
		Documentation: "An inner error declaration can hide an outer error whose value is observed after the inner scope ends. Glippy reports two bounded stale-flow shapes: breaking out of a loop after checking the inner error and then observing the unchanged outer error, and assigning a shadowed named result from a deferred closure so the returned result is never updated. Ordinary locally handled error declarations are excluded.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeFile},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Only identical error-implementing types are compared; differently typed errors and assignments without declarations are excluded.",
			"Loop findings require an inner err != nil check that can break the containing loop and an explicit outer-error return or nil check after that loop.",
			"Deferred findings require a named error result and a deferred function literal that assigns the shadowing error object.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Carry the loop error into the returned variable",
				Incorrect: "var err error\nfor { _, err := reader.Read(buf); if err != nil { break } }\nreturn err",
				Correct: "var err error\nfor { _, err = reader.Read(buf); if err != nil { break } }\nreturn err",
			},
		},
	}
}

type shadowedErrorKind uint8

const (
	shadowedErrorNone shadowedErrorKind = iota
	shadowedErrorLoop
	shadowedErrorDeferredResult
)

type shadowedErrorCandidate struct {
	identifier *ast.Ident
	inner types.Object
	outer types.Object
	function ast.Node
}

func (shadowedErrorRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	file, ok := node.(*ast.File)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, nil
	}
	parents := shadowedErrorParents(file)
	findings := make([]Finding, 0)
	for _, candidate := range shadowedErrorCandidates(file, ctx.Info(), parents) {
		kind := shadowedErrorNone
		if shadowedErrorLeavesLoopStale(candidate, ctx.Info(), parents) {
			kind = shadowedErrorLoop
		} else if shadowedErrorDeferredResultStale(candidate, ctx.Info(), parents) {
			kind = shadowedErrorDeferredResult
		}
		if kind == shadowedErrorNone {
			continue
		}
		range_, err := ctx.Range(candidate.identifier)
		if err != nil {
			return nil, err
		}
		message := fmt.Sprintf(
			"this %s shadows an outer error whose stale value is observed later",
			candidate.identifier.Name,
		)
		help := "assign the operation error to the outer variable or use a distinct local error with explicit propagation"
		if kind == shadowedErrorDeferredResult {
			message = fmt.Sprintf(
				"this %s shadows the named error result updated by the deferred closure",
				candidate.identifier.Name,
			)
			help = "assign the acquisition error to the named result or update the named result explicitly in the deferred closure"
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "shadowed-error",
				Message: message,
				Range: range_,
				Help: help,
			},
		)
	}
	return findings, nil
}

func shadowedErrorCandidates(
	file *ast.File,
	info *types.Info,
	parents map[ast.Node]ast.Node,
) []shadowedErrorCandidate {
	errorObject := types.Universe.Lookup("error")
	if errorObject == nil {
		return nil
	}
	errorType := errorObject.Type()
	candidates := make([]shadowedErrorCandidate, 0)
	ast.Inspect(
		file,
		func(node ast.Node) bool {
			var identifiers []*ast.Ident
			switch node := node.(type) {
			case *ast.AssignStmt:
				if node.Tok != token.DEFINE {
					return true
				}
				for _, expression := range node.Lhs {
					if identifier, ok := expression.(*ast.Ident); ok {
						identifiers = append(identifiers, identifier)
					}
				}
			case *ast.ValueSpec:
				if len(node.Values) == 0 {
					return true
				}
				identifiers = append(identifiers, node.Names...)
			default:
				return true
			}
			for _, identifier := range identifiers {
				inner := info.Defs[identifier]
				if inner == nil ||
					inner.Name() == "_" ||
					inner.Parent() == nil ||
					inner.Parent().Parent() == nil ||
					!types.AssignableTo(inner.Type(), errorType) {
					continue
				}
				_, outer := inner.Parent().Parent().LookupParent(
					inner.Name(),
					inner.Pos(),
				)
				if outer == nil ||
					!types.Identical(inner.Type(), outer.Type()) ||
					!types.AssignableTo(outer.Type(), errorType) {
					continue
				}
				function := shadowedErrorParentFunction(parents, identifier)
				if function == nil ||
					function != shadowedErrorFunctionAt(file, outer.Pos()) {
					continue
				}
				candidates = append(
					candidates,
					shadowedErrorCandidate{
						identifier: identifier,
						inner: inner,
						outer: outer,
						function: function,
					},
				)
			}
			return true
		},
	)
	return candidates
}

func shadowedErrorLeavesLoopStale(
	candidate shadowedErrorCandidate,
	info *types.Info,
	parents map[ast.Node]ast.Node,
) bool {
	loop := shadowedErrorParentLoop(parents, candidate.identifier)
	if loop == nil ||
		!shadowedErrorCheckedBeforeLoopExit(loop, candidate.inner, info, parents) {
		return false
	}
	return shadowedErrorOuterObservedAfterLoop(candidate.function, loop, candidate.outer, info)
}

func shadowedErrorParentLoop(parents map[ast.Node]ast.Node, node ast.Node) ast.Node {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch parent.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return nil
		case *ast.ForStmt, *ast.RangeStmt:
			return parent
		}
	}
	return nil
}

func shadowedErrorCheckedBeforeLoopExit(
	loop ast.Node,
	inner types.Object,
	info *types.Info,
	parents map[ast.Node]ast.Node,
) bool {
	found := false
	ast.Inspect(
		loop,
		func(node ast.Node) bool {
			if node == nil || found {
				return false
			}
			if node != loop {
				switch node.(type) {
				case *ast.FuncDecl, *ast.FuncLit:
					return false
				}
			}
			ifStatement, ok := node.(*ast.IfStmt)
			if !ok || !shadowedErrorComparedWithNil(ifStatement.Cond, inner, info) {
				return true
			}
			ast.Inspect(
				ifStatement.Body,
				func(child ast.Node) bool {
					if child == nil || found {
						return false
					}
					branch, ok := child.(*ast.BranchStmt)
					if ok &&
						branch.Tok == token.BREAK &&
						branch.Label == nil &&
						shadowedErrorBreakTarget(parents, branch) == loop {
						found = true
						return false
					}
					switch child.(type) {
					case *ast.FuncDecl, *ast.FuncLit:
						return false
					}
					return true
				},
			)
			return !found
		},
	)
	return found
}

func shadowedErrorComparedWithNil(expression ast.Expr, object types.Object, info *types.Info) bool {
	binary, ok := ast.Unparen(expression).(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if binary.Op == token.LAND || binary.Op == token.LOR {
		return shadowedErrorComparedWithNil(binary.X, object, info) ||
			shadowedErrorComparedWithNil(binary.Y, object, info)
	}
	if binary.Op != token.NEQ {
		return false
	}
	return shadowedErrorObjectIdentifier(binary.X, object, info) &&
		shadowedErrorNilIdentifier(binary.Y) ||
		shadowedErrorObjectIdentifier(binary.Y, object, info) &&
			shadowedErrorNilIdentifier(binary.X)
}

func shadowedErrorObjectIdentifier(
	expression ast.Expr,
	object types.Object,
	info *types.Info,
) bool {
	identifier, ok := ast.Unparen(expression).(*ast.Ident)
	return ok && info.ObjectOf(identifier) == object
}

func shadowedErrorNilIdentifier(expression ast.Expr) bool {
	identifier, ok := ast.Unparen(expression).(*ast.Ident)
	return ok && identifier.Name == "nil"
}

func shadowedErrorBreakTarget(parents map[ast.Node]ast.Node, branch *ast.BranchStmt) ast.Node {
	for parent := parents[branch]; parent != nil; parent = parents[parent] {
		switch parent.(type) {
		case *ast.ForStmt,
			*ast.RangeStmt,
			*ast.SwitchStmt,
			*ast.TypeSwitchStmt,
			*ast.SelectStmt:
			return parent
		case *ast.FuncDecl, *ast.FuncLit:
			return nil
		}
	}
	return nil
}

func shadowedErrorOuterObservedAfterLoop(
	function ast.Node,
	loop ast.Node,
	outer types.Object,
	info *types.Info,
) bool {
	found := false
	concluded := false
	ast.Inspect(
		function,
		func(node ast.Node) bool {
			if node == nil || concluded {
				return false
			}
			if node != function {
				switch node.(type) {
				case *ast.FuncDecl, *ast.FuncLit:
					return false
				}
			}
			if node.Pos() <= loop.End() {
				return true
			}
			switch node := node.(type) {
			case *ast.AssignStmt:
				if shadowedErrorAssignmentTargets(node.Lhs, outer, info) {
					concluded = true
					return false
				}
			case *ast.RangeStmt:
				if shadowedErrorAssignmentTargets(
					[]ast.Expr{node.Key, node.Value},
					outer,
					info,
				) {
					concluded = true
					return false
				}
			case *ast.ReturnStmt:
				if len(node.Results) == 0 &&
					shadowedErrorNamedResult(function, outer, info) {
					found = true
					concluded = true
					return false
				}
				for _, result := range node.Results {
					if shadowedErrorExpressionUses(result, outer, info) {
						found = true
						concluded = true
						return false
					}
				}
			case *ast.IfStmt:
				if shadowedErrorComparedWithNil(node.Cond, outer, info) {
					found = true
					concluded = true
					return false
				}
			}
			return true
		},
	)
	return found
}

func shadowedErrorAssignmentTargets(
	expressions []ast.Expr,
	object types.Object,
	info *types.Info,
) bool {
	for _, expression := range expressions {
		if expression == nil {
			continue
		}
		identifier, ok := ast.Unparen(expression).(*ast.Ident)
		if ok && info.ObjectOf(identifier) == object {
			return true
		}
	}
	return false
}

func shadowedErrorDeferredResultStale(
	candidate shadowedErrorCandidate,
	info *types.Info,
	parents map[ast.Node]ast.Node,
) bool {
	if !shadowedErrorNamedResult(candidate.function, candidate.outer, info) ||
		candidate.inner.Parent() == nil {
		return false
	}
	scope := candidate.inner.Parent()
	found := false
	ast.Inspect(
		candidate.function,
		func(node ast.Node) bool {
			if node == nil || found {
				return false
			}
			if node != candidate.function {
				switch node.(type) {
				case *ast.FuncDecl, *ast.FuncLit:
					return false
				}
			}
			deferStatement, ok := node.(*ast.DeferStmt)
			if !ok ||
				deferStatement.Pos() <= candidate.identifier.Pos() ||
				deferStatement.Pos() < scope.Pos() ||
				deferStatement.End() > scope.End() {
				return true
			}
			function, ok := ast.Unparen(deferStatement.Call.Fun).(*ast.FuncLit)
			if !ok {
				return true
			}
			found = shadowedErrorDeferredAssignment(
				function,
				candidate.inner,
				info,
				parents,
			)
			return !found
		},
	)
	return found
}

func shadowedErrorDeferredAssignment(
	function *ast.FuncLit,
	inner types.Object,
	info *types.Info,
	parents map[ast.Node]ast.Node,
) bool {
	found := false
	ast.Inspect(
		function.Body,
		func(node ast.Node) bool {
			if node == nil || found {
				return false
			}
			if node != function.Body {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, destination := range assignment.Lhs {
				identifier, ok := destination.(*ast.Ident)
				if ok &&
					info.ObjectOf(identifier) == inner &&
					shadowedErrorParentFunction(parents, identifier) ==
						function {
					found = true
					return false
				}
			}
			return true
		},
	)
	return found
}

func shadowedErrorNamedResult(function ast.Node, object types.Object, info *types.Info) bool {
	var results *ast.FieldList
	switch function := function.(type) {
	case *ast.FuncDecl:
		results = function.Type.Results
	case *ast.FuncLit:
		results = function.Type.Results
	}
	if results == nil {
		return false
	}
	for _, field := range results.List {
		for _, name := range field.Names {
			if info.Defs[name] == object {
				return true
			}
		}
	}
	return false
}

func shadowedErrorExpressionUses(expression ast.Expr, object types.Object, info *types.Info) bool {
	found := false
	ast.Inspect(
		expression,
		func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && info.ObjectOf(identifier) == object {
				found = true
				return false
			}
			return !found
		},
	)
	return found
}

func shadowedErrorParents(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	stack := make([]ast.Node, 0)
	ast.Inspect(
		file,
		func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack) - 1]
				return false
			}
			if len(stack) != 0 {
				parents[node] = stack[len(stack) - 1]
			}
			stack = append(stack, node)
			return true
		},
	)
	return parents
}

func shadowedErrorParentFunction(parents map[ast.Node]ast.Node, node ast.Node) ast.Node {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch parent.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return parent
		}
	}
	return nil
}

func shadowedErrorFunctionAt(file *ast.File, position token.Pos) ast.Node {
	var owner ast.Node
	ast.Inspect(
		file,
		func(node ast.Node) bool {
			if node == nil || position < node.Pos() || position >= node.End() {
				return false
			}
			switch node.(type) {
			case *ast.FuncDecl, *ast.FuncLit:
				owner = node
			}
			return true
		},
	)
	return owner
}
