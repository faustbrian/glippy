package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
)

const nativeEffectFactSchemaVersion = 14

type returnStateKey struct {
	value int
	error int
}

type returnAliasKey struct {
	result int
	argument int
}

// nativeEffectFacts contains conservative, versioned semantic summaries whose
// stable identities survive independent package loads.
type nativeEffectFacts struct {
	noReturns map[string]struct{}
	testingSkips map[string]struct{}
	parameters map[string]map[int]rules.ParameterEffectSummary
	receivers map[string]rules.ParameterEffectSummary
	noOpCloses map[string]struct{}
	returns map[string]map[returnStateKey]rules.ReturnStateSummary
	results map[string]map[int]rules.NilState
	mustUse map[string]map[int]struct{}
	blocking map[string]struct{}
	aliases map[string]map[returnAliasKey]struct{}
	cleanupManaged map[string]map[int]struct{}
}

func newNativeEffectFacts() *nativeEffectFacts {
	return &nativeEffectFacts{
		noReturns: make(map[string]struct{}),
		testingSkips: make(map[string]struct{}),
		parameters: make(map[string]map[int]rules.ParameterEffectSummary),
		receivers: make(map[string]rules.ParameterEffectSummary),
		noOpCloses: make(map[string]struct{}),
		returns: make(map[string]map[returnStateKey]rules.ReturnStateSummary),
		results: make(map[string]map[int]rules.NilState),
		mustUse: make(map[string]map[int]struct{}),
		blocking: make(map[string]struct{}),
		aliases: make(map[string]map[returnAliasKey]struct{}),
		cleanupManaged: make(map[string]map[int]struct{}),
	}
}

func cloneNativeEffectFacts(facts *nativeEffectFacts) *nativeEffectFacts {
	result := newNativeEffectFacts()
	if facts == nil {
		return result
	}
	for identity := range facts.noReturns {
		result.noReturns[identity] = struct{}{}
	}
	for identity := range facts.testingSkips {
		result.testingSkips[identity] = struct{}{}
	}
	for identity, parameters := range facts.parameters {
		cloned := make(map[int]rules.ParameterEffectSummary, len(parameters))
		for index, summary := range parameters {
			cloned[index] = summary
		}
		result.parameters[identity] = cloned
	}
	for identity, summary := range facts.receivers {
		result.receivers[identity] = summary
	}
	for identity := range facts.noOpCloses {
		result.noOpCloses[identity] = struct{}{}
	}
	for identity, summaries := range facts.returns {
		cloned := make(map[returnStateKey]rules.ReturnStateSummary, len(summaries))
		for key, summary := range summaries {
			cloned[key] = summary
		}
		result.returns[identity] = cloned
	}
	for identity, states := range facts.results {
		cloned := make(map[int]rules.NilState, len(states))
		for index, state := range states {
			cloned[index] = state
		}
		result.results[identity] = cloned
	}
	for identity, indexes := range facts.mustUse {
		cloned := make(map[int]struct{}, len(indexes))
		for index := range indexes {
			cloned[index] = struct{}{}
		}
		result.mustUse[identity] = cloned
	}
	for identity := range facts.blocking {
		result.blocking[identity] = struct{}{}
	}
	for identity, aliases := range facts.aliases {
		cloned := make(map[returnAliasKey]struct{}, len(aliases))
		for key := range aliases {
			cloned[key] = struct{}{}
		}
		result.aliases[identity] = cloned
	}
	for identity, indexes := range facts.cleanupManaged {
		cloned := make(map[int]struct{}, len(indexes))
		for index := range indexes {
			cloned[index] = struct{}{}
		}
		result.cleanupManaged[identity] = cloned
	}
	return result
}

// ReturnState implements rules.EffectFacts across independent package loads.
func (f *nativeEffectFacts) ReturnState(
	function *types.Func,
	valueResult int,
	errorResult int,
) rules.ReturnStateSummary {
	if f == nil || valueResult < 0 || errorResult < 0 {
		return rules.ReturnStateSummary{}
	}
	return f.returns[stableFunctionIdentity(
		function,
	)][returnStateKey{value: valueResult, error: errorResult}]
}

// ResultState implements rules.EffectFacts across independent package loads.
func (f *nativeEffectFacts) ResultState(function *types.Func, result int) rules.NilState {
	if f == nil || function == nil || result < 0 {
		return rules.NilStateUnknown
	}
	return f.results[stableFunctionIdentity(function)][result]
}

func (f *nativeEffectFacts) noReturn(function *types.Func) bool {
	if f == nil {
		return false
	}
	_, found := f.noReturns[stableFunctionIdentity(function)]
	return found
}

func (f *nativeEffectFacts) testingSkip(function *types.Func) bool {
	if f == nil {
		return false
	}
	_, found := f.testingSkips[stableFunctionIdentity(function)]
	return found
}

// ParameterEffect implements rules.EffectFacts using a stable identity that
// survives the independent dependency and root package loads.
func (f *nativeEffectFacts) ParameterEffect(
	function *types.Func,
	index int,
) rules.ParameterEffectSummary {
	if f == nil || index < 0 {
		return rules.ParameterEffectSummary{}
	}
	parameters := f.parameters[stableFunctionIdentity(function)]
	return parameters[index]
}

// ReceiverEffect implements rules.EffectFacts using the same stable method
// identity across independent package loads.
func (f *nativeEffectFacts) ReceiverEffect(function *types.Func) rules.ParameterEffectSummary {
	if f == nil || function == nil {
		return rules.ParameterEffectSummary{}
	}
	return f.receivers[stableFunctionIdentity(function)]
}

// NoOpClose implements rules.EffectFacts across independent package loads.
func (f *nativeEffectFacts) NoOpClose(function *types.Func) bool {
	if f == nil || function == nil {
		return false
	}
	_, found := f.noOpCloses[stableFunctionIdentity(function)]
	return found
}

// MustUseResult implements rules.EffectFacts.
func (f *nativeEffectFacts) MustUseResult(function *types.Func, index int) bool {
	if f == nil || function == nil || index < 0 {
		return false
	}
	_, found := f.mustUse[stableFunctionIdentity(function)][index]
	return found
}

// Blocking implements rules.EffectFacts.
func (f *nativeEffectFacts) Blocking(function *types.Func) bool {
	if f == nil || function == nil {
		return false
	}
	_, found := f.blocking[stableFunctionIdentity(function)]
	return found
}

// ReturnAliasesArgument implements rules.EffectFacts.
func (f *nativeEffectFacts) ReturnAliasesArgument(
	function *types.Func,
	result int,
	argument int,
) bool {
	if f == nil || function == nil || result < 0 || argument < 0 {
		return false
	}
	_, found := f.aliases[stableFunctionIdentity(
		function,
	)][returnAliasKey{result: result, argument: argument}]
	return found
}

// CleanupManagedResult implements rules.EffectFacts.
func (f *nativeEffectFacts) CleanupManagedResult(function *types.Func, result int) bool {
	if f == nil || function == nil || result < 0 {
		return false
	}
	_, found := f.cleanupManaged[stableFunctionIdentity(function)][result]
	return found
}

func (f *nativeEffectFacts) addContracts(resolved contracts.Resolved) {
	if f == nil {
		return
	}
	for _, binding := range resolved.Bindings() {
		identity := stableFunctionIdentity(binding.Function)
		if identity == "" {
			continue
		}
		contract := binding.Contract
		if contract.NoReturn {
			f.noReturns[identity] = struct{}{}
		}
		if contract.Blocking {
			f.blocking[identity] = struct{}{}
		}
		if len(contract.MustUse) != 0 {
			indexes := make(map[int]struct{}, len(contract.MustUse))
			for _, index := range contract.MustUse {
				indexes[index] = struct{}{}
			}
			f.mustUse[identity] = indexes
		}
		parameters := make(map[int]rules.ParameterEffectSummary)
		addParameterKinds := func(indices []int, kind rules.ParameterEffectKind) {
			for _, index := range indices {
				summary := parameters[index]
				summary.Known = true
				summary.Always = true
				terminal := rules.ParameterEffectClose |
					rules.ParameterEffectTransactionComplete |
					rules.ParameterEffectCancelInvoke
				if kind == rules.ParameterEffectTransfer &&
					summary.GuaranteedKinds & terminal != 0 {
					parameters[index] = summary
					continue
				}
				if kind & terminal != 0 {
					summary.Kinds &^= rules.ParameterEffectTransfer
					summary.GuaranteedKinds &^= rules.ParameterEffectTransfer
				}
				summary.Kinds |= kind
				summary.GuaranteedKinds |= kind
				parameters[index] = summary
			}
		}
		addParameterKinds(contract.Closes, rules.ParameterEffectClose)
		addParameterKinds(contract.TakesOwnership, rules.ParameterEffectTransfer)
		addParameterKinds(
			contract.CompletesTransaction,
			rules.ParameterEffectTransactionComplete,
		)
		addParameterKinds(contract.InvokesCancellation, rules.ParameterEffectCancelInvoke)
		if len(parameters) != 0 {
			f.parameters[identity] = parameters
		}
		if len(contract.NilError) != 0 {
			states := make(
				map[returnStateKey]rules.ReturnStateSummary,
				len(contract.NilError),
			)
			for _, relation := range contract.NilError {
				states[returnStateKey{
					value: relation.Value,
					error: relation.Error,
				}] = rules.ReturnStateSummary{
						WhenErrorNil: contractNilState(
							relation.WhenErrorNil,
						),
						WhenErrorNonNil: contractNilState(
							relation.WhenErrorNonNil,
						),
					}
			}
			f.returns[identity] = states
		}
		if len(contract.ReturnsAlias) != 0 {
			aliases := make(map[returnAliasKey]struct{}, len(contract.ReturnsAlias))
			for _, relation := range contract.ReturnsAlias {
				aliases[returnAliasKey{
					result: relation.Result,
					argument: relation.Argument,
				}] = struct{}{}
			}
			f.aliases[identity] = aliases
		}
	}
}

func contractNilState(state contracts.NilState) rules.NilState {
	switch state {
	case contracts.NilStateNil:
		return rules.NilStateNil
	case contracts.NilStateNonNil:
		return rules.NilStateNonNil
	default:
		return rules.NilStateUnknown
	}
}

func (f *nativeEffectFacts) addNoReturns(analysis *noReturnAnalysis) {
	if f == nil || analysis == nil {
		return
	}
	testingSkips := make(map[string]bool)
	for function, definition := range analysis.definitions {
		identity := stableFunctionIdentity(function)
		if identity == "" {
			continue
		}
		if definition != nil && definition.noReturn {
			f.noReturns[identity] = struct{}{}
			analysis.buildTestingSkip(definition)
		}
		candidate := definition != nil && definition.noReturn && definition.testingSkip
		if existing, found := testingSkips[identity]; found {
			testingSkips[identity] = existing && candidate
		} else {
			testingSkips[identity] = candidate
		}
	}
	for identity, testingSkip := range testingSkips {
		if testingSkip {
			f.testingSkips[identity] = struct{}{}
		} else {
			delete(f.testingSkips, identity)
		}
	}
}

func (f *nativeEffectFacts) addParameterEffects(analysis *parameterEffectAnalysis) {
	if f == nil || analysis == nil {
		return
	}
	for function, definition := range analysis.definitions {
		identity := stableFunctionIdentity(function)
		if identity == "" || definition == nil {
			continue
		}
		parameters := make(map[int]rules.ParameterEffectSummary)
		for index, summary := range definition.summaries {
			if summary.Known {
				parameters[index] = summary
			}
		}
		if len(parameters) != 0 {
			existing := f.parameters[identity]
			if existing == nil {
				existing = make(map[int]rules.ParameterEffectSummary)
				f.parameters[identity] = existing
			}
			for index, summary := range parameters {
				if _, configured := existing[index]; !configured {
					existing[index] = summary
				}
			}
		}
	}
	type receiverAggregate struct {
		summary rules.ParameterEffectSummary
		valid bool
	}
	receivers := make(map[string]receiverAggregate)
	for function, definition := range analysis.definitions {
		if definition == nil ||
			definition.signature == nil ||
			definition.signature.Recv() == nil {
			continue
		}
		identity := stableFunctionIdentity(function)
		if identity == "" {
			continue
		}
		summary := definition.receiver
		candidate := receiverAggregate{
			summary: summary,
			valid: definition.receiverBuilt && summary.Known,
		}
		if existing, found := receivers[identity]; found {
			candidate.valid = existing.valid &&
				candidate.valid &&
				existing.summary == candidate.summary
			candidate.summary = existing.summary
		}
		receivers[identity] = candidate
	}
	for identity, candidate := range receivers {
		if !candidate.valid {
			delete(f.receivers, identity)
			continue
		}
		if _, configured := f.receivers[identity]; !configured {
			f.receivers[identity] = candidate.summary
		}
	}
	noOpCloses := make(map[string]bool)
	seenNoOpClose := make(map[string]struct{})
	for function, definition := range analysis.definitions {
		if definition == nil || !definition.closeMethod {
			continue
		}
		identity := stableFunctionIdentity(function)
		if identity == "" {
			continue
		}
		if _, seen := seenNoOpClose[identity]; !seen {
			seenNoOpClose[identity] = struct{}{}
			noOpCloses[identity] = definition.noOpClose
			continue
		}
		noOpCloses[identity] = noOpCloses[identity] && definition.noOpClose
	}
	for identity, noOp := range noOpCloses {
		if !noOp {
			delete(f.noOpCloses, identity)
			continue
		}
		f.noOpCloses[identity] = struct{}{}
	}
}

func (f *nativeEffectFacts) addReturnStates(analysis *returnStateAnalysis) {
	if f == nil || analysis == nil {
		return
	}
	for function, summaries := range analysis.summaries {
		identity := stableFunctionIdentity(function)
		if identity == "" || len(summaries) == 0 {
			continue
		}
		cloned := make(map[returnStateKey]rules.ReturnStateSummary, len(summaries))
		for key, summary := range summaries {
			cloned[key] = summary
		}
		existing := f.returns[identity]
		if existing == nil {
			existing = make(map[returnStateKey]rules.ReturnStateSummary)
			f.returns[identity] = existing
		}
		for key, summary := range cloned {
			if _, configured := existing[key]; !configured {
				existing[key] = summary
			}
		}
	}
}

func (f *nativeEffectFacts) addResultStates(analysis *returnStateAnalysis) {
	if f == nil || analysis == nil {
		return
	}
	aggregated := make(map[string]map[int]rules.NilState)
	seen := make(map[string]struct{})
	for _, definition := range analysis.definitions {
		identity := stableFunctionIdentity(definition.function)
		if identity == "" {
			continue
		}
		states := analysis.resultStates[definition.function]
		if _, found := seen[identity]; !found {
			seen[identity] = struct{}{}
			cloned := make(map[int]rules.NilState, len(states))
			for index, state := range states {
				cloned[index] = state
			}
			aggregated[identity] = cloned
			continue
		}
		for index, state := range aggregated[identity] {
			if candidate, found := states[index]; !found || candidate != state {
				delete(aggregated[identity], index)
			}
		}
	}
	for identity, states := range aggregated {
		if len(states) == 0 {
			delete(f.results, identity)
			continue
		}
		existing := f.results[identity]
		if existing == nil {
			f.results[identity] = states
			continue
		}
		for index, state := range existing {
			if candidate, found := states[index]; !found || candidate != state {
				delete(existing, index)
			}
		}
		if len(existing) == 0 {
			delete(f.results, identity)
		}
	}
}

func (f *nativeEffectFacts) addCleanupManagedResults(analysis *managedResultAnalysis) {
	if f == nil || analysis == nil {
		return
	}
	aggregated := make(map[string]map[int]struct{})
	seen := make(map[string]struct{})
	for _, definition := range analysis.definitions {
		identity := stableFunctionIdentity(definition.function)
		if identity == "" {
			continue
		}
		indexes := analysis.summaries[definition.function]
		if _, found := seen[identity]; !found {
			seen[identity] = struct{}{}
			cloned := make(map[int]struct{}, len(indexes))
			for index := range indexes {
				cloned[index] = struct{}{}
			}
			aggregated[identity] = cloned
			continue
		}
		for index := range aggregated[identity] {
			if _, managed := indexes[index]; !managed {
				delete(aggregated[identity], index)
			}
		}
	}
	for identity, indexes := range aggregated {
		if len(indexes) == 0 {
			delete(f.cleanupManaged, identity)
			continue
		}
		f.cleanupManaged[identity] = indexes
	}
}

func (f *nativeEffectFacts) digest() cache.Digest {
	digest := sha256.New()
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], nativeEffectFactSchemaVersion)
	_, _ = digest.Write(version[:])
	identities := make([]string, 0)
	if f != nil {
		identities = make([]string, 0, len(f.noReturns))
		for identity := range f.noReturns {
			identities = append(identities, identity)
		}
	}
	sort.Strings(identities)
	for _, identity := range identities {
		_, _ = digest.Write([]byte{0})
		binary.BigEndian.PutUint64(version[:], uint64(len(identity)))
		_, _ = digest.Write(version[:])
		_, _ = digest.Write([]byte(identity))
	}
	testingSkips := make([]string, 0)
	if f != nil {
		testingSkips = make([]string, 0, len(f.testingSkips))
		for identity := range f.testingSkips {
			testingSkips = append(testingSkips, identity)
		}
	}
	sort.Strings(testingSkips)
	for _, identity := range testingSkips {
		_, _ = digest.Write([]byte{13})
		writeEffectIdentity(digest, version[:], identity)
	}
	type parameterRecord struct {
		identity string
		index int
		summary rules.ParameterEffectSummary
	}
	parameters := make([]parameterRecord, 0)
	if f != nil {
		for identity, summaries := range f.parameters {
			for index, summary := range summaries {
				parameters = append(
					parameters,
					parameterRecord{
						identity: identity,
						index: index,
						summary: summary,
					},
				)
			}
		}
	}
	sort.Slice(
		parameters,
		func(first, second int) bool {
			if parameters[first].identity != parameters[second].identity {
				return parameters[first].identity < parameters[second].identity
			}
			return parameters[first].index < parameters[second].index
		},
	)
	for _, parameter := range parameters {
		_, _ = digest.Write([]byte{1})
		binary.BigEndian.PutUint64(version[:], uint64(len(parameter.identity)))
		_, _ = digest.Write(version[:])
		_, _ = digest.Write([]byte(parameter.identity))
		binary.BigEndian.PutUint64(version[:], uint64(parameter.index))
		_, _ = digest.Write(version[:])
		if parameter.summary.Known {
			_, _ = digest.Write([]byte{1})
		} else {
			_, _ = digest.Write([]byte{0})
		}
		if parameter.summary.Always {
			_, _ = digest.Write([]byte{1})
		} else {
			_, _ = digest.Write([]byte{0})
		}
		_, _ = digest.Write([]byte{byte(parameter.summary.Kinds)})
		_, _ = digest.Write([]byte{byte(parameter.summary.GuaranteedKinds)})
	}
	type receiverRecord struct {
		identity string
		summary rules.ParameterEffectSummary
	}
	receivers := make([]receiverRecord, 0)
	if f != nil {
		for identity, summary := range f.receivers {
			receivers = append(
				receivers,
				receiverRecord{identity: identity, summary: summary},
			)
		}
	}
	sort.Slice(
		receivers,
		func(left, right int) bool {
			return receivers[left].identity < receivers[right].identity
		},
	)
	for _, receiver := range receivers {
		_, _ = digest.Write([]byte{7})
		writeEffectIdentity(digest, version[:], receiver.identity)
		if receiver.summary.Known {
			_, _ = digest.Write([]byte{1})
		} else {
			_, _ = digest.Write([]byte{0})
		}
		if receiver.summary.Always {
			_, _ = digest.Write([]byte{1})
		} else {
			_, _ = digest.Write([]byte{0})
		}
		_, _ = digest.Write([]byte{byte(receiver.summary.Kinds)})
		_, _ = digest.Write([]byte{byte(receiver.summary.GuaranteedKinds)})
	}
	noOpCloses := make([]string, 0)
	if f != nil {
		noOpCloses = make([]string, 0, len(f.noOpCloses))
		for identity := range f.noOpCloses {
			noOpCloses = append(noOpCloses, identity)
		}
	}
	sort.Strings(noOpCloses)
	for _, identity := range noOpCloses {
		_, _ = digest.Write([]byte{12})
		writeEffectIdentity(digest, version[:], identity)
	}
	type returnRecord struct {
		identity string
		key returnStateKey
		summary rules.ReturnStateSummary
	}
	returns := make([]returnRecord, 0)
	if f != nil {
		for identity, summaries := range f.returns {
			for key, summary := range summaries {
				returns = append(
					returns,
					returnRecord{
						identity: identity,
						key: key,
						summary: summary,
					},
				)
			}
		}
	}
	sort.Slice(
		returns,
		func(first, second int) bool {
			if returns[first].identity != returns[second].identity {
				return returns[first].identity < returns[second].identity
			}
			if returns[first].key.value != returns[second].key.value {
				return returns[first].key.value < returns[second].key.value
			}
			return returns[first].key.error < returns[second].key.error
		},
	)
	for _, returned := range returns {
		_, _ = digest.Write([]byte{2})
		binary.BigEndian.PutUint64(version[:], uint64(len(returned.identity)))
		_, _ = digest.Write(version[:])
		_, _ = digest.Write([]byte(returned.identity))
		binary.BigEndian.PutUint64(version[:], uint64(returned.key.value))
		_, _ = digest.Write(version[:])
		binary.BigEndian.PutUint64(version[:], uint64(returned.key.error))
		_, _ = digest.Write(version[:])
		_, _ = digest.Write([]byte{byte(returned.summary.WhenErrorNil)})
		_, _ = digest.Write([]byte{byte(returned.summary.WhenErrorNonNil)})
	}
	type resultRecord struct {
		identity string
		index int
		state rules.NilState
	}
	results := make([]resultRecord, 0)
	if f != nil {
		for identity, states := range f.results {
			for index, state := range states {
				results = append(
					results,
					resultRecord{
						identity: identity,
						index: index,
						state: state,
					},
				)
			}
		}
	}
	sort.Slice(
		results,
		func(left, right int) bool {
			if results[left].identity != results[right].identity {
				return results[left].identity < results[right].identity
			}
			return results[left].index < results[right].index
		},
	)
	for _, result := range results {
		_, _ = digest.Write([]byte{8})
		writeEffectIdentity(digest, version[:], result.identity)
		binary.BigEndian.PutUint64(version[:], uint64(result.index))
		_, _ = digest.Write(version[:])
		_, _ = digest.Write([]byte{byte(result.state)})
	}
	type indexedRecord struct {
		identity string
		index int
	}
	mustUse := make([]indexedRecord, 0)
	if f != nil {
		for identity, indexes := range f.mustUse {
			for index := range indexes {
				mustUse = append(
					mustUse,
					indexedRecord{identity: identity, index: index},
				)
			}
		}
	}
	sort.Slice(
		mustUse,
		func(left, right int) bool {
			if mustUse[left].identity != mustUse[right].identity {
				return mustUse[left].identity < mustUse[right].identity
			}
			return mustUse[left].index < mustUse[right].index
		},
	)
	for _, record := range mustUse {
		_, _ = digest.Write([]byte{3})
		writeEffectIdentity(digest, version[:], record.identity)
		binary.BigEndian.PutUint64(version[:], uint64(record.index))
		_, _ = digest.Write(version[:])
	}
	blocking := make([]string, 0)
	if f != nil {
		for identity := range f.blocking {
			blocking = append(blocking, identity)
		}
	}
	sort.Strings(blocking)
	for _, identity := range blocking {
		_, _ = digest.Write([]byte{4})
		writeEffectIdentity(digest, version[:], identity)
	}
	type aliasRecord struct {
		identity string
		key returnAliasKey
	}
	aliases := make([]aliasRecord, 0)
	if f != nil {
		for identity, values := range f.aliases {
			for key := range values {
				aliases = append(aliases, aliasRecord{identity: identity, key: key})
			}
		}
	}
	sort.Slice(
		aliases,
		func(left, right int) bool {
			if aliases[left].identity != aliases[right].identity {
				return aliases[left].identity < aliases[right].identity
			}
			if aliases[left].key.result != aliases[right].key.result {
				return aliases[left].key.result < aliases[right].key.result
			}
			return aliases[left].key.argument < aliases[right].key.argument
		},
	)
	for _, record := range aliases {
		_, _ = digest.Write([]byte{5})
		writeEffectIdentity(digest, version[:], record.identity)
		binary.BigEndian.PutUint64(version[:], uint64(record.key.result))
		_, _ = digest.Write(version[:])
		binary.BigEndian.PutUint64(version[:], uint64(record.key.argument))
		_, _ = digest.Write(version[:])
	}
	cleanupManaged := make([]indexedRecord, 0)
	if f != nil {
		for identity, indexes := range f.cleanupManaged {
			for index := range indexes {
				cleanupManaged = append(
					cleanupManaged,
					indexedRecord{identity: identity, index: index},
				)
			}
		}
	}
	sort.Slice(
		cleanupManaged,
		func(left, right int) bool {
			if cleanupManaged[left].identity != cleanupManaged[right].identity {
				return cleanupManaged[left].identity <
					cleanupManaged[right].identity
			}
			return cleanupManaged[left].index < cleanupManaged[right].index
		},
	)
	for _, record := range cleanupManaged {
		_, _ = digest.Write([]byte{6})
		writeEffectIdentity(digest, version[:], record.identity)
		binary.BigEndian.PutUint64(version[:], uint64(record.index))
		_, _ = digest.Write(version[:])
	}
	var result cache.Digest
	copy(result[:], digest.Sum(nil))
	return result
}

func writeEffectIdentity(
	digest interface {
		Write([]byte) (int, error)
	},
	scratch []byte,
	identity string,
) {
	binary.BigEndian.PutUint64(scratch, uint64(len(identity)))
	_, _ = digest.Write(scratch)
	_, _ = digest.Write([]byte(identity))
}

func stableFunctionIdentity(function *types.Func) string {
	if function == nil || function.Pkg() == nil {
		return ""
	}
	return types.ObjectString(
		function,
		func(package_ *types.Package) string {
			if package_ == nil {
				return ""
			}
			return package_.Path()
		},
	)
}

func loadNativeEffectFacts(
	ctx context.Context,
	options PackageLoadOptions,
	roots []*packages.Package,
	rootSources PackageSourceSet,
) (*nativeEffectFacts, error) {
	facts := newNativeEffectFacts()
	resolved, err := contracts.Resolve(options.Contracts, effectTypePackages(roots))
	if err != nil {
		return nil, fmt.Errorf("resolve project semantic contracts: %w", err)
	}
	facts.addContracts(resolved)
	prefixes := effectModulePrefixes(roots)
	if len(prefixes) == 0 {
		return facts, nil
	}
	seen := make(map[string]struct{})
	for _, root := range roots {
		if root != nil && root.PkgPath != "" {
			seen[root.PkgPath] = struct{}{}
		}
	}
	current := localEffectImports(roots, prefixes, seen)
	layers := make([][]*packages.Package, 0)
	maximumPackages := options.MaxPackages
	if maximumPackages == 0 {
		maximumPackages = DefaultMaxPackages
	}
	maximumSourceFiles := options.MaxSourceFiles
	if maximumSourceFiles == 0 {
		maximumSourceFiles = DefaultMaxSourceFiles
	}
	maximumSourceBytes := options.MaxSourceBytes
	if maximumSourceBytes == 0 {
		maximumSourceBytes = DefaultMaxSourceBytes
	}
	loadedCount := len(roots)
	loadedSources := make(map[string]struct{})
	var loadedBytes int64
	for _, path := range rootSources.Paths() {
		file, found := rootSources.Lookup(path)
		if !found {
			return nil, fmt.Errorf("native effect root source %q is missing", path)
		}
		loadedSources[path] = struct{}{}
		loadedBytes += int64(len(file.Bytes()))
	}
	for len(current) != 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		layerOptions := clonePackageLoadOptions(options)
		layerOptions.Patterns = current
		layerOptions.Tests = false
		layerOptions.LoadDependencySyntax = false
		layerOptions.LoadEffectFacts = false
		loaded, err := LoadPackages(ctx, layerOptions)
		if err != nil {
			return nil, fmt.Errorf("load native effect inputs: %w", err)
		}
		loadedCount += len(loaded.Packages)
		if loadedCount > maximumPackages {
			return nil, fmt.Errorf(
				"native effect graph exceeds %d-package limit",
				maximumPackages,
			)
		}
		for _, path := range loaded.Sources.Paths() {
			if _, found := loadedSources[path]; found {
				continue
			}
			file, found := loaded.Sources.Lookup(path)
			if !found {
				return nil, fmt.Errorf("native effect source %q is missing", path)
			}
			loadedSources[path] = struct{}{}
			loadedBytes += int64(len(file.Bytes()))
			if len(loadedSources) > maximumSourceFiles {
				return nil, fmt.Errorf(
					"native effect source set exceeds %d-file limit",
					maximumSourceFiles,
				)
			}
			if loadedBytes > maximumSourceBytes {
				return nil, fmt.Errorf(
					"native effect source set exceeds %d-byte limit",
					maximumSourceBytes,
				)
			}
		}
		layers = append(layers, loaded.Packages)
		current = localEffectImports(loaded.Packages, prefixes, seen)
	}
	for index := len(layers) - 1; index >= 0; index-- {
		analysis := newNoReturnAnalysis(ctx, layers[index], facts)
		analysis.buildAll()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		facts.addNoReturns(analysis)
		parameterEffects := newParameterEffectAnalysis(ctx, layers[index], facts, analysis)
		parameterEffects.buildAll()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		facts.addParameterEffects(parameterEffects)
		managedResults := newManagedResultAnalysis(ctx, layers[index], facts, analysis)
		managedResults.buildAll()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		facts.addCleanupManagedResults(managedResults)
		returnStates := newReturnStateAnalysis(ctx, layers[index], facts, analysis)
		returnStates.buildAll()
		facts.addReturnStates(returnStates)
		facts.addResultStates(returnStates)
	}
	return facts, nil
}

func effectTypePackages(roots []*packages.Package) []*types.Package {
	seen := make(map[*types.Package]struct{})
	result := make([]*types.Package, 0)
	var visit func(*types.Package)
	visit = func(package_ *types.Package) {
		if package_ == nil || package_.Path() == "" {
			return
		}
		if _, found := seen[package_]; found {
			return
		}
		seen[package_] = struct{}{}
		result = append(result, package_)
		for _, imported := range package_.Imports() {
			visit(imported)
		}
	}
	for _, root := range roots {
		if root != nil {
			visit(root.Types)
		}
	}
	sort.SliceStable(
		result,
		func(left, right int) bool {
			return result[left].Path() < result[right].Path()
		},
	)
	return result
}

func effectModulePrefixes(roots []*packages.Package) []string {
	prefixes := make([]string, 0)
	seenModules := make(map[string]struct{})
	seenPackages := make(map[*packages.Package]struct{})
	work := append([]*packages.Package(nil), roots...)
	for len(work) != 0 {
		pkg := work[len(work) - 1]
		work = work[:len(work) - 1]
		if pkg == nil {
			continue
		}
		if _, found := seenPackages[pkg]; found {
			continue
		}
		seenPackages[pkg] = struct{}{}
		module := pkg.Module
		if effectModuleProvidesLocalSource(module) {
			if _, found := seenModules[module.Path]; !found {
				seenModules[module.Path] = struct{}{}
				prefixes = append(prefixes, module.Path)
			}
		}
		paths := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			paths = append(paths, path)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(paths)))
		for _, path := range paths {
			work = append(work, pkg.Imports[path])
		}
	}
	sort.Strings(prefixes)
	return prefixes
}

func effectModuleProvidesLocalSource(module *packages.Module) bool {
	if module == nil || module.Path == "" {
		return false
	}
	if module.Main {
		return true
	}
	return module.Replace != nil && module.Replace.Dir != "" && module.Replace.Version == ""
}

func localEffectImports(
	packages_ []*packages.Package,
	prefixes []string,
	seen map[string]struct{},
) []string {
	imports := make(map[string]struct{})
	for _, pkg := range packages_ {
		if pkg == nil || pkg.Types == nil {
			continue
		}
		for _, imported := range pkg.Types.Imports() {
			path := imported.Path()
			if !effectPathWithinModules(path, prefixes) {
				continue
			}
			if _, found := seen[path]; found {
				continue
			}
			seen[path] = struct{}{}
			imports[path] = struct{}{}
		}
	}
	result := make([]string, 0, len(imports))
	for path := range imports {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func effectPathWithinModules(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix + "/") {
			return true
		}
	}
	return false
}
