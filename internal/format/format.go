// Package format lowers Go syntax into the Gox document model.
package format

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"sort"
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
	lower := newLowerer(arena, file)
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
	arena          *doc.Arena
	source         *source.File
	physical       []byte
	tokens         []source.Token
	comments       []source.Comment
	commentByStart map[int]int
	emittedComment []bool
}

func newLowerer(arena *doc.Arena, file *source.File) lowerer {
	comments := file.Comments()
	commentByStart := make(map[int]int, len(comments))
	for index, comment := range comments {
		commentByStart[comment.Range.Start] = index
	}
	return lowerer{
		arena:          arena,
		source:         file,
		physical:       file.Bytes(),
		tokens:         file.Tokens(),
		comments:       comments,
		commentByStart: commentByStart,
		emittedComment: make([]bool, len(comments)),
	}
}

func (l *lowerer) file(file *ast.File) (doc.ID, error) {
	parts := make([]doc.ID, 0, len(file.Decls)*3+3)
	packageOffset, found := l.source.PhysicalOffset(file.Package)
	if !found {
		return doc.ID{}, errors.New("package clause has no physical offset")
	}
	hasPrefixComments := false
	for index, comment := range l.comments {
		if comment.Range.End <= packageOffset {
			l.emittedComment[index] = true
			hasPrefixComments = true
		}
	}
	if hasPrefixComments {
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
	boundary, found := l.source.PhysicalOffset(file.Name.End())
	if !found {
		return doc.ID{}, errors.New("package name has no physical end offset")
	}
	firstDeclaration := len(l.physical)
	if len(file.Decls) > 0 {
		firstDeclaration, found = l.source.PhysicalOffset(file.Decls[0].Pos())
		if !found {
			return doc.ID{}, errors.New("first declaration has no physical offset")
		}
	}
	parts = append(parts, l.withTrailingComments(l.arena.Empty(), l.trailingComments(boundary, firstDeclaration)))
	for index, declaration := range file.Decls {
		declarationStart, found := l.source.PhysicalOffset(declaration.Pos())
		if !found {
			return doc.ID{}, errors.New("declaration has no physical start offset")
		}
		leading := l.commentsBetween(boundary, declarationStart)
		lowered, err := l.declaration(declaration)
		if err != nil {
			return doc.ID{}, err
		}
		if len(leading) > 0 {
			lowered = l.arena.Concat(l.boundaryCommentsDocument(leading, declarationStart), lowered)
		}
		limit := len(l.physical)
		if index+1 < len(file.Decls) {
			var found bool
			limit, found = l.source.PhysicalOffset(file.Decls[index+1].Pos())
			if !found {
				return doc.ID{}, errors.New("following declaration has no physical offset")
			}
		}
		declarationEnd, found := l.source.PhysicalOffset(declaration.End())
		if !found {
			return doc.ID{}, errors.New("declaration has no physical end offset")
		}
		trailing := l.trailingComments(declarationEnd, limit)
		lowered = l.withTrailingComments(lowered, trailing)
		parts = append(parts, l.arena.HardLine(), l.arena.HardLine(), lowered)
		boundary = declarationEnd
	}
	if suffix := l.commentsBetween(boundary, len(l.physical)); len(suffix) > 0 {
		parts = append(parts, l.arena.HardLine(), l.arena.HardLine(), l.commentsDocument(suffix))
	}
	for index, emitted := range l.emittedComment {
		if !emitted {
			return doc.ID{}, fmt.Errorf("comment %d has no proven output owner", l.comments[index].ID)
		}
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) commentsDocument(owned []source.Comment) doc.ID {
	comments := make([]doc.ID, 0, len(owned))
	for _, comment := range owned {
		comments = append(comments, l.arena.Verbatim(comment.Raw))
	}
	return l.join(l.arena.HardLine(), comments)
}

func (l *lowerer) boundaryCommentsDocument(owned []source.Comment, following int) doc.ID {
	parts := make([]doc.ID, 0, len(owned)*3)
	for index, comment := range owned {
		if index > 0 {
			parts = append(parts, l.commentGap(owned[index-1].Range.End, comment.Range.Start))
		}
		parts = append(parts, l.arena.Verbatim(comment.Raw))
	}
	parts = append(parts, l.commentGap(owned[len(owned)-1].Range.End, following))
	return l.arena.Concat(parts...)
}

func (l *lowerer) commentGap(start, end int) doc.ID {
	if start >= 0 && end >= start && end <= len(l.physical) && bytes.Count(l.physical[start:end], []byte{'\n'}) >= 2 {
		return l.arena.Concat(l.arena.HardLine(), l.arena.HardLine())
	}
	return l.arena.HardLine()
}

func (l *lowerer) consumeCommentGroup(group *ast.CommentGroup) ([]source.Comment, error) {
	comments := make([]source.Comment, 0, len(group.List))
	for _, astComment := range group.List {
		start, found := l.source.PhysicalOffset(astComment.Slash)
		if !found {
			return nil, errors.New("attached comment has no physical offset")
		}
		index, found := l.commentByStart[start]
		if !found {
			return nil, fmt.Errorf("attached comment at byte %d has no stable identity", start)
		}
		if l.emittedComment[index] {
			return nil, fmt.Errorf("comment %d has multiple output owners", l.comments[index].ID)
		}
		l.emittedComment[index] = true
		comments = append(comments, l.comments[index])
	}
	return comments, nil
}

func (l *lowerer) trailingComments(start, limit int) []source.Comment {
	var trailing []source.Comment
	for index, comment := range l.comments {
		if l.emittedComment[index] || comment.Range.Start < start || comment.Range.Start >= limit {
			continue
		}
		if !l.samePhysicalLine(start, comment.Range.Start) {
			continue
		}
		l.emittedComment[index] = true
		trailing = append(trailing, comment)
	}
	return trailing
}

func (l *lowerer) commentsBetween(start, end int) []source.Comment {
	var owned []source.Comment
	for index, comment := range l.comments {
		if l.emittedComment[index] || comment.Range.Start < start || comment.Range.End > end {
			continue
		}
		l.emittedComment[index] = true
		owned = append(owned, comment)
	}
	return owned
}

func (l *lowerer) withTrailingComments(item doc.ID, comments []source.Comment) doc.ID {
	if len(comments) == 0 {
		return item
	}
	suffix := make([]doc.ID, 0, len(comments)*2)
	for _, comment := range comments {
		suffix = append(suffix, l.arena.Text(" "), l.arena.Verbatim(comment.Raw))
	}
	return l.arena.Concat(item, l.arena.LineSuffix(l.arena.Concat(suffix...)))
}

func (l *lowerer) samePhysicalLine(left, right int) bool {
	if left < 0 || right < left || right > len(l.physical) {
		return false
	}
	return !bytes.ContainsRune(l.physical[left:right], '\n')
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

func (l *lowerer) fieldComments(item doc.ID, field *ast.Field) (doc.ID, error) {
	if field.Comment != nil {
		comments, err := l.consumeCommentGroup(field.Comment)
		if err != nil {
			return doc.ID{}, err
		}
		suffix := make([]doc.ID, 0, len(comments)*2)
		for _, comment := range comments {
			suffix = append(suffix, l.arena.Text(" "), l.arena.Verbatim(comment.Raw))
		}
		item = l.arena.Concat(item, l.arena.LineSuffix(l.arena.Concat(suffix...)))
	}
	return item, nil
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
	tail, err := l.blockTail(block)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(l.arena.Text("{"), tail), nil
}

func (l *lowerer) blockTail(block *ast.BlockStmt) (doc.ID, error) {
	opening, openingFound := l.source.PhysicalOffset(block.Lbrace)
	closing, closingFound := l.source.PhysicalOffset(block.Rbrace)
	if !openingFound || !closingFound {
		return doc.ID{}, errors.New("block delimiter has no physical offset")
	}
	statements, err := l.statementRange(block.List, opening+1, closing)
	if err != nil {
		return doc.ID{}, err
	}
	if len(statements) == 0 {
		return l.arena.Text("}"), nil
	}
	return l.arena.Concat(
		l.statementSequence(statements),
		l.arena.HardLine(),
		l.arena.Text("}"),
	), nil
}

type loweredStatement struct {
	document  doc.ID
	outdented bool
}

func (l *lowerer) statementRange(statements []ast.Stmt, boundary, closing int) ([]loweredStatement, error) {
	loweredStatements := make([]loweredStatement, 0, len(statements)+1)
	for index, statement := range statements {
		statementStart, found := l.source.PhysicalOffset(statement.Pos())
		if !found {
			return nil, errors.New("statement has no physical start offset")
		}
		leading := l.commentsBetween(boundary, statementStart)
		limit := closing
		if index+1 < len(statements) {
			limit, found = l.source.PhysicalOffset(statements[index+1].Pos())
			if !found {
				return nil, errors.New("following statement has no physical offset")
			}
		}
		lowered, err := l.statementWithLimit(statement, limit)
		if err != nil {
			return nil, err
		}
		if len(leading) > 0 {
			lowered = l.arena.Concat(l.boundaryCommentsDocument(leading, statementStart), lowered)
		}
		statementEnd, found := l.source.PhysicalOffset(statement.End())
		if !found {
			return nil, errors.New("statement has no physical end offset")
		}
		lowered = l.withTrailingComments(lowered, l.trailingComments(statementEnd, limit))
		outdented := statementIsOutdented(statement)
		loweredStatements = append(loweredStatements, loweredStatement{document: lowered, outdented: outdented})
		boundary = statementEnd
	}
	if trailingBoundary := l.commentsBetween(boundary, closing); len(trailingBoundary) > 0 {
		outdented := len(statements) > 0 && statementIsClause(statements[len(statements)-1])
		loweredStatements = append(loweredStatements, loweredStatement{
			document:  l.commentsDocument(trailingBoundary),
			outdented: outdented,
		})
	}
	return loweredStatements, nil
}

func statementIsOutdented(statement ast.Stmt) bool {
	switch statement.(type) {
	case *ast.LabeledStmt, *ast.CaseClause, *ast.CommClause:
		return true
	default:
		return false
	}
}

func statementIsClause(statement ast.Stmt) bool {
	switch statement.(type) {
	case *ast.CaseClause, *ast.CommClause:
		return true
	default:
		return false
	}
}

func (l *lowerer) statementSequence(statements []loweredStatement) doc.ID {
	parts := make([]doc.ID, 0, len(statements)*2)
	for _, statement := range statements {
		line := l.arena.Concat(l.arena.HardLine(), statement.document)
		if !statement.outdented {
			line = l.arena.Indent(line)
		}
		parts = append(parts, line)
	}
	return l.arena.Concat(parts...)
}

func (l *lowerer) statementWithLimit(statement ast.Stmt, limit int) (doc.ID, error) {
	switch value := statement.(type) {
	case *ast.CaseClause:
		return l.caseClause(value, limit)
	case *ast.CommClause:
		return l.communicationClause(value, limit)
	default:
		return l.statement(statement)
	}
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
	case *ast.SendStmt:
		channel, err := l.expression(value.Chan)
		if err != nil {
			return doc.ID{}, err
		}
		sent, err := l.expression(value.Value)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Group(l.arena.Concat(
			channel,
			l.arena.Text(" <-"),
			l.arena.Indent(l.arena.Concat(l.arena.Line(), sent)),
		)), nil
	case *ast.GoStmt:
		call, err := l.call(value.Call)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("go "), call), nil
	case *ast.DeferStmt:
		call, err := l.call(value.Call)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("defer "), call), nil
	case *ast.ReturnStmt:
		if len(value.Results) == 0 {
			return l.arena.Text("return"), nil
		}
		results, err := l.expressions(value.Results)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("return "), results), nil
	case *ast.DeclStmt:
		return l.declaration(value.Decl)
	case *ast.IfStmt:
		return l.ifStatement(value)
	case *ast.ForStmt:
		return l.forStatement(value)
	case *ast.RangeStmt:
		return l.rangeStatement(value)
	case *ast.BranchStmt:
		if value.Label == nil {
			return l.arena.Text(value.Tok.String()), nil
		}
		return l.arena.Text(value.Tok.String() + " " + value.Label.Name), nil
	case *ast.LabeledStmt:
		return l.labeledStatement(value)
	case *ast.BlockStmt:
		return l.block(value)
	case *ast.SwitchStmt:
		return l.switchStatement(value)
	case *ast.TypeSwitchStmt:
		return l.typeSwitchStatement(value)
	case *ast.SelectStmt:
		body, err := l.block(value.Body)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("select "), body), nil
	case *ast.CaseClause, *ast.CommClause:
		return doc.ID{}, fmt.Errorf("clause %T requires an enclosing boundary", statement)
	case *ast.EmptyStmt:
		return l.arena.Empty(), nil
	default:
		return doc.ID{}, fmt.Errorf("unsupported statement %T", statement)
	}
}

func (l *lowerer) forStatement(statement *ast.ForStmt) (doc.ID, error) {
	classicClause, err := l.hasClassicForClause(statement)
	if err != nil {
		return doc.ID{}, err
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	if classicClause {
		parts := []doc.ID{l.arena.Text("for ")}
		if statement.Init != nil {
			initializer, err := l.statement(statement.Init)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, initializer)
		}
		parts = append(parts, l.arena.Text(";"))
		continuation := []doc.ID{l.arena.Line()}
		if statement.Cond != nil {
			condition, err := l.expression(statement.Cond)
			if err != nil {
				return doc.ID{}, err
			}
			continuation = append(continuation, condition)
		}
		continuation = append(continuation, l.arena.Text(";"))
		if statement.Post != nil {
			post, err := l.statement(statement.Post)
			if err != nil {
				return doc.ID{}, err
			}
			continuation = append(continuation, l.arena.Line(), post)
		}
		parts = append(parts, l.arena.Indent(l.arena.Concat(continuation...)), l.arena.Text(" {"))
		return l.arena.Concat(l.arena.Group(l.arena.Concat(parts...)), tail), nil
	}
	if statement.Cond != nil {
		condition, err := l.expression(statement.Cond)
		if err != nil {
			return doc.ID{}, err
		}
		header := l.arena.Group(l.arena.Concat(
			l.arena.Text("for"),
			l.arena.Indent(l.arena.Concat(l.arena.Line(), condition)),
			l.arena.Text(" {"),
		))
		return l.arena.Concat(header, tail), nil
	}
	return l.arena.Concat(l.arena.Text("for {"), tail), nil
}

func (l *lowerer) hasClassicForClause(statement *ast.ForStmt) (bool, error) {
	start, startFound := l.source.PhysicalOffset(statement.For)
	end, endFound := l.source.PhysicalOffset(statement.Body.Lbrace)
	if !startFound || !endFound {
		return false, errors.New("for clause has no physical boundary")
	}
	parentheses := 0
	brackets := 0
	braces := 0
	semicolons := 0
	first := sort.Search(len(l.tokens), func(index int) bool {
		return l.tokens[index].Range.Start > start
	})
	for _, item := range l.tokens[first:] {
		if item.Range.Start >= end {
			break
		}
		switch item.Kind {
		case token.LPAREN:
			parentheses++
		case token.RPAREN:
			parentheses--
		case token.LBRACK:
			brackets++
		case token.RBRACK:
			brackets--
		case token.LBRACE:
			braces++
		case token.RBRACE:
			braces--
		case token.SEMICOLON:
			if item.Semicolon == source.SemicolonExplicit && parentheses == 0 && brackets == 0 && braces == 0 {
				semicolons++
			}
		}
		if parentheses < 0 || brackets < 0 || braces < 0 {
			return false, errors.New("for clause token nesting is unbalanced")
		}
	}
	if parentheses != 0 || brackets != 0 || braces != 0 {
		return false, errors.New("for clause token nesting is unbalanced")
	}
	switch semicolons {
	case 0:
		return false, nil
	case 2:
		return true, nil
	default:
		return false, fmt.Errorf("for clause contains %d top-level explicit semicolons", semicolons)
	}
}

func (l *lowerer) rangeStatement(statement *ast.RangeStmt) (doc.ID, error) {
	clause := make([]doc.ID, 0, 7)
	if statement.Key != nil {
		key, err := l.expression(statement.Key)
		if err != nil {
			return doc.ID{}, err
		}
		clause = append(clause, key)
		if statement.Value != nil {
			value, err := l.expression(statement.Value)
			if err != nil {
				return doc.ID{}, err
			}
			clause = append(clause, l.arena.Text(", "), value)
		}
		clause = append(clause, l.arena.Text(" "+statement.Tok.String()+" "))
	}
	iterable, err := l.expression(statement.X)
	if err != nil {
		return doc.ID{}, err
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	clause = append(clause, l.arena.Text("range"), l.arena.Line(), iterable)
	header := l.arena.Group(l.arena.Concat(
		l.arena.Text("for"),
		l.arena.Indent(l.arena.Concat(l.arena.Line(), l.arena.Concat(clause...))),
		l.arena.Text(" {"),
	))
	return l.arena.Concat(header, tail), nil
}

func (l *lowerer) labeledStatement(statement *ast.LabeledStmt) (doc.ID, error) {
	if _, empty := statement.Stmt.(*ast.EmptyStmt); empty {
		return l.arena.Text(statement.Label.Name + ":"), nil
	}
	labeled, err := l.statement(statement.Stmt)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(
		l.arena.Text(statement.Label.Name+":"),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), labeled)),
	), nil
}

func (l *lowerer) switchStatement(statement *ast.SwitchStmt) (doc.ID, error) {
	parts := []doc.ID{l.arena.Text("switch")}
	if statement.Init != nil {
		initializer, err := l.statement(statement.Init)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" "), initializer, l.arena.Text(";"))
	}
	if statement.Tag != nil {
		tag, err := l.expression(statement.Tag)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Indent(l.arena.Concat(l.arena.Line(), tag)))
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, l.arena.Text(" {"))
	return l.arena.Concat(l.arena.Group(l.arena.Concat(parts...)), tail), nil
}

func (l *lowerer) typeSwitchStatement(statement *ast.TypeSwitchStmt) (doc.ID, error) {
	parts := []doc.ID{l.arena.Text("switch")}
	if statement.Init != nil {
		initializer, err := l.statement(statement.Init)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Text(" "), initializer, l.arena.Text(";"))
	}
	guard, err := l.typeSwitchGuard(statement.Assign)
	if err != nil {
		return doc.ID{}, err
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, l.arena.Indent(l.arena.Concat(l.arena.Line(), guard)), l.arena.Text(" {"))
	return l.arena.Concat(l.arena.Group(l.arena.Concat(parts...)), tail), nil
}

func (l *lowerer) typeSwitchGuard(statement ast.Stmt) (doc.ID, error) {
	switch value := statement.(type) {
	case *ast.ExprStmt:
		return l.typeSwitchAssertion(value.X)
	case *ast.AssignStmt:
		if len(value.Rhs) != 1 {
			return doc.ID{}, errors.New("type-switch assignment must contain one assertion")
		}
		left, err := l.expressions(value.Lhs)
		if err != nil {
			return doc.ID{}, err
		}
		assertion, err := l.typeSwitchAssertion(value.Rhs[0])
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(left, l.arena.Text(" "+value.Tok.String()+" "), assertion), nil
	default:
		return doc.ID{}, fmt.Errorf("unsupported type-switch guard %T", statement)
	}
}

func (l *lowerer) typeSwitchAssertion(expression ast.Expr) (doc.ID, error) {
	assertion, ok := expression.(*ast.TypeAssertExpr)
	if !ok || assertion.Type != nil {
		return doc.ID{}, fmt.Errorf("type-switch guard contains %T", expression)
	}
	base, err := l.expression(assertion.X)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(base, l.arena.Text(".(type)")), nil
}

func (l *lowerer) caseClause(clause *ast.CaseClause, _ int) (doc.ID, error) {
	parts := make([]doc.ID, 0, 2)
	if len(clause.List) == 0 {
		parts = append(parts, l.arena.Text("default:"))
	} else {
		items := make([]doc.ID, 0, len(clause.List))
		for _, expression := range clause.List {
			item, err := l.expression(expression)
			if err != nil {
				return doc.ID{}, err
			}
			items = append(items, item)
		}
		header := []doc.ID{l.arena.Text("case "), items[0]}
		for _, item := range items[1:] {
			header = append(header, l.arena.Text(","), l.arena.Indent(l.arena.Concat(l.arena.Line(), item)))
		}
		header = append(header, l.arena.Text(":"))
		parts = append(parts, l.arena.Group(l.arena.Concat(header...)))
	}
	colon, found := l.source.PhysicalOffset(clause.Colon)
	if !found {
		return doc.ID{}, errors.New("case clause has no physical colon offset")
	}
	end, found := l.source.PhysicalOffset(clause.End())
	if !found {
		return doc.ID{}, errors.New("case clause has no physical end offset")
	}
	body, err := l.statementRange(clause.Body, colon+1, end)
	if err != nil {
		return doc.ID{}, err
	}
	if len(body) > 0 {
		parts = append(parts, l.statementSequence(body))
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) communicationClause(clause *ast.CommClause, _ int) (doc.ID, error) {
	parts := make([]doc.ID, 0, 2)
	if clause.Comm == nil {
		parts = append(parts, l.arena.Text("default:"))
	} else {
		communication, err := l.communicationStatement(clause.Comm)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Group(l.arena.Concat(
			l.arena.Text("case"),
			l.arena.Indent(l.arena.Concat(l.arena.Line(), communication)),
			l.arena.Text(":"),
		)))
	}
	colon, found := l.source.PhysicalOffset(clause.Colon)
	if !found {
		return doc.ID{}, errors.New("communication clause has no physical colon offset")
	}
	end, found := l.source.PhysicalOffset(clause.End())
	if !found {
		return doc.ID{}, errors.New("communication clause has no physical end offset")
	}
	body, err := l.statementRange(clause.Body, colon+1, end)
	if err != nil {
		return doc.ID{}, err
	}
	if len(body) > 0 {
		parts = append(parts, l.statementSequence(body))
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) communicationStatement(statement ast.Stmt) (doc.ID, error) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok {
		return l.statement(statement)
	}
	left, err := l.expressions(assignment.Lhs)
	if err != nil {
		return doc.ID{}, err
	}
	right, err := l.expressions(assignment.Rhs)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Group(l.arena.Concat(
		left,
		l.arena.Text(" "+assignment.Tok.String()),
		l.arena.Indent(l.arena.Concat(l.arena.Line(), right)),
	)), nil
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
	closing, closingFound := l.source.PhysicalOffset(literal.Rbrace)
	if !closingFound {
		return doc.ID{}, errors.New("composite literal has no physical closing offset")
	}
	for index, rawElement := range literal.Elts {
		element, err := l.expression(rawElement)
		if err != nil {
			return doc.ID{}, err
		}
		elementEnd, found := l.source.PhysicalOffset(rawElement.End())
		if !found {
			return doc.ID{}, errors.New("composite element has no physical end offset")
		}
		limit := closing
		if index+1 < len(literal.Elts) {
			limit, found = l.source.PhysicalOffset(literal.Elts[index+1].Pos())
			if !found {
				return doc.ID{}, errors.New("following composite element has no physical offset")
			}
		}
		if comments := l.commentsBetween(elementEnd, limit); len(comments) > 0 {
			element = l.arena.Concat(l.withTrailingComments(element, comments), l.arena.BreakParent())
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
	if fields == nil {
		return l.arena.Text(keyword + "{}"), nil
	}
	boundary, found := l.source.PhysicalOffset(fields.Opening)
	if !found {
		return doc.ID{}, fmt.Errorf("%s opening delimiter has no physical offset", keyword)
	}
	boundary++
	closing, found := l.source.PhysicalOffset(fields.Closing)
	if !found {
		return doc.ID{}, fmt.Errorf("%s closing delimiter has no physical offset", keyword)
	}
	items := make([]doc.ID, 0, len(fields.List)+1)
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
		fieldStart, found := l.source.PhysicalOffset(field.Pos())
		if !found {
			return doc.ID{}, errors.New("field has no physical start offset")
		}
		if leading := l.commentsBetween(boundary, fieldStart); len(leading) > 0 {
			item = l.arena.Concat(l.boundaryCommentsDocument(leading, fieldStart), item)
		}
		item, err = l.fieldComments(item, field)
		if err != nil {
			return doc.ID{}, err
		}
		items = append(items, item)
		boundary, found = l.source.PhysicalOffset(field.End())
		if !found {
			return doc.ID{}, errors.New("field has no physical end offset")
		}
	}
	if closingComments := l.commentsBetween(boundary, closing); len(closingComments) > 0 {
		items = append(items, l.commentsDocument(closingComments))
	}
	if len(items) == 0 {
		return l.arena.Text(keyword + "{}"), nil
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
	closing, closingFound := l.source.PhysicalOffset(call.Rparen)
	if !closingFound {
		return doc.ID{}, errors.New("call has no physical closing offset")
	}
	arguments := make([]doc.ID, 0, len(call.Args))
	for index, argument := range call.Args {
		lowered, err := l.expression(argument)
		if err != nil {
			return doc.ID{}, err
		}
		argumentEnd, found := l.source.PhysicalOffset(argument.End())
		if !found {
			return doc.ID{}, errors.New("call argument has no physical end offset")
		}
		limit := closing
		if index+1 < len(call.Args) {
			limit, found = l.source.PhysicalOffset(call.Args[index+1].Pos())
			if !found {
				return doc.ID{}, errors.New("following call argument has no physical offset")
			}
		}
		if comments := l.commentsBetween(argumentEnd, limit); len(comments) > 0 {
			lowered = l.arena.Concat(l.withTrailingComments(lowered, comments), l.arena.BreakParent())
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
	operators := make([]*ast.BinaryExpr, 0, 3)
	l.flattenBinary(expression, expression.Op, &operands, &operators)
	items := make([]doc.ID, 0, len(operands))
	for _, operand := range operands {
		lowered, err := l.expression(operand)
		if err != nil {
			return doc.ID{}, err
		}
		items = append(items, lowered)
	}
	for index, operator := range operators {
		operatorStart, found := l.source.PhysicalOffset(operator.OpPos)
		if !found {
			return doc.ID{}, errors.New("binary operator has no physical offset")
		}
		operatorRaw, found := l.source.RawToken(operator.OpPos)
		if !found {
			return doc.ID{}, errors.New("binary operator has no physical token")
		}
		leftEnd, leftFound := l.source.PhysicalOffset(operands[index].End())
		rightStart, rightFound := l.source.PhysicalOffset(operands[index+1].Pos())
		if !leftFound || !rightFound {
			return doc.ID{}, errors.New("binary operand has no physical boundary")
		}
		leftComments := l.commentsBetween(leftEnd, operatorStart)
		rightComments := l.commentsBetween(operatorStart+len(operatorRaw), rightStart)
		leftDocument, err := l.inlineComments(leftComments, true)
		if err != nil {
			return doc.ID{}, err
		}
		rightDocument, err := l.inlineComments(rightComments, false)
		if err != nil {
			return doc.ID{}, err
		}
		items[index] = l.arena.Concat(items[index], leftDocument, l.arena.Text(" "+operator.Op.String()))
		items[index+1] = l.arena.Concat(rightDocument, items[index+1])
	}
	return l.arena.Group(l.arena.Concat(
		items[0],
		l.arena.Indent(l.arena.Concat(l.arena.Line(), l.join(l.arena.Line(), items[1:]))),
	)), nil
}

func (l *lowerer) inlineComments(comments []source.Comment, leadingSpace bool) (doc.ID, error) {
	if len(comments) == 0 {
		return l.arena.Empty(), nil
	}
	parts := make([]doc.ID, 0, len(comments)*2+1)
	if leadingSpace {
		parts = append(parts, l.arena.Text(" "))
	}
	for index, comment := range comments {
		if strings.HasPrefix(comment.Raw, "//") {
			return doc.ID{}, fmt.Errorf("line comment %d requires a proven binary boundary layout", comment.ID)
		}
		if index > 0 {
			parts = append(parts, l.arena.Text(" "))
		}
		parts = append(parts, l.arena.Verbatim(comment.Raw))
	}
	if !leadingSpace {
		parts = append(parts, l.arena.Text(" "))
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) flattenBinary(expression ast.Expr, operator token.Token, operands *[]ast.Expr, operators *[]*ast.BinaryExpr) {
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != operator {
		*operands = append(*operands, expression)
		return
	}
	l.flattenBinary(binary.X, operator, operands, operators)
	*operators = append(*operators, binary)
	l.flattenBinary(binary.Y, operator, operands, operators)
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
