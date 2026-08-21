package analysis

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"

	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/rules"
)

const nativePackageCacheEntryVersion = 3

type nativePackageCacheEntry struct {
	Version int `json:"version"`
	Requirement rules.Requirement `json:"requirement"`
	Rules []nativeRuleSnapshot `json:"rules"`
	Diagnostics []nativePersistedDiagnostic `json:"diagnostics"`
}

type nativeRuleSnapshot struct {
	ID string `json:"id"`
	Severity rules.Severity `json:"severity"`
	Requirement rules.Requirement `json:"requirement"`
	Execution string `json:"execution"`
	NodeInterests []rules.NodeKind `json:"nodeInterests"`
	RequiresDependencySyntax bool `json:"requiresDependencySyntax"`
	RequiresEffectFacts bool `json:"requiresEffectFacts"`
	RunOnGenerated bool `json:"runOnGenerated"`
	RunDespiteTypeErrors bool `json:"runDespiteTypeErrors"`
	MinimumGoVersion string `json:"minimumGoVersion"`
	Fixes []nativeFixSnapshot `json:"fixes"`
	OptionsDigest string `json:"optionsDigest"`
}

type nativeFixSnapshot struct {
	Name string `json:"name"`
	Safety rules.FixSafety `json:"safety"`
}

type nativePersistedDiagnostic struct {
	PackageID string `json:"packageId"`
	Diagnostic persistedDiagnostic `json:"diagnostic"`
}

func runNativePackageAnalysis(
	ctx context.Context,
	loaded PackageLoadResult,
	loadOptions PackageLoadOptions,
	cachePlan *packageCachePlan,
	registry *rules.Registry,
	typesSelection []rules.Selection,
	controlFlowSelection []rules.Selection,
	ssaSelection []rules.Selection,
) ([]rules.Diagnostic, error) {
	selection := make(
		[]rules.Selection,
		0,
		len(typesSelection) + len(controlFlowSelection) + len(ssaSelection),
	)
	selection = append(selection, typesSelection...)
	selection = append(selection, controlFlowSelection...)
	selection = append(selection, ssaSelection...)
	if len(selection) == 0 {
		return []rules.Diagnostic{}, nil
	}
	snapshots, err := nativeRuleSnapshots(
		registry,
		typesSelection,
		controlFlowSelection,
		ssaSelection,
	)
	if err != nil {
		return nil, err
	}

	key, cacheable := nativePackageCacheKey(loaded, loadOptions, cachePlan)
	statistics := statisticsFromContext(ctx)
	invalidHit := false
	if cacheable {
		encoded, found, err := cachePlan.options.Store.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("read native analysis cache: %w", err)
		}
		if found {
			diagnostics, restoreErr := restoreNativePackageCacheEntry(
				registry,
				selection,
				snapshots,
				loaded,
				encoded,
			)
			if restoreErr == nil {
				statistics.recordCacheLookup(true, false)
				return diagnostics, nil
			}
			invalidHit = true
			statistics.recordCacheLookup(false, true)
		} else {
			statistics.recordCacheLookup(false, false)
		}
	}

	typedDiagnostics, err := RunTypes(ctx, loaded, registry, typesSelection)
	if err != nil {
		return nil, err
	}
	if err := captureProfilePhase(ctx, ProfilePhaseTypes); err != nil {
		return nil, err
	}
	controlFlowDiagnostics, err := RunControlFlow(ctx, loaded, registry, controlFlowSelection)
	if err != nil {
		return nil, err
	}
	if err := captureProfilePhase(ctx, ProfilePhaseControlFlow); err != nil {
		return nil, err
	}
	ssaDiagnostics, err := RunSSA(ctx, loaded, registry, ssaSelection)
	if err != nil {
		return nil, err
	}
	if err := captureProfilePhase(ctx, ProfilePhaseSSA); err != nil {
		return nil, err
	}
	diagnostics := make(
		[]rules.Diagnostic,
		0,
		len(typedDiagnostics) + len(controlFlowDiagnostics) + len(ssaDiagnostics),
	)
	diagnostics = append(diagnostics, typedDiagnostics...)
	diagnostics = append(diagnostics, controlFlowDiagnostics...)
	diagnostics = append(diagnostics, ssaDiagnostics...)
	diagnostics = OrderDiagnostics(diagnostics)

	if cacheable {
		encoded, err := encodeNativePackageCacheEntry(
			selection,
			snapshots,
			loaded,
			diagnostics,
		)
		if err != nil {
			return nil, err
		}
		if err := cachePlan.options.Store.Put(ctx, key, encoded); err != nil {
			if !invalidHit || !errors.Is(err, cache.ErrConflict) {
				return nil, fmt.Errorf("write native analysis cache: %w", err)
			}
		} else {
			statistics.recordCacheWrite()
		}
	}
	return diagnostics, nil
}

func nativePackageCacheKey(
	loaded PackageLoadResult,
	loadOptions PackageLoadOptions,
	plan *packageCachePlan,
) (cache.Key, bool) {
	if plan == nil {
		return cache.Key{}, false
	}
	key, err := buildPackageCacheKey(
		packageCacheKeyInput{
			Namespace: "native-analysis-v1",
			ToolVersion: plan.options.ToolVersion,
			BuildGoVersion: plan.options.BuildGoVersion,
			SourceGoVersion: plan.options.SourceGoVersion,
			Configuration: plan.options.Configuration,
			Rules: plan.rules,
			CGOEnabled: plan.options.CGOEnabled,
			FormatterMode: plan.options.FormatterMode,
			LoadOptions: loadOptions,
			Loaded: loaded,
			Facts: nativeEffectCacheDigests(loaded.effectFacts),
		},
	)
	if err != nil {
		return cache.Key{}, false
	}
	return key, true
}

func nativeRuleSnapshots(
	registry *rules.Registry,
	typesSelection []rules.Selection,
	controlFlowSelection []rules.Selection,
	ssaSelection []rules.Selection,
) ([]nativeRuleSnapshot, error) {
	if _, _, err := prepareTypesRules(registry, typesSelection); err != nil {
		return nil, err
	}
	if _, err := prepareControlFlowRules(registry, controlFlowSelection); err != nil {
		return nil, err
	}
	if _, err := prepareSSARules(registry, ssaSelection); err != nil {
		return nil, err
	}
	selection := make(
		[]rules.Selection,
		0,
		len(typesSelection) + len(controlFlowSelection) + len(ssaSelection),
	)
	selection = append(selection, typesSelection...)
	selection = append(selection, controlFlowSelection...)
	selection = append(selection, ssaSelection...)
	sort.Slice(
		selection,
		func(left, right int) bool {
			return selection[left].ID < selection[right].ID
		},
	)
	snapshots := make([]nativeRuleSnapshot, len(selection))
	for index, selected := range selection {
		rule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf(
				"native analysis cache selected rule %q is unknown",
				selected.ID,
			)
		}
		metadata, _ := registry.Metadata(selected.ID)
		execution := ""
		switch selected.Requirement {
		case rules.RequireTypes:
			if _, packageWide := rule.(rules.PackageRule); packageWide {
				execution = "package"
			} else {
				execution = "types"
			}
		case rules.RequireControlFlow:
			execution = "control-flow"
		case rules.RequireSSA:
			execution = "ssa"
		default:
			return nil, fmt.Errorf(
				"native analysis cache selected rule %q has invalid requirement",
				selected.ID,
			)
		}
		fixes := make([]nativeFixSnapshot, len(metadata.Fixes))
		for fixIndex, fix := range metadata.Fixes {
			fixes[fixIndex] = nativeFixSnapshot{Name: fix.Name, Safety: fix.Safety}
		}
		optionsDigest := cache.DigestOf(selected.Options.CanonicalBytes())
		snapshots[index] = nativeRuleSnapshot{
			ID: selected.ID,
			Severity: selected.Severity,
			Requirement: selected.Requirement,
			Execution: execution,
			NodeInterests: slices.Clone(metadata.NodeInterests),
			RequiresDependencySyntax: metadata.RequiresDependencySyntax,
			RequiresEffectFacts: metadata.RequiresEffectFacts,
			RunOnGenerated: metadata.RunOnGenerated,
			RunDespiteTypeErrors: metadata.RunDespiteTypeErrors,
			MinimumGoVersion: metadata.MinimumGoVersion,
			Fixes: fixes,
			OptionsDigest: hex.EncodeToString(optionsDigest[:]),
		}
	}
	return snapshots, nil
}

func nativeEffectCacheDigests(facts *nativeEffectFacts) map[string]cache.Digest {
	if facts == nil {
		return map[string]cache.Digest{}
	}
	return map[string]cache.Digest{"native-effects-v13": facts.digest()}
}

func nativeSourceOwners(loaded PackageLoadResult) (map[string]string, error) {
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return nil, err
	}
	files, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return nil, err
	}
	owners := make(map[string]string, len(files))
	for _, work := range files {
		owners[work.file.path] = work.package_.ID
	}
	return owners, nil
}

func encodeNativePackageCacheEntry(
	selection []rules.Selection,
	snapshots []nativeRuleSnapshot,
	loaded PackageLoadResult,
	diagnostics []rules.Diagnostic,
) ([]byte, error) {
	if len(selection) == 0 || len(snapshots) != len(selection) {
		return nil, fmt.Errorf("encode native analysis cache entry requires selected rules")
	}
	if err := validateCanonicalDiagnostics(diagnostics); err != nil {
		return nil, err
	}
	owners, err := nativeSourceOwners(loaded)
	if err != nil {
		return nil, err
	}
	entry := nativePackageCacheEntry{
		Version: nativePackageCacheEntryVersion,
		Requirement: rules.MaximumRequirement(selection),
		Rules: slices.Clone(snapshots),
		Diagnostics: make([]nativePersistedDiagnostic, len(diagnostics)),
	}
	for index, diagnostic := range diagnostics {
		owner, found := owners[diagnostic.Path]
		if !found {
			return nil, fmt.Errorf("native diagnostic %d source is not owned", index)
		}
		entry.Diagnostics[index] = nativePersistedDiagnostic{
			PackageID: owner,
			Diagnostic: persistDiagnostic(diagnostic),
		}
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("encode native analysis cache entry: %w", err)
	}
	if len(encoded) > cache.MaxEntrySize {
		return nil, fmt.Errorf(
			"native analysis cache entry is %d bytes; maximum is %d",
			len(encoded),
			cache.MaxEntrySize,
		)
	}
	return encoded, nil
}

func restoreNativePackageCacheEntry(
	registry *rules.Registry,
	selection []rules.Selection,
	snapshots []nativeRuleSnapshot,
	loaded PackageLoadResult,
	encoded []byte,
) ([]rules.Diagnostic, error) {
	if registry == nil || len(selection) == 0 || len(snapshots) != len(selection) {
		return nil, fmt.Errorf(
			"restore native analysis cache entry requires selected rules",
		)
	}
	entry, err := decodeNativePackageCacheEntry(encoded)
	if err != nil {
		return nil, err
	}
	if entry.Requirement != rules.MaximumRequirement(selection) {
		return nil, fmt.Errorf("native analysis cache requirement is stale")
	}
	if !reflect.DeepEqual(entry.Rules, snapshots) {
		return nil, fmt.Errorf("native analysis cache rule metadata is stale")
	}
	owners, err := nativeSourceOwners(loaded)
	if err != nil {
		return nil, err
	}
	type activeRule struct {
		metadata rules.Metadata
		severity rules.Severity
	}
	active := make(map[string]activeRule, len(selection))
	for _, selected := range selection {
		if _, duplicate := active[selected.ID]; duplicate {
			return nil, fmt.Errorf(
				"native analysis cache selected rule %q is duplicated",
				selected.ID,
			)
		}
		rule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf(
				"native analysis cache selected rule %q is unknown",
				selected.ID,
			)
		}
		if _, adapted := rule.(*packageAnalyzerRule); adapted {
			return nil, fmt.Errorf(
				"native analysis cache selected rule %q is adapted",
				selected.ID,
			)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if metadata.Requirement != selected.Requirement ||
			metadata.Requirement < rules.RequireTypes {
			return nil, fmt.Errorf(
				"native analysis cache selected rule %q has stale metadata",
				selected.ID,
			)
		}
		active[selected.ID] = activeRule{metadata: metadata, severity: selected.Severity}
	}

	diagnostics := make([]rules.Diagnostic, len(entry.Diagnostics))
	for index, persisted := range entry.Diagnostics {
		diagnostic, err := restorePersistedDiagnostic(persisted.Diagnostic)
		if err != nil {
			return nil, fmt.Errorf(
				"restore cached native diagnostic %d: %w",
				index,
				err,
			)
		}
		execution, found := active[diagnostic.RuleID]
		if !found {
			return nil, fmt.Errorf(
				"cached native diagnostic %d rule is not selected",
				index,
			)
		}
		if owner, found := owners[diagnostic.Path]; !found || owner != persisted.PackageID {
			return nil, fmt.Errorf(
				"cached native diagnostic %d source is not owned",
				index,
			)
		}
		file, found := loaded.Sources.Lookup(diagnostic.Path)
		if !found {
			return nil, fmt.Errorf(
				"cached native diagnostic %d source is missing",
				index,
			)
		}
		if file.Metadata().Generated && !execution.metadata.RunOnGenerated {
			return nil, fmt.Errorf(
				"cached native diagnostic %d is in an ineligible generated source",
				index,
			)
		}
		finding := rules.Finding{
			MessageKey: diagnostic.MessageKey,
			Message: diagnostic.Message,
			Range: diagnostic.Range,
			Related: diagnostic.Related,
			Notes: diagnostic.Notes,
			Help: diagnostic.Help,
			Fixes: diagnostic.Fixes,
			WithheldFixes: diagnostic.WithheldFixes,
		}
		validated, err := diagnosticForFinding(
			file,
			execution.metadata,
			execution.severity,
			finding,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"validate cached native diagnostic %d: %w",
				index,
				err,
			)
		}
		if !reflect.DeepEqual(validated, diagnostic) {
			return nil, fmt.Errorf(
				"cached native diagnostic %d identity is stale",
				index,
			)
		}
		diagnostics[index] = diagnostic
	}
	if err := validateCanonicalDiagnostics(diagnostics); err != nil {
		return nil, err
	}
	return diagnostics, nil
}

func decodeNativePackageCacheEntry(encoded []byte) (nativePackageCacheEntry, error) {
	if len(encoded) == 0 || len(encoded) > cache.MaxEntrySize {
		return nativePackageCacheEntry{}, fmt.Errorf(
			"native analysis cache entry size %d is outside 1..%d",
			len(encoded),
			cache.MaxEntrySize,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var entry nativePackageCacheEntry
	if err := decoder.Decode(&entry); err != nil {
		return nativePackageCacheEntry{}, fmt.Errorf(
			"decode native analysis cache entry: %w",
			err,
		)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nativePackageCacheEntry{}, fmt.Errorf(
			"decode native analysis cache entry trailing data",
		)
	}
	canonical, err := json.Marshal(entry)
	if err != nil {
		return nativePackageCacheEntry{}, fmt.Errorf(
			"re-encode native analysis cache entry: %w",
			err,
		)
	}
	if !bytes.Equal(canonical, encoded) {
		return nativePackageCacheEntry{}, fmt.Errorf(
			"native analysis cache entry is not canonically encoded",
		)
	}
	if entry.Version != nativePackageCacheEntryVersion {
		return nativePackageCacheEntry{}, fmt.Errorf(
			"native analysis cache entry version %d is unsupported",
			entry.Version,
		)
	}
	if entry.Requirement < rules.RequireTypes ||
		entry.Requirement > rules.RequireSSA ||
		entry.Rules == nil ||
		entry.Diagnostics == nil {
		return nativePackageCacheEntry{}, fmt.Errorf(
			"native analysis cache entry identity is incomplete",
		)
	}
	return entry, nil
}
