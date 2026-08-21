package analysis

import (
	"context"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestManagedResultAnalysisDelegatesOnlyExactReturningResults(t *testing.T) {
	t.Parallel()

	package_ := returnStateTestPackage(
		t,
		`package fixture

import "testing"

type Resource struct{}

func (*Resource) Close() error { return nil }

func Direct(t *testing.T) *Resource {
	value := &Resource{}
	t.Cleanup(func() { _ = value.Close() })
	return value
}

func DirectTuple(t *testing.T) (*Resource, error) {
	value := &Resource{}
	t.Cleanup(func() { _ = value.Close() })
	return value, nil
}

func Delegated(t *testing.T) *Resource { return Direct(t) }
func DelegatedTwice(t *testing.T) *Resource { return Delegated(t) }
func DelegatedTuple(t *testing.T) (*Resource, error) { return DirectTuple(t) }

func Recursive(t *testing.T, recurse bool) *Resource {
	if recurse { return Recursive(t, false) }
	return Direct(t)
}

func MutualA(t *testing.T) *Resource { return MutualB(t) }
func MutualB(t *testing.T) *Resource { return MutualA(t) }

func Dynamic(t *testing.T) *Resource {
	operation := Direct
	return operation(t)
}

func Mixed(t *testing.T, managed bool) *Resource {
	if managed { return Direct(t) }
	return &Resource{}
}

type ConvertedResource Resource

func (*ConvertedResource) Close() error { return nil }

func Converted(t *testing.T) *ConvertedResource {
	return (*ConvertedResource)(Direct(t))
}

func DeferredPanic(t *testing.T) *Resource {
	defer func() { panic("deferred") }()
	return Direct(t)
}

func DeferredHelperPanic(t *testing.T) *Resource {
	defer stop()
	return Direct(t)
}

func Unreachable(t *testing.T) *Resource {
	stop()
	return Direct(t)
}

func stop() { panic("stop") }
`,
	)
	ctx := context.Background()
	noReturns := newNoReturnAnalysis(ctx, []*packages.Package{package_}, nil)
	analysis := newManagedResultAnalysis(ctx, []*packages.Package{package_}, nil, noReturns)
	analysis.buildAll()

	for _, name := range
		[]string{"Direct", "DirectTuple", "Delegated", "DelegatedTwice", "DelegatedTuple"} {
		if !managedResultForTest(analysis, package_, name, 0) {
			t.Fatalf("%s result 0 is not cleanup-managed", name)
		}
	}
	for _, name := range
		[]string{
			"Recursive",
			"MutualA",
			"MutualB",
			"Dynamic",
			"Mixed",
			"Converted",
			"DeferredPanic",
			"DeferredHelperPanic",
			"Unreachable",
		} {
		if managedResultForTest(analysis, package_, name, 0) {
			t.Fatalf("%s result 0 is unexpectedly cleanup-managed", name)
		}
	}
	if managedResultForTest(analysis, package_, "DelegatedTuple", 1) {
		t.Fatal("DelegatedTuple error result is unexpectedly cleanup-managed")
	}
}

func managedResultForTest(
	analysis *managedResultAnalysis,
	package_ *packages.Package,
	name string,
	index int,
) bool {
	function, _ := package_.Types.Scope().Lookup(name).(*types.Func)
	_, managed := analysis.summaries[function][index]
	return managed
}
