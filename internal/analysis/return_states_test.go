package analysis

import (
	"context"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
)

func TestReturnStateAnalysisSummarizesOnlyProvenExplicitRelationships(t *testing.T) {
	t.Parallel()

	package_ := returnStateTestPackage(
		t,
		`package fixture

import (
	"errors"
	"fmt"
)

type Value struct{}
type typedError struct{}
func (*typedError) Error() string { return "typed" }

func Known(found bool) (*Value, error) {
	if !found { return nil, errors.New("missing") }
	return &Value{}, nil
}

func Formatted(found bool) (*Value, error) {
	if !found { return nil, fmt.Errorf("missing") }
	return new(Value), nil
}

func DelegatedKnown(found bool) (*Value, error) { return Known(found) }
func DelegatedFormatted(found bool) (*Value, error) { return Formatted(found) }
func MutualKnownA(found bool) (*Value, error) { return MutualKnownB(found) }
func MutualKnownB(found bool) (*Value, error) { return MutualKnownA(found) }
func DynamicKnown(operation func(bool) (*Value, error), found bool) (*Value, error) {
	return operation(found)
}
func DeferredKnown(found bool) (*Value, error) {
	defer func() { panic("deferred") }()
	return Known(found)
}
func DeferredHelperKnown(found bool) (*Value, error) {
	defer stopDeferredReturn()
	return Known(found)
}
func UnreachableKnown(found bool) (*Value, error) {
	stopDeferredReturn()
	return Known(found)
}

func Nested() (*Value, error) {
	_ = func() (*Value, error) { return nil, errors.New("nested") }
	return &Value{}, nil
}

func Bare() (value *Value, err error) { return }
func Unknown(value *Value, err error) (*Value, error) { return value, err }
func Recursive() (*Value, error) { return Recursive() }
func Indirect(value *Value) (*Value, error) { return &*value, nil }

func NilError() error { return nil }
func NonNilError() error { return errors.New("failed") }
func NilTuple() (*Value, error) { return &Value{}, nil }
func DelegatedNilError() error { return NilError() }
func DelegatedNonNilError() error { return NonNilError() }
func DelegatedTuple() (*Value, error) { return NilTuple() }
func UnknownError(err error) error { return err }
func RecursiveError() error { return RecursiveError() }
func MutualErrorA() error { return MutualErrorB() }
func MutualErrorB() error { return MutualErrorA() }
func TypedNilError() error {
	var err *typedError
	return err
}

func DeferredNilError() (err error) {
	defer func() { err = errors.New("deferred") }()
	return nil
}

func DeferredPanicNilError() error {
	defer func() { panic("deferred") }()
	return nil
}

func DeferredHelperPanicNilError() error {
	defer stopDeferredReturn()
	return nil
}

func stopDeferredReturn() { panic("deferred") }

func UnreachableNilError() error {
	stopDeferredReturn()
	return nil
}

func AddressEscapedNilError() (err error) {
	captureError(&err)
	return nil
}

func HarmlessDeferredNilError() (err error) {
	defer func() {}()
	return nil
}

func captureError(*error) {}

func ConflictingError(found bool) error {
	if found { return nil }
	return errors.New("failed")
}

func Conflicting(found bool) (*Value, error) {
	if found { return nil, nil }
	return &Value{}, nil
}
`,
	)
	ctx := context.Background()
	noReturns := newNoReturnAnalysis(ctx, []*packages.Package{package_}, nil)
	analysis := newReturnStateAnalysis(ctx, []*packages.Package{package_}, nil, noReturns)
	analysis.buildAll()

	wantKnown := rules.ReturnStateSummary{
		WhenErrorNil: rules.NilStateNonNil,
		WhenErrorNonNil: rules.NilStateNil,
	}
	for _, name := range
		[]string{"Known", "Formatted", "DelegatedKnown", "DelegatedFormatted"} {
		if got := returnStateForTest(analysis, package_, name); got != wantKnown {
			t.Fatalf("%s summary = %#v, want %#v", name, got, wantKnown)
		}
	}
	if got := returnStateForTest(analysis, package_, "Nested");
		got.WhenErrorNil != rules.NilStateNonNil ||
			got.WhenErrorNonNil != rules.NilStateUnknown {
		t.Fatalf("Nested summary = %#v", got)
	}
	for _, name := range
		[]string{
			"Bare",
			"Unknown",
			"Recursive",
			"MutualKnownA",
			"MutualKnownB",
			"DynamicKnown",
			"DeferredKnown",
			"DeferredHelperKnown",
			"UnreachableKnown",
			"Indirect",
			"Conflicting",
		} {
		if got := returnStateForTest(analysis, package_, name);
			got != (rules.ReturnStateSummary{}) {
			t.Fatalf("%s summary = %#v, want unknown", name, got)
		}
	}
	if got := resultStateForTest(analysis, package_, "NilError", 0); got != rules.NilStateNil {
		t.Fatalf("NilError result state = %v, want nil", got)
	}
	if got := resultStateForTest(analysis, package_, "NonNilError", 0);
		got != rules.NilStateNonNil {
		t.Fatalf("NonNilError result state = %v, want non-nil", got)
	}
	if got := resultStateForTest(analysis, package_, "DelegatedNilError", 0);
		got != rules.NilStateNil {
		t.Fatalf("DelegatedNilError result state = %v, want nil", got)
	}
	if got := resultStateForTest(analysis, package_, "DelegatedNonNilError", 0);
		got != rules.NilStateNonNil {
		t.Fatalf("DelegatedNonNilError result state = %v, want non-nil", got)
	}
	if got := resultStateForTest(analysis, package_, "DelegatedTuple", 1);
		got != rules.NilStateNil {
		t.Fatalf("DelegatedTuple error result state = %v, want nil", got)
	}
	if got := resultStateForTest(analysis, package_, "Known", 1); got != rules.NilStateUnknown {
		t.Fatalf("Known error result state = %v, want unknown", got)
	}
	for _, name := range
		[]string{
			"UnknownError",
			"RecursiveError",
			"MutualErrorA",
			"MutualErrorB",
			"TypedNilError",
			"DeferredNilError",
			"DeferredPanicNilError",
			"DeferredHelperPanicNilError",
			"UnreachableNilError",
			"AddressEscapedNilError",
			"ConflictingError",
		} {
		if got := resultStateForTest(analysis, package_, name, 0);
			got != rules.NilStateUnknown {
			t.Fatalf("%s result state = %v, want unknown", name, got)
		}
	}
	if got := resultStateForTest(analysis, package_, "HarmlessDeferredNilError", 0);
		got != rules.NilStateNil {
		t.Fatalf("HarmlessDeferredNilError result state = %v, want nil", got)
	}
}

func TestReturnStateAnalysisStopsAfterCancellation(t *testing.T) {
	t.Parallel()

	package_ := returnStateTestPackage(
		t,
		`package fixture

type Value struct{}
func Lookup() (*Value, error) { return &Value{}, nil }
`,
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	noReturns := newNoReturnAnalysis(ctx, []*packages.Package{package_}, nil)
	analysis := newReturnStateAnalysis(ctx, []*packages.Package{package_}, nil, noReturns)
	analysis.buildAll()
	if len(analysis.summaries) != 0 || len(analysis.resultStates) != 0 {
		t.Fatalf(
			"canceled return-state analysis = summaries %#v, results %#v",
			analysis.summaries,
			analysis.resultStates,
		)
	}
}

func returnStateTestPackage(t *testing.T, source string) *packages.Package {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "fixture.go", source, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	checked, err := (&types.Config{
		Importer: importer.Default(),
	}).Check("example.com/fixture", fileSet, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	return &packages.Package{
		ID: "example.com/fixture",
		PkgPath: "example.com/fixture",
		Fset: fileSet,
		Types: checked,
		TypesInfo: info,
		Syntax: []*ast.File{file},
	}
}

func returnStateForTest(
	analysis *returnStateAnalysis,
	package_ *packages.Package,
	name string,
) rules.ReturnStateSummary {
	function, _ := package_.Types.Scope().Lookup(name).(*types.Func)
	return analysis.summaries[function][returnStateKey{value: 0, error: 1}]
}

func resultStateForTest(
	analysis *returnStateAnalysis,
	package_ *packages.Package,
	name string,
	index int,
) rules.NilState {
	function, _ := package_.Types.Scope().Lookup(name).(*types.Func)
	return analysis.resultStates[function][index]
}
