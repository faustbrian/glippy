package rules

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"
)

type discardedErrorRule struct{}

// NewDiscardedErrorRule constructs the ignored-error-result rule for product
// registry composition.
func NewDiscardedErrorRule() Rule {
	return discardedErrorRule{}
}

func (discardedErrorRule) Metadata() Metadata {
	includeTestsDefault := BooleanOption(false)
	return Metadata{
		ID: "discarded-error",
		Summary: "detects call statements that discard an error result",
		Documentation: "A call used as a statement discards every returned value. When one result implements error, that can silently hide a failed operation. The rule intentionally remains opt-in because explicitly best-effort operations may need a narrow suppression.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeExprStmt},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		Options: []OptionMetadata{
			{
				Name: "include-tests",
				Summary: "report discarded errors in files whose base name ends in _test.go",
				Kind: OptionBoolean,
				Default: &includeTestsDefault,
			},
		},
		KnownLimitations: []string{
			"The rule covers direct call statements; errors explicitly assigned to a blank identifier remain outside this rule.",
			"Known in-memory writers whose error result is documented as always nil are excluded.",
			"Exact standard-library buffered-writer Flush and Close calls are owned by the default correctness rule unchecked-writer-error to avoid duplicate diagnostics.",
			"Test files are excluded by default because fixture-driving calls frequently discard deliberately injected errors; include-tests enables them.",
			"Best-effort calls must be handled explicitly or suppressed with a reason.",
		},
		Examples: []Example{
			{
				Title: "Handle a returned error",
				Incorrect: "persist(value)",
				Correct: "if err := persist(value); err != nil { return err }",
			},
		},
	}
}

func (discardedErrorRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.ExprStmt)
	if !ok {
		return nil, fmt.Errorf("discarded-error requires an expression statement")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("discarded-error requires complete type information")
	}
	includeTests, found := ctx.BooleanOption("include-tests")
	if !found {
		return nil, fmt.Errorf("discarded-error requires the include-tests option")
	}
	if !includeTests && strings.HasSuffix(filepath.Base(ctx.File().Path()), "_test.go") {
		return nil, nil
	}
	call, _ := ast.Unparen(statement.X).(*ast.CallExpr)
	if call == nil ||
		infallibleDiscardedCall(ctx.Info(), call) ||
		isWriterFinalizer(ctx.Info(), call) {
		return nil, nil
	}
	signature, _ := types.Unalias(ctx.Info().TypeOf(call.Fun)).(*types.Signature)
	if signature == nil || !tupleIncludesError(signature.Results()) {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "discarded-error",
			Message: "this call's error result is discarded",
			Range: range_,
			Help: "handle the error explicitly or suppress this best-effort call with a reason",
		},
	}, nil
}

func tupleIncludesError(results *types.Tuple) bool {
	if results == nil {
		return false
	}
	errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	for index := range results.Len() {
		resultType := results.At(index).Type()
		if types.Implements(resultType, errorType) ||
			types.Implements(types.NewPointer(resultType), errorType) {
			return true
		}
	}
	return false
}

func infallibleDiscardedCall(info *types.Info, call *ast.CallExpr) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	if function, _ := info.ObjectOf(selector.Sel).(*types.Func);
		function != nil && function.Pkg() != nil && function.Pkg().Path() == "fmt" {
		switch function.Name() {
		case "Print", "Printf", "Println", "Fprint", "Fprintf", "Fprintln":
			return true
		}
	}
	selection := info.Selections[selector]
	if selection == nil {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil || function.Pkg() == nil {
		return false
	}
	switch function.Pkg().Path() {
	case "bytes":
		switch function.Name() {
		case "Write", "WriteByte", "WriteRune", "WriteString":
			return namedReceiver(selection.Recv(), "bytes", "Buffer")
		}
	case "strings":
		switch function.Name() {
		case "Write", "WriteByte", "WriteRune", "WriteString":
			return namedReceiver(selection.Recv(), "strings", "Builder")
		}
	}
	return false
}

func namedReceiver(type_ types.Type, packagePath, name string) bool {
	if pointer, ok := types.Unalias(type_).(*types.Pointer); ok {
		type_ = pointer.Elem()
	}
	named, _ := types.Unalias(type_).(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == packagePath &&
		named.Obj().Name() == name
}
