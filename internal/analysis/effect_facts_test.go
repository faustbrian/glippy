package analysis

import (
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
)

func TestNativeEffectFactsUseStableCrossLoadFunctionIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/terminate", "Now")
	second := effectTestFunction("example.com/project/terminate", "Now")
	other := effectTestFunction("example.com/project/terminate", "Later")
	facts := newNativeEffectFacts()
	facts.noReturns[stableFunctionIdentity(first)] = struct{}{}
	if !facts.noReturn(second) {
		t.Fatal("effect fact did not survive an independent type identity")
	}
	if facts.noReturn(other) {
		t.Fatal("effect fact matched another function")
	}
}

func TestNativeEffectFactDigestIsOrderedAndContentSensitive(t *testing.T) {
	t.Parallel()

	first := newNativeEffectFacts()
	first.noReturns["z"] = struct{}{}
	first.noReturns["a"] = struct{}{}
	second := newNativeEffectFacts()
	second.noReturns["a"] = struct{}{}
	second.noReturns["z"] = struct{}{}
	first.parameters["function"] = map[int]rules.ParameterEffectSummary{
		1: {
			Known: true,
			Always: true,
			Kinds: rules.ParameterEffectTransfer,
			GuaranteedKinds: rules.ParameterEffectTransfer,
		},
		0: {Known: true},
	}
	second.parameters["function"] = map[int]rules.ParameterEffectSummary{
		0: {Known: true},
		1: {
			Known: true,
			Always: true,
			Kinds: rules.ParameterEffectTransfer,
			GuaranteedKinds: rules.ParameterEffectTransfer,
		},
	}
	first.receivers["method"] = rules.ParameterEffectSummary{
		Known: true,
		Always: true,
		Kinds: rules.ParameterEffectClose,
		GuaranteedKinds: rules.ParameterEffectClose,
	}
	second.receivers["method"] = rules.ParameterEffectSummary{
		Known: true,
		Always: true,
		Kinds: rules.ParameterEffectClose,
		GuaranteedKinds: rules.ParameterEffectClose,
	}
	first.cleanupManaged["function"] = map[int]struct{}{1: {}, 0: {}}
	second.cleanupManaged["function"] = map[int]struct{}{0: {}, 1: {}}
	changed := newNativeEffectFacts()
	changed.noReturns["a"] = struct{}{}
	if first.digest() != second.digest() {
		t.Fatal("effect fact digest depends on insertion order")
	}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a summary change")
	}
	changed = cloneNativeEffectFacts(first)
	changed.parameters["function"][1] = rules.ParameterEffectSummary{
		Known: true,
		Always: true,
		Kinds: rules.ParameterEffectClose,
		GuaranteedKinds: rules.ParameterEffectClose,
	}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a parameter effect change")
	}
	changed = cloneNativeEffectFacts(first)
	delete(changed.receivers, "method")
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a receiver effect change")
	}
	changed = cloneNativeEffectFacts(first)
	summary := changed.parameters["function"][1]
	summary.GuaranteedKinds = 0
	changed.parameters["function"][1] = summary
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored independently guaranteed effects")
	}
	changed = cloneNativeEffectFacts(first)
	changed.returns["function"] = map[returnStateKey]rules.ReturnStateSummary{
		{
			value: 0,
			error: 1,
		}: {WhenErrorNil: rules.NilStateNonNil, WhenErrorNonNil: rules.NilStateNil},
	}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a return-state change")
	}
	changed = cloneNativeEffectFacts(first)
	changed.mustUse["function"] = map[int]struct{}{0: {}}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a must-use change")
	}
	changed = cloneNativeEffectFacts(first)
	changed.blocking["function"] = struct{}{}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a blocking change")
	}
	changed = cloneNativeEffectFacts(first)
	changed.aliases["function"] = map[returnAliasKey]struct{}{{result: 0, argument: 0}: {}}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a return-alias change")
	}
	changed = cloneNativeEffectFacts(first)
	delete(changed.cleanupManaged["function"], 1)
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a cleanup-managed result change")
	}
}

func TestNativeParameterEffectsUseStableCrossLoadFunctionIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/resource", "Consume")
	second := effectTestFunction("example.com/project/resource", "Consume")
	facts := newNativeEffectFacts()
	facts.parameters[stableFunctionIdentity(first)] = map[int]rules.ParameterEffectSummary{
		0: {Known: true, Always: true, Kinds: rules.ParameterEffectClose},
	}
	if summary := facts.ParameterEffect(second, 0);
		!summary.GuaranteesAny(rules.ParameterEffectClose) {
		t.Fatalf(
			"parameter effect did not survive an independent type identity: %#v",
			summary,
		)
	}
	if summary := facts.ParameterEffect(second, 1); summary.Known {
		t.Fatalf("parameter effect matched another parameter: %#v", summary)
	}
}

func TestNativeReceiverEffectsUseStableCrossLoadMethodIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestMethod("example.com/project/resource", "Resource", "Shutdown")
	second := effectTestMethod("example.com/project/resource", "Resource", "Shutdown")
	other := effectTestMethod("example.com/project/resource", "Resource", "Observe")
	facts := newNativeEffectFacts()
	facts.receivers[stableFunctionIdentity(first)] = rules.ParameterEffectSummary{
		Known: true,
		Always: true,
		Kinds: rules.ParameterEffectClose,
		GuaranteedKinds: rules.ParameterEffectClose,
	}
	if summary := facts.ReceiverEffect(second);
		!summary.GuaranteesAny(rules.ParameterEffectClose) {
		t.Fatalf(
			"receiver effect did not survive an independent type identity: %#v",
			summary,
		)
	}
	if summary := facts.ReceiverEffect(other); summary.Known {
		t.Fatalf("receiver effect matched another method: %#v", summary)
	}
}

func TestReceiverEffectFactsIntersectPackageVariants(t *testing.T) {
	t.Parallel()

	first := effectTestMethod("example.com/project/resource", "Resource", "Shutdown")
	second := effectTestMethod("example.com/project/resource", "Resource", "Shutdown")
	closeSummary := rules.ParameterEffectSummary{
		Known: true,
		Always: true,
		Kinds: rules.ParameterEffectClose,
		GuaranteedKinds: rules.ParameterEffectClose,
	}
	analysis := &parameterEffectAnalysis{
		definitions: map[*types.Func]*parameterEffectDefinition{
			first: {
				signature: first.Type().(*types.Signature),
				receiver: closeSummary,
				receiverBuilt: true,
			},
			second: {
				signature: second.Type().(*types.Signature),
				receiver: rules.ParameterEffectSummary{Known: true},
				receiverBuilt: true,
			},
		},
	}
	facts := newNativeEffectFacts()
	facts.addParameterEffects(analysis)
	if summary := facts.ReceiverEffect(first); summary.Known {
		t.Fatalf("receiver effect survived a disagreeing package variant: %#v", summary)
	}

	analysis.definitions[second].receiver = closeSummary
	facts = newNativeEffectFacts()
	facts.addParameterEffects(analysis)
	if summary := facts.ReceiverEffect(second);
		!summary.GuaranteesAny(rules.ParameterEffectClose) {
		t.Fatalf("receiver effect was lost across agreeing package variants: %#v", summary)
	}
}

func TestNativeReturnStatesUseStableCrossLoadFunctionIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/value", "Lookup")
	second := effectTestFunction("example.com/project/value", "Lookup")
	facts := newNativeEffectFacts()
	facts.returns[stableFunctionIdentity(first)] = map[returnStateKey]rules.ReturnStateSummary{
		{
			value: 0,
			error: 1,
		}: {WhenErrorNil: rules.NilStateNonNil, WhenErrorNonNil: rules.NilStateNil},
	}
	summary := facts.ReturnState(second, 0, 1)
	if summary.WhenErrorNil != rules.NilStateNonNil ||
		summary.WhenErrorNonNil != rules.NilStateNil {
		t.Fatalf("return state did not survive an independent type identity: %#v", summary)
	}
	if summary := facts.ReturnState(second, 1, 0); summary != (rules.ReturnStateSummary{}) {
		t.Fatalf("return state matched another result pair: %#v", summary)
	}
}

func TestNativeCleanupManagedResultsUseStableCrossLoadFunctionIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/resource", "Open")
	second := effectTestFunction("example.com/project/resource", "Open")
	facts := newNativeEffectFacts()
	facts.cleanupManaged[stableFunctionIdentity(first)] = map[int]struct{}{0: {}}
	if !facts.CleanupManagedResult(second, 0) {
		t.Fatal("cleanup-managed result did not survive an independent type identity")
	}
	if facts.CleanupManagedResult(second, 1) {
		t.Fatal("cleanup-managed result matched another result index")
	}
}

func TestCleanupManagedFactsIntersectPackageVariants(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/resource", "Open")
	second := effectTestFunction("example.com/project/resource", "Open")
	analysis := &managedResultAnalysis{
		definitions: []managedResultDefinition{{function: first}, {function: second}},
		summaries: map[*types.Func]map[int]struct{}{first: {0: {}}},
	}
	facts := newNativeEffectFacts()
	facts.addCleanupManagedResults(analysis)
	if facts.CleanupManagedResult(first, 0) {
		t.Fatal("cleanup-managed result survived a disagreeing package variant")
	}
	analysis.summaries[second] = map[int]struct{}{0: {}}
	facts = newNativeEffectFacts()
	facts.addCleanupManagedResults(analysis)
	if !facts.CleanupManagedResult(second, 0) {
		t.Fatal("cleanup-managed result was lost across agreeing package variants")
	}
}

func TestEffectPathsStayWithinSelectedModuleBoundaries(t *testing.T) {
	t.Parallel()

	prefixes := []string{"example.com/project"}
	if !effectPathWithinModules("example.com/project/internal/stop", prefixes) {
		t.Fatal("same-module effect path was rejected")
	}
	for _, path := range
		[]string{"example.com/projector/stop", "example.com/dependency/stop", "runtime"} {
		if effectPathWithinModules(path, prefixes) {
			t.Fatalf("external effect path %q was accepted", path)
		}
	}
}

func TestEffectModulePrefixesIncludeReachableLocalModulesOnly(t *testing.T) {
	t.Parallel()

	workspace := &packages.Package{
		PkgPath: "example.com/workspace/helper",
		Module: &packages.Module{Path: "example.com/workspace", Main: true},
	}
	replacement := &packages.Package{
		PkgPath: "example.com/replaced/helper",
		Module: &packages.Module{
			Path: "example.com/replaced",
			Replace: &packages.Module{Dir: "/workspace/replaced"},
		},
	}
	thirdParty := &packages.Package{
		PkgPath: "example.com/thirdparty/helper",
		Module: &packages.Module{
			Path: "example.com/thirdparty",
			Dir: "/module-cache/example.com/thirdparty",
		},
	}
	root := &packages.Package{
		PkgPath: "example.com/root",
		Module: &packages.Module{Path: "example.com/root", Main: true},
		Imports: map[string]*packages.Package{
			workspace.PkgPath: workspace,
			replacement.PkgPath: replacement,
			thirdParty.PkgPath: thirdParty,
		},
	}
	prefixes := effectModulePrefixes([]*packages.Package{root})
	want := []string{"example.com/replaced", "example.com/root", "example.com/workspace"}
	if !slices.Equal(prefixes, want) {
		t.Fatalf("effect module prefixes = %q, want %q", prefixes, want)
	}
}

func effectTestFunction(packagePath string, name string) *types.Func {
	package_ := types.NewPackage(packagePath, "terminate")
	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	return types.NewFunc(token.NoPos, package_, name, signature)
}

func effectTestMethod(packagePath string, typeName string, name string) *types.Func {
	package_ := types.NewPackage(packagePath, "resource")
	named := types.NewNamed(
		types.NewTypeName(token.NoPos, package_, typeName, nil),
		types.NewStruct(nil, nil),
		nil,
	)
	receiver := types.NewVar(token.NoPos, package_, "receiver", types.NewPointer(named))
	signature := types.NewSignatureType(receiver, nil, nil, nil, nil, false)
	return types.NewFunc(token.NoPos, package_, name, signature)
}
