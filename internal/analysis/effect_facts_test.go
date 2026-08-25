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

func TestNativeTestingSkipFactsUseStableCrossLoadFunctionIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/terminate", "Skip")
	second := effectTestFunction("example.com/project/terminate", "Skip")
	other := effectTestFunction("example.com/project/terminate", "Stop")
	facts := newNativeEffectFacts()
	facts.testingSkips[stableFunctionIdentity(first)] = struct{}{}
	if !facts.testingSkip(second) {
		t.Fatal("testing-skip effect fact did not survive an independent type identity")
	}
	if facts.testingSkip(other) {
		t.Fatal("testing-skip effect fact matched another function")
	}
	if !cloneNativeEffectFacts(facts).testingSkip(second) {
		t.Fatal("testing-skip effect fact did not survive cloning")
	}
}

func TestNativeTestingFailureFactsUseStableCrossLoadFunctionIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/terminate", "Fatal")
	second := effectTestFunction("example.com/project/terminate", "Fatal")
	other := effectTestFunction("example.com/project/terminate", "Stop")
	facts := newNativeEffectFacts()
	facts.testingFailures[stableFunctionIdentity(first)] = struct{}{}
	if !facts.testingFailure(second) {
		t.Fatal("testing-failure effect fact did not survive an independent type identity")
	}
	if facts.testingFailure(other) {
		t.Fatal("testing-failure effect fact matched another function")
	}
	if !cloneNativeEffectFacts(facts).testingFailure(second) {
		t.Fatal("testing-failure effect fact did not survive cloning")
	}
}

func TestTestingSkipFactsIntersectPackageVariants(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/terminate", "Skip")
	second := effectTestFunction("example.com/project/terminate", "Skip")
	analysis := &noReturnAnalysis{
		definitions: map[*types.Func]*noReturnDefinition{
			first: {
				noReturn: true,
				testingSkipBuilt: true,
				testingSkip: true,
				testingFailureBuilt: true,
			},
			second: {noReturn: true, testingSkipBuilt: true, testingFailureBuilt: true},
		},
	}
	facts := newNativeEffectFacts()
	facts.addNoReturns(analysis)
	if facts.testingSkip(first) {
		t.Fatal("testing-skip fact survived a disagreeing package variant")
	}

	analysis.definitions[second].testingSkip = true
	facts = newNativeEffectFacts()
	facts.addNoReturns(analysis)
	if !facts.testingSkip(first) {
		t.Fatal("testing-skip fact was lost across agreeing package variants")
	}
}

func TestTestingFailureFactsIntersectPackageVariants(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/terminate", "Fatal")
	second := effectTestFunction("example.com/project/terminate", "Fatal")
	analysis := &noReturnAnalysis{
		definitions: map[*types.Func]*noReturnDefinition{
			first: {
				noReturn: true,
				testingSkipBuilt: true,
				testingFailureBuilt: true,
				testingFailure: true,
			},
			second: {noReturn: true, testingSkipBuilt: true, testingFailureBuilt: true},
		},
	}
	facts := newNativeEffectFacts()
	facts.addNoReturns(analysis)
	if facts.testingFailure(first) {
		t.Fatal("testing-failure fact survived a disagreeing package variant")
	}

	analysis.definitions[second].testingFailure = true
	facts = newNativeEffectFacts()
	facts.addNoReturns(analysis)
	if !facts.testingFailure(first) {
		t.Fatal("testing-failure fact was lost across agreeing package variants")
	}
}

func TestTestingFailureMethodsAreAuthoritativeNoReturnCalls(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"FailNow", "Fatal", "Fatalf"} {
		failure := effectTestMethod("testing", "T", name)
		if !isAuthoritativeNoReturn(failure) || !isAuthoritativeTestingFailure(failure) {
			t.Fatalf("testing.T.%s was not classified as a testing failure", name)
		}
	}
	for _, name := range []string{"Skip", "Skipf", "SkipNow", "Error", "Fail"} {
		nonFailure := effectTestMethod("testing", "T", name)
		if isAuthoritativeTestingFailure(nonFailure) {
			t.Fatalf("testing.T.%s was classified as a testing failure", name)
		}
	}
	lookalike := effectTestMethod("example.com/testing", "T", "Fatal")
	if isAuthoritativeNoReturn(lookalike) || isAuthoritativeTestingFailure(lookalike) {
		t.Fatal("testing failure lookalike was classified as authoritative")
	}
}

func TestGinkgoSkipIsAnAuthoritativeTestingTermination(t *testing.T) {
	t.Parallel()

	for _, packagePath := range
		[]string{"github.com/onsi/ginkgo", "github.com/onsi/ginkgo/v2"} {
		skip := effectTestFunction(packagePath, "Skip")
		if !isAuthoritativeNoReturn(skip) || !isAuthoritativeTestingSkip(skip) {
			t.Fatalf(
				"%s.Skip was not classified as an authoritative testing termination",
				packagePath,
			)
		}
		lookalike := effectTestFunction(packagePath, "Skipf")
		if isAuthoritativeNoReturn(lookalike) || isAuthoritativeTestingSkip(lookalike) {
			t.Fatalf(
				"%s.Skipf was classified as an authoritative testing termination",
				packagePath,
			)
		}
	}
	other := effectTestFunction("example.com/ginkgo", "Skip")
	if isAuthoritativeNoReturn(other) || isAuthoritativeTestingSkip(other) {
		t.Fatal("non-Ginkgo Skip was classified as an authoritative testing termination")
	}
}

func TestNativeEffectFactDigestIsOrderedAndContentSensitive(t *testing.T) {
	t.Parallel()

	first := newNativeEffectFacts()
	first.noReturns["z"] = struct{}{}
	first.noReturns["a"] = struct{}{}
	first.testingSkips["z-skip"] = struct{}{}
	first.testingSkips["a-skip"] = struct{}{}
	first.testingFailures["z-failure"] = struct{}{}
	first.testingFailures["a-failure"] = struct{}{}
	second := newNativeEffectFacts()
	second.noReturns["a"] = struct{}{}
	second.noReturns["z"] = struct{}{}
	second.testingSkips["a-skip"] = struct{}{}
	second.testingSkips["z-skip"] = struct{}{}
	second.testingFailures["a-failure"] = struct{}{}
	second.testingFailures["z-failure"] = struct{}{}
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
	first.noOpCloses["no-op-close"] = struct{}{}
	second.noOpCloses["no-op-close"] = struct{}{}
	first.results["function"] = map[int]rules.NilState{
		1: rules.NilStateNil,
		0: rules.NilStateNonNil,
	}
	second.results["function"] = map[int]rules.NilState{
		0: rules.NilStateNonNil,
		1: rules.NilStateNil,
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
	delete(changed.testingSkips, "a-skip")
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a testing-skip change")
	}
	changed = cloneNativeEffectFacts(first)
	delete(changed.testingFailures, "a-failure")
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a testing-failure change")
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
	delete(changed.noOpCloses, "no-op-close")
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a no-op close change")
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
	changed.results["function"] = map[int]rules.NilState{0: rules.NilStateNil}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a result-state change")
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

func TestConventionalCloseMethodAcceptsBuiltInErrorResult(t *testing.T) {
	t.Parallel()

	package_ := types.NewPackage("example.com/project/resource", "resource")
	named := types.NewNamed(
		types.NewTypeName(token.NoPos, package_, "Resource", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	receiver := types.NewVar(token.NoPos, package_, "receiver", types.NewPointer(named))
	results := types.NewTuple(
		types.NewVar(token.NoPos, package_, "", types.Universe.Lookup("error").Type()),
	)
	signature := types.NewSignatureType(receiver, nil, nil, nil, results, false)
	function := types.NewFunc(token.NoPos, package_, "Close", signature)
	if !conventionalCloseMethod(function, signature) {
		t.Fatal("built-in error result was not recognized as a conventional close method")
	}
}

func TestNativeNoOpClosesUseStableCrossLoadMethodIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestCloseMethod("example.com/project/resource", "Resource")
	second := effectTestCloseMethod("example.com/project/resource", "Resource")
	other := effectTestCloseMethod("example.com/project/resource", "Other")
	facts := newNativeEffectFacts()
	facts.noOpCloses[stableFunctionIdentity(first)] = struct{}{}
	if !facts.NoOpClose(second) {
		t.Fatal("no-op close did not survive an independent type identity")
	}
	if facts.NoOpClose(other) {
		t.Fatal("no-op close matched another receiver type")
	}
}

func TestNoOpCloseFactsIntersectPackageVariants(t *testing.T) {
	t.Parallel()

	first := effectTestCloseMethod("example.com/project/resource", "Resource")
	second := effectTestCloseMethod("example.com/project/resource", "Resource")
	analysis := &parameterEffectAnalysis{
		definitions: map[*types.Func]*parameterEffectDefinition{
			first: {
				signature: first.Type().(*types.Signature),
				closeMethod: true,
				noOpClose: true,
			},
			second: {signature: second.Type().(*types.Signature), closeMethod: true},
		},
	}
	facts := newNativeEffectFacts()
	facts.addParameterEffects(analysis)
	if facts.NoOpClose(first) {
		t.Fatal("no-op close survived a disagreeing package variant")
	}

	analysis.definitions[second].noOpClose = true
	facts = newNativeEffectFacts()
	facts.addParameterEffects(analysis)
	if !facts.NoOpClose(second) {
		t.Fatal("no-op close was lost across agreeing package variants")
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

func TestNativeResultStatesUseStableCrossLoadFunctionIdentity(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/value", "Complete")
	second := effectTestFunction("example.com/project/value", "Complete")
	facts := newNativeEffectFacts()
	facts.results[stableFunctionIdentity(first)] = map[int]rules.NilState{0: rules.NilStateNil}
	if state := facts.ResultState(second, 0); state != rules.NilStateNil {
		t.Fatalf("result state did not survive an independent type identity: %v", state)
	}
	if state := facts.ResultState(second, 1); state != rules.NilStateUnknown {
		t.Fatalf("result state matched another result: %v", state)
	}
}

func TestResultStateFactsIntersectPackageVariants(t *testing.T) {
	t.Parallel()

	first := effectTestFunction("example.com/project/value", "Complete")
	second := effectTestFunction("example.com/project/value", "Complete")
	analysis := &returnStateAnalysis{
		definitions: []returnStateDefinition{{function: first}, {function: second}},
		resultStates: map[*types.Func]map[int]rules.NilState{first: {0: rules.NilStateNil}},
	}
	facts := newNativeEffectFacts()
	facts.addResultStates(analysis)
	if state := facts.ResultState(first, 0); state != rules.NilStateUnknown {
		t.Fatalf("result state survived a disagreeing package variant: %v", state)
	}

	analysis.resultStates[second] = map[int]rules.NilState{0: rules.NilStateNil}
	facts = newNativeEffectFacts()
	facts.addResultStates(analysis)
	if state := facts.ResultState(second, 0); state != rules.NilStateNil {
		t.Fatalf("result state was lost across agreeing package variants: %v", state)
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

func effectTestCloseMethod(packagePath string, typeName string) *types.Func {
	package_ := types.NewPackage(packagePath, "resource")
	named := types.NewNamed(
		types.NewTypeName(token.NoPos, package_, typeName, nil),
		types.NewStruct(nil, nil),
		nil,
	)
	receiver := types.NewVar(token.NoPos, package_, "receiver", types.NewPointer(named))
	results := types.NewTuple(
		types.NewVar(token.NoPos, package_, "", types.Universe.Lookup("error").Type()),
	)
	signature := types.NewSignatureType(receiver, nil, nil, nil, results, false)
	return types.NewFunc(token.NoPos, package_, "Close", signature)
}
