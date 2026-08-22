package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

const AdjudicationSchemaVersion = 1

var (
	adjudicationProfiles = []string{"default", "recommended"}
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type AdjudicationSummary struct {
	Repositories int
	Findings int
	Gaps int
	Unresolved int
}

type adjudicationDocument struct {
	SchemaVersion int `json:"schema_version"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Run evidenceIdentity `json:"run"`
	Repositories []repositoryAdjudication `json:"repositories"`
	Gaps []corpusGap `json:"gaps"`
}

type evidenceIdentity struct {
	ID string `json:"id"`
	Glippy string `json:"glippy"`
	Go string `json:"go"`
	Staticcheck string `json:"staticcheck"`
}

type repositoryAdjudication struct {
	ID string `json:"id"`
	Revision string `json:"revision"`
	ResultSHA256 string `json:"result_sha256"`
	IncompleteProfiles []string `json:"incomplete_profiles"`
	IncompleteComparators []string `json:"incomplete_comparators"`
	Profiles []profileAdjudication `json:"profiles"`
}

type profileAdjudication struct {
	Profile string `json:"profile"`
	Findings []findingAdjudication `json:"findings"`
}

type findingAdjudication struct {
	Fingerprint string `json:"fingerprint"`
	Classification string `json:"classification"`
	Reason string `json:"reason"`
}

type corpusGap struct {
	ID string `json:"id"`
	Repository string `json:"repository"`
	Source string `json:"source"`
	Kind string `json:"kind"`
	Summary string `json:"summary"`
	Evidence string `json:"evidence"`
	Disposition string `json:"disposition"`
	RuleID string `json:"rule_id"`
	Reason string `json:"reason"`
}

type jsonShape struct {
	Fields map[string]*jsonShape
	Optional map[string]struct{}
	Element *jsonShape
	Scalar scalarKind
}

type scalarKind uint8

const (
	anyScalar scalarKind = iota
	stringScalar
	numberScalar
	boolScalar
)

var (
	jsonString = &jsonShape{Scalar: stringScalar}
	jsonNumber = &jsonShape{Scalar: numberScalar}
	jsonBool = &jsonShape{Scalar: boolScalar}
	findingAdjudicationShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"fingerprint": jsonString,
			"classification": jsonString,
			"reason": jsonString,
		},
	}
	profileAdjudicationShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"profile": jsonString,
			"findings": {Element: findingAdjudicationShape},
		},
	}
	repositoryAdjudicationShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"id": jsonString,
			"revision": jsonString,
			"result_sha256": jsonString,
			"incomplete_profiles": {Element: jsonString},
			"incomplete_comparators": {Element: jsonString},
			"profiles": {Element: profileAdjudicationShape},
		},
	}
	corpusGapShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"id": jsonString,
			"repository": jsonString,
			"source": jsonString,
			"kind": jsonString,
			"summary": jsonString,
			"evidence": jsonString,
			"disposition": jsonString,
			"rule_id": jsonString,
			"reason": jsonString,
		},
	}
	adjudicationShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"schema_version": jsonNumber,
			"manifest_sha256": jsonString,
			"run": {
				Fields: map[string]*jsonShape{
					"id": jsonString,
					"glippy": jsonString,
					"go": jsonString,
					"staticcheck": jsonString,
				},
			},
			"repositories": {Element: repositoryAdjudicationShape},
			"gaps": {Element: corpusGapShape},
		},
	}
	artifactResultShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"file": jsonString,
			"sha256": jsonString,
			"valid_json": jsonBool,
		},
		Optional: map[string]struct{}{"valid_json": {}},
	}
	measurementArtifactResultShape = &jsonShape{
		Fields: map[string]*jsonShape{"file": jsonString, "valid_json": jsonBool},
		Optional: map[string]struct{}{"valid_json": {}},
	}
	repositoryResultRepositoryShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"id": jsonString,
			"repository": jsonString,
			"revision": jsonString,
			"license": jsonString,
			"license_path": jsonString,
			"roles": {Element: jsonString},
			"go_directive": jsonString,
			"source_version_policy": jsonString,
			"cgo": jsonBool,
			"generated": jsonBool,
			"patterns": {Element: jsonString},
		},
	}
	toolVersionsShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"glippy": jsonString,
			"go": jsonString,
			"staticcheck": jsonString,
		},
	}
	profileResultShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"profile": jsonString,
			"exit_code": jsonNumber,
			"diagnostics": artifactResultShape,
			"statistics": measurementArtifactResultShape,
			"findings": artifactResultShape,
			"diagnostic_count": jsonNumber,
			"complete": jsonBool,
		},
	}
	comparatorResultShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"name": jsonString,
			"exit_code": jsonNumber,
			"output": artifactResultShape,
		},
	}
	repositoryResultShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"schema_version": jsonNumber,
			"run_id": jsonString,
			"repository": repositoryResultRepositoryShape,
			"staticcheck_version": jsonString,
			"tools": toolVersionsShape,
			"profiles": {Element: profileResultShape},
			"comparators": {Element: comparatorResultShape},
		},
	}
	findingRangeShape = &jsonShape{
		Fields: map[string]*jsonShape{"start": jsonNumber, "end": jsonNumber},
	}
	findingShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"fingerprint": jsonString,
			"rule_id": jsonString,
			"severity": jsonString,
			"message_key": jsonString,
			"message": jsonString,
			"path": jsonString,
			"range": findingRangeShape,
		},
	}
	findingInventoryShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"schema_version": jsonNumber,
			"repository": jsonString,
			"revision": jsonString,
			"profile": jsonString,
			"diagnostics": {Element: findingShape},
		},
	}
)

func ValidateAdjudication(
	manifest Manifest,
	manifestInput []byte,
	resultRoot string,
	input []byte,
) (AdjudicationSummary, error) {
	if err := manifest.Validate(); err != nil {
		return AdjudicationSummary{}, err
	}
	if err := validateJSONShape(input, adjudicationShape, "corpus adjudication"); err != nil {
		return AdjudicationSummary{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var document adjudicationDocument
	if err := decoder.Decode(&document); err != nil {
		return AdjudicationSummary{}, fmt.Errorf("decode corpus adjudication: %w", err)
	}
	if err := requireJSONEOF(decoder, "corpus adjudication"); err != nil {
		return AdjudicationSummary{}, err
	}
	if err := document.validate(manifest, manifestInput); err != nil {
		return AdjudicationSummary{}, err
	}

	summary := AdjudicationSummary{
		Repositories: len(document.Repositories),
		Gaps: len(document.Gaps),
	}
	for index, repository := range document.Repositories {
		manifestRepository := manifest.Repositories[index]
		state, err := inspectBoundResult(
			resultRoot,
			manifestRepository,
			manifest.StaticcheckVersion,
			repository.ResultSHA256,
		)
		if err != nil {
			return AdjudicationSummary{}, err
		}
		if !slices.Equal(repository.IncompleteComparators, state.IncompleteComparators) {
			return AdjudicationSummary{}, fmt.Errorf(
				"repository %q incomplete comparator state does not match result",
				repository.ID,
			)
		}
		if state.Identity != document.Run {
			return AdjudicationSummary{}, fmt.Errorf(
				"repository %q corpus run identity does not match adjudication",
				repository.ID,
			)
		}
		for _, profile := range repository.Profiles {
			findings, complete, err := loadBoundFindings(
				resultRoot,
				manifestRepository,
				manifest.StaticcheckVersion,
				repository.ResultSHA256,
				profile.Profile,
			)
			if err != nil {
				return AdjudicationSummary{}, err
			}
			declaredIncomplete := slices.Contains(
				repository.IncompleteProfiles,
				profile.Profile,
			)
			if declaredIncomplete == complete {
				return AdjudicationSummary{}, fmt.Errorf(
					"repository %q profile %q incomplete state does not match result",
					repository.ID,
					profile.Profile,
				)
			}
			unresolved, err := validateFindingAdjudications(profile, findings)
			if err != nil {
				return AdjudicationSummary{}, fmt.Errorf(
					"repository %q profile %q: %w",
					repository.ID,
					profile.Profile,
					err,
				)
			}
			summary.Findings += len(profile.Findings)
			summary.Unresolved += unresolved
		}
		summary.Unresolved += len(repository.IncompleteProfiles)
		summary.Unresolved += len(repository.IncompleteComparators)
	}
	return summary, nil
}

func BuildAdjudicationTemplate(
	manifest Manifest,
	manifestInput []byte,
	resultRoot string,
) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(manifestInput)
	document := adjudicationDocument{
		SchemaVersion: AdjudicationSchemaVersion,
		ManifestSHA256: hex.EncodeToString(digest[:]),
		Run: evidenceIdentity{},
		Repositories: make([]repositoryAdjudication, 0, len(manifest.Repositories)),
		Gaps: []corpusGap{},
	}
	for repositoryIndex, repository := range manifest.Repositories {
		resultInput, err := readRegularFile(
			filepath.Join(resultRoot, repository.ID, "result.json"),
		)
		if err != nil {
			return nil, fmt.Errorf("read result for %q: %w", repository.ID, err)
		}
		resultDigest := sha256.Sum256(resultInput)
		resultSHA256 := hex.EncodeToString(resultDigest[:])
		state, err := inspectBoundResult(
			resultRoot,
			repository,
			manifest.StaticcheckVersion,
			resultSHA256,
		)
		if err != nil {
			return nil, err
		}
		if repositoryIndex == 0 {
			document.Run = state.Identity
		} else if state.Identity != document.Run {
			return nil, fmt.Errorf(
				"repository %q has mixed corpus run identity",
				repository.ID,
			)
		}
		entry := repositoryAdjudication{
			ID: repository.ID,
			Revision: repository.Revision,
			ResultSHA256: resultSHA256,
			IncompleteProfiles: []string{},
			IncompleteComparators: state.IncompleteComparators,
			Profiles: make([]profileAdjudication, 0, len(adjudicationProfiles)),
		}
		for _, profile := range adjudicationProfiles {
			findings, complete, err := loadBoundFindings(
				resultRoot,
				repository,
				manifest.StaticcheckVersion,
				entry.ResultSHA256,
				profile,
			)
			if err != nil {
				return nil, err
			}
			if !complete {
				entry.IncompleteProfiles = append(entry.IncompleteProfiles, profile)
			}
			adjudications := make([]findingAdjudication, 0, len(findings))
			for _, value := range findings {
				adjudications = append(
					adjudications,
					findingAdjudication{
						Fingerprint: value.Fingerprint,
						Classification: "unresolved",
						Reason: "pending manual adjudication",
					},
				)
			}
			slices.SortFunc(
				adjudications,
				func(left, right findingAdjudication) int {
					return strings.Compare(left.Fingerprint, right.Fingerprint)
				},
			)
			entry.Profiles = append(
				entry.Profiles,
				profileAdjudication{Profile: profile, Findings: adjudications},
			)
		}
		document.Repositories = append(document.Repositories, entry)
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode corpus adjudication template: %w", err)
	}
	return append(encoded, '\n'), nil
}

func (d adjudicationDocument) validate(manifest Manifest, manifestInput []byte) error {
	if d.SchemaVersion != AdjudicationSchemaVersion {
		return fmt.Errorf(
			"adjudication schema_version = %d, want %d",
			d.SchemaVersion,
			AdjudicationSchemaVersion,
		)
	}
	digest := sha256.Sum256(manifestInput)
	wantDigest := hex.EncodeToString(digest[:])
	if d.ManifestSHA256 != wantDigest {
		return fmt.Errorf(
			"adjudication manifest digest = %q, want %q",
			d.ManifestSHA256,
			wantDigest,
		)
	}
	if !runIDPattern.MatchString(d.Run.ID) {
		return fmt.Errorf("adjudication has invalid corpus run ID %q", d.Run.ID)
	}
	for name, value := range
		map[string]string{
			"Glippy": d.Run.Glippy,
			"Go": d.Run.Go,
			"Staticcheck": d.Run.Staticcheck,
		} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("adjudication has invalid %s run identity", name)
		}
	}
	if len(d.Repositories) != len(manifest.Repositories) {
		return fmt.Errorf(
			"adjudication repository count = %d, want %d",
			len(d.Repositories),
			len(manifest.Repositories),
		)
	}
	repositoryIDs := make(map[string]struct{}, len(manifest.Repositories))
	for index, repository := range d.Repositories {
		want := manifest.Repositories[index]
		if repository.ID != want.ID {
			return fmt.Errorf(
				"adjudication repository %d ID = %q, want %q",
				index,
				repository.ID,
				want.ID,
			)
		}
		if repository.Revision != want.Revision {
			return fmt.Errorf(
				"repository %q revision = %q, want %q",
				repository.ID,
				repository.Revision,
				want.Revision,
			)
		}
		if !digestPattern.MatchString(repository.ResultSHA256) {
			return fmt.Errorf(
				"repository %q has invalid result_sha256 %q",
				repository.ID,
				repository.ResultSHA256,
			)
		}
		for incompleteIndex, profile := range repository.IncompleteProfiles {
			if !slices.Contains(adjudicationProfiles, profile) ||
				incompleteIndex > 0 &&
					repository.IncompleteProfiles[incompleteIndex - 1] >=
						profile {
				return fmt.Errorf(
					"repository %q has invalid or unordered incomplete profile %q",
					repository.ID,
					profile,
				)
			}
		}
		for comparatorIndex, comparator := range repository.IncompleteComparators {
			wantComparators := []string{"go-vet", "staticcheck"}
			if !slices.Contains(wantComparators, comparator) ||
				comparatorIndex > 0 &&
					repository.IncompleteComparators[comparatorIndex - 1] >=
						comparator {
				return fmt.Errorf(
					"repository %q has invalid or unordered incomplete comparator %q",
					repository.ID,
					comparator,
				)
			}
		}
		if len(repository.Profiles) != len(adjudicationProfiles) {
			return fmt.Errorf(
				"repository %q profile count = %d, want %d",
				repository.ID,
				len(repository.Profiles),
				len(adjudicationProfiles),
			)
		}
		for profileIndex, profile := range repository.Profiles {
			if profile.Profile != adjudicationProfiles[profileIndex] {
				return fmt.Errorf(
					"repository %q profile %d = %q, want %q",
					repository.ID,
					profileIndex,
					profile.Profile,
					adjudicationProfiles[profileIndex],
				)
			}
			if err := validateFindingReferences(profile.Findings); err != nil {
				return fmt.Errorf(
					"repository %q profile %q: %w",
					repository.ID,
					profile.Profile,
					err,
				)
			}
		}
		repositoryIDs[repository.ID] = struct{}{}
	}
	if err := validateCorpusGaps(d.Gaps, repositoryIDs); err != nil {
		return err
	}
	for _, repository := range d.Repositories {
		if len(repository.IncompleteProfiles) == 0 &&
			len(repository.IncompleteComparators) == 0 {
			continue
		}
		hasGap := false
		for _, gap := range d.Gaps {
			if gap.Repository == repository.ID &&
				(gap.Kind == "crash" || gap.Kind == "unsupported-construct") {
				hasGap = true
				break
			}
		}
		if !hasGap {
			return fmt.Errorf(
				"repository %q incomplete profiles require a crash or unsupported-construct gap",
				repository.ID,
			)
		}
	}
	return nil
}

func validateFindingReferences(findings []findingAdjudication) error {
	for index, finding := range findings {
		if !digestPattern.MatchString(finding.Fingerprint) {
			return fmt.Errorf(
				"finding %d has invalid fingerprint %q",
				index,
				finding.Fingerprint,
			)
		}
		if index > 0 && findings[index - 1].Fingerprint >= finding.Fingerprint {
			return errors.New(
				"finding adjudications must be ordered by fingerprint without duplicates",
			)
		}
		switch finding.Classification {
		case "true-positive",
			"false-positive",
			"duplicate-vet",
			"duplicate-staticcheck",
			"unsupported-source",
			"unsupported-build",
			"unresolved":
		default:
			return fmt.Errorf(
				"finding %q has invalid classification %q",
				finding.Fingerprint,
				finding.Classification,
			)
		}
		if strings.TrimSpace(finding.Reason) == "" ||
			strings.TrimSpace(finding.Reason) != finding.Reason {
			return fmt.Errorf(
				"finding %q must have a canonical reason",
				finding.Fingerprint,
			)
		}
	}
	return nil
}

func validateCorpusGaps(gaps []corpusGap, repositories map[string]struct{}) error {
	for index, gap := range gaps {
		if !repositoryIDPattern.MatchString(gap.ID) {
			return fmt.Errorf("gap %d has invalid ID %q", index, gap.ID)
		}
		if index > 0 && gaps[index - 1].ID >= gap.ID {
			return errors.New("gaps must be ordered by ID without duplicates")
		}
		if _, found := repositories[gap.Repository]; !found {
			return fmt.Errorf(
				"gap %q names unknown repository %q",
				gap.ID,
				gap.Repository,
			)
		}
		if !slices.Contains([]string{"manual", "staticcheck", "vet"}, gap.Source) {
			return fmt.Errorf("gap %q has invalid source %q", gap.ID, gap.Source)
		}
		if !slices.Contains(
			[]string{"crash", "missed-defect", "unsupported-construct"},
			gap.Kind,
		) {
			return fmt.Errorf("gap %q has invalid kind %q", gap.ID, gap.Kind)
		}
		if !slices.Contains(
			[]string{"backlog", "not-actionable", "nursery"},
			gap.Disposition,
		) {
			return fmt.Errorf(
				"gap %q has invalid disposition %q",
				gap.ID,
				gap.Disposition,
			)
		}
		for name, value := range
			map[string]string{
				"summary": gap.Summary,
				"evidence": gap.Evidence,
				"reason": gap.Reason,
			} {
			if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
				return fmt.Errorf("gap %q must have a canonical %s", gap.ID, name)
			}
		}
		if gap.Disposition == "not-actionable" {
			if gap.RuleID != "" {
				return fmt.Errorf(
					"gap %q must not name a rule for not-actionable disposition",
					gap.ID,
				)
			}
		} else if !repositoryIDPattern.MatchString(gap.RuleID) {
			return fmt.Errorf("gap %q has invalid rule_id %q", gap.ID, gap.RuleID)
		}
	}
	return nil
}

type boundResultState struct {
	IncompleteComparators []string
	Identity evidenceIdentity
}

func inspectBoundResult(
	resultRoot string,
	repository Repository,
	staticcheckVersion, expectedResultDigest string,
) (boundResultState, error) {
	repositoryRoot := filepath.Join(resultRoot, repository.ID)
	input, err := readRegularFile(filepath.Join(repositoryRoot, "result.json"))
	if err != nil {
		return boundResultState{}, fmt.Errorf("read result for %q: %w", repository.ID, err)
	}
	digest := sha256.Sum256(input)
	if hex.EncodeToString(digest[:]) != expectedResultDigest {
		return boundResultState{}, fmt.Errorf(
			"result digest mismatch for %q",
			repository.ID,
		)
	}
	if err := validateJSONShape(input, repositoryResultShape, "result for " + repository.ID);
		err != nil {
		return boundResultState{}, err
	}
	var result repositoryResult
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return boundResultState{}, fmt.Errorf(
			"decode result for %q: %w",
			repository.ID,
			err,
		)
	}
	if err := requireJSONEOF(decoder, "result for " + repository.ID); err != nil {
		return boundResultState{}, err
	}
	if result.SchemaVersion != ResultSchemaVersion {
		return boundResultState{}, fmt.Errorf(
			"result schema_version for %q = %d, want %d",
			repository.ID,
			result.SchemaVersion,
			ResultSchemaVersion,
		)
	}
	if !reflect.DeepEqual(result.Repository, repository) {
		return boundResultState{}, fmt.Errorf(
			"result repository metadata does not match manifest for %q",
			repository.ID,
		)
	}
	if result.StaticcheckVersion != staticcheckVersion {
		return boundResultState{}, fmt.Errorf(
			"result staticcheck_version for %q = %q, want %q",
			repository.ID,
			result.StaticcheckVersion,
			staticcheckVersion,
		)
	}
	if !runIDPattern.MatchString(result.RunID) {
		return boundResultState{}, fmt.Errorf(
			"result for %q has invalid run_id %q",
			repository.ID,
			result.RunID,
		)
	}
	incomplete, err := validateBoundComparators(
		repositoryRoot,
		repository.ID,
		staticcheckVersion,
		result,
	)
	if err != nil {
		return boundResultState{}, err
	}
	return boundResultState{
		IncompleteComparators: incomplete,
		Identity: evidenceIdentity{
			ID: result.RunID,
			Glippy: result.Tools.Glippy,
			Go: result.Tools.Go,
			Staticcheck: result.Tools.Staticcheck,
		},
	}, nil
}

func loadBoundFindings(
	resultRoot string,
	repository Repository,
	staticcheckVersion string,
	expectedResultDigest string,
	profile string,
) ([]finding, bool, error) {
	repositoryRoot := filepath.Join(resultRoot, repository.ID)
	resultInput, err := readRegularFile(filepath.Join(repositoryRoot, "result.json"))
	if err != nil {
		return nil, false, fmt.Errorf("read result for %q: %w", repository.ID, err)
	}
	resultDigest := sha256.Sum256(resultInput)
	if hex.EncodeToString(resultDigest[:]) != expectedResultDigest {
		return nil, false, fmt.Errorf("result digest mismatch for %q", repository.ID)
	}
	if err := validateJSONShape(
		resultInput,
		repositoryResultShape,
		"result for " + repository.ID,
	);
		err != nil {
		return nil, false, err
	}
	var result repositoryResult
	decoder := json.NewDecoder(bytes.NewReader(resultInput))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return nil, false, fmt.Errorf("decode result for %q: %w", repository.ID, err)
	}
	if err := requireJSONEOF(decoder, "result for " + repository.ID); err != nil {
		return nil, false, err
	}
	if result.SchemaVersion != ResultSchemaVersion {
		return nil, false, fmt.Errorf(
			"result schema_version for %q = %d, want %d",
			repository.ID,
			result.SchemaVersion,
			ResultSchemaVersion,
		)
	}
	if !reflect.DeepEqual(result.Repository, repository) {
		return nil, false, fmt.Errorf(
			"result repository metadata does not match manifest for %q",
			repository.ID,
		)
	}
	if result.StaticcheckVersion != staticcheckVersion {
		return nil, false, fmt.Errorf(
			"result staticcheck_version for %q = %q, want %q",
			repository.ID,
			result.StaticcheckVersion,
			staticcheckVersion,
		)
	}
	if _, err := validateBoundComparators(
		repositoryRoot,
		repository.ID,
		staticcheckVersion,
		result,
	);
		err != nil {
		return nil, false, err
	}
	var selected profileResult
	found := false
	seenProfiles := make(map[string]struct{}, len(result.Profiles))
	for index, candidate := range result.Profiles {
		if _, duplicate := seenProfiles[candidate.Profile]; duplicate {
			return nil, false, fmt.Errorf(
				"result for %q contains duplicate profile %q",
				repository.ID,
				candidate.Profile,
			)
		}
		seenProfiles[candidate.Profile] = struct{}{}
		if index >= len(corpusProfiles) || candidate.Profile != corpusProfiles[index] {
			return nil, false, fmt.Errorf(
				"result for %q profile %d = %q, want canonical corpus profile",
				repository.ID,
				index,
				candidate.Profile,
			)
		}
		if candidate.Profile == profile {
			selected = candidate
			found = true
		}
	}
	if len(result.Profiles) != len(corpusProfiles) {
		return nil, false, fmt.Errorf(
			"result for %q has %d profiles, want %d",
			repository.ID,
			len(result.Profiles),
			len(corpusProfiles),
		)
	}
	if !found {
		return nil, false, fmt.Errorf(
			"result for %q is missing profile %q",
			repository.ID,
			profile,
		)
	}
	artifact := selected.Findings
	if artifact.File != "findings.json" ||
		!digestPattern.MatchString(artifact.SHA256) ||
		!artifact.ValidJSON {
		return nil, false, fmt.Errorf(
			"result for %q has invalid %s finding artifact",
			repository.ID,
			profile,
		)
	}
	input, err := readRegularFile(filepath.Join(repositoryRoot, profile, artifact.File))
	if err != nil {
		return nil, false, fmt.Errorf(
			"read %s findings for %q: %w",
			profile,
			repository.ID,
			err,
		)
	}
	digest := sha256.Sum256(input)
	if hex.EncodeToString(digest[:]) != artifact.SHA256 {
		return nil, false, fmt.Errorf(
			"%s finding digest mismatch for %q",
			profile,
			repository.ID,
		)
	}
	if err := validateJSONShape(
		input,
		findingInventoryShape,
		profile + " findings for " + repository.ID,
	);
		err != nil {
		return nil, false, err
	}
	var inventory findingInventory
	decoder = json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return nil, false, fmt.Errorf(
			"decode %s findings for %q: %w",
			profile,
			repository.ID,
			err,
		)
	}
	if err := requireJSONEOF(decoder, profile + " findings for " + repository.ID); err != nil {
		return nil, false, err
	}
	if inventory.SchemaVersion != ResultSchemaVersion {
		return nil, false, fmt.Errorf(
			"%s finding schema_version for %q = %d, want %d",
			profile,
			repository.ID,
			inventory.SchemaVersion,
			ResultSchemaVersion,
		)
	}
	if inventory.Repository != repository.ID ||
		inventory.Revision != repository.Revision ||
		inventory.Profile != profile {
		return nil, false, fmt.Errorf(
			"%s finding identity mismatch for %q",
			profile,
			repository.ID,
		)
	}
	seen := make(map[string]struct{}, len(inventory.Diagnostics))
	for index, value := range inventory.Diagnostics {
		if value.Fingerprint != findingFingerprint(value) {
			return nil, false, fmt.Errorf(
				"%s finding %d fingerprint mismatch for %q",
				profile,
				index,
				repository.ID,
			)
		}
		if _, duplicate := seen[value.Fingerprint]; duplicate {
			return nil, false, fmt.Errorf(
				"%s findings contain duplicate fingerprint for %q",
				profile,
				repository.ID,
			)
		}
		seen[value.Fingerprint] = struct{}{}
	}
	complete, err := validateBoundProfile(repositoryRoot, repository.ID, selected, inventory)
	if err != nil {
		return nil, false, err
	}
	return inventory.Diagnostics, complete, nil
}

func validateBoundComparators(
	repositoryRoot, repositoryID, staticcheckVersion string,
	result repositoryResult,
) ([]string, error) {
	for name, value := range
		map[string]string{
			"Glippy": result.Tools.Glippy,
			"Go": result.Tools.Go,
			"Staticcheck": result.Tools.Staticcheck,
		} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return nil, fmt.Errorf(
				"result for %q has invalid %s tool version",
				repositoryID,
				name,
			)
		}
	}
	if staticcheckModuleVersion(result.Tools.Staticcheck) != staticcheckVersion {
		return nil, fmt.Errorf(
			"result Staticcheck tool version for %q does not match %q",
			repositoryID,
			staticcheckVersion,
		)
	}
	wantNames := []string{"analysis-preflight", "go-vet", "staticcheck"}
	if len(result.Comparators) != len(wantNames) {
		return nil, fmt.Errorf(
			"result for %q has %d comparators, want %d",
			repositoryID,
			len(result.Comparators),
			len(wantNames),
		)
	}
	incomplete := make([]string, 0, len(result.Comparators))
	for index, comparator := range result.Comparators {
		wantName := wantNames[index]
		if comparator.Name != wantName {
			return nil, fmt.Errorf(
				"result for %q comparator %d = %q, want %q",
				repositoryID,
				index,
				comparator.Name,
				wantName,
			)
		}
		wantFile := wantName + ".txt"
		if comparator.Output.File != wantFile ||
			!digestPattern.MatchString(comparator.Output.SHA256) ||
			comparator.Output.ValidJSON {
			return nil, fmt.Errorf(
				"result for %q has invalid %s comparator artifact",
				repositoryID,
				wantName,
			)
		}
		input, err := readRegularFile(filepath.Join(repositoryRoot, wantFile))
		if err != nil {
			return nil, fmt.Errorf(
				"read %s comparator for %q: %w",
				wantName,
				repositoryID,
				err,
			)
		}
		digest := sha256.Sum256(input)
		if hex.EncodeToString(digest[:]) != comparator.Output.SHA256 {
			return nil, fmt.Errorf(
				"%s comparator digest mismatch for %q",
				wantName,
				repositoryID,
			)
		}
	}
	preflightComplete := result.Comparators[0].ExitCode == 0
	for _, comparator := range result.Comparators[1:] {
		if !preflightComplete || comparator.ExitCode != 0 && comparator.ExitCode != 1 {
			incomplete = append(incomplete, comparator.Name)
		}
	}
	return incomplete, nil
}

func validateBoundProfile(
	repositoryRoot, repositoryID string,
	profile profileResult,
	inventory findingInventory,
) (bool, error) {
	artifact := profile.Diagnostics
	wantFile := "diagnostics.txt"
	if artifact.ValidJSON {
		wantFile = "diagnostics.json"
	}
	if artifact.File != wantFile || !digestPattern.MatchString(artifact.SHA256) {
		return false, fmt.Errorf(
			"result for %q has invalid %s diagnostic artifact",
			repositoryID,
			profile.Profile,
		)
	}
	input, err := readRegularFile(filepath.Join(repositoryRoot, profile.Profile, artifact.File))
	if err != nil {
		return false, fmt.Errorf(
			"read %s diagnostics for %q: %w",
			profile.Profile,
			repositoryID,
			err,
		)
	}
	digest := sha256.Sum256(input)
	if hex.EncodeToString(digest[:]) != artifact.SHA256 {
		return false, fmt.Errorf(
			"%s diagnostic digest mismatch for %q",
			profile.Profile,
			repositoryID,
		)
	}
	if !profile.Complete {
		return false, nil
	}
	if !artifact.ValidJSON {
		return false, fmt.Errorf(
			"result for %q has invalid %s diagnostic artifact",
			repositoryID,
			profile.Profile,
		)
	}
	if profile.DiagnosticCount != len(inventory.Diagnostics) {
		return false, fmt.Errorf(
			"repository %q profile %q diagnostic_count = %d, want %d",
			repositoryID,
			profile.Profile,
			profile.DiagnosticCount,
			len(inventory.Diagnostics),
		)
	}
	wantExitCode := 0
	if profile.DiagnosticCount != 0 {
		wantExitCode = 1
	}
	if profile.ExitCode != wantExitCode {
		return false, fmt.Errorf(
			"repository %q profile %q exit_code = %d, want %d",
			repositoryID,
			profile.Profile,
			profile.ExitCode,
			wantExitCode,
		)
	}
	var diagnostics normalizedLintResult
	decoder := json.NewDecoder(bytes.NewReader(input))
	if err := decoder.Decode(&diagnostics); err != nil {
		return false, fmt.Errorf(
			"decode %s diagnostics for %q: %w",
			profile.Profile,
			repositoryID,
			err,
		)
	}
	if err := requireJSONEOF(decoder, profile.Profile + " diagnostics for " + repositoryID);
		err != nil {
		return false, err
	}
	if !diagnostics.Summary.Complete ||
		diagnostics.Summary.Diagnostics != profile.DiagnosticCount ||
		len(diagnostics.Diagnostics) != profile.DiagnosticCount {
		return false, fmt.Errorf(
			"%s diagnostic summary mismatch for %q",
			profile.Profile,
			repositoryID,
		)
	}
	normalizedFindings := slices.Clone(diagnostics.Diagnostics)
	for index := range normalizedFindings {
		normalizedFindings[index].Fingerprint = findingFingerprint(
			normalizedFindings[index],
		)
	}
	sortFindings(normalizedFindings)
	for index := range normalizedFindings {
		if normalizedFindings[index].Fingerprint !=
			inventory.Diagnostics[index].Fingerprint {
			return false, fmt.Errorf(
				"%s diagnostic inventory mismatch for %q",
				profile.Profile,
				repositoryID,
			)
		}
	}
	return true, nil
}

func validateFindingAdjudications(profile profileAdjudication, findings []finding) (int, error) {
	expected := make(map[string]struct{}, len(findings))
	for _, value := range findings {
		expected[value.Fingerprint] = struct{}{}
	}
	unresolved := 0
	for _, adjudication := range profile.Findings {
		if _, found := expected[adjudication.Fingerprint]; !found {
			return 0, fmt.Errorf("unknown finding %q", adjudication.Fingerprint)
		}
		delete(expected, adjudication.Fingerprint)
		if adjudication.Classification == "unresolved" {
			unresolved++
		}
	}
	if len(expected) != 0 {
		return 0, fmt.Errorf("missing %d finding adjudication(s)", len(expected))
	}
	return unresolved, nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode() & os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	return os.ReadFile(path)
}

func validateJSONShape(input []byte, shape *jsonShape, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(input))
	if err := validateJSONValue(decoder, shape, label); err != nil {
		return err
	}
	return requireJSONEOF(decoder, label)
}

func validateJSONValue(decoder *json.Decoder, shape *jsonShape, label string) error {
	switch {
	case shape.Fields != nil:
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode %s: %w", label, err)
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
			return fmt.Errorf("decode %s: want JSON object", label)
		}
		seen := make(map[string]struct{}, len(shape.Fields))
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("decode %s field: %w", label, err)
			}
			name, ok := token.(string)
			if !ok {
				return fmt.Errorf("decode %s field: want string name", label)
			}
			child, found := shape.Fields[name]
			if !found {
				return fmt.Errorf("decode %s: unknown field %q", label, name)
			}
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("decode %s: duplicate field %q", label, name)
			}
			seen[name] = struct{}{}
			if err := validateJSONValue(decoder, child, label + "." + name);
				err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return fmt.Errorf("decode %s: %w", label, err)
		}
		for name := range shape.Fields {
			if _, optional := shape.Optional[name]; optional {
				continue
			}
			if _, found := seen[name]; !found {
				return fmt.Errorf(
					"decode %s: missing required field %q",
					label,
					name,
				)
			}
		}
		return nil
	case shape.Element != nil:
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode %s: %w", label, err)
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
			return fmt.Errorf("decode %s: want JSON array", label)
		}
		index := 0
		for decoder.More() {
			if err := validateJSONValue(
				decoder,
				shape.Element,
				fmt.Sprintf("%s[%d]", label, index),
			);
				err != nil {
				return err
			}
			index++
		}
		if _, err := decoder.Token(); err != nil {
			return fmt.Errorf("decode %s: %w", label, err)
		}
		return nil
	default:
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode %s: %w", label, err)
		}
		trimmed := bytes.TrimSpace(value)
		matches := shape.Scalar == anyScalar
		if len(trimmed) != 0 {
			switch shape.Scalar {
			case stringScalar:
				matches = trimmed[0] == '"'
			case numberScalar:
				matches = trimmed[0] == '-' ||
					trimmed[0] >= '0' && trimmed[0] <= '9'
			case boolScalar:
				matches = bytes.Equal(trimmed, []byte("true")) ||
					bytes.Equal(trimmed, []byte("false"))
			}
		}
		if !matches {
			kind := map[scalarKind]string{
				stringScalar: "string",
				numberScalar: "number",
				boolScalar: "boolean",
			}[shape.Scalar]
			return fmt.Errorf("decode %s: want JSON %s", label, kind)
		}
		return nil
	}
}

func requireJSONEOF(decoder *json.Decoder, label string) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: multiple JSON values", label)
		}
		return fmt.Errorf("decode %s trailing data: %w", label, err)
	}
	return nil
}
