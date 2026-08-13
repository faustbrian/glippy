// Package baseline owns deterministic, source-bound lint adoption waivers.
package baseline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

// SchemaVersion is the only accepted baseline document version.
const SchemaVersion = 1

const fingerprintDomain = "gox-baseline-source-v1\x00"

// Document is one portable, deterministic lint baseline.
type Document struct {
	SchemaVersion int `json:"schema_version"`
	Entries []Entry `json:"entries"`
}

// Entry suppresses an exact number of structurally identical diagnostics.
type Entry struct {
	RuleID string `json:"rule_id"`
	Path string `json:"path"`
	MessageKey string `json:"message_key"`
	SourceFingerprint string `json:"source_fingerprint"`
	Count int `json:"count"`
	Reason string `json:"reason,omitempty"`
	ExpiresOn string `json:"expires_on,omitempty"`
}

// ParseOptions supplies registry identity for strict baseline validation.
type ParseOptions struct {
	KnownRules []string
}

// InputFile binds diagnostics to the exact immutable source used to produce them.
type InputFile struct {
	File *source.File
	Diagnostics []rules.Diagnostic
}

// ProblemKind classifies an adoption waiver that no longer applies.
type ProblemKind string

const (
	// ProblemStale identifies an unused baseline count.
	ProblemStale ProblemKind = "stale"
	// ProblemExpired identifies an entry disabled by the configured cutoff.
	ProblemExpired ProblemKind = "expired"
)

// Problem is one deterministic baseline policy finding.
type Problem struct {
	Kind ProblemKind
	Entry Entry
	Remaining int
}

// ApplyOptions controls explicit stale and expiry policy.
type ApplyOptions struct {
	ReportStale bool
	ExpiryCutoff string
}

// AppliedFile separates visible diagnostics from baseline-owned diagnostics.
type AppliedFile struct {
	Path string
	Diagnostics []rules.Diagnostic
	Baselined []rules.Diagnostic
}

// ApplyResult is one complete baseline application over exact input files.
type ApplyResult struct {
	Files []AppliedFile
	Problems []Problem
}

type entryKey struct {
	RuleID string
	Path string
	MessageKey string
	SourceFingerprint string
}

// Parse strictly decodes and validates one baseline document.
func Parse(name string, input []byte, options ParseOptions) (Document, error) {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("%s: decode baseline: %w", name, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Document{}, fmt.Errorf(
				"%s: decode baseline: trailing JSON value",
				name,
			)
		}
		return Document{}, fmt.Errorf("%s: decode baseline: %w", name, err)
	}
	known := make(map[string]struct{}, len(options.KnownRules))
	for _, ruleID := range options.KnownRules {
		known[ruleID] = struct{}{}
	}
	if err := validateDocument(document, known); err != nil {
		return Document{}, fmt.Errorf("%s: %w", name, err)
	}
	document.Entries = canonicalEntries(document.Entries)
	return document, nil
}

// Encode returns the canonical indented representation with one final newline.
func Encode(document Document) ([]byte, error) {
	if err := validateDocument(document, nil); err != nil {
		return nil, err
	}
	document.Entries = canonicalEntries(document.Entries)
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode baseline: %w", err)
	}
	return append(encoded, '\n'), nil
}

// Generate creates one portable document from visible source diagnostics.
func Generate(root string, inputs []InputFile) (Document, error) {
	if err := validateRoot(root); err != nil {
		return Document{}, err
	}
	counts := make(map[entryKey]int)
	for _, input := range inputs {
		portable, err := portablePath(root, input.File)
		if err != nil {
			return Document{}, err
		}
		for _, diagnostic := range input.Diagnostics {
			key, err := diagnosticKey(portable, input.File, diagnostic)
			if err != nil {
				return Document{}, err
			}
			counts[key]++
		}
	}
	entries := make([]Entry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, entryFromKey(key, count))
	}
	return Document{SchemaVersion: SchemaVersion, Entries: canonicalEntries(entries)}, nil
}

// Apply consumes exact baseline counts without hiding changed diagnostics.
func Apply(
	root string,
	document Document,
	inputs []InputFile,
	options ApplyOptions,
) (ApplyResult, error) {
	if err := validateRoot(root); err != nil {
		return ApplyResult{}, err
	}
	if err := validateDocument(document, nil); err != nil {
		return ApplyResult{}, err
	}
	if options.ExpiryCutoff != "" {
		if err := validateDate(options.ExpiryCutoff); err != nil {
			return ApplyResult{}, fmt.Errorf("baseline expiry cutoff: %w", err)
		}
	}
	entries := canonicalEntries(document.Entries)
	remaining := make(map[entryKey]int, len(entries))
	expired := make(map[entryKey]bool, len(entries))
	for _, entry := range entries {
		key := keyFromEntry(entry)
		if options.ExpiryCutoff != "" &&
			entry.ExpiresOn != "" &&
			entry.ExpiresOn <= options.ExpiryCutoff {
			expired[key] = true
			continue
		}
		remaining[key] = entry.Count
	}
	result := ApplyResult{Files: make([]AppliedFile, 0, len(inputs)), Problems: []Problem{}}
	seenPaths := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		portable, err := portablePath(root, input.File)
		if err != nil {
			return ApplyResult{}, err
		}
		if _, duplicate := seenPaths[portable]; duplicate {
			return ApplyResult{}, fmt.Errorf(
				"duplicate baseline input path %q",
				portable,
			)
		}
		seenPaths[portable] = struct{}{}
		applied := AppliedFile{
			Path: input.File.Path(),
			Diagnostics: make([]rules.Diagnostic, 0, len(input.Diagnostics)),
			Baselined: make([]rules.Diagnostic, 0),
		}
		for _, diagnostic := range input.Diagnostics {
			key, err := diagnosticKey(portable, input.File, diagnostic)
			if err != nil {
				return ApplyResult{}, err
			}
			if !expired[key] && remaining[key] > 0 {
				remaining[key]--
				applied.Baselined = append(applied.Baselined, diagnostic)
				continue
			}
			applied.Diagnostics = append(applied.Diagnostics, diagnostic)
		}
		result.Files = append(result.Files, applied)
	}
	for _, entry := range entries {
		if _, analyzed := seenPaths[entry.Path]; !analyzed {
			continue
		}
		key := keyFromEntry(entry)
		if expired[key] {
			result.Problems = append(
				result.Problems,
				Problem{Kind: ProblemExpired, Entry: entry, Remaining: entry.Count},
			)
			continue
		}
		if options.ReportStale && remaining[key] > 0 {
			result.Problems = append(
				result.Problems,
				Problem{
					Kind: ProblemStale,
					Entry: entry,
					Remaining: remaining[key],
				},
			)
		}
	}
	return result, nil
}

// ValidPath reports whether a baseline entry or configured document path is portable.
func ValidPath(value string) bool {
	return value != "" &&
		!strings.Contains(value, "\\") &&
		!strings.ContainsAny(value, ":\x00") &&
		!strings.HasPrefix(value, "/") &&
		path.Clean(value) == value &&
		value != "." &&
		value != ".." &&
		!strings.HasPrefix(value, "../")
}

func validateDocument(document Document, known map[string]struct{}) error {
	if document.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported baseline schema version %d", document.SchemaVersion)
	}
	if document.Entries == nil {
		return fmt.Errorf("baseline entries must be an array")
	}
	seen := make(map[entryKey]struct{}, len(document.Entries))
	for index, entry := range document.Entries {
		if entry.RuleID == "" {
			return fmt.Errorf("baseline entry %d has no rule ID", index)
		}
		if known != nil {
			if _, found := known[entry.RuleID]; !found {
				return fmt.Errorf(
					"baseline entry %d has unknown rule %q",
					index,
					entry.RuleID,
				)
			}
		}
		if !ValidPath(entry.Path) {
			return fmt.Errorf(
				"baseline entry %d path %q is not a portable relative path",
				index,
				entry.Path,
			)
		}
		if entry.MessageKey == "" {
			return fmt.Errorf("baseline entry %d has no message key", index)
		}
		decoded, err := hex.DecodeString(entry.SourceFingerprint)
		if err != nil ||
			len(decoded) != sha256.Size ||
			entry.SourceFingerprint != strings.ToLower(entry.SourceFingerprint) {
			return fmt.Errorf(
				"baseline entry %d source fingerprint must be lowercase SHA-256",
				index,
			)
		}
		if entry.Count <= 0 {
			return fmt.Errorf("baseline entry %d count must be positive", index)
		}
		if entry.ExpiresOn != "" {
			if err := validateDate(entry.ExpiresOn); err != nil {
				return fmt.Errorf("baseline entry %d expires_on: %w", index, err)
			}
		}
		key := keyFromEntry(entry)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate baseline entry for %s", describeKey(key))
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateDate(value string) error {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil || parsed.Format(time.DateOnly) != value {
		return fmt.Errorf("must be a valid YYYY-MM-DD date")
	}
	return nil
}

func validateRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("baseline root %q is not normalized absolute", root)
	}
	return nil
}

func portablePath(root string, file *source.File) (string, error) {
	if file == nil {
		return "", fmt.Errorf("baseline input requires a source file")
	}
	filePath := file.Path()
	if !filepath.IsAbs(filePath) || filepath.Clean(filePath) != filePath {
		return "", fmt.Errorf(
			"baseline source path %q is not normalized absolute",
			filePath,
		)
	}
	relative, err := filepath.Rel(root, filePath)
	if err != nil {
		return "", fmt.Errorf("baseline source path %q: %w", filePath, err)
	}
	portable := filepath.ToSlash(relative)
	if !ValidPath(portable) {
		return "", fmt.Errorf("baseline source path %q is outside root %q", filePath, root)
	}
	return portable, nil
}

func diagnosticKey(
	portable string,
	file *source.File,
	diagnostic rules.Diagnostic,
) (entryKey, error) {
	if diagnostic.RuleID == "" || diagnostic.MessageKey == "" {
		return entryKey{}, fmt.Errorf("baseline diagnostic has no rule ID or message key")
	}
	if diagnostic.Path != file.Path() || diagnostic.Digest != file.Digest() {
		return entryKey{}, fmt.Errorf(
			"baseline diagnostic source identity does not match %q",
			file.Path(),
		)
	}
	span, valid := file.Slice(diagnostic.Range)
	if !valid {
		return entryKey{}, fmt.Errorf(
			"baseline diagnostic %q has invalid source range",
			diagnostic.RuleID,
		)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(fingerprintDomain))
	_, _ = hash.Write([]byte(span))
	return entryKey{
		RuleID: diagnostic.RuleID,
		Path: portable,
		MessageKey: diagnostic.MessageKey,
		SourceFingerprint: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func canonicalEntries(entries []Entry) []Entry {
	result := slices.Clone(entries)
	if result == nil {
		result = []Entry{}
	}
	sort.Slice(
		result,
		func(first, second int) bool {
			left := keyFromEntry(result[first])
			right := keyFromEntry(result[second])
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			if left.RuleID != right.RuleID {
				return left.RuleID < right.RuleID
			}
			if left.MessageKey != right.MessageKey {
				return left.MessageKey < right.MessageKey
			}
			return left.SourceFingerprint < right.SourceFingerprint
		},
	)
	return result
}

func keyFromEntry(entry Entry) entryKey {
	return entryKey{
		RuleID: entry.RuleID,
		Path: entry.Path,
		MessageKey: entry.MessageKey,
		SourceFingerprint: entry.SourceFingerprint,
	}
}

func entryFromKey(key entryKey, count int) Entry {
	return Entry{
		RuleID: key.RuleID,
		Path: key.Path,
		MessageKey: key.MessageKey,
		SourceFingerprint: key.SourceFingerprint,
		Count: count,
	}
}

func describeKey(key entryKey) string {
	return fmt.Sprintf(
		"%s/%s/%s/%s",
		key.Path,
		key.RuleID,
		key.MessageKey,
		key.SourceFingerprint,
	)
}
