package rules

import (
	"go/ast"
	"go/token"
)

type localCallAcquisition struct {
	node ast.Node
	call *ast.CallExpr
	identifiers []*ast.Ident
}

func localCallAcquisitionAtStatement(statement ast.Stmt) (localCallAcquisition, bool) {
	if statement == nil {
		return localCallAcquisition{}, false
	}
	if assignment, ok := statement.(*ast.AssignStmt); ok {
		return localCallAcquisitionAtNode(assignment)
	}
	declaration, ok := statement.(*ast.DeclStmt)
	if !ok {
		return localCallAcquisition{}, false
	}
	general, ok := declaration.Decl.(*ast.GenDecl)
	if !ok || general.Tok != token.VAR || len(general.Specs) != 1 {
		return localCallAcquisition{}, false
	}
	specification, ok := general.Specs[0].(*ast.ValueSpec)
	if !ok {
		return localCallAcquisition{}, false
	}
	return localCallAcquisitionAtNode(specification)
}

func localCallAcquisitionAtNode(node ast.Node) (localCallAcquisition, bool) {
	var (
		values []ast.Expr
		identifiers []*ast.Ident
	)
	switch node := node.(type) {
	case *ast.AssignStmt:
		values = node.Rhs
		identifiers = make([]*ast.Ident, len(node.Lhs))
		for index, expression := range node.Lhs {
			identifiers[index], _ = expression.(*ast.Ident)
		}
	case *ast.ValueSpec:
		values = node.Values
		identifiers = append([]*ast.Ident(nil), node.Names...)
	default:
		return localCallAcquisition{}, false
	}
	if len(values) != 1 {
		return localCallAcquisition{}, false
	}
	call, _ := ast.Unparen(values[0]).(*ast.CallExpr)
	if call == nil {
		return localCallAcquisition{}, false
	}
	return localCallAcquisition{node: node, call: call, identifiers: identifiers}, true
}
