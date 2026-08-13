package rules

import "go/ast"

// NodeKind is a stable native interest in one Go syntax node type.
type NodeKind string

const (
	NodeFile NodeKind = "file"
	NodeComment NodeKind = "comment"
	NodeCommentGroup NodeKind = "comment-group"
	NodeField NodeKind = "field"
	NodeFieldList NodeKind = "field-list"
	NodeBadExpr NodeKind = "bad-expr"
	NodeIdent NodeKind = "ident"
	NodeEllipsis NodeKind = "ellipsis"
	NodeBasicLit NodeKind = "basic-lit"
	NodeFuncLit NodeKind = "func-lit"
	NodeCompositeLit NodeKind = "composite-lit"
	NodeParenExpr NodeKind = "paren-expr"
	NodeSelectorExpr NodeKind = "selector-expr"
	NodeIndexExpr NodeKind = "index-expr"
	NodeIndexListExpr NodeKind = "index-list-expr"
	NodeSliceExpr NodeKind = "slice-expr"
	NodeTypeAssertExpr NodeKind = "type-assert-expr"
	NodeCallExpr NodeKind = "call-expr"
	NodeStarExpr NodeKind = "star-expr"
	NodeUnaryExpr NodeKind = "unary-expr"
	NodeBinaryExpr NodeKind = "binary-expr"
	NodeKeyValueExpr NodeKind = "key-value-expr"
	NodeArrayType NodeKind = "array-type"
	NodeStructType NodeKind = "struct-type"
	NodeFuncType NodeKind = "func-type"
	NodeInterfaceType NodeKind = "interface-type"
	NodeMapType NodeKind = "map-type"
	NodeChanType NodeKind = "chan-type"
	NodeBadStmt NodeKind = "bad-stmt"
	NodeDeclStmt NodeKind = "decl-stmt"
	NodeEmptyStmt NodeKind = "empty-stmt"
	NodeLabeledStmt NodeKind = "labeled-stmt"
	NodeExprStmt NodeKind = "expr-stmt"
	NodeSendStmt NodeKind = "send-stmt"
	NodeIncDecStmt NodeKind = "inc-dec-stmt"
	NodeAssignStmt NodeKind = "assign-stmt"
	NodeGoStmt NodeKind = "go-stmt"
	NodeDeferStmt NodeKind = "defer-stmt"
	NodeReturnStmt NodeKind = "return-stmt"
	NodeBranchStmt NodeKind = "branch-stmt"
	NodeBlockStmt NodeKind = "block-stmt"
	NodeIfStmt NodeKind = "if-stmt"
	NodeCaseClause NodeKind = "case-clause"
	NodeSwitchStmt NodeKind = "switch-stmt"
	NodeTypeSwitchStmt NodeKind = "type-switch-stmt"
	NodeCommClause NodeKind = "comm-clause"
	NodeSelectStmt NodeKind = "select-stmt"
	NodeForStmt NodeKind = "for-stmt"
	NodeRangeStmt NodeKind = "range-stmt"
	NodeBadDecl NodeKind = "bad-decl"
	NodeGenDecl NodeKind = "gen-decl"
	NodeFuncDecl NodeKind = "func-decl"
	NodeImportSpec NodeKind = "import-spec"
	NodeValueSpec NodeKind = "value-spec"
	NodeTypeSpec NodeKind = "type-spec"
)

// NodePrototype returns the ast/inspector filter prototype for one interest.
func NodePrototype(kind NodeKind) (ast.Node, bool) {
	switch kind {
	case NodeFile:
		return (*ast.File)(nil), true
	case NodeComment:
		return (*ast.Comment)(nil), true
	case NodeCommentGroup:
		return (*ast.CommentGroup)(nil), true
	case NodeField:
		return (*ast.Field)(nil), true
	case NodeFieldList:
		return (*ast.FieldList)(nil), true
	case NodeBadExpr:
		return (*ast.BadExpr)(nil), true
	case NodeIdent:
		return (*ast.Ident)(nil), true
	case NodeEllipsis:
		return (*ast.Ellipsis)(nil), true
	case NodeBasicLit:
		return (*ast.BasicLit)(nil), true
	case NodeFuncLit:
		return (*ast.FuncLit)(nil), true
	case NodeCompositeLit:
		return (*ast.CompositeLit)(nil), true
	case NodeParenExpr:
		return (*ast.ParenExpr)(nil), true
	case NodeSelectorExpr:
		return (*ast.SelectorExpr)(nil), true
	case NodeIndexExpr:
		return (*ast.IndexExpr)(nil), true
	case NodeIndexListExpr:
		return (*ast.IndexListExpr)(nil), true
	case NodeSliceExpr:
		return (*ast.SliceExpr)(nil), true
	case NodeTypeAssertExpr:
		return (*ast.TypeAssertExpr)(nil), true
	case NodeCallExpr:
		return (*ast.CallExpr)(nil), true
	case NodeStarExpr:
		return (*ast.StarExpr)(nil), true
	case NodeUnaryExpr:
		return (*ast.UnaryExpr)(nil), true
	case NodeBinaryExpr:
		return (*ast.BinaryExpr)(nil), true
	case NodeKeyValueExpr:
		return (*ast.KeyValueExpr)(nil), true
	case NodeArrayType:
		return (*ast.ArrayType)(nil), true
	case NodeStructType:
		return (*ast.StructType)(nil), true
	case NodeFuncType:
		return (*ast.FuncType)(nil), true
	case NodeInterfaceType:
		return (*ast.InterfaceType)(nil), true
	case NodeMapType:
		return (*ast.MapType)(nil), true
	case NodeChanType:
		return (*ast.ChanType)(nil), true
	case NodeBadStmt:
		return (*ast.BadStmt)(nil), true
	case NodeDeclStmt:
		return (*ast.DeclStmt)(nil), true
	case NodeEmptyStmt:
		return (*ast.EmptyStmt)(nil), true
	case NodeLabeledStmt:
		return (*ast.LabeledStmt)(nil), true
	case NodeExprStmt:
		return (*ast.ExprStmt)(nil), true
	case NodeSendStmt:
		return (*ast.SendStmt)(nil), true
	case NodeIncDecStmt:
		return (*ast.IncDecStmt)(nil), true
	case NodeAssignStmt:
		return (*ast.AssignStmt)(nil), true
	case NodeGoStmt:
		return (*ast.GoStmt)(nil), true
	case NodeDeferStmt:
		return (*ast.DeferStmt)(nil), true
	case NodeReturnStmt:
		return (*ast.ReturnStmt)(nil), true
	case NodeBranchStmt:
		return (*ast.BranchStmt)(nil), true
	case NodeBlockStmt:
		return (*ast.BlockStmt)(nil), true
	case NodeIfStmt:
		return (*ast.IfStmt)(nil), true
	case NodeCaseClause:
		return (*ast.CaseClause)(nil), true
	case NodeSwitchStmt:
		return (*ast.SwitchStmt)(nil), true
	case NodeTypeSwitchStmt:
		return (*ast.TypeSwitchStmt)(nil), true
	case NodeCommClause:
		return (*ast.CommClause)(nil), true
	case NodeSelectStmt:
		return (*ast.SelectStmt)(nil), true
	case NodeForStmt:
		return (*ast.ForStmt)(nil), true
	case NodeRangeStmt:
		return (*ast.RangeStmt)(nil), true
	case NodeBadDecl:
		return (*ast.BadDecl)(nil), true
	case NodeGenDecl:
		return (*ast.GenDecl)(nil), true
	case NodeFuncDecl:
		return (*ast.FuncDecl)(nil), true
	case NodeImportSpec:
		return (*ast.ImportSpec)(nil), true
	case NodeValueSpec:
		return (*ast.ValueSpec)(nil), true
	case NodeTypeSpec:
		return (*ast.TypeSpec)(nil), true
	default:
		return nil, false
	}
}

// KindOf returns the stable interest name for a concrete syntax node.
func KindOf(node ast.Node) (NodeKind, bool) {
	switch node.(type) {
	case *ast.File:
		return NodeFile, true
	case *ast.Comment:
		return NodeComment, true
	case *ast.CommentGroup:
		return NodeCommentGroup, true
	case *ast.Field:
		return NodeField, true
	case *ast.FieldList:
		return NodeFieldList, true
	case *ast.BadExpr:
		return NodeBadExpr, true
	case *ast.Ident:
		return NodeIdent, true
	case *ast.Ellipsis:
		return NodeEllipsis, true
	case *ast.BasicLit:
		return NodeBasicLit, true
	case *ast.FuncLit:
		return NodeFuncLit, true
	case *ast.CompositeLit:
		return NodeCompositeLit, true
	case *ast.ParenExpr:
		return NodeParenExpr, true
	case *ast.SelectorExpr:
		return NodeSelectorExpr, true
	case *ast.IndexExpr:
		return NodeIndexExpr, true
	case *ast.IndexListExpr:
		return NodeIndexListExpr, true
	case *ast.SliceExpr:
		return NodeSliceExpr, true
	case *ast.TypeAssertExpr:
		return NodeTypeAssertExpr, true
	case *ast.CallExpr:
		return NodeCallExpr, true
	case *ast.StarExpr:
		return NodeStarExpr, true
	case *ast.UnaryExpr:
		return NodeUnaryExpr, true
	case *ast.BinaryExpr:
		return NodeBinaryExpr, true
	case *ast.KeyValueExpr:
		return NodeKeyValueExpr, true
	case *ast.ArrayType:
		return NodeArrayType, true
	case *ast.StructType:
		return NodeStructType, true
	case *ast.FuncType:
		return NodeFuncType, true
	case *ast.InterfaceType:
		return NodeInterfaceType, true
	case *ast.MapType:
		return NodeMapType, true
	case *ast.ChanType:
		return NodeChanType, true
	case *ast.BadStmt:
		return NodeBadStmt, true
	case *ast.DeclStmt:
		return NodeDeclStmt, true
	case *ast.EmptyStmt:
		return NodeEmptyStmt, true
	case *ast.LabeledStmt:
		return NodeLabeledStmt, true
	case *ast.ExprStmt:
		return NodeExprStmt, true
	case *ast.SendStmt:
		return NodeSendStmt, true
	case *ast.IncDecStmt:
		return NodeIncDecStmt, true
	case *ast.AssignStmt:
		return NodeAssignStmt, true
	case *ast.GoStmt:
		return NodeGoStmt, true
	case *ast.DeferStmt:
		return NodeDeferStmt, true
	case *ast.ReturnStmt:
		return NodeReturnStmt, true
	case *ast.BranchStmt:
		return NodeBranchStmt, true
	case *ast.BlockStmt:
		return NodeBlockStmt, true
	case *ast.IfStmt:
		return NodeIfStmt, true
	case *ast.CaseClause:
		return NodeCaseClause, true
	case *ast.SwitchStmt:
		return NodeSwitchStmt, true
	case *ast.TypeSwitchStmt:
		return NodeTypeSwitchStmt, true
	case *ast.CommClause:
		return NodeCommClause, true
	case *ast.SelectStmt:
		return NodeSelectStmt, true
	case *ast.ForStmt:
		return NodeForStmt, true
	case *ast.RangeStmt:
		return NodeRangeStmt, true
	case *ast.BadDecl:
		return NodeBadDecl, true
	case *ast.GenDecl:
		return NodeGenDecl, true
	case *ast.FuncDecl:
		return NodeFuncDecl, true
	case *ast.ImportSpec:
		return NodeImportSpec, true
	case *ast.ValueSpec:
		return NodeValueSpec, true
	case *ast.TypeSpec:
		return NodeTypeSpec, true
	default:
		return "", false
	}
}
