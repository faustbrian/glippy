package analysis

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/faustbrian/glippy/internal/rules"
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
		1: {Known: true, Always: true, Kinds: rules.ParameterEffectTransfer},
		0: {Known: true},
	}
	second.parameters["function"] = map[int]rules.ParameterEffectSummary{
		0: {Known: true},
		1: {Known: true, Always: true, Kinds: rules.ParameterEffectTransfer},
	}
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
	}
	if first.digest() == changed.digest() {
		t.Fatal("effect fact digest ignored a parameter effect change")
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

func effectTestFunction(packagePath string, name string) *types.Func {
	package_ := types.NewPackage(packagePath, "terminate")
	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	return types.NewFunc(token.NoPos, package_, name, signature)
}
