// Package format lowers Go syntax into the Gox document model.
package format

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"

	"github.com/faustbrian/gox/internal/format/doc"
	"github.com/faustbrian/gox/internal/source"
)

// Options controls deterministic formatting.
type Options struct {
	Width     int
	TabWidth  int
	FitBudget int
}

// File formats one valid immutable source unit.
func File(file *source.File, options Options) ([]byte, error) {
	if file == nil {
		return nil, errors.New("source file is required")
	}
	if options.Width <= 0 || options.TabWidth <= 0 || options.FitBudget <= 0 {
		return nil, errors.New("width, tab width, and fit budget must be positive")
	}
	if len(file.Comments()) > 0 {
		return nil, errors.New("comment ownership lowering is not implemented")
	}
	result, err := render(file, options)
	if err != nil {
		return nil, err
	}
	formattedFile, err := source.Load(file.Path(), result)
	if err != nil {
		return nil, fmt.Errorf("formatted output failed validation: %w", err)
	}
	if err := source.ValidateEquivalent(file, formattedFile); err != nil {
		return nil, fmt.Errorf("formatted output failed equivalence: %w", err)
	}
	again, err := render(formattedFile, options)
	if err != nil {
		return nil, fmt.Errorf("repeat formatting failed: %w", err)
	}
	if !bytes.Equal(result, again) {
		return nil, errors.New("formatted output is not byte-idempotent")
	}
	return result, nil
}

func render(file *source.File, options Options) ([]byte, error) {
	arena := doc.NewArena()
	lower := lowerer{arena: arena, source: file}
	var document doc.ID
	if err := file.ReadSyntax(func(syntax *ast.File) error {
		var err error
		document, err = lower.file(syntax)
		return err
	}); err != nil {
		return nil, err
	}
	formatted, err := arena.Render(document, doc.Options{
		Width:     options.Width,
		TabWidth:  options.TabWidth,
		FitBudget: options.FitBudget,
	})
	if err != nil {
		return nil, err
	}
	result := []byte(formatted + "\n")
	if file.Metadata().HasBOM {
		result = append([]byte{0xef, 0xbb, 0xbf}, result...)
	}
	return result, nil
}

type lowerer struct {
	arena  *doc.Arena
	source *source.File
}

func (l *lowerer) file(file *ast.File) (doc.ID, error) {
	parts := []doc.ID{l.arena.Text("package "), l.arena.Text(file.Name.Name)}
	for _, declaration := range file.Decls {
		lowered, err := l.declaration(declaration)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.HardLine(), l.arena.HardLine(), lowered)
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) declaration(declaration ast.Decl) (doc.ID, error) {
	switch value := declaration.(type) {
	case *ast.FuncDecl:
		return l.function(value)
	default:
		return doc.ID{}, fmt.Errorf("unsupported declaration %T", declaration)
	}
}

func (l *lowerer) function(function *ast.FuncDecl) (doc.ID, error) {
	if function.Recv != nil || function.Type.TypeParams != nil {
		return doc.ID{}, errors.New("receivers and type parameters are not implemented")
	}
	parameters, err := l.fieldList(function.Type.Params)
	if err != nil {
		return doc.ID{}, err
	}
	parts := []doc.ID{l.arena.Text("func "), l.arena.Text(function.Name.Name), parameters}
	if function.Type.Results != nil {
		results, err := l.results(function.Type.Results)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" "), results)
	}
	if function.Body != nil {
		body, err := l.block(function.Body)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" "), body)
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) fieldList(fields *ast.FieldList) (doc.ID, error) {
	if fields == nil || len(fields.List) == 0 {
		return l.arena.Text("()"), nil
	}
	items := make([]doc.ID, 0, len(fields.List))
	for _, field := range fields.List {
		item, err := l.field(field)
		if err != nil {
			return doc.ID{}, err
		}
		items = append(items, item)
	}
	return l.commaList("(", ")", items), nil
}

func (l *lowerer) results(fields *ast.FieldList) (doc.ID, error) {
	if len(fields.List) == 1 && len(fields.List[0].Names) == 0 {
		return l.expression(fields.List[0].Type)
	}
	return l.fieldList(fields)
}

func (l *lowerer) field(field *ast.Field) (doc.ID, error) {
	typeDocument, err := l.expression(field.Type)
	if err != nil {
		return doc.ID{}, err
	}
	if len(field.Names) == 0 {
		return typeDocument, nil
	}
	names := make([]doc.ID, 0, len(field.Names))
	for _, name := range field.Names {
		names = append(names, l.arena.Text(name.Name))
	}
	return l.arena.Concat(l.join(l.arena.Text(", "), names), l.arena.Text(" "), typeDocument), nil
}

func (l *lowerer) block(block *ast.BlockStmt) (doc.ID, error) {
	statements := make([]doc.ID, 0, len(block.List))
	for _, statement := range block.List {
		lowered, err := l.statement(statement)
		if err != nil {
			return doc.ID{}, err
		}
		statements = append(statements, lowered)
	}
	if len(statements) == 0 {
		return l.arena.Text("{}"), nil
	}
	body := l.join(l.arena.HardLine(), statements)
	return l.arena.Concat(
		l.arena.Text("{"),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), body)),
		l.arena.HardLine(),
		l.arena.Text("}"),
	), nil
}

func (l *lowerer) statement(statement ast.Stmt) (doc.ID, error) {
	switch value := statement.(type) {
	case *ast.AssignStmt:
		return l.assignment(value)
	case *ast.ExprStmt:
		return l.expression(value.X)
	case *ast.IncDecStmt:
		expression, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(expression, l.arena.Text(value.Tok.String())), nil
	case *ast.ReturnStmt:
		if len(value.Results) == 0 {
			return l.arena.Text("return"), nil
		}
		results, err := l.expressions(value.Results)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("return "), results), nil
	case *ast.IfStmt:
		return l.ifStatement(value)
	case *ast.EmptyStmt:
		return l.arena.Empty(), nil
	default:
		return doc.ID{}, fmt.Errorf("unsupported statement %T", statement)
	}
}

func (l *lowerer) assignment(assignment *ast.AssignStmt) (doc.ID, error) {
	left, err := l.expressions(assignment.Lhs)
	if err != nil {
		return doc.ID{}, err
	}
	right, err := l.expressions(assignment.Rhs)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(
		left,
		l.arena.Text(" "+assignment.Tok.String()+" "),
		right,
	), nil
}

func (l *lowerer) ifStatement(statement *ast.IfStmt) (doc.ID, error) {
	parts := []doc.ID{l.arena.Text("if ")}
	if statement.Init != nil {
		initializer, err := l.statement(statement.Init)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, initializer, l.arena.Text("; "))
	}
	condition, err := l.expression(statement.Cond)
	if err != nil {
		return doc.ID{}, err
	}
	body, err := l.block(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, condition, l.arena.Text(" "), body)
	if statement.Else != nil {
		alternative, err := l.statement(statement.Else)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" else "), alternative)
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) expressions(expressions []ast.Expr) (doc.ID, error) {
	items := make([]doc.ID, 0, len(expressions))
	for _, expression := range expressions {
		item, err := l.expression(expression)
		if err != nil {
			return doc.ID{}, err
		}
		items = append(items, item)
	}
	return l.join(l.arena.Text(", "), items), nil
}

func (l *lowerer) expression(expression ast.Expr) (doc.ID, error) {
	switch value := expression.(type) {
	case *ast.Ident:
		return l.arena.Text(value.Name), nil
	case *ast.BasicLit:
		if raw, found := l.source.RawToken(value.Pos()); found {
			return l.arena.Verbatim(raw), nil
		}
		return doc.ID{}, errors.New("basic literal has no physical token")
	case *ast.SelectorExpr:
		left, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(left, l.arena.Text("."), l.arena.Text(value.Sel.Name)), nil
	case *ast.CallExpr:
		return l.call(value)
	case *ast.BinaryExpr:
		return l.binary(value)
	case *ast.UnaryExpr:
		operand, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text(value.Op.String()), operand), nil
	case *ast.ParenExpr:
		inner, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("("), inner, l.arena.Text(")")), nil
	default:
		return doc.ID{}, fmt.Errorf("unsupported expression %T", expression)
	}
}

func (l *lowerer) call(call *ast.CallExpr) (doc.ID, error) {
	function, err := l.expression(call.Fun)
	if err != nil {
		return doc.ID{}, err
	}
	arguments := make([]doc.ID, 0, len(call.Args))
	for _, argument := range call.Args {
		lowered, err := l.expression(argument)
		if err != nil {
			return doc.ID{}, err
		}
		arguments = append(arguments, lowered)
	}
	if call.Ellipsis.IsValid() {
		if len(arguments) == 0 {
			return doc.ID{}, errors.New("call ellipsis has no argument")
		}
		last := len(arguments) - 1
		arguments[last] = l.arena.Concat(arguments[last], l.arena.Text("..."))
	}
	if len(arguments) == 0 {
		return l.arena.Concat(function, l.arena.Text("()")), nil
	}
	return l.arena.Concat(function, l.commaList("(", ")", arguments)), nil
}

func (l *lowerer) binary(expression *ast.BinaryExpr) (doc.ID, error) {
	operands := make([]ast.Expr, 0, 4)
	l.flattenBinary(expression, expression.Op, &operands)
	items := make([]doc.ID, 0, len(operands))
	for index, operand := range operands {
		lowered, err := l.expression(operand)
		if err != nil {
			return doc.ID{}, err
		}
		if index < len(operands)-1 {
			lowered = l.arena.Concat(lowered, l.arena.Text(" "+expression.Op.String()))
		}
		items = append(items, lowered)
	}
	return l.arena.Group(l.arena.Concat(
		items[0],
		l.arena.Indent(l.arena.Concat(l.arena.Line(), l.join(l.arena.Line(), items[1:]))),
	)), nil
}

func (l *lowerer) flattenBinary(expression ast.Expr, operator token.Token, result *[]ast.Expr) {
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != operator {
		*result = append(*result, expression)
		return
	}
	l.flattenBinary(binary.X, operator, result)
	l.flattenBinary(binary.Y, operator, result)
}

func (l *lowerer) commaList(open, close string, items []doc.ID) doc.ID {
	separator := l.arena.Concat(l.arena.Text(","), l.arena.Line())
	return l.arena.Group(l.arena.Concat(
		l.arena.Text(open),
		l.arena.Indent(l.arena.Concat(l.arena.SoftLine(), l.join(separator, items))),
		l.arena.IfBreak(l.arena.Text(","), l.arena.Empty()),
		l.arena.SoftLine(),
		l.arena.Text(close),
	))
}

func (l *lowerer) join(separator doc.ID, items []doc.ID) doc.ID {
	if len(items) == 0 {
		return l.arena.Empty()
	}
	parts := make([]doc.ID, 0, len(items)*2-1)
	for index, item := range items {
		if index > 0 {
			parts = append(parts, separator)
		}
		parts = append(parts, item)
	}
	return l.arena.Concat(parts...)
}
