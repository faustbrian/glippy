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
	return l.arena.Concat(
		l.boundaryCommentsBody(owned),
		l.commentGap(owned[len(owned)-1].Range.End, following),
	)
}

func (l *lowerer) boundaryCommentsBody(owned []source.Comment) doc.ID {
	parts := make([]doc.ID, 0, len(owned)*3)
	for index, comment := range owned {
		if index > 0 {
			parts = append(parts, l.commentGap(owned[index-1].Range.End, comment.Range.Start))
		}
		parts = append(parts, l.arena.Verbatim(comment.Raw))
	}
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

func (l *lowerer) uniqueTokenBetween(kind token.Token, start, end int) (source.Token, error) {
	if start < 0 || end < start || end > len(l.physical) {
		return source.Token{}, errors.New("token lookup has an invalid physical range")
	}
	first := sort.Search(len(l.tokens), func(index int) bool {
		return l.tokens[index].Range.Start >= start
	})
	var result source.Token
	found := false
	for _, item := range l.tokens[first:] {
		if item.Range.Start >= end {
			break
		}
		if item.Kind != kind {
			continue
		}
		if found {
			return source.Token{}, fmt.Errorf("physical range contains multiple %s tokens", kind)
		}
		result = item
		found = true
	}
	if !found {
		return source.Token{}, fmt.Errorf("physical range contains no %s token", kind)
	}
	return result, nil
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
	keyword := declaration.Tok.String()
	if !declaration.Lparen.IsValid() {
		if len(declaration.Specs) != 1 {
			return doc.ID{}, fmt.Errorf("ungrouped %s declaration must contain one spec", keyword)
		}
		spec, err := l.generalSpec(declaration.Tok, declaration.Specs[0])
		if err != nil {
			return doc.ID{}, err
		}
		return l.keywordWithOperand(declaration.TokPos, keyword, declaration.Specs[0], spec)
	}
	opening, openingFound := l.source.PhysicalOffset(declaration.Lparen)
	closing, closingFound := l.source.PhysicalOffset(declaration.Rparen)
	keywordOffset, keywordFound := l.source.PhysicalOffset(declaration.TokPos)
	if !openingFound || !closingFound || !keywordFound {
		return doc.ID{}, fmt.Errorf("grouped %s declaration has no physical boundary", keyword)
	}
	beforeOpening := l.commentsBetween(keywordOffset+len(keyword), opening)
	hasLineComment := false
	for _, comment := range beforeOpening {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	var header doc.ID
	if hasLineComment {
		header = l.arena.Concat(
			l.arena.Text(keyword),
			l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsBody(beforeOpening),
			)),
			l.commentGap(beforeOpening[len(beforeOpening)-1].Range.End, opening),
			l.arena.Text("("),
		)
	} else {
		beforeOpeningDocument, err := l.inlineComments(beforeOpening, true)
		if err != nil {
			return doc.ID{}, err
		}
		header = l.arena.Concat(l.arena.Text(keyword), beforeOpeningDocument, l.arena.Text(" ("))
	}
	rows := make([]doc.ID, 0, len(declaration.Specs)+1)
	boundary := opening + len("(")
	for index, rawSpec := range declaration.Specs {
		specStart, startFound := l.source.PhysicalOffset(rawSpec.Pos())
		specEnd, endFound := l.source.PhysicalOffset(rawSpec.End())
		if !startFound || !endFound {
			return doc.ID{}, fmt.Errorf("%s declaration spec has no physical boundary", keyword)
		}
		limit := closing
		if index+1 < len(declaration.Specs) {
			limit, startFound = l.source.PhysicalOffset(declaration.Specs[index+1].Pos())
			if !startFound {
				return doc.ID{}, fmt.Errorf("%s declaration following spec has no physical boundary", keyword)
			}
		}
		leading := l.commentsBetween(boundary, specStart)
		lowered, err := l.generalSpec(declaration.Tok, rawSpec)
		if err != nil {
			return doc.ID{}, err
		}
		lowered = l.withTrailingComments(lowered, l.trailingComments(specEnd, limit))
		if len(leading) > 0 {
			lowered = l.arena.Concat(l.boundaryCommentsDocument(leading, specStart), lowered)
		}
		rows = append(rows, lowered)
		boundary = specEnd
	}
	if closingComments := l.commentsBetween(boundary, closing); len(closingComments) > 0 {
		rows = append(rows, l.boundaryCommentsBody(closingComments))
	}
	if len(rows) == 0 {
		return l.arena.Concat(header, l.arena.Text(")")), nil
	}
	return l.arena.Concat(
		header,
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.join(l.arena.HardLine(), rows))),
		l.arena.HardLine(),
		l.arena.Text(")"),
	), nil
}

func (l *lowerer) generalSpec(kind token.Token, spec ast.Spec) (doc.ID, error) {
	switch value := spec.(type) {
	case *ast.ValueSpec:
		return l.valueSpec(value)
	case *ast.TypeSpec:
		return l.typeSpec(value)
	default:
		return doc.ID{}, fmt.Errorf("%s declaration contains %T", kind, spec)
	}
}

func (l *lowerer) valueSpec(spec *ast.ValueSpec) (doc.ID, error) {
	if len(spec.Names) == 0 {
		return doc.ID{}, errors.New("value specification requires a name")
	}
	parts := []doc.ID{l.arena.Text(spec.Names[0].Name)}
	boundary, boundaryFound := l.source.PhysicalOffset(spec.Names[0].End())
	if !boundaryFound {
		return doc.ID{}, errors.New("value specification name has no physical boundary")
	}
	for index := 1; index < len(spec.Names); index++ {
		name := spec.Names[index]
		nameStart, startFound := l.source.PhysicalOffset(name.Pos())
		nameEnd, endFound := l.source.PhysicalOffset(name.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("value specification name has no physical boundary")
		}
		comma, err := l.uniqueTokenBetween(token.COMMA, boundary, nameStart)
		if err != nil {
			return doc.ID{}, fmt.Errorf("value specification name boundary: %w", err)
		}
		afterPrevious, err := l.inlineComments(l.commentsBetween(boundary, comma.Range.Start), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, afterPrevious, l.arena.Text(","))
		beforeName := l.commentsBetween(comma.Range.End, nameStart)
		hasLineComment := false
		for _, comment := range beforeName {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(beforeName, nameStart),
				l.arena.Text(name.Name),
			)))
		} else {
			beforeNameDocument, err := l.inlineComments(beforeName, true)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, beforeNameDocument, l.arena.Text(" "), l.arena.Text(name.Name))
		}
		boundary = nameEnd
	}
	if spec.Type != nil {
		typeDocument, err := l.expression(spec.Type)
		if err != nil {
			return doc.ID{}, err
		}
		typeStart, startFound := l.source.PhysicalOffset(spec.Type.Pos())
		typeEnd, endFound := l.source.PhysicalOffset(spec.Type.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("value specification type has no physical boundary")
		}
		beforeType, err := l.inlineComments(l.commentsBetween(boundary, typeStart), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, beforeType, l.arena.Text(" "), typeDocument)
		boundary = typeEnd
	}
	if len(spec.Values) > 0 {
		values, err := l.expressions(spec.Values)
		if err != nil {
			return doc.ID{}, err
		}
		valuesStart, found := l.source.PhysicalOffset(spec.Values[0].Pos())
		if !found {
			return doc.ID{}, errors.New("value specification values have no physical boundary")
		}
		assign, err := l.uniqueTokenBetween(token.ASSIGN, boundary, valuesStart)
		if err != nil {
			return doc.ID{}, fmt.Errorf("value specification assignment boundary: %w", err)
		}
		beforeAssign, err := l.inlineComments(l.commentsBetween(boundary, assign.Range.Start), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, beforeAssign, l.arena.Text(" ="))
		afterAssign := l.commentsBetween(assign.Range.End, valuesStart)
		hasLineComment := false
		for _, comment := range afterAssign {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(afterAssign, valuesStart),
				values,
			)))
		} else {
			afterAssignDocument, err := l.inlineComments(afterAssign, true)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, afterAssignDocument, l.arena.Text(" "), values)
		}
	}
	return l.arena.Group(l.arena.Concat(parts...)), nil
}

func (l *lowerer) typeSpec(spec *ast.TypeSpec) (doc.ID, error) {
	parts := []doc.ID{l.arena.Text(spec.Name.Name)}
	boundary, boundaryFound := l.source.PhysicalOffset(spec.Name.End())
	if !boundaryFound {
		return doc.ID{}, errors.New("type specification name has no physical boundary")
	}
	if spec.TypeParams != nil {
		typeParameters, err := l.fieldListWithDelimiters(spec.TypeParams, "[", "]")
		if err != nil {
			return doc.ID{}, err
		}
		parametersStart, startFound := l.source.PhysicalOffset(spec.TypeParams.Pos())
		parametersEnd, endFound := l.source.PhysicalOffset(spec.TypeParams.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("type specification parameters have no physical boundary")
		}
		beforeParameters, err := l.inlineComments(l.commentsBetween(boundary, parametersStart), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, beforeParameters, typeParameters)
		boundary = parametersEnd
	}
	typeDocument, err := l.expression(spec.Type)
	if err != nil {
		return doc.ID{}, err
	}
	typeStart, startFound := l.source.PhysicalOffset(spec.Type.Pos())
	if !startFound {
		return doc.ID{}, errors.New("type specification underlying type has no physical boundary")
	}
	if spec.Assign.IsValid() {
		assignOffset, assignFound := l.source.PhysicalOffset(spec.Assign)
		if !assignFound {
			return doc.ID{}, errors.New("type alias has no physical assignment boundary")
		}
		beforeAssign, err := l.inlineComments(l.commentsBetween(boundary, assignOffset), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, beforeAssign, l.arena.Text(" ="))
		afterAssign := l.commentsBetween(assignOffset+len("="), typeStart)
		hasLineComment := false
		for _, comment := range afterAssign {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(afterAssign, typeStart),
				typeDocument,
			)))
		} else {
			afterAssignDocument, err := l.inlineComments(afterAssign, true)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, afterAssignDocument, l.arena.Text(" "), typeDocument)
		}
		return l.arena.Group(l.arena.Concat(parts...)), nil
	}
	beforeType, err := l.inlineComments(l.commentsBetween(boundary, typeStart), true)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, beforeType, l.arena.Text(" "), typeDocument)
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) importDeclaration(declaration *ast.GenDecl) (doc.ID, error) {
	if !declaration.Lparen.IsValid() {
		if len(declaration.Specs) != 1 {
			return doc.ID{}, errors.New("ungrouped import declaration must contain one spec")
		}
		spec, ok := declaration.Specs[0].(*ast.ImportSpec)
		if !ok {
			return doc.ID{}, fmt.Errorf("import declaration contains %T", declaration.Specs[0])
		}
		item, err := l.importSpec(spec)
		if err != nil {
			return doc.ID{}, err
		}
		start, found := l.source.PhysicalOffset(spec.Pos())
		if !found {
			return doc.ID{}, errors.New("import specification has no physical start offset")
		}
		return l.importHeader(declaration, start, item)
	}
	opening, openingFound := l.source.PhysicalOffset(declaration.Lparen)
	closing, closingFound := l.source.PhysicalOffset(declaration.Rparen)
	if !openingFound || !closingFound {
		return doc.ID{}, errors.New("import group has no physical boundary")
	}
	rows := make([]doc.ID, 0, len(declaration.Specs)+1)
	blankBefore := make([]bool, 0, len(declaration.Specs)+1)
	boundary := opening + len("(")
	for index, rawSpec := range declaration.Specs {
		spec, ok := rawSpec.(*ast.ImportSpec)
		if !ok {
			return doc.ID{}, fmt.Errorf("import declaration contains %T", rawSpec)
		}
		item, err := l.importSpec(spec)
		if err != nil {
			return doc.ID{}, err
		}
		specStart, startFound := l.source.PhysicalOffset(spec.Pos())
		specEnd, endFound := l.source.PhysicalOffset(spec.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("import specification has no physical boundary")
		}
		leading := l.commentsBetween(boundary, specStart)
		gapEnd := specStart
		if len(leading) > 0 {
			gapEnd = leading[0].Range.Start
		}
		blank, err := l.hasBlankPhysicalGap(boundary, gapEnd)
		if err != nil {
			return doc.ID{}, err
		}
		limit := closing
		if index+1 < len(declaration.Specs) {
			limit, endFound = l.source.PhysicalOffset(declaration.Specs[index+1].Pos())
			if !endFound {
				return doc.ID{}, errors.New("following import specification has no physical boundary")
			}
		}
		item = l.withTrailingComments(item, l.trailingComments(specEnd, limit))
		if len(leading) > 0 {
			item = l.arena.Concat(l.boundaryCommentsDocument(leading, specStart), item)
		}
		rows = append(rows, item)
		blankBefore = append(blankBefore, blank)
		boundary = specEnd
	}
	if closingComments := l.commentsBetween(boundary, closing); len(closingComments) > 0 {
		blank, err := l.hasBlankPhysicalGap(boundary, closingComments[0].Range.Start)
		if err != nil {
			return doc.ID{}, err
		}
		rows = append(rows, l.boundaryCommentsBody(closingComments))
		blankBefore = append(blankBefore, blank)
	}
	body := make([]doc.ID, 0, len(rows)*3)
	for index, row := range rows {
		if index > 0 {
			body = append(body, l.arena.HardLine())
		}
		if blankBefore[index] {
			body = append(body, l.arena.HardLine())
		}
		body = append(body, row)
	}
	group := l.arena.Text("(")
	if len(rows) > 0 {
		group = l.arena.Concat(
			group,
			l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.arena.Concat(body...))),
			l.arena.HardLine(),
			l.arena.Text(")"),
		)
	} else {
		group = l.arena.Text("()")
	}
	return l.importHeader(declaration, opening, group)
}

func (l *lowerer) importSpec(spec *ast.ImportSpec) (doc.ID, error) {
	path, found := l.source.RawToken(spec.Path.Pos())
	if !found {
		return doc.ID{}, errors.New("import path has no physical token")
	}
	if spec.Name == nil {
		return l.arena.Verbatim(path), nil
	}
	nameEnd, nameFound := l.source.PhysicalOffset(spec.Name.End())
	pathStart, pathFound := l.source.PhysicalOffset(spec.Path.Pos())
	if !nameFound || !pathFound {
		return doc.ID{}, errors.New("import alias has no physical path boundary")
	}
	beforePath, err := l.inlineComments(l.commentsBetween(nameEnd, pathStart), true)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(
		l.arena.Text(spec.Name.Name),
		beforePath,
		l.arena.Text(" "),
		l.arena.Verbatim(path),
	), nil
}

func (l *lowerer) importHeader(declaration *ast.GenDecl, following int, operand doc.ID) (doc.ID, error) {
	keyword, found := l.source.PhysicalOffset(declaration.TokPos)
	if !found {
		return doc.ID{}, errors.New("import declaration has no physical keyword boundary")
	}
	boundary := keyword + len("import")
	comments := l.commentsBetween(boundary, following)
	if len(comments) == 0 {
		return l.arena.Concat(l.arena.Text("import "), operand), nil
	}
	parts := []doc.ID{l.arena.Text("import")}
	previousWasLineComment := false
	for _, comment := range comments {
		if !previousWasLineComment && l.samePhysicalLine(boundary, comment.Range.Start) {
			parts = append(parts, l.arena.Text(" "))
		} else {
			parts = append(parts, l.commentGap(boundary, comment.Range.Start))
		}
		parts = append(parts, l.arena.Verbatim(comment.Raw))
		boundary = comment.Range.End
		previousWasLineComment = strings.HasPrefix(comment.Raw, "//")
	}
	if !previousWasLineComment && l.samePhysicalLine(boundary, following) {
		parts = append(parts, l.arena.Text(" "))
	} else {
		parts = append(parts, l.commentGap(boundary, following))
	}
	parts = append(parts, operand)
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) hasBlankPhysicalGap(start, end int) (bool, error) {
	gap, valid := l.source.Slice(source.Range{Start: start, End: end})
	if !valid {
		return false, errors.New("import group has an invalid physical gap")
	}
	return strings.Count(gap, "\n") >= 2, nil
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
	if fields == nil {
		return l.arena.Text(open + close), nil
	}
	items := make([]delimitedItem, 0, len(fields.List))
	for _, field := range fields.List {
		item, err := l.field(field)
		if err != nil {
			return doc.ID{}, err
		}
		start, startFound := l.source.PhysicalOffset(field.Pos())
		end, endFound := l.source.PhysicalOffset(field.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("field list item has no physical range")
		}
		items = append(items, delimitedItem{document: item, start: start, end: end})
	}
	return l.delimitedCommaList(fields.Opening, fields.Closing, open, close, items)
}

type delimitedItem struct {
	document doc.ID
	start    int
	end      int
}

func (l *lowerer) delimitedCommaList(
	openingPosition token.Pos,
	closingPosition token.Pos,
	open string,
	close string,
	items []delimitedItem,
) (doc.ID, error) {
	opening, openingFound := l.source.PhysicalOffset(openingPosition)
	closing, closingFound := l.source.PhysicalOffset(closingPosition)
	if !openingFound || !closingFound {
		return doc.ID{}, errors.New("delimited list has no physical boundary")
	}
	boundary := opening + len(open)
	rows := make([]doc.ID, 0, len(items)+1)
	plain := make([]doc.ID, 0, len(items))
	hasComments := false
	for index, item := range items {
		leading := l.commentsBetween(boundary, item.start)
		limit := closing
		if index+1 < len(items) {
			limit = items[index+1].start
		}
		trailing := l.trailingComments(item.end, limit)
		hasComments = hasComments || len(leading) > 0 || len(trailing) > 0
		row := l.withTrailingComments(item.document, trailing)
		row = l.arena.Concat(row, l.arena.Text(","))
		if len(leading) > 0 {
			row = l.arena.Concat(l.boundaryCommentsDocument(leading, item.start), row)
		}
		rows = append(rows, row)
		plain = append(plain, item.document)
		boundary = item.end
	}
	closingComments := l.commentsBetween(boundary, closing)
	if len(closingComments) > 0 {
		hasComments = true
		rows = append(rows, l.commentsDocument(closingComments))
	}
	if !hasComments {
		if len(plain) == 0 {
			return l.arena.Text(open + close), nil
		}
		return l.commaList(open, close, plain), nil
	}
	return l.arena.Concat(
		l.arena.Text(open),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.join(l.arena.HardLine(), rows))),
		l.arena.HardLine(),
		l.arena.Text(close),
	), nil
}

func (l *lowerer) delimitedSingle(
	openingPosition token.Pos,
	closingPosition token.Pos,
	open string,
	close string,
	item delimitedItem,
) (doc.ID, error) {
	opening, openingFound := l.source.PhysicalOffset(openingPosition)
	closing, closingFound := l.source.PhysicalOffset(closingPosition)
	if !openingFound || !closingFound {
		return doc.ID{}, errors.New("single-item delimited list has no physical boundary")
	}
	leading := l.commentsBetween(opening+len(open), item.start)
	trailing := l.commentsBetween(item.end, closing)
	if len(leading) == 0 && len(trailing) == 0 {
		return l.arena.Concat(l.arena.Text(open), item.document, l.arena.Text(close)), nil
	}
	hasTrailingLineComment := false
	for _, comment := range trailing {
		hasTrailingLineComment = hasTrailingLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasTrailingLineComment {
		body := item.document
		if len(leading) > 0 {
			body = l.arena.Concat(l.boundaryCommentsDocument(leading, item.start), body)
		}
		boundary := item.end
		previousWasLineComment := false
		for _, comment := range trailing {
			if !previousWasLineComment && l.samePhysicalLine(boundary, comment.Range.Start) {
				body = l.arena.Concat(body, l.arena.Text(" "), l.arena.Verbatim(comment.Raw))
			} else {
				body = l.arena.Concat(body, l.commentGap(boundary, comment.Range.Start), l.arena.Verbatim(comment.Raw))
			}
			boundary = comment.Range.End
			previousWasLineComment = strings.HasPrefix(comment.Raw, "//")
		}
		return l.arena.Concat(
			l.arena.Text(open),
			l.arena.Indent(l.arena.Concat(l.arena.HardLine(), body)),
			l.arena.HardLine(),
			l.arena.Text(close),
		), nil
	}
	trailingDocument, err := l.inlineComments(trailing, true)
	if err != nil {
		return doc.ID{}, err
	}
	body := item.document
	if len(leading) > 0 {
		body = l.arena.Concat(l.boundaryCommentsDocument(leading, item.start), body)
	}
	return l.arena.Concat(
		l.arena.Text(open),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), body)),
		trailingDocument,
		l.arena.Text(close),
	), nil
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
	return l.fieldWithType(field, typeDocument, " ")
}

func (l *lowerer) fieldWithType(field *ast.Field, typeDocument doc.ID, separator string) (doc.ID, error) {
	typeStart, startFound := l.source.PhysicalOffset(field.Type.Pos())
	typeEnd, endFound := l.source.PhysicalOffset(field.Type.End())
	if !startFound || !endFound {
		return doc.ID{}, errors.New("field type has no physical boundary")
	}
	parts := make([]doc.ID, 0, len(field.Names)*4+4)
	boundary := typeEnd
	if len(field.Names) == 0 {
		parts = append(parts, typeDocument)
	} else {
		parts = append(parts, l.arena.Text(field.Names[0].Name))
		boundary, endFound = l.source.PhysicalOffset(field.Names[0].End())
		if !endFound {
			return doc.ID{}, errors.New("field name has no physical boundary")
		}
		for index := 1; index < len(field.Names); index++ {
			name := field.Names[index]
			nameStart, nameStartFound := l.source.PhysicalOffset(name.Pos())
			nameEnd, nameEndFound := l.source.PhysicalOffset(name.End())
			if !nameStartFound || !nameEndFound {
				return doc.ID{}, errors.New("field name has no physical boundary")
			}
			comma, err := l.uniqueTokenBetween(token.COMMA, boundary, nameStart)
			if err != nil {
				return doc.ID{}, fmt.Errorf("field name boundary: %w", err)
			}
			beforeComma, err := l.inlineComments(l.commentsBetween(boundary, comma.Range.Start), true)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, beforeComma, l.arena.Text(","))
			beforeName := l.commentsBetween(comma.Range.End, nameStart)
			hasLineComment := false
			for _, comment := range beforeName {
				hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
			}
			if hasLineComment {
				parts = append(parts, l.arena.Indent(l.arena.Concat(
					l.arena.HardLine(),
					l.boundaryCommentsDocument(beforeName, nameStart),
					l.arena.Text(name.Name),
				)))
			} else {
				beforeNameDocument, err := l.inlineComments(beforeName, true)
				if err != nil {
					return doc.ID{}, err
				}
				parts = append(parts, beforeNameDocument, l.arena.Text(" "), l.arena.Text(name.Name))
			}
			boundary = nameEnd
		}
		beforeType, err := l.inlineComments(l.commentsBetween(boundary, typeStart), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, beforeType, l.arena.Text(separator), typeDocument)
		boundary = typeEnd
	}
	if field.Tag != nil {
		tag, found := l.source.RawToken(field.Tag.Pos())
		if !found {
			return doc.ID{}, errors.New("field tag has no physical token")
		}
		tagStart, startFound := l.source.PhysicalOffset(field.Tag.Pos())
		if !startFound {
			return doc.ID{}, errors.New("field tag has no physical boundary")
		}
		beforeTag, err := l.inlineComments(l.commentsBetween(boundary, tagStart), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, beforeTag, l.arena.Text(" "), l.arena.Verbatim(tag))
	}
	return l.arena.Group(l.arena.Concat(parts...)), nil
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
		return l.incrementOrDecrement(value)
	case *ast.SendStmt:
		return l.sendStatement(value)
	case *ast.GoStmt:
		call, err := l.call(value.Call)
		if err != nil {
			return doc.ID{}, err
		}
		return l.keywordWithOperand(value.Go, "go", value.Call, call)
	case *ast.DeferStmt:
		call, err := l.call(value.Call)
		if err != nil {
			return doc.ID{}, err
		}
		return l.keywordWithOperand(value.Defer, "defer", value.Call, call)
	case *ast.ReturnStmt:
		if len(value.Results) == 0 {
			return l.arena.Text("return"), nil
		}
		results, err := l.expressions(value.Results)
		if err != nil {
			return doc.ID{}, err
		}
		return l.keywordWithOperand(value.Return, "return", value.Results[0], results)
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
		return l.keywordWithOperand(
			value.TokPos,
			value.Tok.String(),
			value.Label,
			l.arena.Text(value.Label.Name),
		)
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

func (l *lowerer) keywordWithOperand(
	keywordPosition token.Pos,
	keyword string,
	operand ast.Node,
	operandDocument doc.ID,
) (doc.ID, error) {
	keywordOffset, keywordFound := l.source.PhysicalOffset(keywordPosition)
	operandStart, operandFound := l.source.PhysicalOffset(operand.Pos())
	if !keywordFound || !operandFound {
		return doc.ID{}, fmt.Errorf("%s statement has no physical operand boundary", keyword)
	}
	comments := l.commentsBetween(keywordOffset+len(keyword), operandStart)
	hasLineComment := false
	for _, comment := range comments {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLineComment {
		return l.arena.Concat(
			l.arena.Text(keyword),
			l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(comments, operandStart),
				operandDocument,
			)),
		), nil
	}
	commentsDocument, err := l.inlineComments(comments, true)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(l.arena.Text(keyword), commentsDocument, l.arena.Text(" "), operandDocument), nil
}

func (l *lowerer) incrementOrDecrement(statement *ast.IncDecStmt) (doc.ID, error) {
	expression, err := l.expression(statement.X)
	if err != nil {
		return doc.ID{}, err
	}
	expressionEnd, expressionEndFound := l.source.PhysicalOffset(statement.X.End())
	operatorOffset, operatorFound := l.source.PhysicalOffset(statement.TokPos)
	if !expressionEndFound || !operatorFound {
		return doc.ID{}, errors.New("increment or decrement has no physical operator boundary")
	}
	beforeOperator, err := l.inlineComments(l.commentsBetween(expressionEnd, operatorOffset), true)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(expression, beforeOperator, l.arena.Text(statement.Tok.String())), nil
}

func (l *lowerer) forStatement(statement *ast.ForStmt) (doc.ID, error) {
	semicolons, classicClause, err := l.classicForSemicolons(statement)
	if err != nil {
		return doc.ID{}, err
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	if classicClause {
		forOffset, forFound := l.source.PhysicalOffset(statement.For)
		braceOffset, braceFound := l.source.PhysicalOffset(statement.Body.Lbrace)
		if !forFound || !braceFound {
			return doc.ID{}, errors.New("classic for header has no physical boundary")
		}

		parts := []doc.ID{l.arena.Text("for ")}
		var initializer doc.ID
		initializerStart := semicolons[0].Range.Start
		initializerEnd := initializerStart
		if statement.Init != nil {
			initializer, err = l.statement(statement.Init)
			if err != nil {
				return doc.ID{}, err
			}
			initializerStart, forFound = l.source.PhysicalOffset(statement.Init.Pos())
			initializerEnd, braceFound = l.source.PhysicalOffset(statement.Init.End())
			if !forFound || !braceFound {
				return doc.ID{}, errors.New("classic for initializer has no physical boundary")
			}
			parts = append(parts, initializer)
		}
		parts = append(parts, l.arena.Text(";"))

		continuation := []doc.ID{l.arena.Line()}
		var condition doc.ID
		conditionStart := semicolons[1].Range.Start
		conditionEnd := conditionStart
		if statement.Cond != nil {
			condition, err = l.expression(statement.Cond)
			if err != nil {
				return doc.ID{}, err
			}
			conditionStart, forFound = l.source.PhysicalOffset(statement.Cond.Pos())
			conditionEnd, braceFound = l.source.PhysicalOffset(statement.Cond.End())
			if !forFound || !braceFound {
				return doc.ID{}, errors.New("classic for condition has no physical boundary")
			}
			continuation = append(continuation, condition)
		}
		continuation = append(continuation, l.arena.Text(";"))

		var post doc.ID
		postStart := braceOffset
		postEnd := postStart
		if statement.Post != nil {
			post, err = l.statement(statement.Post)
			if err != nil {
				return doc.ID{}, err
			}
			postStart, forFound = l.source.PhysicalOffset(statement.Post.Pos())
			postEnd, braceFound = l.source.PhysicalOffset(statement.Post.End())
			if !forFound || !braceFound {
				return doc.ID{}, errors.New("classic for post statement has no physical boundary")
			}
			continuation = append(continuation, l.arena.Line(), post)
		}

		leadingInitializer := l.commentsBetween(forOffset+len("for"), initializerStart)
		betweenInitializerAndCondition := l.commentsBetween(initializerEnd, conditionStart)
		betweenConditionAndPost := l.commentsBetween(conditionEnd, postStart)
		trailingPost := l.commentsBetween(postEnd, braceOffset)
		hasComments := len(leadingInitializer)+len(betweenInitializerAndCondition)+
			len(betweenConditionAndPost)+len(trailingPost) > 0
		if hasComments {
			trailingPostDocument, err := l.inlineComments(trailingPost, true)
			if err != nil {
				return doc.ID{}, err
			}
			header := []doc.ID{l.arena.Text("for")}
			rows := make([]doc.ID, 0, 16)
			if len(leadingInitializer) > 0 {
				rows = append(rows,
					l.arena.HardLine(),
					l.boundaryCommentsDocument(leadingInitializer, initializerStart),
				)
				if statement.Init != nil {
					rows = append(rows, initializer)
				}
				rows = append(rows, l.arena.Text(";"))
			} else {
				header = append(header, l.arena.Text(" "))
				if statement.Init != nil {
					header = append(header, initializer)
				}
				header = append(header, l.arena.Text(";"))
				rows = append(rows, l.arena.HardLine())
			}
			if len(leadingInitializer) > 0 {
				rows = append(rows, l.arena.HardLine())
			}
			if len(betweenInitializerAndCondition) > 0 {
				rows = append(rows, l.boundaryCommentsDocument(betweenInitializerAndCondition, conditionStart))
			}
			if statement.Cond != nil {
				rows = append(rows, condition)
			}
			rows = append(rows, l.arena.Text(";"))
			if statement.Post != nil || len(betweenConditionAndPost) > 0 {
				rows = append(rows, l.arena.HardLine())
			}
			if len(betweenConditionAndPost) > 0 {
				if statement.Post != nil {
					rows = append(rows, l.boundaryCommentsDocument(betweenConditionAndPost, postStart))
				} else {
					rows = append(rows, l.boundaryCommentsBody(betweenConditionAndPost))
				}
			}
			if statement.Post != nil {
				rows = append(rows, post, trailingPostDocument, l.arena.Text(" {"))
			} else if len(betweenConditionAndPost) > 0 {
				header = append(header, l.arena.Indent(l.arena.Concat(rows...)))
				header = append(header,
					l.commentGap(betweenConditionAndPost[len(betweenConditionAndPost)-1].Range.End, braceOffset),
					l.arena.Text("{"),
				)
				return l.arena.Concat(l.arena.Concat(header...), tail), nil
			} else {
				rows = append(rows, trailingPostDocument, l.arena.Text(" {"))
			}
			header = append(header, l.arena.Indent(l.arena.Concat(rows...)))
			return l.arena.Concat(l.arena.Concat(header...), tail), nil
		}
		parts = append(parts, l.arena.Indent(l.arena.Concat(continuation...)), l.arena.Text(" {"))
		return l.arena.Concat(l.arena.Group(l.arena.Concat(parts...)), tail), nil
	}
	if statement.Cond != nil {
		condition, err := l.expression(statement.Cond)
		if err != nil {
			return doc.ID{}, err
		}
		forOffset, forFound := l.source.PhysicalOffset(statement.For)
		conditionStart, startFound := l.source.PhysicalOffset(statement.Cond.Pos())
		conditionEnd, endFound := l.source.PhysicalOffset(statement.Cond.End())
		braceOffset, braceFound := l.source.PhysicalOffset(statement.Body.Lbrace)
		if !forFound || !startFound || !endFound || !braceFound {
			return doc.ID{}, errors.New("for condition has no physical boundary")
		}
		leading := l.commentsBetween(forOffset+len("for"), conditionStart)
		trailingDocument, err := l.inlineComments(l.commentsBetween(conditionEnd, braceOffset), true)
		if err != nil {
			return doc.ID{}, err
		}
		var header doc.ID
		if len(leading) > 0 {
			header = l.arena.Concat(
				l.arena.Text("for"),
				l.arena.Indent(l.arena.Concat(
					l.arena.HardLine(),
					l.boundaryCommentsDocument(leading, conditionStart),
					condition,
				)),
				trailingDocument,
				l.arena.Text(" {"),
			)
		} else {
			header = l.arena.Group(l.arena.Concat(
				l.arena.Text("for"),
				l.arena.Indent(l.arena.Concat(l.arena.Line(), condition)),
				trailingDocument,
				l.arena.Text(" {"),
			))
		}
		return l.arena.Concat(header, tail), nil
	}
	forOffset, forFound := l.source.PhysicalOffset(statement.For)
	braceOffset, braceFound := l.source.PhysicalOffset(statement.Body.Lbrace)
	if !forFound || !braceFound {
		return doc.ID{}, errors.New("infinite for header has no physical boundary")
	}
	headerComments := l.commentsBetween(forOffset+len("for"), braceOffset)
	if len(headerComments) > 0 {
		hasLineComment := false
		for _, comment := range headerComments {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if !hasLineComment {
			commentsDocument, err := l.inlineComments(headerComments, true)
			if err != nil {
				return doc.ID{}, err
			}
			return l.arena.Concat(l.arena.Text("for"), commentsDocument, l.arena.Text(" {"), tail), nil
		}
		return l.arena.Concat(
			l.arena.Text("for"),
			l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsBody(headerComments),
			)),
			l.commentGap(headerComments[len(headerComments)-1].Range.End, braceOffset),
			l.arena.Text("{"),
			tail,
		), nil
	}
	return l.arena.Concat(l.arena.Text("for {"), tail), nil
}

func (l *lowerer) classicForSemicolons(statement *ast.ForStmt) ([2]source.Token, bool, error) {
	var result [2]source.Token
	start, startFound := l.source.PhysicalOffset(statement.For)
	end, endFound := l.source.PhysicalOffset(statement.Body.Lbrace)
	if !startFound || !endFound {
		return result, false, errors.New("for clause has no physical boundary")
	}
	parentheses := 0
	brackets := 0
	braces := 0
	semicolons := make([]source.Token, 0, 2)
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
				semicolons = append(semicolons, item)
			}
		}
		if parentheses < 0 || brackets < 0 || braces < 0 {
			return result, false, errors.New("for clause token nesting is unbalanced")
		}
	}
	if parentheses != 0 || brackets != 0 || braces != 0 {
		return result, false, errors.New("for clause token nesting is unbalanced")
	}
	switch len(semicolons) {
	case 0:
		return result, false, nil
	case 2:
		return [2]source.Token{semicolons[0], semicolons[1]}, true, nil
	default:
		return result, false, fmt.Errorf("for clause contains %d top-level explicit semicolons", len(semicolons))
	}
}

func (l *lowerer) rangeStatement(statement *ast.RangeStmt) (doc.ID, error) {
	forOffset, forFound := l.source.PhysicalOffset(statement.For)
	rangeOffset, rangeFound := l.source.PhysicalOffset(statement.Range)
	iterableStart, iterableStartFound := l.source.PhysicalOffset(statement.X.Pos())
	iterableEnd, iterableEndFound := l.source.PhysicalOffset(statement.X.End())
	braceOffset, braceFound := l.source.PhysicalOffset(statement.Body.Lbrace)
	if !forFound || !rangeFound || !iterableStartFound || !iterableEndFound || !braceFound {
		return doc.ID{}, errors.New("range header has no physical boundary")
	}

	clauseStart := rangeOffset
	clause := make([]doc.ID, 0, 8)
	var beforeRange []source.Comment
	if statement.Key != nil {
		assignment, assignmentStart, operatorOffset, err := l.rangeAssignment(statement)
		if err != nil {
			return doc.ID{}, err
		}
		clauseStart = assignmentStart
		clause = append(clause, assignment)
		beforeRange = l.commentsBetween(operatorOffset+len(statement.Tok.String()), rangeOffset)
		hasLineComment := false
		for _, comment := range beforeRange {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			clause = append(clause,
				l.arena.HardLine(),
				l.boundaryCommentsDocument(beforeRange, rangeOffset),
			)
		} else {
			beforeRangeDocument, err := l.inlineComments(beforeRange, true)
			if err != nil {
				return doc.ID{}, err
			}
			clause = append(clause, beforeRangeDocument, l.arena.Text(" "))
		}
	}
	iterable, err := l.expression(statement.X)
	if err != nil {
		return doc.ID{}, err
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	clause = append(clause, l.arena.Text("range"))
	clauseDocument := l.arena.Concat(clause...)
	leadingClause := l.commentsBetween(forOffset+len("for"), clauseStart)
	leadingIterable := l.commentsBetween(rangeOffset+len("range"), iterableStart)
	trailingIterableDocument, err := l.inlineComments(l.commentsBetween(iterableEnd, braceOffset), true)
	if err != nil {
		return doc.ID{}, err
	}

	var header doc.ID
	if len(leadingClause) > 0 {
		body := []doc.ID{
			l.arena.HardLine(),
			l.boundaryCommentsDocument(leadingClause, clauseStart),
			clauseDocument,
		}
		if len(leadingIterable) > 0 {
			body = append(body,
				l.arena.HardLine(),
				l.boundaryCommentsDocument(leadingIterable, iterableStart),
				iterable,
			)
		} else {
			body = append(body, l.arena.Line(), iterable)
		}
		header = l.arena.Concat(
			l.arena.Text("for"),
			l.arena.Indent(l.arena.Concat(body...)),
			trailingIterableDocument,
			l.arena.Text(" {"),
		)
	} else if len(leadingIterable) > 0 {
		header = l.arena.Concat(
			l.arena.Text("for "),
			clauseDocument,
			l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(leadingIterable, iterableStart),
				iterable,
			)),
			trailingIterableDocument,
			l.arena.Text(" {"),
		)
	} else {
		header = l.arena.Group(l.arena.Concat(
			l.arena.Text("for"),
			l.arena.Indent(l.arena.Concat(
				l.arena.Line(),
				clauseDocument,
				l.arena.Line(),
				iterable,
			)),
			trailingIterableDocument,
			l.arena.Text(" {"),
		))
	}
	return l.arena.Concat(header, tail), nil
}

func (l *lowerer) rangeAssignment(statement *ast.RangeStmt) (doc.ID, int, int, error) {
	keyStart, keyStartFound := l.source.PhysicalOffset(statement.Key.Pos())
	keyEnd, keyEndFound := l.source.PhysicalOffset(statement.Key.End())
	operatorOffset, operatorFound := l.source.PhysicalOffset(statement.TokPos)
	if !keyStartFound || !keyEndFound || !operatorFound {
		return doc.ID{}, 0, 0, errors.New("range assignment has no physical boundary")
	}
	key, err := l.expression(statement.Key)
	if err != nil {
		return doc.ID{}, 0, 0, err
	}
	parts := []doc.ID{key}
	boundary := keyEnd
	if statement.Value != nil {
		valueStart, valueStartFound := l.source.PhysicalOffset(statement.Value.Pos())
		valueEnd, valueEndFound := l.source.PhysicalOffset(statement.Value.End())
		if !valueStartFound || !valueEndFound {
			return doc.ID{}, 0, 0, errors.New("range value has no physical boundary")
		}
		comma, err := l.uniqueTokenBetween(token.COMMA, keyEnd, valueStart)
		if err != nil {
			return doc.ID{}, 0, 0, fmt.Errorf("range assignment boundary: %w", err)
		}
		afterKey, err := l.inlineComments(l.commentsBetween(keyEnd, comma.Range.Start), true)
		if err != nil {
			return doc.ID{}, 0, 0, err
		}
		parts = append(parts, afterKey, l.arena.Text(","))
		value, err := l.expression(statement.Value)
		if err != nil {
			return doc.ID{}, 0, 0, err
		}
		beforeValue := l.commentsBetween(comma.Range.End, valueStart)
		hasLineComment := false
		for _, comment := range beforeValue {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(beforeValue, valueStart),
				value,
			)))
		} else {
			beforeValueDocument, err := l.inlineComments(beforeValue, true)
			if err != nil {
				return doc.ID{}, 0, 0, err
			}
			parts = append(parts, beforeValueDocument, l.arena.Text(" "), value)
		}
		boundary = valueEnd
	}
	beforeOperator, err := l.inlineComments(l.commentsBetween(boundary, operatorOffset), true)
	if err != nil {
		return doc.ID{}, 0, 0, err
	}
	parts = append(parts, beforeOperator, l.arena.Text(" "+statement.Tok.String()))
	return l.arena.Concat(parts...), keyStart, operatorOffset, nil
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
	var tag doc.ID
	if statement.Tag != nil {
		var err error
		tag, err = l.expression(statement.Tag)
		if err != nil {
			return doc.ID{}, err
		}
	}
	header, err := l.switchHeader(statement.Switch, statement.Init, statement.Tag, tag, statement.Body.Lbrace)
	if err != nil {
		return doc.ID{}, err
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(header, tail), nil
}

func (l *lowerer) typeSwitchStatement(statement *ast.TypeSwitchStmt) (doc.ID, error) {
	guard, err := l.typeSwitchGuard(statement.Assign)
	if err != nil {
		return doc.ID{}, err
	}
	header, err := l.switchHeader(statement.Switch, statement.Init, statement.Assign, guard, statement.Body.Lbrace)
	if err != nil {
		return doc.ID{}, err
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(header, tail), nil
}

func (l *lowerer) switchHeader(
	keywordPosition token.Pos,
	initializer ast.Stmt,
	subject ast.Node,
	subjectDocument doc.ID,
	bracePosition token.Pos,
) (doc.ID, error) {
	keywordOffset, keywordFound := l.source.PhysicalOffset(keywordPosition)
	braceOffset, braceFound := l.source.PhysicalOffset(bracePosition)
	if !keywordFound || !braceFound {
		return doc.ID{}, errors.New("switch header has no physical boundary")
	}

	if subject == nil && initializer == nil {
		comments := l.commentsBetween(keywordOffset+len("switch"), braceOffset)
		if len(comments) == 0 {
			return l.arena.Text("switch {"), nil
		}
		hasLineComment := false
		for _, comment := range comments {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if !hasLineComment {
			commentsDocument, err := l.inlineComments(comments, true)
			if err != nil {
				return doc.ID{}, err
			}
			return l.arena.Concat(l.arena.Text("switch"), commentsDocument, l.arena.Text(" {")), nil
		}
		return l.arena.Concat(
			l.arena.Text("switch"),
			l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.boundaryCommentsBody(comments))),
			l.commentGap(comments[len(comments)-1].Range.End, braceOffset),
			l.arena.Text("{"),
		), nil
	}

	subjectStart := braceOffset
	subjectEnd := braceOffset
	if subject != nil {
		var startFound, endFound bool
		subjectStart, startFound = l.source.PhysicalOffset(subject.Pos())
		subjectEnd, endFound = l.source.PhysicalOffset(subject.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("switch subject has no physical boundary")
		}
	}

	var initializerDocument doc.ID
	initializerStart := subjectStart
	initializerEnd := subjectStart
	if initializer != nil {
		var err error
		initializerDocument, err = l.statement(initializer)
		if err != nil {
			return doc.ID{}, err
		}
		var startFound, endFound bool
		initializerStart, startFound = l.source.PhysicalOffset(initializer.Pos())
		initializerEnd, endFound = l.source.PhysicalOffset(initializer.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("switch initializer has no physical boundary")
		}
	}

	leading := l.commentsBetween(keywordOffset+len("switch"), initializerStart)
	var between []source.Comment
	if initializer != nil && subject != nil {
		between = l.commentsBetween(initializerEnd, subjectStart)
	}
	trailingDocument, err := l.inlineComments(l.commentsBetween(subjectEnd, braceOffset), true)
	if err != nil {
		return doc.ID{}, err
	}

	parts := []doc.ID{l.arena.Text("switch")}
	if initializer != nil {
		if len(leading) > 0 {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(leading, initializerStart),
				initializerDocument,
			)))
		} else {
			parts = append(parts, l.arena.Text(" "), initializerDocument)
		}
		parts = append(parts, l.arena.Text(";"))
	}

	if subject != nil {
		if initializer != nil && len(between) > 0 {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(between, subjectStart),
				subjectDocument,
			)))
		} else if initializer == nil && len(leading) > 0 {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(leading, subjectStart),
				subjectDocument,
			)))
		} else {
			parts = append(parts, l.arena.Indent(l.arena.Concat(l.arena.Line(), subjectDocument)))
		}
		parts = append(parts, trailingDocument, l.arena.Text(" {"))
		if len(leading) > 0 || len(between) > 0 {
			return l.arena.Concat(parts...), nil
		}
		return l.arena.Group(l.arena.Concat(parts...)), nil
	}

	comments := l.commentsBetween(initializerEnd, braceOffset)
	if len(comments) == 0 {
		return l.arena.Concat(append(parts, l.arena.Text(" {"))...), nil
	}
	hasLineComment := false
	for _, comment := range comments {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if !hasLineComment {
		commentsDocument, err := l.inlineComments(comments, true)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(append(parts, commentsDocument, l.arena.Text(" {"))...), nil
	}
	return l.arena.Concat(
		l.arena.Concat(parts...),
		l.arena.Indent(l.arena.Concat(l.arena.HardLine(), l.boundaryCommentsBody(comments))),
		l.commentGap(comments[len(comments)-1].Range.End, braceOffset),
		l.arena.Text("{"),
	), nil
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
		return l.assignmentWithDocuments(value, left, assertion, false)
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
	opening, openingFound := l.source.PhysicalOffset(assertion.Lparen)
	closing, closingFound := l.source.PhysicalOffset(assertion.Rparen)
	if !openingFound || !closingFound {
		return doc.ID{}, errors.New("type-switch assertion has no physical boundary")
	}
	keyword, err := l.uniqueTokenBetween(token.TYPE, opening+len("("), closing)
	if err != nil {
		return doc.ID{}, fmt.Errorf("type-switch assertion keyword: %w", err)
	}
	suffix, err := l.delimitedSingle(assertion.Lparen, assertion.Rparen, "(", ")", delimitedItem{
		document: l.arena.Text(keyword.Raw),
		start:    keyword.Range.Start,
		end:      keyword.Range.End,
	})
	if err != nil {
		return doc.ID{}, err
	}
	dotSuffix, err := l.dotSuffix(assertion.X, assertion.Lparen, suffix, false)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(base, dotSuffix), nil
}

func (l *lowerer) caseClause(clause *ast.CaseClause, _ int) (doc.ID, error) {
	colon, found := l.source.PhysicalOffset(clause.Colon)
	if !found {
		return doc.ID{}, errors.New("case clause has no physical colon offset")
	}
	header, err := l.caseClauseHeader(clause, colon)
	if err != nil {
		return doc.ID{}, err
	}
	parts := []doc.ID{header}
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

func (l *lowerer) caseClauseHeader(clause *ast.CaseClause, colon int) (doc.ID, error) {
	caseOffset, found := l.source.PhysicalOffset(clause.Case)
	if !found {
		return doc.ID{}, errors.New("case clause has no physical keyword offset")
	}
	if len(clause.List) == 0 {
		return l.defaultClauseHeader(caseOffset, colon)
	}

	items := make([]delimitedItem, 0, len(clause.List))
	for _, expression := range clause.List {
		document, err := l.expression(expression)
		if err != nil {
			return doc.ID{}, err
		}
		start, startFound := l.source.PhysicalOffset(expression.Pos())
		end, endFound := l.source.PhysicalOffset(expression.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("case expression has no physical boundary")
		}
		items = append(items, delimitedItem{document: document, start: start, end: end})
	}

	parts := []doc.ID{l.arena.Text("case")}
	leading := l.commentsBetween(caseOffset+len("case"), items[0].start)
	hasLeadingLineComment := false
	for _, comment := range leading {
		hasLeadingLineComment = hasLeadingLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLeadingLineComment {
		parts = append(parts, l.arena.Indent(l.arena.Concat(
			l.arena.HardLine(),
			l.boundaryCommentsDocument(leading, items[0].start),
			items[0].document,
		)))
	} else {
		leadingDocument, err := l.inlineComments(leading, true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, leadingDocument, l.arena.Text(" "), items[0].document)
	}

	for index := 1; index < len(items); index++ {
		previous := items[index-1]
		current := items[index]
		comma, err := l.uniqueTokenBetween(token.COMMA, previous.end, current.start)
		if err != nil {
			return doc.ID{}, fmt.Errorf("case expression boundary: %w", err)
		}
		afterPrevious, err := l.inlineComments(l.commentsBetween(previous.end, comma.Range.Start), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, afterPrevious, l.arena.Text(","))
		beforeCurrent := l.commentsBetween(comma.Range.End, current.start)
		hasLineComment := false
		for _, comment := range beforeCurrent {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(beforeCurrent, current.start),
				current.document,
			)))
		} else if len(beforeCurrent) > 0 {
			beforeCurrentDocument, err := l.inlineComments(beforeCurrent, true)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, beforeCurrentDocument, l.arena.Text(" "), current.document)
		} else {
			parts = append(parts, l.arena.Indent(l.arena.Concat(l.arena.Line(), current.document)))
		}
	}

	trailingDocument, err := l.inlineComments(l.commentsBetween(items[len(items)-1].end, colon), true)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, trailingDocument, l.arena.Text(":"))
	return l.arena.Group(l.arena.Concat(parts...)), nil
}

func (l *lowerer) defaultClauseHeader(keywordOffset, colon int) (doc.ID, error) {
	comments := l.commentsBetween(keywordOffset+len("default"), colon)
	hasLineComment := false
	for _, comment := range comments {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if !hasLineComment {
		commentsDocument, err := l.inlineComments(comments, true)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(l.arena.Text("default"), commentsDocument, l.arena.Text(":")), nil
	}
	body := l.arena.Text("default")
	boundary := keywordOffset + len("default")
	previousWasLineComment := false
	for _, comment := range comments {
		if !previousWasLineComment && l.samePhysicalLine(boundary, comment.Range.Start) {
			body = l.arena.Concat(body, l.arena.Text(" "), l.arena.Verbatim(comment.Raw))
		} else {
			body = l.arena.Concat(body, l.commentGap(boundary, comment.Range.Start), l.arena.Verbatim(comment.Raw))
		}
		boundary = comment.Range.End
		previousWasLineComment = strings.HasPrefix(comment.Raw, "//")
	}
	return l.arena.Concat(body, l.commentGap(boundary, colon), l.arena.Text(":")), nil
}

func (l *lowerer) communicationClause(clause *ast.CommClause, _ int) (doc.ID, error) {
	colon, found := l.source.PhysicalOffset(clause.Colon)
	if !found {
		return doc.ID{}, errors.New("communication clause has no physical colon offset")
	}
	header, err := l.communicationClauseHeader(clause, colon)
	if err != nil {
		return doc.ID{}, err
	}
	parts := []doc.ID{header}
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

func (l *lowerer) communicationClauseHeader(clause *ast.CommClause, colon int) (doc.ID, error) {
	caseOffset, found := l.source.PhysicalOffset(clause.Case)
	if !found {
		return doc.ID{}, errors.New("communication clause has no physical keyword offset")
	}
	if clause.Comm == nil {
		return l.defaultClauseHeader(caseOffset, colon)
	}
	communicationStart, startFound := l.source.PhysicalOffset(clause.Comm.Pos())
	communicationEnd, endFound := l.source.PhysicalOffset(clause.Comm.End())
	if !startFound || !endFound {
		return doc.ID{}, errors.New("communication clause has no physical statement boundary")
	}
	communication, err := l.communicationStatement(clause.Comm)
	if err != nil {
		return doc.ID{}, err
	}
	parts := []doc.ID{l.arena.Text("case")}
	leading := l.commentsBetween(caseOffset+len("case"), communicationStart)
	hasLineComment := false
	for _, comment := range leading {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLineComment {
		parts = append(parts, l.arena.Indent(l.arena.Concat(
			l.arena.HardLine(),
			l.boundaryCommentsDocument(leading, communicationStart),
			communication,
		)))
	} else {
		leadingDocument, err := l.inlineComments(leading, false)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, l.arena.Indent(l.arena.Concat(
			l.arena.Line(),
			leadingDocument,
			communication,
		)))
	}
	trailingDocument, err := l.inlineComments(l.commentsBetween(communicationEnd, colon), true)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, trailingDocument, l.arena.Text(":"))
	return l.arena.Group(l.arena.Concat(parts...)), nil
}

func (l *lowerer) communicationStatement(statement ast.Stmt) (doc.ID, error) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok {
		return l.statement(statement)
	}
	return l.assignmentDocument(assignment, true)
}

func (l *lowerer) assignment(assignment *ast.AssignStmt) (doc.ID, error) {
	return l.assignmentDocument(assignment, false)
}

func (l *lowerer) assignmentDocument(assignment *ast.AssignStmt, breakRight bool) (doc.ID, error) {
	left, err := l.expressions(assignment.Lhs)
	if err != nil {
		return doc.ID{}, err
	}
	right, err := l.expressions(assignment.Rhs)
	if err != nil {
		return doc.ID{}, err
	}
	return l.assignmentWithDocuments(assignment, left, right, breakRight)
}

func (l *lowerer) assignmentWithDocuments(
	assignment *ast.AssignStmt,
	left doc.ID,
	right doc.ID,
	breakRight bool,
) (doc.ID, error) {
	if len(assignment.Lhs) == 0 || len(assignment.Rhs) == 0 {
		return doc.ID{}, errors.New("assignment requires left and right expressions")
	}
	leftEnd, leftEndFound := l.source.PhysicalOffset(assignment.Lhs[len(assignment.Lhs)-1].End())
	operatorOffset, operatorFound := l.source.PhysicalOffset(assignment.TokPos)
	rightStart, rightStartFound := l.source.PhysicalOffset(assignment.Rhs[0].Pos())
	if !leftEndFound || !operatorFound || !rightStartFound {
		return doc.ID{}, errors.New("assignment has no physical operator boundary")
	}
	beforeOperator, err := l.inlineComments(l.commentsBetween(leftEnd, operatorOffset), true)
	if err != nil {
		return doc.ID{}, err
	}
	afterOperator := l.commentsBetween(operatorOffset+len(assignment.Tok.String()), rightStart)
	parts := []doc.ID{left, beforeOperator, l.arena.Text(" " + assignment.Tok.String())}
	hasLineComment := false
	for _, comment := range afterOperator {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLineComment {
		parts = append(parts, l.arena.Indent(l.arena.Concat(
			l.arena.HardLine(),
			l.boundaryCommentsDocument(afterOperator, rightStart),
			right,
		)))
	} else {
		afterOperatorDocument, err := l.inlineComments(afterOperator, true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, afterOperatorDocument)
		if breakRight {
			parts = append(parts, l.arena.Indent(l.arena.Concat(l.arena.Line(), right)))
		} else {
			parts = append(parts, l.arena.Text(" "), right)
		}
	}
	return l.arena.Group(l.arena.Concat(parts...)), nil
}

func (l *lowerer) sendStatement(statement *ast.SendStmt) (doc.ID, error) {
	channel, err := l.expression(statement.Chan)
	if err != nil {
		return doc.ID{}, err
	}
	sent, err := l.expression(statement.Value)
	if err != nil {
		return doc.ID{}, err
	}
	channelEnd, channelEndFound := l.source.PhysicalOffset(statement.Chan.End())
	arrowOffset, arrowFound := l.source.PhysicalOffset(statement.Arrow)
	valueStart, valueStartFound := l.source.PhysicalOffset(statement.Value.Pos())
	if !channelEndFound || !arrowFound || !valueStartFound {
		return doc.ID{}, errors.New("send statement has no physical operator boundary")
	}
	beforeArrow, err := l.inlineComments(l.commentsBetween(channelEnd, arrowOffset), true)
	if err != nil {
		return doc.ID{}, err
	}
	afterArrow := l.commentsBetween(arrowOffset+len("<-"), valueStart)
	parts := []doc.ID{channel, beforeArrow, l.arena.Text(" <-")}
	hasLineComment := false
	for _, comment := range afterArrow {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLineComment {
		parts = append(parts, l.arena.Indent(l.arena.Concat(
			l.arena.HardLine(),
			l.boundaryCommentsDocument(afterArrow, valueStart),
			sent,
		)))
	} else {
		afterArrowDocument, err := l.inlineComments(afterArrow, true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, afterArrowDocument, l.arena.Indent(l.arena.Concat(l.arena.Line(), sent)))
	}
	return l.arena.Group(l.arena.Concat(parts...)), nil
}

func (l *lowerer) ifStatement(statement *ast.IfStmt) (doc.ID, error) {
	ifOffset, ifFound := l.source.PhysicalOffset(statement.If)
	conditionStart, conditionStartFound := l.source.PhysicalOffset(statement.Cond.Pos())
	conditionEnd, conditionEndFound := l.source.PhysicalOffset(statement.Cond.End())
	braceOffset, braceFound := l.source.PhysicalOffset(statement.Body.Lbrace)
	if !ifFound || !conditionStartFound || !conditionEndFound || !braceFound {
		return doc.ID{}, errors.New("if header has no physical boundary")
	}

	condition, err := l.expression(statement.Cond)
	if err != nil {
		return doc.ID{}, err
	}
	trailingDocument, err := l.inlineComments(l.commentsBetween(conditionEnd, braceOffset), true)
	if err != nil {
		return doc.ID{}, err
	}

	parts := []doc.ID{l.arena.Text("if")}
	if statement.Init != nil {
		initializer, err := l.statement(statement.Init)
		if err != nil {
			return doc.ID{}, err
		}
		initializerStart, startFound := l.source.PhysicalOffset(statement.Init.Pos())
		initializerEnd, endFound := l.source.PhysicalOffset(statement.Init.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("if initializer has no physical boundary")
		}
		leading := l.commentsBetween(ifOffset+len("if"), initializerStart)
		between := l.commentsBetween(initializerEnd, conditionStart)
		if len(leading) > 0 || len(between) > 0 {
			if len(leading) > 0 {
				parts = append(parts, l.arena.Indent(l.arena.Concat(
					l.arena.HardLine(),
					l.boundaryCommentsDocument(leading, initializerStart),
					initializer,
				)))
			} else {
				parts = append(parts, l.arena.Text(" "), initializer)
			}
			continuation := []doc.ID{l.arena.HardLine()}
			if len(between) > 0 {
				continuation = append(continuation, l.boundaryCommentsDocument(between, conditionStart))
			}
			continuation = append(continuation, condition)
			parts = append(parts,
				l.arena.Text(";"),
				l.arena.Indent(l.arena.Concat(continuation...)),
				trailingDocument,
				l.arena.Text(" {"),
			)
		} else {
			parts = append(parts,
				l.arena.Text(" "),
				initializer,
				l.arena.Text(";"),
				l.arena.Indent(l.arena.Concat(l.arena.Line(), condition)),
				trailingDocument,
				l.arena.Text(" {"),
			)
			parts = []doc.ID{l.arena.Group(l.arena.Concat(parts...))}
		}
	} else {
		leading := l.commentsBetween(ifOffset+len("if"), conditionStart)
		if len(leading) > 0 {
			parts = append(parts,
				l.arena.Indent(l.arena.Concat(
					l.arena.HardLine(),
					l.boundaryCommentsDocument(leading, conditionStart),
					condition,
				)),
				trailingDocument,
				l.arena.Text(" {"),
			)
		} else {
			parts = append(parts,
				l.arena.Indent(l.arena.Concat(l.arena.Line(), condition)),
				trailingDocument,
				l.arena.Text(" {"),
			)
			parts = []doc.ID{l.arena.Group(l.arena.Concat(parts...))}
		}
	}
	tail, err := l.blockTail(statement.Body)
	if err != nil {
		return doc.ID{}, err
	}
	result := []doc.ID{l.arena.Concat(parts...), tail}
	if statement.Else != nil {
		alternative, err := l.statement(statement.Else)
		if err != nil {
			return doc.ID{}, err
		}
		result = append(result, l.arena.Text(" else "), alternative)
	}
	return l.arena.Concat(result...), nil
}

func (l *lowerer) expressions(expressions []ast.Expr) (doc.ID, error) {
	if len(expressions) == 0 {
		return l.arena.Empty(), nil
	}
	items := make([]delimitedItem, 0, len(expressions))
	for _, expression := range expressions {
		document, err := l.expression(expression)
		if err != nil {
			return doc.ID{}, err
		}
		start, startFound := l.source.PhysicalOffset(expression.Pos())
		end, endFound := l.source.PhysicalOffset(expression.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("expression list item has no physical boundary")
		}
		items = append(items, delimitedItem{document: document, start: start, end: end})
	}
	parts := []doc.ID{items[0].document}
	for index := 1; index < len(items); index++ {
		previous := items[index-1]
		current := items[index]
		comma, err := l.uniqueTokenBetween(token.COMMA, previous.end, current.start)
		if err != nil {
			return doc.ID{}, fmt.Errorf("expression list boundary: %w", err)
		}
		afterPrevious, err := l.inlineComments(l.commentsBetween(previous.end, comma.Range.Start), true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, afterPrevious, l.arena.Text(","))
		beforeCurrent := l.commentsBetween(comma.Range.End, current.start)
		hasLineComment := false
		for _, comment := range beforeCurrent {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(beforeCurrent, current.start),
				current.document,
			)))
		} else {
			beforeCurrentDocument, err := l.inlineComments(beforeCurrent, true)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, beforeCurrentDocument, l.arena.Text(" "), current.document)
		}
	}
	return l.arena.Concat(parts...), nil
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
		if chain, ok, err := l.selectorChain(value); ok || err != nil {
			return chain, err
		}
		left, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		suffix, err := l.dotSuffix(value.X, value.Sel.Pos(), l.arena.Text(value.Sel.Name), false)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(left, suffix), nil
	case *ast.CallExpr:
		return l.call(value)
	case *ast.CompositeLit:
		return l.compositeLiteral(value)
	case *ast.KeyValueExpr:
		return l.keyValue(value)
	case *ast.IndexExpr:
		base, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		suffix, err := l.indexSuffix(value)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(base, suffix), nil
	case *ast.IndexListExpr:
		base, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		suffix, err := l.indexListSuffix(value)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(base, suffix), nil
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
		start, startFound := l.source.PhysicalOffset(value.Type.Pos())
		end, endFound := l.source.PhysicalOffset(value.Type.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("type assertion has no physical range")
		}
		suffix, err := l.delimitedSingle(value.Lparen, value.Rparen, "(", ")", delimitedItem{
			document: asserted,
			start:    start,
			end:      end,
		})
		if err != nil {
			return doc.ID{}, err
		}
		dotSuffix, err := l.dotSuffix(value.X, value.Lparen, suffix, false)
		if err != nil {
			return doc.ID{}, err
		}
		return l.arena.Concat(base, dotSuffix), nil
	case *ast.BinaryExpr:
		return l.binary(value)
	case *ast.UnaryExpr:
		return l.unary(value)
	case *ast.ParenExpr:
		inner, err := l.expression(value.X)
		if err != nil {
			return doc.ID{}, err
		}
		start, startFound := l.source.PhysicalOffset(value.X.Pos())
		end, endFound := l.source.PhysicalOffset(value.X.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("parenthesized expression has no physical range")
		}
		return l.delimitedSingle(value.Lparen, value.Rparen, "(", ")", delimitedItem{
			document: inner,
			start:    start,
			end:      end,
		})
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

func (l *lowerer) keyValue(expression *ast.KeyValueExpr) (doc.ID, error) {
	key, err := l.expression(expression.Key)
	if err != nil {
		return doc.ID{}, err
	}
	value, err := l.expression(expression.Value)
	if err != nil {
		return doc.ID{}, err
	}
	keyEnd, keyEndFound := l.source.PhysicalOffset(expression.Key.End())
	colonOffset, colonFound := l.source.PhysicalOffset(expression.Colon)
	valueStart, valueStartFound := l.source.PhysicalOffset(expression.Value.Pos())
	if !keyEndFound || !colonFound || !valueStartFound {
		return doc.ID{}, errors.New("key/value expression has no physical colon boundary")
	}
	beforeColon, err := l.inlineComments(l.commentsBetween(keyEnd, colonOffset), true)
	if err != nil {
		return doc.ID{}, err
	}
	afterColon := l.commentsBetween(colonOffset+len(":"), valueStart)
	parts := []doc.ID{key, beforeColon, l.arena.Text(":")}
	hasLineComment := false
	for _, comment := range afterColon {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLineComment {
		parts = append(parts, l.arena.Indent(l.arena.Concat(
			l.arena.HardLine(),
			l.boundaryCommentsDocument(afterColon, valueStart),
			value,
		)))
	} else {
		afterColonDocument, err := l.inlineComments(afterColon, true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, afterColonDocument, l.arena.Text(" "), value)
	}
	return l.arena.Group(l.arena.Concat(parts...)), nil
}

func (l *lowerer) unary(expression *ast.UnaryExpr) (doc.ID, error) {
	operand, err := l.expression(expression.X)
	if err != nil {
		return doc.ID{}, err
	}
	operatorOffset, operatorFound := l.source.PhysicalOffset(expression.OpPos)
	operandStart, operandStartFound := l.source.PhysicalOffset(expression.X.Pos())
	operatorRaw, tokenFound := l.source.RawToken(expression.OpPos)
	if !operatorFound || !operandStartFound || !tokenFound {
		return doc.ID{}, errors.New("unary expression has no physical operator boundary")
	}
	comments := l.commentsBetween(operatorOffset+len(operatorRaw), operandStart)
	if len(comments) == 0 {
		return l.arena.Concat(l.arena.Text(operatorRaw), operand), nil
	}
	hasLineComment := false
	for _, comment := range comments {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLineComment {
		return l.arena.Concat(
			l.arena.Text(operatorRaw),
			l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(comments, operandStart),
				operand,
			)),
		), nil
	}
	commentsDocument, err := l.inlineComments(comments, true)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(l.arena.Text(operatorRaw), commentsDocument, l.arena.Text(" "), operand), nil
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
	elements := make([]delimitedItem, 0, len(literal.Elts))
	for _, rawElement := range literal.Elts {
		element, err := l.expression(rawElement)
		if err != nil {
			return doc.ID{}, err
		}
		start, startFound := l.source.PhysicalOffset(rawElement.Pos())
		end, endFound := l.source.PhysicalOffset(rawElement.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("composite element has no physical range")
		}
		elements = append(elements, delimitedItem{document: element, start: start, end: end})
	}
	list, err := l.delimitedCommaList(literal.Lbrace, literal.Rbrace, "{", "}", elements)
	if err != nil {
		return doc.ID{}, err
	}
	parts = append(parts, list)
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) indexSuffix(expression *ast.IndexExpr) (doc.ID, error) {
	index, err := l.expression(expression.Index)
	if err != nil {
		return doc.ID{}, err
	}
	start, startFound := l.source.PhysicalOffset(expression.Index.Pos())
	end, endFound := l.source.PhysicalOffset(expression.Index.End())
	if !startFound || !endFound {
		return doc.ID{}, errors.New("index has no physical range")
	}
	return l.delimitedSingle(expression.Lbrack, expression.Rbrack, "[", "]", delimitedItem{
		document: index,
		start:    start,
		end:      end,
	})
}

func (l *lowerer) indexListSuffix(expression *ast.IndexListExpr) (doc.ID, error) {
	indices := make([]delimitedItem, 0, len(expression.Indices))
	for _, rawIndex := range expression.Indices {
		index, err := l.expression(rawIndex)
		if err != nil {
			return doc.ID{}, err
		}
		start, startFound := l.source.PhysicalOffset(rawIndex.Pos())
		end, endFound := l.source.PhysicalOffset(rawIndex.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("index list item has no physical range")
		}
		indices = append(indices, delimitedItem{document: index, start: start, end: end})
	}
	return l.delimitedCommaList(expression.Lbrack, expression.Rbrack, "[", "]", indices)
}

func (l *lowerer) slice(expression *ast.SliceExpr) (doc.ID, error) {
	base, err := l.expression(expression.X)
	if err != nil {
		return doc.ID{}, err
	}
	baseEnd, baseEndFound := l.source.PhysicalOffset(expression.X.End())
	opening, openingFound := l.source.PhysicalOffset(expression.Lbrack)
	closing, closingFound := l.source.PhysicalOffset(expression.Rbrack)
	if !baseEndFound || !openingFound || !closingFound {
		return doc.ID{}, errors.New("slice expression has no physical delimiter boundary")
	}
	beforeOpening, err := l.inlineComments(l.commentsBetween(baseEnd, opening), true)
	if err != nil {
		return doc.ID{}, err
	}
	colons, err := l.sliceColons(expression, opening, closing)
	if err != nil {
		return doc.ID{}, err
	}
	type slicePiece struct {
		document doc.ID
		start    int
		end      int
		colon    bool
		closing  bool
	}
	pieces := make([]slicePiece, 0, 6)
	appendBound := func(bound ast.Expr) error {
		if bound == nil {
			return nil
		}
		document, lowerErr := l.expression(bound)
		if lowerErr != nil {
			return lowerErr
		}
		start, startFound := l.source.PhysicalOffset(bound.Pos())
		end, endFound := l.source.PhysicalOffset(bound.End())
		if !startFound || !endFound {
			return errors.New("slice bound has no physical boundary")
		}
		pieces = append(pieces, slicePiece{document: document, start: start, end: end})
		return nil
	}
	if err := appendBound(expression.Low); err != nil {
		return doc.ID{}, err
	}
	pieces = append(pieces, slicePiece{
		document: l.arena.Text(":"),
		start:    colons[0].Range.Start,
		end:      colons[0].Range.End,
		colon:    true,
	})
	if err := appendBound(expression.High); err != nil {
		return doc.ID{}, err
	}
	if expression.Slice3 {
		pieces = append(pieces, slicePiece{
			document: l.arena.Text(":"),
			start:    colons[1].Range.Start,
			end:      colons[1].Range.End,
			colon:    true,
		})
		if err := appendBound(expression.Max); err != nil {
			return doc.ID{}, err
		}
	}
	pieces = append(pieces, slicePiece{
		document: l.arena.Text("]"),
		start:    closing,
		end:      closing + len("]"),
		closing:  true,
	})
	parts := []doc.ID{base, beforeOpening, l.arena.Text("[")}
	previousEnd := opening + len("[")
	previousOpen := true
	previousColon := false
	for _, piece := range pieces {
		comments := l.commentsBetween(previousEnd, piece.start)
		hasLineComment := false
		for _, comment := range comments {
			hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
		}
		if hasLineComment {
			if !previousOpen && !previousColon {
				return doc.ID{}, errors.New("slice line comment has no grammar-safe break boundary")
			}
			parts = append(parts, l.arena.Indent(l.arena.Concat(
				l.arena.HardLine(),
				l.boundaryCommentsDocument(comments, piece.start),
				piece.document,
			)))
		} else {
			commentsDocument, err := l.inlineCommentsWithSpacing(
				comments,
				!previousOpen,
				!piece.colon && !piece.closing,
			)
			if err != nil {
				return doc.ID{}, err
			}
			parts = append(parts, commentsDocument, piece.document)
		}
		previousEnd = piece.end
		previousOpen = false
		previousColon = piece.colon
	}
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) sliceColons(expression *ast.SliceExpr, opening, closing int) ([]source.Token, error) {
	parentheses := 0
	brackets := 0
	braces := 0
	colons := make([]source.Token, 0, 2)
	first := sort.Search(len(l.tokens), func(index int) bool {
		return l.tokens[index].Range.Start > opening
	})
	for _, item := range l.tokens[first:] {
		if item.Range.Start >= closing {
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
		case token.COLON:
			if parentheses == 0 && brackets == 0 && braces == 0 {
				colons = append(colons, item)
			}
		}
		if parentheses < 0 || brackets < 0 || braces < 0 {
			return nil, errors.New("slice token nesting is unbalanced")
		}
	}
	if parentheses != 0 || brackets != 0 || braces != 0 {
		return nil, errors.New("slice token nesting is unbalanced")
	}
	want := 1
	if expression.Slice3 {
		want = 2
	}
	if len(colons) != want {
		return nil, fmt.Errorf("slice expression contains %d top-level colons, want %d", len(colons), want)
	}
	return colons, nil
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
			item, err = l.fieldWithType(field, signature, "")
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
	if chain, ok, err := l.selectorChain(call); ok || err != nil {
		return chain, err
	}
	function, err := l.expression(call.Fun)
	if err != nil {
		return doc.ID{}, err
	}
	arguments, err := l.callArguments(call)
	if err != nil {
		return doc.ID{}, err
	}
	return l.arena.Concat(function, arguments), nil
}

type selectorChainPart struct {
	selector  *ast.SelectorExpr
	call      *ast.CallExpr
	index     *ast.IndexExpr
	indexList *ast.IndexListExpr
}

func (l *lowerer) selectorChain(expression ast.Expr) (doc.ID, bool, error) {
	current := expression
	parts := make([]selectorChainPart, 0, 4)
	selectors := 0
	for {
		switch value := current.(type) {
		case *ast.SelectorExpr:
			parts = append(parts, selectorChainPart{selector: value})
			selectors++
			current = value.X
		case *ast.CallExpr:
			parts = append(parts, selectorChainPart{call: value})
			current = value.Fun
		case *ast.IndexExpr:
			parts = append(parts, selectorChainPart{index: value})
			current = value.X
		case *ast.IndexListExpr:
			parts = append(parts, selectorChainPart{indexList: value})
			current = value.X
		default:
			if selectors < 2 {
				return l.arena.Empty(), false, nil
			}
			base, err := l.expression(current)
			if err != nil {
				return doc.ID{}, false, err
			}
			continuation := make([]doc.ID, 0, len(parts)*3)
			for index := len(parts) - 1; index >= 0; index-- {
				part := parts[index]
				if part.selector != nil {
					suffix, err := l.dotSuffix(
						part.selector.X,
						part.selector.Sel.Pos(),
						l.arena.Text(part.selector.Sel.Name),
						true,
					)
					if err != nil {
						return doc.ID{}, false, err
					}
					continuation = append(continuation, suffix)
					continue
				}
				switch {
				case part.call != nil:
					arguments, err := l.callArguments(part.call)
					if err != nil {
						return doc.ID{}, false, err
					}
					continuation = append(continuation, arguments)
				case part.index != nil:
					suffix, err := l.indexSuffix(part.index)
					if err != nil {
						return doc.ID{}, false, err
					}
					continuation = append(continuation, suffix)
				case part.indexList != nil:
					suffix, err := l.indexListSuffix(part.indexList)
					if err != nil {
						return doc.ID{}, false, err
					}
					continuation = append(continuation, suffix)
				}
			}
			return l.arena.Group(l.arena.Concat(
				base,
				l.arena.Indent(l.arena.Concat(continuation...)),
			)), true, nil
		}
	}
}

func (l *lowerer) dotSuffix(
	left ast.Expr,
	rightPosition token.Pos,
	suffix doc.ID,
	breakable bool,
) (doc.ID, error) {
	leftEnd, leftEndFound := l.source.PhysicalOffset(left.End())
	rightStart, rightStartFound := l.source.PhysicalOffset(rightPosition)
	if !leftEndFound || !rightStartFound {
		return doc.ID{}, errors.New("dot expression has no physical boundary")
	}
	dot, err := l.uniqueTokenBetween(token.PERIOD, leftEnd, rightStart)
	if err != nil {
		return doc.ID{}, fmt.Errorf("dot expression boundary: %w", err)
	}
	beforeDot, err := l.inlineComments(l.commentsBetween(leftEnd, dot.Range.Start), true)
	if err != nil {
		return doc.ID{}, err
	}
	afterDot := l.commentsBetween(dot.Range.End, rightStart)
	parts := []doc.ID{beforeDot, l.arena.Text(".")}
	hasLineComment := false
	for _, comment := range afterDot {
		hasLineComment = hasLineComment || strings.HasPrefix(comment.Raw, "//")
	}
	if hasLineComment {
		parts = append(parts, l.arena.Indent(l.arena.Concat(
			l.arena.HardLine(),
			l.boundaryCommentsDocument(afterDot, rightStart),
			suffix,
		)))
		return l.arena.Concat(parts...), nil
	}
	if len(afterDot) > 0 {
		afterDotDocument, err := l.inlineComments(afterDot, true)
		if err != nil {
			return doc.ID{}, err
		}
		parts = append(parts, afterDotDocument)
		if breakable {
			parts = append(parts, l.arena.Line())
		} else {
			parts = append(parts, l.arena.Text(" "))
		}
	} else if breakable {
		parts = append(parts, l.arena.SoftLine())
	}
	parts = append(parts, suffix)
	return l.arena.Concat(parts...), nil
}

func (l *lowerer) callArguments(call *ast.CallExpr) (doc.ID, error) {
	arguments := make([]delimitedItem, 0, len(call.Args))
	for index, argument := range call.Args {
		lowered, err := l.expression(argument)
		if err != nil {
			return doc.ID{}, err
		}
		start, startFound := l.source.PhysicalOffset(argument.Pos())
		end, endFound := l.source.PhysicalOffset(argument.End())
		if !startFound || !endFound {
			return doc.ID{}, errors.New("call argument has no physical range")
		}
		if index == len(call.Args)-1 && call.Ellipsis.IsValid() {
			ellipsis, found := l.source.PhysicalOffset(call.Ellipsis)
			if !found {
				return doc.ID{}, errors.New("call ellipsis has no physical offset")
			}
			lowered = l.arena.Concat(lowered, l.arena.Text("..."))
			end = ellipsis + len("...")
		}
		arguments = append(arguments, delimitedItem{document: lowered, start: start, end: end})
	}
	list, err := l.delimitedCommaList(call.Lparen, call.Rparen, "(", ")", arguments)
	if err != nil {
		return doc.ID{}, err
	}
	return list, nil
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
	return l.inlineCommentsWithSpacing(comments, leadingSpace, !leadingSpace)
}

func (l *lowerer) inlineCommentsWithSpacing(
	comments []source.Comment,
	leadingSpace bool,
	trailingSpace bool,
) (doc.ID, error) {
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
	if trailingSpace {
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
