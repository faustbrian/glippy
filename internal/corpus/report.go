package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// AdjudicationReportSchemaVersion identifies the deterministic corpus report
// contract. The report summarizes one exact, validated corpus run without
// replacing the bound result and adjudication artifacts.
const AdjudicationReportSchemaVersion = 3

var findingClassifications = []string{
	"true-positive",
	"intentional",
	"false-positive",
	"duplicate-vet",
	"duplicate-staticcheck",
	"unsupported-source",
	"unsupported-build",
	"unresolved",
}

var (
	recordedStatisticsMetricShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"calls": jsonNumber,
			"duration_ns": jsonNumber,
			"allocations": jsonNumber,
			"allocated_bytes": jsonNumber,
		},
	}
	recordedStatisticsShape = &jsonShape{
		Fields: map[string]*jsonShape{
			"schema_version": jsonNumber,
			"command": jsonString,
			"outcome": {
				Fields: map[string]*jsonShape{
					"category": jsonString,
					"exit_code": jsonNumber,
				},
			},
			"complete": jsonBool,
			"maximum_tier": jsonString,
			"packages": jsonNumber,
			"files": jsonNumber,
			"loaded_files": jsonNumber,
			"total": recordedStatisticsMetricShape,
			"phases": {
				Element: &jsonShape{
					Fields: map[string]*jsonShape{
						"name": jsonString,
						"metric": recordedStatisticsMetricShape,
					},
				},
			},
			"tiers": {
				Element: &jsonShape{
					Fields: map[string]*jsonShape{
						"tier": jsonString,
						"reasons": {Element: jsonString},
						"metric": recordedStatisticsMetricShape,
					},
				},
			},
			"rules": {
				Element: &jsonShape{
					Fields: map[string]*jsonShape{
						"id": jsonString,
						"tier": jsonString,
						"calls": jsonNumber,
						"duration_ns": jsonNumber,
						"allocations": jsonNumber,
						"allocated_bytes": jsonNumber,
						"findings": jsonNumber,
						"diagnostics": jsonNumber,
						"preexisting": jsonNumber,
						"suppressed": jsonNumber,
						"baselined": jsonNumber,
					},
				},
			},
			"cache": {
				Fields: map[string]*jsonShape{
					"lookups": jsonNumber,
					"hits": jsonNumber,
					"misses": jsonNumber,
					"invalidations": jsonNumber,
					"writes": jsonNumber,
				},
			},
			"dependency_syntax": {
				Fields: map[string]*jsonShape{
					"loaded": jsonBool,
					"reasons": {Element: jsonString},
				},
			},
			"effect_facts": {
				Fields: map[string]*jsonShape{
					"loaded": jsonBool,
					"reasons": {Element: jsonString},
				},
			},
		},
	}
)

type adjudicationReport struct {
	SchemaVersion int `json:"schema_version"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Run evidenceIdentity `json:"run"`
	Summary adjudicationReportSummary `json:"summary"`
	Formatter formatterReport `json:"formatter"`
	FixPreview fixPreviewReport `json:"fix_preview"`
	Classifications []classificationCount `json:"classifications"`
	GapCounts []gapCount `json:"gap_counts"`
	Profiles []profileReport `json:"profiles"`
	Measurements []profileMeasurement `json:"measurements"`
	RuleQueue []ruleQueueEntry `json:"rule_queue"`
}

type formatterReport struct {
	Repositories int `json:"repositories"`
	CompleteRepositories int `json:"complete_repositories"`
	Files int `json:"files"`
	Differences int `json:"differences"`
}

type fixPreviewReport struct {
	Repositories int `json:"repositories"`
	CompleteRepositories int `json:"complete_repositories"`
}

type adjudicationReportSummary struct {
	Repositories int `json:"repositories"`
	Findings int `json:"findings"`
	Gaps int `json:"gaps"`
	Unresolved int `json:"unresolved"`
}

type classificationCount struct {
	Profile string `json:"profile"`
	Classification string `json:"classification"`
	Count int `json:"count"`
}

type gapCount struct {
	Kind string `json:"kind"`
	Disposition string `json:"disposition"`
	Count int `json:"count"`
}

type profileReport struct {
	Profile string `json:"profile"`
	Repositories int `json:"repositories"`
	CompleteRepositories int `json:"complete_repositories"`
	MeasuredRepositories int `json:"measured_repositories"`
	Findings int `json:"findings"`
	Packages int `json:"packages"`
	Files int `json:"files"`
	LoadedFiles int `json:"loaded_files"`
	DurationNanoseconds int64 `json:"duration_ns"`
	Allocations uint64 `json:"allocations"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
}

type profileMeasurement struct {
	Repository string `json:"repository"`
	Profile string `json:"profile"`
	StatisticsSHA256 string `json:"statistics_sha256"`
	Complete bool `json:"complete"`
	Measured bool `json:"measured"`
	Findings int `json:"findings"`
	Packages int `json:"packages"`
	Files int `json:"files"`
	LoadedFiles int `json:"loaded_files"`
	DurationNanoseconds int64 `json:"duration_ns"`
	Allocations uint64 `json:"allocations"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
}

type ruleQueueEntry struct {
	RuleID string `json:"rule_id"`
	Disposition string `json:"disposition"`
	Evidence []ruleQueueEvidence `json:"evidence"`
}

type ruleQueueEvidence struct {
	GapID string `json:"gap_id"`
	Repository string `json:"repository"`
	Source string `json:"source"`
	Kind string `json:"kind"`
	Summary string `json:"summary"`
	Evidence string `json:"evidence"`
	Reason string `json:"reason"`
}

type recordedStatistics struct {
	SchemaVersion int `json:"schema_version"`
	Command string `json:"command"`
	Complete bool `json:"complete"`
	Packages int `json:"packages"`
	Files int `json:"files"`
	LoadedFiles int `json:"loaded_files"`
	Total recordedStatisticsMetric `json:"total"`
}

type recordedStatisticsMetric struct {
	Calls uint64 `json:"calls"`
	DurationNanoseconds int64 `json:"duration_ns"`
	Allocations uint64 `json:"allocations"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
}

// BuildAdjudicationReport validates one adjudication against its exact result
// artifacts and produces a canonical report of signal, gaps, cost, and queued
// rule candidates.
func BuildAdjudicationReport(
	manifest Manifest,
	manifestInput []byte,
	resultRoot string,
	adjudicationInput []byte,
) ([]byte, error) {
	summary, err := ValidateAdjudication(manifest, manifestInput, resultRoot, adjudicationInput)
	if err != nil {
		return nil, err
	}
	document, err := decodeAdjudicationDocument(adjudicationInput)
	if err != nil {
		return nil, err
	}
	report := adjudicationReport{
		SchemaVersion: AdjudicationReportSchemaVersion,
		ManifestSHA256: document.ManifestSHA256,
		Run: document.Run,
		Summary: adjudicationReportSummary{
			Repositories: summary.Repositories,
			Findings: summary.Findings,
			Gaps: summary.Gaps,
			Unresolved: summary.Unresolved,
		},
		Classifications: buildClassificationCounts(document),
		GapCounts: buildGapCounts(document.Gaps),
		Profiles: make([]profileReport, len(corpusProfiles)),
		Measurements: make(
			[]profileMeasurement,
			0,
			len(manifest.Repositories) * len(corpusProfiles),
		),
		RuleQueue: buildRuleQueue(document.Gaps),
	}
	profileIndexes := make(map[string]int, len(corpusProfiles))
	for index, profile := range corpusProfiles {
		profileIndexes[profile] = index
		report.Profiles[index].Profile = profile
	}
	for repositoryIndex, repository := range manifest.Repositories {
		adjudicationRepository := document.Repositories[repositoryIndex]
		formatMeasurement, err := loadFormatMeasurement(
			resultRoot,
			repository,
			adjudicationRepository.ResultSHA256,
		)
		if err != nil {
			return nil, err
		}
		if err := addFormatMeasurement(&report.Formatter, formatMeasurement); err != nil {
			return nil, fmt.Errorf("aggregate formatter: %w", err)
		}
		fixPreviewMeasurement, err := loadFixPreviewMeasurement(
			resultRoot,
			repository,
			adjudicationRepository.ResultSHA256,
		)
		if err != nil {
			return nil, err
		}
		if err := addFixPreviewMeasurement(&report.FixPreview, fixPreviewMeasurement);
			err != nil {
			return nil, fmt.Errorf("aggregate safe-fix preview: %w", err)
		}
		for profileIndex, profile := range corpusProfiles {
			measurement, err := loadProfileMeasurement(
				resultRoot,
				repository,
				adjudicationRepository.ResultSHA256,
				adjudicationRepository.Measurements[profileIndex].StatisticsSHA256,
				profile,
			)
			if err != nil {
				return nil, err
			}
			report.Measurements = append(report.Measurements, measurement)
			aggregate := &report.Profiles[profileIndexes[profile]]
			if err := addProfileMeasurement(aggregate, measurement); err != nil {
				return nil, fmt.Errorf("aggregate profile %q: %w", profile, err)
			}
		}
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode corpus adjudication report: %w", err)
	}
	return append(encoded, '\n'), nil
}

func loadFixPreviewMeasurement(
	resultRoot string,
	repository Repository,
	expectedResultDigest string,
) (fixPreviewResult, error) {
	resultInput, err := readRegularFile(filepath.Join(resultRoot, repository.ID, "result.json"))
	if err != nil {
		return fixPreviewResult{}, fmt.Errorf("read result for %q: %w", repository.ID, err)
	}
	digest := sha256.Sum256(resultInput)
	if hex.EncodeToString(digest[:]) != expectedResultDigest {
		return fixPreviewResult{}, fmt.Errorf(
			"result digest mismatch for %q",
			repository.ID,
		)
	}
	var result repositoryResult
	decoder := json.NewDecoder(bytes.NewReader(resultInput))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return fixPreviewResult{}, fmt.Errorf(
			"decode result for %q: %w",
			repository.ID,
			err,
		)
	}
	if err := requireJSONEOF(decoder, "result for " + repository.ID); err != nil {
		return fixPreviewResult{}, err
	}
	if result.SchemaVersion != ResultSchemaVersion ||
		!reflect.DeepEqual(result.Repository, repository) {
		return fixPreviewResult{}, fmt.Errorf(
			"result identity does not match manifest for %q",
			repository.ID,
		)
	}
	return result.FixPreview, nil
}

func loadFormatMeasurement(
	resultRoot string,
	repository Repository,
	expectedResultDigest string,
) (formatResult, error) {
	resultInput, err := readRegularFile(filepath.Join(resultRoot, repository.ID, "result.json"))
	if err != nil {
		return formatResult{}, fmt.Errorf("read result for %q: %w", repository.ID, err)
	}
	digest := sha256.Sum256(resultInput)
	if hex.EncodeToString(digest[:]) != expectedResultDigest {
		return formatResult{}, fmt.Errorf("result digest mismatch for %q", repository.ID)
	}
	var result repositoryResult
	decoder := json.NewDecoder(bytes.NewReader(resultInput))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return formatResult{}, fmt.Errorf("decode result for %q: %w", repository.ID, err)
	}
	if err := requireJSONEOF(decoder, "result for " + repository.ID); err != nil {
		return formatResult{}, err
	}
	if result.SchemaVersion != ResultSchemaVersion ||
		!reflect.DeepEqual(result.Repository, repository) {
		return formatResult{}, fmt.Errorf(
			"result identity does not match manifest for %q",
			repository.ID,
		)
	}
	return result.Format, nil
}

func addFormatMeasurement(target *formatterReport, value formatResult) error {
	var ok bool
	target.Repositories, ok = checkedAddInt(target.Repositories, 1)
	if !ok {
		return fmt.Errorf("repository count overflow")
	}
	if value.Complete {
		target.CompleteRepositories, ok = checkedAddInt(target.CompleteRepositories, 1)
		if !ok {
			return fmt.Errorf("complete repository count overflow")
		}
	}
	target.Files, ok = checkedAddInt(target.Files, value.FileCount)
	if !ok {
		return fmt.Errorf("file count overflow")
	}
	target.Differences, ok = checkedAddInt(target.Differences, value.DifferenceCount)
	if !ok {
		return fmt.Errorf("difference count overflow")
	}
	return nil
}

func addFixPreviewMeasurement(target *fixPreviewReport, value fixPreviewResult) error {
	var ok bool
	target.Repositories, ok = checkedAddInt(target.Repositories, 1)
	if !ok {
		return fmt.Errorf("repository count overflow")
	}
	if value.Complete {
		target.CompleteRepositories, ok = checkedAddInt(target.CompleteRepositories, 1)
		if !ok {
			return fmt.Errorf("complete repository count overflow")
		}
	}
	return nil
}

func addProfileMeasurement(target *profileReport, value profileMeasurement) error {
	repositories, ok := checkedAddInt(target.Repositories, 1)
	if !ok {
		return fmt.Errorf("repository count overflow")
	}
	completeRepositories := target.CompleteRepositories
	if value.Complete {
		completeRepositories, ok = checkedAddInt(completeRepositories, 1)
		if !ok {
			return fmt.Errorf("complete repository count overflow")
		}
	}
	measuredRepositories := target.MeasuredRepositories
	if value.Measured {
		measuredRepositories, ok = checkedAddInt(measuredRepositories, 1)
		if !ok {
			return fmt.Errorf("measured repository count overflow")
		}
	}
	findings, ok := checkedAddInt(target.Findings, value.Findings)
	if !ok {
		return fmt.Errorf("findings overflow")
	}
	packages, ok := checkedAddInt(target.Packages, value.Packages)
	if !ok {
		return fmt.Errorf("packages overflow")
	}
	files, ok := checkedAddInt(target.Files, value.Files)
	if !ok {
		return fmt.Errorf("files overflow")
	}
	loadedFiles, ok := checkedAddInt(target.LoadedFiles, value.LoadedFiles)
	if !ok {
		return fmt.Errorf("loaded files overflow")
	}
	duration, ok := checkedAddInt64(target.DurationNanoseconds, value.DurationNanoseconds)
	if !ok {
		return fmt.Errorf("duration overflow")
	}
	allocations, ok := checkedAddUint64(target.Allocations, value.Allocations)
	if !ok {
		return fmt.Errorf("allocations overflow")
	}
	allocatedBytes, ok := checkedAddUint64(target.AllocatedBytes, value.AllocatedBytes)
	if !ok {
		return fmt.Errorf("allocated bytes overflow")
	}
	target.Repositories = repositories
	target.CompleteRepositories = completeRepositories
	target.MeasuredRepositories = measuredRepositories
	target.Findings = findings
	target.Packages = packages
	target.Files = files
	target.LoadedFiles = loadedFiles
	target.DurationNanoseconds = duration
	target.Allocations = allocations
	target.AllocatedBytes = allocatedBytes
	return nil
}

func checkedAddInt(left, right int) (int, bool) {
	maximum := int(^uint(0) >> 1)
	if left < 0 || right < 0 || left > maximum - right {
		return 0, false
	}
	return left + right, true
}

func checkedAddInt64(left, right int64) (int64, bool) {
	maximum := int64(^uint64(0) >> 1)
	if left < 0 || right < 0 || left > maximum - right {
		return 0, false
	}
	return left + right, true
}

func checkedAddUint64(left, right uint64) (uint64, bool) {
	maximum := ^uint64(0)
	if left > maximum - right {
		return 0, false
	}
	return left + right, true
}

func decodeAdjudicationDocument(input []byte) (adjudicationDocument, error) {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var document adjudicationDocument
	if err := decoder.Decode(&document); err != nil {
		return adjudicationDocument{}, fmt.Errorf("decode corpus adjudication: %w", err)
	}
	if err := requireJSONEOF(decoder, "corpus adjudication"); err != nil {
		return adjudicationDocument{}, err
	}
	return document, nil
}

func buildClassificationCounts(document adjudicationDocument) []classificationCount {
	counts := make(map[string]int)
	for _, repository := range document.Repositories {
		for _, profile := range repository.Profiles {
			for _, finding := range profile.Findings {
				counts[profile.Profile + "\x00" + finding.Classification]++
			}
		}
	}
	result := make([]classificationCount, 0, len(counts))
	for _, profile := range adjudicationProfiles {
		for _, classification := range findingClassifications {
			count := counts[profile + "\x00" + classification]
			if count == 0 {
				continue
			}
			result = append(
				result,
				classificationCount{
					Profile: profile,
					Classification: classification,
					Count: count,
				},
			)
		}
	}
	return result
}

func buildGapCounts(gaps []corpusGap) []gapCount {
	counts := make(map[string]int)
	for _, gap := range gaps {
		counts[gap.Kind + "\x00" + gap.Disposition]++
	}
	result := make([]gapCount, 0, len(counts))
	for _, kind := range []string{"crash", "missed-defect", "unsupported-construct"} {
		for _, disposition := range []string{"backlog", "nursery", "not-actionable"} {
			count := counts[kind + "\x00" + disposition]
			if count == 0 {
				continue
			}
			result = append(
				result,
				gapCount{Kind: kind, Disposition: disposition, Count: count},
			)
		}
	}
	return result
}

func buildRuleQueue(gaps []corpusGap) []ruleQueueEntry {
	entries := make(map[string]*ruleQueueEntry)
	for _, gap := range gaps {
		if gap.Disposition == "not-actionable" {
			continue
		}
		key := gap.RuleID + "\x00" + gap.Disposition
		entry := entries[key]
		if entry == nil {
			entry = &ruleQueueEntry{
				RuleID: gap.RuleID,
				Disposition: gap.Disposition,
				Evidence: []ruleQueueEvidence{},
			}
			entries[key] = entry
		}
		entry.Evidence = append(
			entry.Evidence,
			ruleQueueEvidence{
				GapID: gap.ID,
				Repository: gap.Repository,
				Source: gap.Source,
				Kind: gap.Kind,
				Summary: gap.Summary,
				Evidence: gap.Evidence,
				Reason: gap.Reason,
			},
		)
	}
	result := make([]ruleQueueEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, *entry)
	}
	slices.SortFunc(
		result,
		func(left, right ruleQueueEntry) int {
			if compared := strings.Compare(left.RuleID, right.RuleID); compared != 0 {
				return compared
			}
			return strings.Compare(left.Disposition, right.Disposition)
		},
	)
	return result
}

func loadProfileMeasurement(
	resultRoot string,
	repository Repository,
	expectedResultDigest, expectedStatisticsDigest, profile string,
) (profileMeasurement, error) {
	artifact, err := loadBoundStatisticsArtifact(
		resultRoot,
		repository,
		expectedResultDigest,
		profile,
	)
	if err != nil {
		return profileMeasurement{}, err
	}
	if artifact.SHA256 != expectedStatisticsDigest {
		return profileMeasurement{}, fmt.Errorf(
			"%s statistics digest mismatch for %q",
			profile,
			repository.ID,
		)
	}
	if !artifact.Profile.Statistics.ValidJSON {
		if !artifact.Profile.Complete {
			return profileMeasurement{
				Repository: repository.ID,
				Profile: profile,
				StatisticsSHA256: artifact.SHA256,
				Complete: false,
				Measured: false,
				Findings: artifact.Profile.DiagnosticCount,
			}, nil
		}
		return profileMeasurement{}, fmt.Errorf(
			"result for %q has invalid %s statistics artifact",
			repository.ID,
			profile,
		)
	}
	if err := validateJSONShape(
		artifact.Input,
		recordedStatisticsShape,
		profile + " statistics for " + repository.ID,
	);
		err != nil {
		return profileMeasurement{}, fmt.Errorf(
			"invalid statistics for %q profile %q: %w",
			repository.ID,
			profile,
			err,
		)
	}
	var statistics recordedStatistics
	decoder := json.NewDecoder(bytes.NewReader(artifact.Input))
	if err := decoder.Decode(&statistics); err != nil {
		return profileMeasurement{}, fmt.Errorf(
			"decode invalid statistics for %q profile %q: %w",
			repository.ID,
			profile,
			err,
		)
	}
	if err := requireJSONEOF(decoder, profile + " statistics for " + repository.ID);
		err != nil {
		return profileMeasurement{}, err
	}
	if statistics.SchemaVersion != 1 ||
		statistics.Command != "lint" ||
		statistics.Complete != artifact.Profile.Complete ||
		artifact.Profile.DiagnosticCount < 0 ||
		statistics.Packages < 0 ||
		statistics.Files < 0 ||
		statistics.LoadedFiles < 0 ||
		statistics.Total.DurationNanoseconds < 0 {
		return profileMeasurement{}, fmt.Errorf(
			"invalid statistics for %q profile %q",
			repository.ID,
			profile,
		)
	}
	return profileMeasurement{
		Repository: repository.ID,
		Profile: profile,
		StatisticsSHA256: artifact.SHA256,
		Complete: statistics.Complete,
		Measured: true,
		Findings: artifact.Profile.DiagnosticCount,
		Packages: statistics.Packages,
		Files: statistics.Files,
		LoadedFiles: statistics.LoadedFiles,
		DurationNanoseconds: statistics.Total.DurationNanoseconds,
		Allocations: statistics.Total.Allocations,
		AllocatedBytes: statistics.Total.AllocatedBytes,
	}, nil
}

type boundStatisticsArtifact struct {
	Profile profileResult
	Input []byte
	SHA256 string
}

func loadBoundStatisticsDigest(
	resultRoot string,
	repository Repository,
	expectedResultDigest, profile string,
) (string, error) {
	artifact, err := loadBoundStatisticsArtifact(
		resultRoot,
		repository,
		expectedResultDigest,
		profile,
	)
	if err != nil {
		return "", err
	}
	return artifact.SHA256, nil
}

func loadBoundStatisticsArtifact(
	resultRoot string,
	repository Repository,
	expectedResultDigest, profile string,
) (boundStatisticsArtifact, error) {
	repositoryRoot := filepath.Join(resultRoot, repository.ID)
	resultInput, err := readRegularFile(filepath.Join(repositoryRoot, "result.json"))
	if err != nil {
		return boundStatisticsArtifact{}, fmt.Errorf(
			"read result for %q: %w",
			repository.ID,
			err,
		)
	}
	resultDigest := sha256.Sum256(resultInput)
	if hex.EncodeToString(resultDigest[:]) != expectedResultDigest {
		return boundStatisticsArtifact{}, fmt.Errorf(
			"result digest mismatch for %q",
			repository.ID,
		)
	}
	if err := validateJSONShape(
		resultInput,
		repositoryResultShape,
		"result for " + repository.ID,
	);
		err != nil {
		return boundStatisticsArtifact{}, err
	}
	var result repositoryResult
	decoder := json.NewDecoder(bytes.NewReader(resultInput))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return boundStatisticsArtifact{}, fmt.Errorf(
			"decode result for %q: %w",
			repository.ID,
			err,
		)
	}
	if err := requireJSONEOF(decoder, "result for " + repository.ID); err != nil {
		return boundStatisticsArtifact{}, err
	}
	if result.SchemaVersion != ResultSchemaVersion ||
		!reflect.DeepEqual(result.Repository, repository) {
		return boundStatisticsArtifact{}, fmt.Errorf(
			"result identity does not match manifest for %q",
			repository.ID,
		)
	}
	var selected profileResult
	found := false
	seenProfiles := make(map[string]struct{}, len(result.Profiles))
	for index, candidate := range result.Profiles {
		if _, duplicate := seenProfiles[candidate.Profile]; duplicate {
			return boundStatisticsArtifact{}, fmt.Errorf(
				"result for %q contains duplicate profile %q",
				repository.ID,
				candidate.Profile,
			)
		}
		seenProfiles[candidate.Profile] = struct{}{}
		if index >= len(corpusProfiles) || candidate.Profile != corpusProfiles[index] {
			return boundStatisticsArtifact{}, fmt.Errorf(
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
		return boundStatisticsArtifact{}, fmt.Errorf(
			"result for %q has %d profiles, want %d",
			repository.ID,
			len(result.Profiles),
			len(corpusProfiles),
		)
	}
	if !found {
		return boundStatisticsArtifact{}, fmt.Errorf(
			"result for %q is missing profile %q",
			repository.ID,
			profile,
		)
	}
	wantFile := "statistics.txt"
	if selected.Statistics.ValidJSON {
		wantFile = "statistics.json"
	}
	if selected.Statistics.File != wantFile {
		return boundStatisticsArtifact{}, fmt.Errorf(
			"result for %q has invalid %s statistics artifact",
			repository.ID,
			profile,
		)
	}
	statisticsInput, err := readRegularFile(
		filepath.Join(repositoryRoot, profile, selected.Statistics.File),
	)
	if err != nil {
		return boundStatisticsArtifact{}, fmt.Errorf(
			"read %s statistics for %q: %w",
			profile,
			repository.ID,
			err,
		)
	}
	statisticsDigest := sha256.Sum256(statisticsInput)
	return boundStatisticsArtifact{
		Profile: selected,
		Input: statisticsInput,
		SHA256: hex.EncodeToString(statisticsDigest[:]),
	}, nil
}
