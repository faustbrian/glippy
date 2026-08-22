// Package corpus owns the pinned external-repository validation contract.
package corpus

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	ManifestSchemaVersion = 1
	MaximumRepositories = 32
)

var (
	repositoryIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	repositoryURLPattern = regexp.MustCompile(
		`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\.git$`,
	)
	revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	licensePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]*$`)
	goDirectivePattern = regexp.MustCompile(`^1\.[0-9]+(?:\.[0-9]+)?$`)
	staticcheckVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	rolePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	patternSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

type SourceVersionPolicy string

const (
	SourceVersionSupported SourceVersionPolicy = "supported"
	SourceVersionUnsupported SourceVersionPolicy = "unsupported"
)

// Manifest identifies one bounded, reproducible external validation corpus.
type Manifest struct {
	SchemaVersion int `json:"schema_version"`
	StaticcheckVersion string `json:"staticcheck_version"`
	Repositories []Repository `json:"repositories"`
}

// Repository records provenance and workload traits without copying source.
type Repository struct {
	ID string `json:"id"`
	Repository string `json:"repository"`
	Revision string `json:"revision"`
	License string `json:"license"`
	LicensePath string `json:"license_path"`
	Roles []string `json:"roles"`
	GoDirective string `json:"go_directive"`
	SourceVersionPolicy SourceVersionPolicy `json:"source_version_policy"`
	CGO bool `json:"cgo"`
	Generated bool `json:"generated"`
	Patterns []string `json:"patterns"`
}

// ParseManifest strictly decodes and validates one canonical manifest.
func ParseManifest(input []byte) (Manifest, error) {
	if err := validateManifestShape(input); err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode corpus manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New(
				"decode corpus manifest: multiple JSON values",
			)
		}
		return Manifest{}, fmt.Errorf("decode corpus manifest trailing data: %w", err)
	}
	if err := validateRequiredManifestFields(input); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func validateManifestShape(input []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(input))
	if err := beginJSONObject(decoder, "corpus manifest"); err != nil {
		return err
	}
	allowed := map[string]struct{}{
		"schema_version": {},
		"staticcheck_version": {},
		"repositories": {},
	}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		name, err := jsonObjectField(decoder, "corpus manifest", allowed, seen)
		if err != nil {
			return err
		}
		if name != "repositories" {
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("decode corpus manifest field %q: %w", name, err)
			}
			continue
		}
		if err := validateRepositoryArray(decoder); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("decode corpus manifest: %w", err)
	}
	return nil
}

func validateRepositoryArray(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode corpus repositories: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return errors.New("decode corpus repositories: want JSON array")
	}
	index := 0
	for decoder.More() {
		if err := validateRepositoryObject(decoder, index); err != nil {
			return err
		}
		index++
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("decode corpus repositories: %w", err)
	}
	return nil
}

func validateRepositoryObject(decoder *json.Decoder, index int) error {
	label := fmt.Sprintf("repository %d", index)
	if err := beginJSONObject(decoder, label); err != nil {
		return err
	}
	allowed := map[string]struct{}{
		"id": {},
		"repository": {},
		"revision": {},
		"license": {},
		"license_path": {},
		"roles": {},
		"go_directive": {},
		"source_version_policy": {},
		"cgo": {},
		"generated": {},
		"patterns": {},
	}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		name, err := jsonObjectField(decoder, label, allowed, seen)
		if err != nil {
			return err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode %s field %q: %w", label, name, err)
		}
		if (name == "cgo" || name == "generated") &&
			!bytes.Equal(value, []byte("true")) &&
			!bytes.Equal(value, []byte("false")) {
			return fmt.Errorf("decode %s: %s must be a JSON boolean", label, name)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	return nil
}

func beginJSONObject(decoder *json.Decoder, label string) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return fmt.Errorf("decode %s: want JSON object", label)
	}
	return nil
}

func jsonObjectField(
	decoder *json.Decoder,
	label string,
	allowed, seen map[string]struct{},
) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", fmt.Errorf("decode %s field: %w", label, err)
	}
	name, ok := token.(string)
	if !ok {
		return "", fmt.Errorf("decode %s field: want string name", label)
	}
	if _, found := allowed[name]; !found {
		return "", fmt.Errorf("decode %s: unknown field %q", label, name)
	}
	if _, found := seen[name]; found {
		return "", fmt.Errorf("decode %s: duplicate field %q", label, name)
	}
	seen[name] = struct{}{}
	return name, nil
}

func validateRequiredManifestFields(input []byte) error {
	var document struct {
		Repositories []map[string]json.RawMessage `json:"repositories"`
	}
	if err := json.Unmarshal(input, &document); err != nil {
		return fmt.Errorf("inspect required corpus fields: %w", err)
	}
	for index, repository := range document.Repositories {
		for _, field := range []string{"cgo", "generated"} {
			if _, found := repository[field]; !found {
				return fmt.Errorf(
					"repository %d is missing required %s flag",
					index,
					field,
				)
			}
		}
	}
	return nil
}

// Validate checks provenance, canonical ordering, and bounded workload shape.
func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf(
			"corpus schema_version = %d, want %d",
			m.SchemaVersion,
			ManifestSchemaVersion,
		)
	}
	if !staticcheckVersionPattern.MatchString(m.StaticcheckVersion) {
		return fmt.Errorf("invalid staticcheck_version %q", m.StaticcheckVersion)
	}
	if len(m.Repositories) == 0 || len(m.Repositories) > MaximumRepositories {
		return fmt.Errorf(
			"repository count = %d, want 1..%d",
			len(m.Repositories),
			MaximumRepositories,
		)
	}
	seenIDs := make(map[string]struct{}, len(m.Repositories))
	seenRepositories := make(map[string]struct{}, len(m.Repositories))
	for index, repository := range m.Repositories {
		if index > 0 && m.Repositories[index - 1].ID >= repository.ID {
			return errors.New("repositories must be ordered by ID without duplicates")
		}
		if err := repository.validate(); err != nil {
			return fmt.Errorf("repository %q: %w", repository.ID, err)
		}
		if _, found := seenIDs[repository.ID]; found {
			return fmt.Errorf("duplicate repository ID %q", repository.ID)
		}
		seenIDs[repository.ID] = struct{}{}
		if _, found := seenRepositories[repository.Repository]; found {
			return fmt.Errorf("duplicate repository URL %q", repository.Repository)
		}
		seenRepositories[repository.Repository] = struct{}{}
	}
	return nil
}

func (r Repository) validate() error {
	if !repositoryIDPattern.MatchString(r.ID) {
		return fmt.Errorf("invalid ID %q", r.ID)
	}
	if !repositoryURLPattern.MatchString(r.Repository) {
		return fmt.Errorf("invalid GitHub repository URL %q", r.Repository)
	}
	if !revisionPattern.MatchString(r.Revision) {
		return fmt.Errorf("revision %q is not a full lowercase Git SHA", r.Revision)
	}
	if !licensePattern.MatchString(r.License) {
		return fmt.Errorf("invalid license %q", r.License)
	}
	if !safeRelativePath(r.LicensePath) {
		return fmt.Errorf("invalid license_path %q", r.LicensePath)
	}
	if err := validateCanonicalStrings("roles", r.Roles, rolePattern.MatchString); err != nil {
		return err
	}
	if !goDirectivePattern.MatchString(r.GoDirective) {
		return fmt.Errorf("invalid go_directive %q", r.GoDirective)
	}
	switch r.SourceVersionPolicy {
	case SourceVersionSupported, SourceVersionUnsupported:
	default:
		return fmt.Errorf("invalid source_version_policy %q", r.SourceVersionPolicy)
	}
	if err := validateCanonicalStrings("patterns", r.Patterns, safePattern); err != nil {
		return err
	}
	return nil
}

func validateCanonicalStrings(name string, values []string, valid func(string) bool) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	if !slices.IsSorted(values) {
		return fmt.Errorf("%s must be ordered", name)
	}
	for index, value := range values {
		if !valid(value) {
			if name == "patterns" {
				return fmt.Errorf("unsafe pattern %q", value)
			}
			return fmt.Errorf("invalid %s value %q", name, value)
		}
		if index > 0 && values[index - 1] == value {
			return fmt.Errorf("%s must not contain duplicates", name)
		}
	}
	return nil
}

func safeRelativePath(value string) bool {
	return value != "" &&
		!strings.Contains(value, `\`) &&
		path.Clean(value) == value &&
		value != "." &&
		value != ".." &&
		!strings.HasPrefix(value, "../") &&
		!strings.HasPrefix(value, "/")
}

func safePattern(value string) bool {
	if value == "." || value == "./..." {
		return true
	}
	if !strings.HasPrefix(value, "./") || strings.Contains(value, `\`) {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "./"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		if segment == "..." {
			continue
		}
		if !patternSegmentPattern.MatchString(segment) {
			return false
		}
	}
	return true
}
