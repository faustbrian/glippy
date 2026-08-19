package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

type uncheckedWriterErrorRule struct{}

type writerFinalizerSpec struct {
	packagePath string
	typeName string
	methodName string
	constructor string
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

var writerEncoderSpecs = []writerFinalizerSpec{
	{packagePath: "encoding/ascii85", methodName: "Close", constructor: "NewEncoder"},
	{packagePath: "encoding/base32", methodName: "Close", constructor: "NewEncoder"},
	{packagePath: "encoding/base64", methodName: "Close", constructor: "NewEncoder"},
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
		Documentation: "Buffered, compressed, archive, multipart, and encoded writers can report their first failed output or emit required trailers only from Flush or Close. Discarding that result can report success while leaving output truncated or structurally incomplete. The rule targets exact standard-library finalizers whose documented contract writes pending data or required framing, including direct stable values returned by the streaming encoder constructors.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeExprStmt, NodeAssignStmt, NodeGoStmt, NodeDeferStmt},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only exact standard-library writer finalizers with an error result are covered; user-defined writers and unproven interface-dispatched finalizers remain outside the contract.",
			"Streaming encoder coverage requires a direct constructor result or a direct identifier initialized by encoding/ascii85, encoding/base32, or encoding/base64 NewEncoder and not reassigned before Close.",
			"encoding/csv.Writer.Flush returns no error; unchecked-csv-writer-error owns its separate Flush then Error observation protocol.",
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
	spec, matched := writerFinalizer(ctx, call)
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
				"error returned by %s is discarded; buffered output may be incomplete",
				spec.target(),
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

func (s writerFinalizerSpec) target() string {
	if s.constructor != "" {
		return fmt.Sprintf("%s.%s result %s", s.packagePath, s.constructor, s.methodName)
	}
	return fmt.Sprintf("%s.%s.%s", s.packagePath, s.typeName, s.methodName)
}

func writerFinalizer(ctx *TypesContext, call *ast.CallExpr) (writerFinalizerSpec, bool) {
	if ctx == nil || ctx.Info() == nil || call == nil {
		return writerFinalizerSpec{}, false
	}
	info := ctx.Info()
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
	if spec, matched := acquiredEncoderFinalizer(info, ctx.Syntax(), call, selector); matched {
		return spec, true
	}
	return writerFinalizerSpec{}, false
}

func acquiredEncoderFinalizer(
	info *types.Info,
	syntax *ast.File,
	call *ast.CallExpr,
	selector *ast.SelectorExpr,
) (writerFinalizerSpec, bool) {
	if info == nil || syntax == nil || call == nil || selector == nil || len(call.Args) != 0 {
		return writerFinalizerSpec{}, false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return writerFinalizerSpec{}, false
	}
	method, _ := selection.Obj().(*types.Func)
	if method == nil || method.Name() != "Close" {
		return writerFinalizerSpec{}, false
	}
	if constructor, _ := ast.Unparen(selector.X).(*ast.CallExpr); constructor != nil {
		return writerEncoderConstructor(info, constructor)
	}
	receiver := directObject(info, selector.X)
	if receiver == nil {
		return writerFinalizerSpec{}, false
	}
	return stableWriterEncoderAcquisition(info, syntax, receiver, call)
}

func stableWriterEncoderAcquisition(
	info *types.Info,
	syntax *ast.File,
	receiver types.Object,
	finalizer ast.Node,
) (writerFinalizerSpec, bool) {
	var spec writerFinalizerSpec
	acquired := false
	stable := true
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			if node == nil || !stable || node.Pos() >= finalizer.Pos() {
				return stable
			}
			switch node := node.(type) {
			case *ast.AssignStmt:
				if len(node.Lhs) != len(node.Rhs) {
					return true
				}
				for index, left := range node.Lhs {
					if directObject(info, left) != receiver {
						continue
					}
					identifier, _ := left.(*ast.Ident)
					constructor, _ := ast.Unparen(
						node.Rhs[index],
					).(*ast.CallExpr)
					candidate, matched := writerEncoderConstructor(
						info,
						constructor,
					)
					if !acquired &&
						identifier != nil &&
						info.Defs[identifier] == receiver &&
						matched {
						spec = candidate
						acquired = true
						continue
					}
					stable = false
					return false
				}
			case *ast.ValueSpec:
				if len(node.Names) != len(node.Values) {
					return true
				}
				for index, name := range node.Names {
					if info.Defs[name] != receiver {
						continue
					}
					constructor, _ := ast.Unparen(
						node.Values[index],
					).(*ast.CallExpr)
					candidate, matched := writerEncoderConstructor(
						info,
						constructor,
					)
					if acquired || !matched {
						stable = false
						return false
					}
					spec = candidate
					acquired = true
				}
			}
			return true
		},
	)
	return spec, acquired && stable
}

func writerEncoderConstructor(info *types.Info, call *ast.CallExpr) (writerFinalizerSpec, bool) {
	if info == nil || call == nil {
		return writerFinalizerSpec{}, false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil || function.Pkg() == nil || function.Name() != "NewEncoder" {
		return writerFinalizerSpec{}, false
	}
	for _, spec := range writerEncoderSpecs {
		if function.Pkg().Path() == spec.packagePath {
			return spec, true
		}
	}
	return writerFinalizerSpec{}, false
}

func isWriterFinalizer(ctx *TypesContext, call *ast.CallExpr) bool {
	_, matched := writerFinalizer(ctx, call)
	return matched
}
