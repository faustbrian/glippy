package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type uncheckedWriterErrorRule struct{}

type writerFinalizerSpec struct {
	packagePath string
	typeName string
	methodName string
}

var writerFinalizerSpecs = []writerFinalizerSpec{
	{packagePath: "archive/tar", typeName: "Writer", methodName: "Close"},
	{packagePath: "archive/tar", typeName: "Writer", methodName: "Flush"},
	{packagePath: "archive/zip", typeName: "Writer", methodName: "Close"},
	{packagePath: "archive/zip", typeName: "Writer", methodName: "Flush"},
	{packagePath: "bufio", typeName: "Writer", methodName: "Flush"},
	{packagePath: "compress/flate", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/flate", typeName: "Writer", methodName: "Flush"},
	{packagePath: "compress/gzip", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/gzip", typeName: "Writer", methodName: "Flush"},
	{packagePath: "compress/lzw", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/zlib", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/zlib", typeName: "Writer", methodName: "Flush"},
	{packagePath: "encoding/xml", typeName: "Encoder", methodName: "Close"},
	{packagePath: "encoding/xml", typeName: "Encoder", methodName: "Flush"},
	{packagePath: "mime/multipart", typeName: "Writer", methodName: "Close"},
	{packagePath: "mime/quotedprintable", typeName: "Writer", methodName: "Close"},
	{packagePath: "text/tabwriter", typeName: "Writer", methodName: "Flush"},
}

// NewUncheckedWriterErrorRule constructs the buffered-writer finalization rule
// for product registry composition.
func NewUncheckedWriterErrorRule() Rule {
	return uncheckedWriterErrorRule{}
}

func (uncheckedWriterErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "unchecked-writer-error",
		Summary: "detects discarded errors from buffered writer finalization",
		Documentation: "Buffered, compressed, archive, multipart, and encoded writers can report their first failed output or emit required trailers only from Flush or Close. Discarding that result can report success while leaving output truncated or structurally incomplete. The rule targets exact standard-library finalizers whose documented contract writes pending data or required framing.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeExprStmt, NodeAssignStmt, NodeGoStmt, NodeDeferStmt},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only exact standard-library writer finalizers with an error result are covered; user-defined writers and interface-dispatched finalizers remain outside the initial contract.",
			"Encoders returned as io.WriteCloser by encoding/ascii85, encoding/base32, and encoding/base64 require acquisition tracking before their concrete finalization contract can be proven.",
			"encoding/csv.Writer.Flush returns no error and requires a separate rule that proves whether Writer.Error is observed after flushing.",
			"No fix is offered because correct propagation from a deferred, asynchronous, or ordinary call depends on the surrounding function contract.",
		},
		Examples: []Example{
			{
				Title: "Propagate gzip finalization failures",
				Incorrect: "defer writer.Close()",
				Correct: "if err := writer.Close(); err != nil { return err }",
			},
		},
	}
}

func (uncheckedWriterErrorRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("unchecked-writer-error requires complete type information")
	}
	call, discarded := discardedCall(node)
	if !discarded {
		return nil, nil
	}
	spec, matched := writerFinalizer(ctx.Info(), call)
	if !matched {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "unchecked-writer-error",
			Message: fmt.Sprintf(
				"error returned by %s.%s.%s is discarded; buffered output may be incomplete",
				spec.packagePath,
				spec.typeName,
				spec.methodName,
			),
			Range: range_,
			Help: "observe and propagate the finalization error before reporting success",
		},
	}, nil
}

func discardedCall(node ast.Node) (*ast.CallExpr, bool) {
	switch statement := node.(type) {
	case *ast.ExprStmt:
		call, _ := ast.Unparen(statement.X).(*ast.CallExpr)
		return call, call != nil
	case *ast.AssignStmt:
		if len(statement.Lhs) != 1 ||
			len(statement.Rhs) != 1 ||
			!blankIdentifier(statement.Lhs[0]) {
			return nil, false
		}
		call, _ := ast.Unparen(statement.Rhs[0]).(*ast.CallExpr)
		return call, call != nil
	case *ast.GoStmt:
		return statement.Call, statement.Call != nil
	case *ast.DeferStmt:
		return statement.Call, statement.Call != nil
	default:
		return nil, false
	}
}

func writerFinalizer(info *types.Info, call *ast.CallExpr) (writerFinalizerSpec, bool) {
	if info == nil || call == nil {
		return writerFinalizerSpec{}, false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return writerFinalizerSpec{}, false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return writerFinalizerSpec{}, false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil || function.Pkg() == nil {
		return writerFinalizerSpec{}, false
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return writerFinalizerSpec{}, false
	}
	for _, spec := range writerFinalizerSpecs {
		if function.Pkg().Path() == spec.packagePath &&
			function.Name() == spec.methodName &&
			namedReceiver(signature.Recv().Type(), spec.packagePath, spec.typeName) {
			return spec, true
		}
	}
	return writerFinalizerSpec{}, false
}

func isWriterFinalizer(info *types.Info, call *ast.CallExpr) bool {
	_, matched := writerFinalizer(info, call)
	return matched
}
