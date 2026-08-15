package analysis

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

const packageAnalyzerCacheEntryVersion = 2

// PackageCacheOptions binds a caller-owned persistent store to the complete
// non-source identity of one typed analysis run. Source, package, module,
// workspace, environment, export, overlay, and imported-fact inputs are
// derived from the exact package load.
type PackageCacheOptions struct {
	Store *cache.Store
	ToolVersion string
	BuildGoVersion string
	SourceGoVersion string
	Configuration cache.Digest
	CGOEnabled bool
	FormatterMode string
}

type packageCachePlan struct {
	options PackageCacheOptions
	rules []cache.RuleInput
}

func preparePackageCachePlan(
	options *PackageCacheOptions,
	selection []rules.Selection,
	loadOptions PackageLoadOptions,
) (*packageCachePlan, error) {
	if options == nil {
		return nil, nil
	}
	if options.Store == nil {
		return nil, fmt.Errorf("package analysis cache requires a store")
	}
	values := []struct {
		name string
		value string
	}{
		{name: "tool version", value: options.ToolVersion},
		{name: "build Go version", value: options.BuildGoVersion},
		{name: "source Go version", value: options.SourceGoVersion},
		{name: "formatter mode", value: options.FormatterMode},
	}
	for _, value := range values {
		if strings.TrimSpace(value.value) == "" {
			return nil, fmt.Errorf("package analysis cache %s is required", value.name)
		}
	}
	if options.Configuration == (cache.Digest{}) {
		return nil, fmt.Errorf("package analysis cache configuration digest is required")
	}
	if err := validatePackageCacheLoadIdentity(loadOptions, options.CGOEnabled); err != nil {
		return nil, err
	}
	inputs := make([]cache.RuleInput, len(selection))
	for index, selected := range selection {
		inputs[index] = cache.RuleInput{
			ID: selected.ID,
			Severity: string(selected.Severity),
			Options: cache.DigestOf(selected.Options.CanonicalBytes()),
		}
	}
	return &packageCachePlan{options: *options, rules: inputs}, nil
}

type persistedRange struct {
	Start int `json:"start"`
	End int `json:"end"`
}

type persistedRelated struct {
	Range persistedRange `json:"range"`
	Message string `json:"message"`
}

type persistedEdit struct {
	Range persistedRange `json:"range"`
	NewText string `json:"newText"`
}

type persistedFix struct {
	Name string `json:"name"`
	Safety rules.FixSafety `json:"safety"`
	Edits []persistedEdit `json:"edits"`
}

type persistedWithheldFix struct {
	Name string `json:"name"`
	Reason rules.FixWithholdingReason `json:"reason"`
	Message string `json:"message"`
}

type persistedDiagnostic struct {
	RuleID string `json:"rule"`
	Severity rules.Severity `json:"severity"`
	MessageKey string `json:"messageKey"`
	Message string `json:"message"`
	Path string `json:"path"`
	Digest string `json:"digest"`
	Range persistedRange `json:"range"`
	Related []persistedRelated `json:"related"`
	Notes []string `json:"notes"`
	Help string `json:"help"`
	Fixes []persistedFix `json:"fixes"`
	WithheldFixes []persistedWithheldFix `json:"withheldFixes"`
}

type packageAnalyzerCacheEntry struct {
	Version int `json:"version"`
	RuleID string `json:"rule"`
	PackageID string `json:"packageId"`
	PackagePath string `json:"packagePath"`
	Diagnostics []persistedDiagnostic `json:"diagnostics"`
	FactSnapshots []packageFactSnapshot `json:"factSnapshots"`
}

func (r *packageAnalyzerRule) encodePackageCacheEntry(
	pkg *packages.Package,
	diagnostics []rules.Diagnostic,
	facts *analyzerFactSet,
) ([]byte, error) {
	if r == nil || strings.TrimSpace(r.metadata.ID) == "" || len(r.steps) == 0 {
		return nil, fmt.Errorf(
			"encode package analyzer cache entry requires a rule and execution plan",
		)
	}
	if pkg == nil ||
		strings.TrimSpace(pkg.ID) == "" ||
		pkg.Types == nil ||
		pkg.Types.Path() == "" {
		return nil, fmt.Errorf(
			"encode package analyzer cache entry requires an identified typed package",
		)
	}
	if facts == nil {
		return nil, fmt.Errorf("encode package analyzer cache entry requires facts")
	}
	if err := validateCanonicalDiagnostics(diagnostics); err != nil {
		return nil, err
	}
	entry := packageAnalyzerCacheEntry{
		Version: packageAnalyzerCacheEntryVersion,
		RuleID: r.metadata.ID,
		PackageID: pkg.ID,
		PackagePath: pkg.Types.Path(),
		Diagnostics: make([]persistedDiagnostic, len(diagnostics)),
		FactSnapshots: make([]packageFactSnapshot, len(r.steps)),
	}
	for index, diagnostic := range diagnostics {
		entry.Diagnostics[index] = persistDiagnostic(diagnostic)
	}
	for index, step := range r.steps {
		encoded, err := facts.encodePackageFactSnapshot(step.original, pkg.Types)
		if err != nil {
			return nil, fmt.Errorf(
				"encode analyzer %q facts: %w",
				step.original.Name,
				err,
			)
		}
		snapshot, err := decodePackageFactSnapshot(encoded)
		if err != nil {
			return nil, fmt.Errorf(
				"decode analyzer %q facts: %w",
				step.original.Name,
				err,
			)
		}
		entry.FactSnapshots[index] = snapshot
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("encode package analyzer cache entry: %w", err)
	}
	if len(encoded) > cache.MaxEntrySize {
		return nil, fmt.Errorf(
			"package analyzer cache entry is %d bytes; maximum is %d",
			len(encoded),
			cache.MaxEntrySize,
		)
	}
	return encoded, nil
}

func (r *packageAnalyzerRule) restorePackageCacheEntry(
	pkg *packages.Package,
	sources PackageSourceSet,
	owners map[string]string,
	severity rules.Severity,
	facts *analyzerFactSet,
	encoded []byte,
) ([]rules.Diagnostic, error) {
	if r == nil || strings.TrimSpace(r.metadata.ID) == "" || len(r.steps) == 0 {
		return nil, fmt.Errorf(
			"restore package analyzer cache entry requires a rule and execution plan",
		)
	}
	if pkg == nil ||
		strings.TrimSpace(pkg.ID) == "" ||
		pkg.Types == nil ||
		pkg.Types.Path() == "" {
		return nil, fmt.Errorf(
			"restore package analyzer cache entry requires an identified typed package",
		)
	}
	if facts == nil {
		return nil, fmt.Errorf("restore package analyzer cache entry requires facts")
	}
	entry, err := decodePackageAnalyzerCacheEntry(encoded)
	if err != nil {
		return nil, err
	}
	if entry.RuleID != r.metadata.ID {
		return nil, fmt.Errorf(
			"package analyzer cache rule %q does not match %q",
			entry.RuleID,
			r.metadata.ID,
		)
	}
	if entry.PackageID != pkg.ID || entry.PackagePath != pkg.Types.Path() {
		return nil, fmt.Errorf(
			"package analyzer cache package identity does not match %q",
			pkg.ID,
		)
	}
	if len(entry.FactSnapshots) != len(r.steps) {
		return nil, fmt.Errorf(
			"package analyzer cache fact plan has %d steps; want %d",
			len(entry.FactSnapshots),
			len(r.steps),
		)
	}
	diagnostics := make([]rules.Diagnostic, len(entry.Diagnostics))
	for index, persisted := range entry.Diagnostics {
		diagnostic, err := restorePersistedDiagnostic(persisted)
		if err != nil {
			return nil, fmt.Errorf("restore cached diagnostic %d: %w", index, err)
		}
		file, found := sources.Lookup(diagnostic.Path)
		if !found || owners[diagnostic.Path] != pkg.ID {
			return nil, fmt.Errorf(
				"cached diagnostic %d source is not owned by package %q",
				index,
				pkg.ID,
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
		validated, err := diagnosticForFinding(file, r.metadata, severity, finding)
		if err != nil {
			return nil, fmt.Errorf("validate cached diagnostic %d: %w", index, err)
		}
		if !reflect.DeepEqual(validated, diagnostic) {
			return nil, fmt.Errorf("cached diagnostic %d identity is stale", index)
		}
		diagnostics[index] = diagnostic
	}
	if err := validateCanonicalDiagnostics(diagnostics); err != nil {
		return nil, err
	}

	candidate := cloneAnalyzerFactSet(facts)
	for index, snapshot := range entry.FactSnapshots {
		step := r.steps[index].original
		if snapshot.Analyzer != step.Name || snapshot.PackagePath != pkg.Types.Path() {
			return nil, fmt.Errorf(
				"package analyzer cache fact snapshot %d identity is stale",
				index,
			)
		}
		snapshotBytes, err := json.Marshal(snapshot)
		if err != nil {
			return nil, fmt.Errorf("encode cached fact snapshot %d: %w", index, err)
		}
		if err := candidate.restorePackageFactSnapshot(step, pkg, snapshotBytes);
			err != nil {
			return nil, fmt.Errorf("restore cached fact snapshot %d: %w", index, err)
		}
	}
	facts.packageValues = candidate.packageValues
	facts.objectViews = candidate.objectViews
	facts.packages = candidate.packages
	return diagnostics, nil
}

func decodePackageAnalyzerCacheEntry(encoded []byte) (packageAnalyzerCacheEntry, error) {
	if len(encoded) == 0 || len(encoded) > cache.MaxEntrySize {
		return packageAnalyzerCacheEntry{}, fmt.Errorf(
			"package analyzer cache entry size %d is outside 1..%d",
			len(encoded),
			cache.MaxEntrySize,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var entry packageAnalyzerCacheEntry
	if err := decoder.Decode(&entry); err != nil {
		return packageAnalyzerCacheEntry{}, fmt.Errorf(
			"decode package analyzer cache entry: %w",
			err,
		)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return packageAnalyzerCacheEntry{}, fmt.Errorf(
			"decode package analyzer cache entry trailing data",
		)
	}
	canonical, err := json.Marshal(entry)
	if err != nil {
		return packageAnalyzerCacheEntry{}, fmt.Errorf(
			"re-encode package analyzer cache entry: %w",
			err,
		)
	}
	if !bytes.Equal(canonical, encoded) {
		return packageAnalyzerCacheEntry{}, fmt.Errorf(
			"package analyzer cache entry is not canonically encoded",
		)
	}
	if entry.Version != packageAnalyzerCacheEntryVersion {
		return packageAnalyzerCacheEntry{}, fmt.Errorf(
			"package analyzer cache entry version %d is unsupported",
			entry.Version,
		)
	}
	if strings.TrimSpace(entry.RuleID) == "" ||
		strings.TrimSpace(entry.PackageID) == "" ||
		strings.TrimSpace(entry.PackagePath) == "" ||
		entry.Diagnostics == nil ||
		entry.FactSnapshots == nil {
		return packageAnalyzerCacheEntry{}, fmt.Errorf(
			"package analyzer cache entry identity is incomplete",
		)
	}
	return entry, nil
}

func persistDiagnostic(diagnostic rules.Diagnostic) persistedDiagnostic {
	var related []persistedRelated
	if diagnostic.Related != nil {
		related = make([]persistedRelated, len(diagnostic.Related))
	}
	for index, item := range diagnostic.Related {
		related[index] = persistedRelated{
			Range: persistRange(item.Range),
			Message: item.Message,
		}
	}
	var fixes []persistedFix
	if diagnostic.Fixes != nil {
		fixes = make([]persistedFix, len(diagnostic.Fixes))
	}
	for index, fix := range diagnostic.Fixes {
		var edits []persistedEdit
		if fix.Edits != nil {
			edits = make([]persistedEdit, len(fix.Edits))
		}
		for editIndex, edit := range fix.Edits {
			edits[editIndex] = persistedEdit{
				Range: persistRange(edit.Range),
				NewText: edit.NewText,
			}
		}
		fixes[index] = persistedFix{Name: fix.Name, Safety: fix.Safety, Edits: edits}
	}
	var withheldFixes []persistedWithheldFix
	if diagnostic.WithheldFixes != nil {
		withheldFixes = make([]persistedWithheldFix, len(diagnostic.WithheldFixes))
	}
	for index, fix := range diagnostic.WithheldFixes {
		withheldFixes[index] = persistedWithheldFix{
			Name: fix.Name,
			Reason: fix.Reason,
			Message: fix.Message,
		}
	}
	return persistedDiagnostic{
		RuleID: diagnostic.RuleID,
		Severity: diagnostic.Severity,
		MessageKey: diagnostic.MessageKey,
		Message: diagnostic.Message,
		Path: diagnostic.Path,
		Digest: hex.EncodeToString(diagnostic.Digest[:]),
		Range: persistRange(diagnostic.Range),
		Related: related,
		Notes: slices.Clone(diagnostic.Notes),
		Help: diagnostic.Help,
		Fixes: fixes,
		WithheldFixes: withheldFixes,
	}
}

func restorePersistedDiagnostic(persisted persistedDiagnostic) (rules.Diagnostic, error) {
	digestBytes, err := hex.DecodeString(persisted.Digest)
	if err != nil ||
		len(digestBytes) != len(source.Digest{}) ||
		persisted.Digest != hex.EncodeToString(digestBytes) {
		return rules.Diagnostic{}, fmt.Errorf("diagnostic source digest is invalid")
	}
	var digest source.Digest
	copy(digest[:], digestBytes)
	var related []rules.Related
	if persisted.Related != nil {
		related = make([]rules.Related, len(persisted.Related))
	}
	for index, item := range persisted.Related {
		related[index] = rules.Related{
			Range: restoreRange(item.Range),
			Message: item.Message,
		}
	}
	var fixes []rules.Fix
	if persisted.Fixes != nil {
		fixes = make([]rules.Fix, len(persisted.Fixes))
	}
	for index, fix := range persisted.Fixes {
		var edits []rules.Edit
		if fix.Edits != nil {
			edits = make([]rules.Edit, len(fix.Edits))
		}
		for editIndex, edit := range fix.Edits {
			edits[editIndex] = rules.Edit{
				Range: restoreRange(edit.Range),
				NewText: edit.NewText,
			}
		}
		fixes[index] = rules.Fix{Name: fix.Name, Safety: fix.Safety, Edits: edits}
	}
	var withheldFixes []rules.WithheldFix
	if persisted.WithheldFixes != nil {
		withheldFixes = make([]rules.WithheldFix, len(persisted.WithheldFixes))
	}
	for index, fix := range persisted.WithheldFixes {
		withheldFixes[index] = rules.WithheldFix{
			Name: fix.Name,
			Reason: fix.Reason,
			Message: fix.Message,
		}
	}
	return rules.Diagnostic{
		RuleID: persisted.RuleID,
		Severity: persisted.Severity,
		MessageKey: persisted.MessageKey,
		Message: persisted.Message,
		Path: persisted.Path,
		Digest: digest,
		Range: restoreRange(persisted.Range),
		Related: related,
		Notes: slices.Clone(persisted.Notes),
		Help: persisted.Help,
		Fixes: fixes,
		WithheldFixes: withheldFixes,
	}, nil
}

func validateCanonicalDiagnostics(diagnostics []rules.Diagnostic) error {
	if diagnostics == nil {
		return fmt.Errorf("package analyzer cache diagnostics are required")
	}
	ordered := OrderDiagnostics(diagnostics)
	if !reflect.DeepEqual(ordered, diagnostics) {
		return fmt.Errorf("package analyzer cache diagnostics are unordered")
	}
	for index := 1; index < len(diagnostics); index++ {
		if reflect.DeepEqual(diagnostics[index - 1], diagnostics[index]) {
			return fmt.Errorf("package analyzer cache diagnostics are duplicated")
		}
	}
	return nil
}

func cloneAnalyzerFactSet(facts *analyzerFactSet) *analyzerFactSet {
	clone := newAnalyzerFactSet()
	for key, value := range facts.packageValues {
		clone.packageValues[key] = bytes.Clone(value)
	}
	for key, view := range facts.objectViews {
		clonedView := &objectFactView{
			values: make(map[objectFactKey][]byte, len(view.values)),
		}
		for fact, value := range view.values {
			clonedView.values[fact] = bytes.Clone(value)
		}
		clone.objectViews[key] = clonedView
	}
	for package_, loaded := range facts.packages {
		clone.packages[package_] = loaded
	}
	return clone
}

func persistRange(range_ source.Range) persistedRange {
	return persistedRange{Start: range_.Start, End: range_.End}
}

func restoreRange(range_ persistedRange) source.Range {
	return source.Range{Start: range_.Start, End: range_.End}
}
