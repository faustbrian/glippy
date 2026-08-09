// Package format lowers Go syntax into the Gox document model.
package format

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"strings"

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
	parts := make([]doc.ID, 0, len(file.Decls)*3+3)
	packageOffset, found := l.source.PhysicalOffset(file.Package)
	if !found {
		return doc.ID{}, errors.New("package clause has no physical offset")
	}
	comments := l.source.Comments()
	for _, comment := range comments {
		if comment.Range.End > packageOffset {
			return doc.ID{}, errors.New("comment ownership lowering is not implemented outside the file prefix")
		}
	}
	if len(comments) > 0 {
		prefixStart := 0
		if l.source.Metadata().HasBOM {
			prefixStart = 3
		}
		prefix, ok := l.source.Slice(source.Range{Start: prefixStart, End: packageOffset})
		if !ok {
			return doc.ID{}, errors.New("file prefix has an invalid physical range")
		}
		parts = append(parts, l.arena.Verbatim(prefix))
	}
	parts = append(parts, l.arena.Text("package "), l.arena.Text(file.Name.Name))
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
	case *ast.GenDecl:
		if value.Tok == token.IMPORT {
			return l.importDeclaration(value)
		}
		return l.generalDeclaration(value)
	default:
		return doc.ID{}, fmt.Errorf("unsupported declaration %T", declaration)
	}
}

func (l *lowerer) generalDeclaration(declaration *ast.GenDecl) (doc.ID, error) {
	if declaration.Tok != token.CONST && declaration.Tok != token.VAR && declaration.Tok != token.TYPE {
		return doc.ID{}, fmt.Errorf("unsupported declaration token %s", declaration.Tok)
	}
	specs := make([]doc.ID, 0, len(declaration.Specs))
	for _, spec := range declaration.Specs {
		var (
			lowered doc.ID
			err     error
		)
		switch value := spec.(type) {
		case *ast.ValueSpec:
			lowered, err = l.valueSpec(value)
		case *ast.TypeSpec:
			lowered, err = l.typeSpec(value)
		default:
			err = fmt.Errorf("%s declaration contains %T", declaration.Tok, spec)
		}
		if err != nil {
			return doc.ID{}, err
		}
		specs = append(specs, lowered)
	}
	keyword := declaration.Tok.String()
	if !declaration.Lparen.IsValid() {
		if len(specs) != 1 {
			return doc.ID{}, fmt.Errorf("ungrouped %s declaration must contain one spec", keyword)
		}
		return l.arena.Concat(l.arena.Text(keyword+" "), specs[0]), nil
	}
	if len(specs) == 0 {
		return l.arena.Text(keyword + " ()"), nil
	}
	return l.arena.Concat(
		l.arena.Text(keyword+" ("),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.join(l.arena.HardLine(), specs))),
		l.arena.HardLine(),
		l.arena.Text(")"),
	), nil
}

func (l *lowerer) valueSpec(spec *ast.ValueSpec) (doc.ID, error) {
	names := make([]doc.ID, 0, len(spec.Names))
	for _, name := range spec.Names {
		names = append(names, l.arena.Text(name.Name))
	}
	parts := []doc.ID{l.join(l.arena.Text(", "), names)}
	if spec.Type != nil {
		typeDocument, err := l.expression(spec.Type)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" "), typeDocument)
	}
	if len(spec.Values) > 0 {
		values, err := l.expressions(spec.Values)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" = "), values)
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) typeSpec(spec *ast.TypeSpec) (doc.ID, error) {
	parts := []doc.ID{l.arena.Text(spec.Name.Name)}
	if spec.TypeParams != nil {
		typeParameters, err := l.fieldListWithDelimiters(spec.TypeParams, "[", "]")
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, typeParameters)
	}
	typeDocument, err := l.expression(spec.Type)
	if err != nil {
		return doc.ID{}, err
	}
	separator := " "
	if spec.Assign.IsValid() {
		separator = " = "
	}
	parts = append(parts, l.arena.Text(separator), typeDocument)
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) importDeclaration(declaration *ast.GenDecl) (doc.ID, error) {
	imports := make([]doc.ID, 0, len(declaration.Specs))
	for _, rawSpec := range declaration.Specs {
		spec, ok := rawSpec.(*ast.ImportSpec)
		if !ok {
			return doc.ID{}, fmt.Errorf("import declaration contains %T", rawSpec)
		}
		path, found := l.source.RawToken(spec.Path.Pos())
		if !found {
			return doc.ID{}, errors.New("import path has no physical token")
		}
		parts := make([]doc.ID, 0, 2)
		if spec.Name != nil {
			parts = append(parts, l.arena.Text(spec.Name.Name), l.arena.Text(" "))
		}
		parts = append(parts, l.arena.Verbatim(path))
		imports = append(imports, l.arena.Concat(parts...))
	}
	if !declaration.Lparen.IsValid() {
		if len(imports) != 1 {
			return doc.ID{}, errors.New("ungrouped import declaration must contain one spec")
		}
		return l.arena.Concat(l.arena.Text("import "), imports[0]), nil
	}
	if len(imports) == 0 {
		return l.arena.Text("import ()"), nil
	}
	body := make([]doc.ID, 0, len(imports)*3-1)
	for index, item := range imports {
		if index > 0 {
			separator := l.arena.HardLine()
			previousEnd, previousFound := l.source.PhysicalOffset(declaration.Specs[index-1].End())
			currentStart, currentFound := l.source.PhysicalOffset(declaration.Specs[index].Pos())
			if !previousFound || !currentFound {
				return doc.ID{}, errors.New("import group has no physical gap")
			}
			gap, valid := l.source.Slice(source.Range{Start: previousEnd, End: currentStart})
			if !valid {
				return doc.ID{}, errors.New("import group has an invalid physical gap")
			}
			body = append(body, separator)
			if strings.Count(gap, "\n") >= 2 {
				body = append(body, l.arena.HardLine())
			}
		}
		body = append(body, item)
	}
	return l.arena.Concat(
		l.arena.Text("import ("),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.arena.Concat(body...))),
		l.arena.HardLine(),
		l.arena.Text(")"),
	), nil
}

func (l *lowerer) function(function *ast.FuncDecl) (doc.ID, error) {
	parameters, err := l.fieldList(function.Type.Params)
	if err != nil {
		return doc.ID{}, err
	}
	parts := []doc.ID{l.arena.Text("func ")}
	if function.Recv != nil {
		receiver, err := l.fieldList(function.Recv)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, receiver, l.arena.Text(" "))
	}
	parts = append(parts, l.arena.Text(function.Name.Name))
	if function.Type.TypeParams != nil {
		typeParameters, err := l.fieldListWithDelimiters(function.Type.TypeParams, "[", "]")
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, typeParameters)
	}
	parts = append(parts, parameters)
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
	return l.fieldListWithDelimiters(fields, "(", ")")
}

func (l *lowerer) fieldListWithDelimiters(fields *ast.FieldList, open, close string) (doc.ID, error) {
	if fields == nil || len(fields.List) == 0 {
		return l.arena.Text(open + close), nil
	}
	items := make([]doc.ID, 0, len(fields.List))
	for _, field := range fields.List {
		item, err := l.field(field)
		if err != nil {
			return doc.ID{}, err
		}
		items = append(items, item)
	}
	return l.commaList(open, close, items), nil
}

func (l *lowerer) results(fields *ast.FieldList) (doc.ID, error) {
	if !fields.Opening.IsValid() && len(fields.List) == 1 && len(fields.List[0].Names) == 0 {
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
		if field.Tag == nil {
			return typeDocument, nil
		}
		tag, found := l.source.RawToken(field.Tag.Pos())
		if !found {
			return doc.ID{}, errors.New("field tag has no physical token")
		}
		return l.arena.Concat(typeDocument, l.arena.Text(" "), l.arena.Verbatim(tag)), nil
	}
	names := make([]doc.ID, 0, len(field.Names))
	for _, name := range field.Names {
		names = append(names, l.arena.Text(name.Name))
	}
	parts := []doc.ID{l.join(l.arena.Text(", "), names), l.arena.Text(" "), typeDocument}
	if field.Tag != nil {
		tag, found := l.source.RawToken(field.Tag.Pos())
		if !found {
			return doc.ID{}, errors.New("field tag has no physical token")
		}
		parts = append(parts, l.arena.Text(" "), l.arena.Verbatim(tag))
	}
	return l.arena.Concat(parts...), nil
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
	case *ast.CompositeLit:
		return l.compositeLiteral(value)
	case *ast.KeyValueExpr:
		key, err := l.expression(value.Key)
		if err != nil {
			return doc.ID{}, err
		}
		item, err := l.expression(value.Value)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(key, l.arena.Text(": "), item), nil
	case *ast.IndexExpr:
		base, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		index, err := l.expression(value.Index)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(base, l.arena.Text("["), index, l.arena.Text("]")), nil
	case *ast.IndexListExpr:
		base, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		indices := make([]doc.ID, 0, len(value.Indices))
		for _, rawIndex := range value.Indices {
			index, err := l.expression(rawIndex)
			if err != nil {
				return doc.ID{}, err
			}
			indices = append(indices, index)
		}
		return l.arena.Concat(base, l.commaList("[", "]", indices)), nil
	case *ast.SliceExpr:
		return l.slice(value)
	case *ast.TypeAssertExpr:
		base, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		if value.Type == nil {
			return doc.ID{}, errors.New("type-switch assertion is not an ordinary expression")
		}
		asserted, err := l.expression(value.Type)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(base, l.arena.Text(".("), asserted, l.arena.Text(")")), nil
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
	case *ast.StarExpr:
		operand, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("*"), operand), nil
	case *ast.ArrayType:
		element, err := l.expression(value.Elt)
		if err != nil {
			return doc.ID{}, err
		}
		if value.Len == nil {
			return l.arena.Concat(l.arena.Text("[]"), element), nil
		}
		length, err := l.expression(value.Len)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("["), length, l.arena.Text("]"), element), nil
	case *ast.MapType:
		key, err := l.expression(value.Key)
		if err != nil {
			return doc.ID{}, err
		}
		element, err := l.expression(value.Value)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("map["), key, l.arena.Text("]"), element), nil
	case *ast.ChanType:
		element, err := l.expression(value.Value)
		if err != nil {
			return doc.ID{}, err
		}
		prefix := "chan "
		switch value.Dir {
		case ast.SEND:
			prefix = "chan<- "
		case ast.RECV:
			prefix = "<-chan "
		}
		return l.arena.Concat(l.arena.Text(prefix), element), nil
	case *ast.Ellipsis:
		if value.Elt == nil {
			return l.arena.Text("..."), nil
		}
		element, err := l.expression(value.Elt)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("..."), element), nil
	case *ast.FuncType:
		return l.functionType(value, true)
	case *ast.FuncLit:
		signature, err := l.functionType(value.Type, true)
		if err != nil {
			return doc.ID{}, err
		}
		body, err := l.block(value.Body)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(signature, l.arena.Text(" "), body), nil
	case *ast.StructType:
		return l.aggregateType("struct", value.Fields, false)
	case *ast.InterfaceType:
		return l.aggregateType("interface", value.Methods, true)
	default:
		return doc.ID{}, fmt.Errorf("unsupported expression %T", expression)
	}
}

func (l *lowerer) compositeLiteral(literal *ast.CompositeLit) (doc.ID, error) {
	parts := make([]doc.ID, 0, 2)
	if literal.Type != nil {
		typeDocument, err := l.expression(literal.Type)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, typeDocument)
	}
	elements := make([]doc.ID, 0, len(literal.Elts))
	for _, rawElement := range literal.Elts {
		element, err := l.expression(rawElement)
		if err != nil {
			return doc.ID{}, err
		}
		elements = append(elements, element)
	}
	if len(elements) == 0 {
		parts = append(parts, l.arena.Text("{}"))
	} else {
		parts = append(parts, l.commaList("{", "}", elements))
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) slice(expression *ast.SliceExpr) (doc.ID, error) {
	base, err := l.expression(expression.X)
	if err != nil {
		return doc.ID{}, err
	}
	parts := []doc.ID{base, l.arena.Text("[")}
	for index, bound := range []ast.Expr{expression.Low, expression.High, expression.Max} {
		if index == 2 && !expression.Slice3 {
			break
		}
		if index > 0 {
			parts = append(parts, l.arena.Text(":"))
		}
		if bound != nil {
			lowered, err := l.expression(bound)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, lowered)
		}
	}
	parts = append(parts, l.arena.Text("]"))
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) functionType(function *ast.FuncType, includeKeyword bool) (doc.ID, error) {
	parts := make([]doc.ID, 0, 5)
	if includeKeyword {
		parts = append(parts, l.arena.Text("func"))
	}
	if function.TypeParams != nil {
		typeParameters, err := l.fieldListWithDelimiters(function.TypeParams, "[", "]")
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, typeParameters)
	}
	parameters, err := l.fieldList(function.Params)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, parameters)
	if function.Results != nil {
		results, err := l.results(function.Results)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" "), results)
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) aggregateType(keyword string, fields *ast.FieldList, methods bool) (doc.ID, error) {
	if fields == nil || len(fields.List) == 0 {
		return l.arena.Text(keyword + "{}"), nil
	}
	items := make([]doc.ID, 0, len(fields.List))
	for _, field := range fields.List {
		var (
			item doc.ID
			err  error
		)
		if methods && len(field.Names) > 0 {
			function, ok := field.Type.(*ast.FuncType)
			if !ok {
				return doc.ID{}, fmt.Errorf("named interface field has type %T", field.Type)
			}
			signature, signatureErr := l.functionType(function, false)
			if signatureErr != nil {
				return doc.ID{}, signatureErr
			}
			names := make([]doc.ID, 0, len(field.Names))
			for _, name := range field.Names {
				names = append(names, l.arena.Text(name.Name))
			}
			item = l.arena.Concat(l.join(l.arena.Text(", "), names), signature)
		} else {
			item, err = l.field(field)
		}
		if err != nil {
			return doc.ID{}, err
		}
		items = append(items, item)
	}
	return l.arena.Concat(
		l.arena.Text(keyword+" {"),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.join(l.arena.HardLine(), items))),
		l.arena.HardLine(),
		l.arena.Text("}"),
	), nil
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
