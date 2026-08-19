package rules

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"
)

type blankErrorDiscardRule struct{}

// NewBlankErrorDiscardRule constructs the explicit blank-error policy rule.
func NewBlankErrorDiscardRule() Rule {
	return blankErrorDiscardRule{}
}

func (blankErrorDiscardRule) Metadata() Metadata {
	includeTestsDefault := BooleanOption(false)
	return Metadata{
		ID: "blank-error-discard",
		Summary: "detects error values explicitly assigned to blank identifiers",
		Documentation: "Assigning an error value to the blank identifier suppresses the compiler's unused-value protection while discarding the failure channel. This restriction rule makes that choice auditable for projects that require every error to be handled or suppressed with a reason. It is enabled only by exact rule ID and complements the suspicious discarded-error rule for bare call statements.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetRestriction},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeAssignStmt},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		Options: []OptionMetadata{
			{
				Name: "include-tests",
				Summary: "report blank error discards in files whose base name ends in _test.go",
				Kind: OptionBoolean,
				Default: &includeTestsDefault,
			},
		},
		KnownLimitations: []string{
			"The rule intentionally reports deliberate best-effort error discards; enable it only when project policy requires a reasoned suppression for those sites.",
			"Formatted-output and documented always-nil in-memory writer results excluded by discarded-error are excluded here as well.",
			"Exact standard-library buffered-writer Flush and Close calls are owned by the default correctness rule unchecked-writer-error to avoid duplicate diagnostics.",
			"Blank identifiers in declarations and implicit discards outside assignment statements are not covered.",
			"Test files are excluded by default; include-tests enables the same policy for tests.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Handle or explain discarded errors",
				Incorrect: "_ = file.Close()",
				Correct: "if err := file.Close(); err != nil { return err }",
			},
		},
	}
}

func (blankErrorDiscardRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	assignment, ok := node.(*ast.AssignStmt)
	if !ok {
		return nil, fmt.Errorf("blank-error-discard requires an assignment statement")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("blank-error-discard requires complete type information")
	}
	includeTests, found := ctx.BooleanOption("include-tests")
	if !found {
		return nil, fmt.Errorf("blank-error-discard requires the include-tests option")
	}
	if !includeTests && strings.HasSuffix(filepath.Base(ctx.File().Path()), "_test.go") {
		return nil, nil
	}
	resultTypes := blankAssignmentResultTypes(ctx.Info(), assignment)
	findings := make([]Finding, 0)
	for index, expression := range assignment.Lhs {
		if !blankIdentifier(expression) ||
			index >= len(resultTypes) ||
			!assignableToError(resultTypes[index]) {
			continue
		}
		range_, err := ctx.Range(expression)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "blank-error-discard",
				Message: "this error value is discarded through the blank identifier",
				Range: range_,
				Help: "handle the error or suppress this deliberate discard with a reason",
			},
		)
	}
	return findings, nil
}

func blankAssignmentResultTypes(info *types.Info, assignment *ast.AssignStmt) []types.Type {
	if info == nil || assignment == nil || len(assignment.Lhs) == 0 {
		return nil
	}
	if len(assignment.Rhs) == 1 {
		if call, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr);
			call != nil &&
				(infallibleDiscardedCall(info, call) ||
					isWriterFinalizer(info, call)) {
			return nil
		}
		type_ := info.TypeOf(assignment.Rhs[0])
		if tuple, _ := types.Unalias(type_).(*types.Tuple); tuple != nil {
			if tuple.Len() != len(assignment.Lhs) {
				return nil
			}
			result := make([]types.Type, tuple.Len())
			for index := range tuple.Len() {
				result[index] = tuple.At(index).Type()
			}
			return result
		}
		if len(assignment.Lhs) == 1 {
			return []types.Type{type_}
		}
		return nil
	}
	if len(assignment.Rhs) != len(assignment.Lhs) {
		return nil
	}
	result := make([]types.Type, len(assignment.Rhs))
	for index, expression := range assignment.Rhs {
		if call, _ := ast.Unparen(expression).(*ast.CallExpr);
			call != nil &&
				(infallibleDiscardedCall(info, call) ||
					isWriterFinalizer(info, call)) {
			continue
		}
		result[index] = info.TypeOf(expression)
	}
	return result
}

func assignableToError(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	return types.AssignableTo(type_, errorType)
}
