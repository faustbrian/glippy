package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/corpus"
)

func TestRunValidatesThePinnedManifestWithoutExecutingRepositories(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{
			"--manifest",
			filepath.Join("..", "..", "corpus", "manifest.json"),
			"--validate-only",
		},
		&stdout,
		&stderr,
	)
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("run() = exit %d, stderr %q", exitCode, stderr.String())
	}
	if stdout.String() != "valid corpus manifest: 17 repositories\n" {
		t.Fatalf("run() stdout = %q", stdout.String())
	}
}

func TestRunRejectsMissingExecutionInputs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"--manifest", filepath.Join("..", "..", "corpus", "manifest.json")},
		&stdout,
		&stderr,
	)
	if exitCode != 2 ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "--checkouts is required") {
		t.Fatalf(
			"run() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunRejectsAdjudicationModesWithoutResultArtifacts(t *testing.T) {
	t.Parallel()

	for _, arguments := range
		[][]string{
			{
				"--manifest",
				filepath.Join("..", "..", "corpus", "manifest.json"),
				"--adjudication-template",
			},
			{
				"--manifest",
				filepath.Join("..", "..", "corpus", "manifest.json"),
				"--adjudication",
				"review.json",
			},
			{
				"--manifest",
				filepath.Join("..", "..", "corpus", "manifest.json"),
				"--adjudication-report",
				"review.json",
			},
		} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := run(context.Background(), arguments, &stdout, &stderr)
		if exitCode != 2 ||
			stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), "--results is required") {
			t.Fatalf(
				"run(%v) = exit %d, stdout %q, stderr %q",
				arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestRunRejectsConflictingCorpusModes(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "corpus", "manifest.json")
	for _, test := range
		[]struct {
			arguments []string
			want string
		}{
			{
				arguments: []string{
					"--manifest",
					manifestPath,
					"--results",
					"results",
					"--validate-only",
					"--adjudication-template",
				},
				want: "--validate-only cannot be combined",
			},
			{
				arguments: []string{
					"--manifest",
					manifestPath,
					"--results",
					"results",
					"--adjudication-template",
					"--adjudication",
					"review.json",
				},
				want: "mutually exclusive",
			},
			{
				arguments: []string{
					"--manifest",
					manifestPath,
					"--results",
					"results",
					"--adjudication",
					"review.json",
					"--adjudication-report",
					"review.json",
				},
				want: "mutually exclusive",
			},
			{
				arguments: []string{
					"--manifest",
					manifestPath,
					"--results",
					"results",
					"--adjudication-template",
					"--output",
					"output",
				},
				want: "cannot be combined with execution inputs",
			},
			{
				arguments: []string{
					"--manifest",
					manifestPath,
					"--results",
					"results",
				},
				want: "--results requires an adjudication mode",
			},
		} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := run(context.Background(), test.arguments, &stdout, &stderr)
		if exitCode != 2 ||
			stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), test.want) {
			t.Fatalf(
				"run(%v) = exit %d, stdout %q, stderr %q; want %q",
				test.arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
				test.want,
			)
		}
	}
}

func TestRunBuildsAndValidatesAdjudicationFromBoundResults(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "corpus", "manifest.json")
	manifestInput, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	resultsRoot := filepath.Join(t.TempDir(), "results")
	writeEmptyResultArtifacts(t, manifest, resultsRoot, false)

	var template bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{
			"--manifest",
			manifestPath,
			"--results",
			resultsRoot,
			"--adjudication-template",
		},
		&template,
		&stderr,
	)
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("template run = exit %d, stderr %q", exitCode, stderr.String())
	}
	adjudicationPath := filepath.Join(t.TempDir(), "adjudication.json")
	if err := os.WriteFile(adjudicationPath, template.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	stderr.Reset()
	exitCode = run(
		context.Background(),
		[]string{
			"--manifest",
			manifestPath,
			"--results",
			resultsRoot,
			"--adjudication",
			adjudicationPath,
		},
		&stdout,
		&stderr,
	)
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("validation run = exit %d, stderr %q", exitCode, stderr.String())
	}
	if stdout.String() !=
		"valid corpus adjudication: 17 repositories, 0 findings, 0 gaps, 0 unresolved\n" {
		t.Fatalf("validation stdout = %q", stdout.String())
	}
}

func TestRunWritesDeterministicAdjudicationReport(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "corpus", "manifest.json")
	manifestInput, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	resultsRoot := filepath.Join(t.TempDir(), "results")
	writeEmptyResultArtifacts(t, manifest, resultsRoot, false)
	template, err := corpus.BuildAdjudicationTemplate(manifest, manifestInput, resultsRoot)
	if err != nil {
		t.Fatal(err)
	}
	adjudicationPath := filepath.Join(t.TempDir(), "adjudication.json")
	if err := os.WriteFile(adjudicationPath, template, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{
			"--manifest",
			manifestPath,
			"--results",
			resultsRoot,
			"--adjudication-report",
			adjudicationPath,
		},
		&stdout,
		&stderr,
	)
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("report run = exit %d, stderr %q", exitCode, stderr.String())
	}
	var report struct {
		SchemaVersion int `json:"schema_version"`
		Summary struct {
			Repositories int `json:"repositories"`
			Unresolved int `json:"unresolved"`
		} `json:"summary"`
		Profiles []struct {
			Profile string `json:"profile"`
			Repositories int `json:"repositories"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != corpus.AdjudicationReportSchemaVersion ||
		report.Summary.Repositories != len(manifest.Repositories) ||
		report.Summary.Unresolved != 0 ||
		len(report.Profiles) != 4 ||
		report.Profiles[0].Profile != "default" ||
		report.Profiles[0].Repositories != len(manifest.Repositories) {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunRejectsAdjudicationWithUnresolvedProfileEvidence(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "corpus", "manifest.json")
	manifestInput, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	resultsRoot := filepath.Join(t.TempDir(), "results")
	writeEmptyResultArtifacts(t, manifest, resultsRoot, true)

	var template bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := run(
		context.Background(),
		[]string{
			"--manifest",
			manifestPath,
			"--results",
			resultsRoot,
			"--adjudication-template",
		},
		&template,
		&stderr,
	);
		exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("template run = exit %d, stderr %q", exitCode, stderr.String())
	}
	var document map[string]any
	if err := json.Unmarshal(template.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	document["gaps"] = []any{
		map[string]any{
			"id": "caddy-default-incomplete",
			"repository": "caddy",
			"source": "manual",
			"kind": "crash",
			"summary": "default analysis did not complete",
			"evidence": "caddy/result.json default profile",
			"disposition": "not-actionable",
			"rule_id": "",
			"reason": "requires a successful rerun before release",
		},
	}
	adjudicationPath := filepath.Join(t.TempDir(), "adjudication.json")
	if err := os.WriteFile(adjudicationPath, marshalJSON(t, document), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	stderr.Reset()
	exitCode := run(
		context.Background(),
		[]string{
			"--manifest",
			manifestPath,
			"--results",
			resultsRoot,
			"--adjudication",
			adjudicationPath,
		},
		&stdout,
		&stderr,
	)
	if exitCode != 1 ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "1 unresolved") {
		t.Fatalf(
			"validation run = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunRejectsMixedCorpusRunIdentity(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "corpus", "manifest.json")
	manifestInput, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	resultsRoot := filepath.Join(t.TempDir(), "results")
	writeEmptyResultArtifacts(t, manifest, resultsRoot, false)
	replaceFileText(
		t,
		filepath.Join(resultsRoot, manifest.Repositories[1].ID, "result.json"),
		`"run_id": "source-aaaaaaaa-run-1"`,
		`"run_id": "source-bbbbbbbb-run-2"`,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{
			"--manifest",
			manifestPath,
			"--results",
			resultsRoot,
			"--adjudication-template",
		},
		&stdout,
		&stderr,
	)
	if exitCode != 1 ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "mixed corpus run identity") {
		t.Fatalf(
			"template run = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func writeEmptyResultArtifacts(
	t *testing.T,
	manifest corpus.Manifest,
	root string,
	incompleteDefault bool,
) {
	t.Helper()
	profiles := []string{"default", "recommended", "strict", "pedantic"}
	for _, repository := range manifest.Repositories {
		resultProfiles := make([]map[string]any, 0, len(profiles))
		for _, profile := range profiles {
			complete := !incompleteDefault ||
				profile != "default" ||
				repository.ID != manifest.Repositories[0].ID
			diagnostics := map[string]any{
				"summary": map[string]any{"diagnostics": 0, "complete": complete},
				"diagnostics": []any{},
			}
			diagnosticsInput := marshalJSON(t, diagnostics)
			inventory := map[string]any{
				"schema_version": corpus.ResultSchemaVersion,
				"repository": repository.ID,
				"revision": repository.Revision,
				"profile": profile,
				"diagnostics": []any{},
			}
			input := marshalJSON(t, inventory)
			profileRoot := filepath.Join(root, repository.ID, profile)
			if err := os.MkdirAll(profileRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(
				filepath.Join(profileRoot, "diagnostics.json"),
				diagnosticsInput,
				0o600,
			);
				err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(
				filepath.Join(profileRoot, "findings.json"),
				input,
				0o600,
			);
				err != nil {
				t.Fatal(err)
			}
			statistics := map[string]any{
				"schema_version": 1,
				"command": "lint",
				"outcome": map[string]any{"category": "success", "exit_code": 0},
				"complete": complete,
				"maximum_tier": "syntax",
				"packages": 1,
				"files": 1,
				"loaded_files": 1,
				"total": map[string]any{
					"calls": 1,
					"duration_ns": 1,
					"allocations": 1,
					"allocated_bytes": 1,
				},
				"phases": []any{},
				"tiers": []any{},
				"rules": []any{},
				"cache": map[string]any{
					"lookups": 0,
					"hits": 0,
					"misses": 0,
					"invalidations": 0,
					"writes": 0,
				},
				"dependency_syntax": map[string]any{
					"loaded": false,
					"reasons": []any{},
				},
				"effect_facts": map[string]any{"loaded": false, "reasons": []any{}},
			}
			if err := os.WriteFile(
				filepath.Join(profileRoot, "statistics.json"),
				marshalJSON(t, statistics),
				0o600,
			);
				err != nil {
				t.Fatal(err)
			}
			diagnosticsDigest := sha256.Sum256(diagnosticsInput)
			digest := sha256.Sum256(input)
			resultProfiles = append(
				resultProfiles,
				map[string]any{
					"profile": profile,
					"exit_code": 0,
					"diagnostics": map[string]any{
						"file": "diagnostics.json",
						"sha256": hex.EncodeToString(diagnosticsDigest[:]),
						"valid_json": true,
					},
					"statistics": map[string]any{
						"file": "statistics.json",
						"valid_json": true,
					},
					"findings": map[string]any{
						"file": "findings.json",
						"sha256": hex.EncodeToString(digest[:]),
						"valid_json": true,
					},
					"diagnostic_count": 0,
					"complete": complete,
				},
			)
		}
		repositoryRoot := filepath.Join(root, repository.ID)
		emptyDigest := sha256.Sum256(nil)
		comparators := make([]map[string]any, 0, 3)
		for _, name := range []string{"analysis-preflight", "go-vet", "staticcheck"} {
			if err := os.WriteFile(
				filepath.Join(repositoryRoot, name + ".txt"),
				nil,
				0o600,
			);
				err != nil {
				t.Fatal(err)
			}
			comparators = append(
				comparators,
				map[string]any{
					"name": name,
					"exit_code": 0,
					"output": map[string]any{
						"file": name + ".txt",
						"sha256": hex.EncodeToString(emptyDigest[:]),
					},
				},
			)
		}
		result := map[string]any{
			"schema_version": corpus.ResultSchemaVersion,
			"run_id": "source-aaaaaaaa-run-1",
			"repository": repository,
			"staticcheck_version": manifest.StaticcheckVersion,
			"tools": map[string]any{
				"glippy": "glippy v0.6-test",
				"go": "go version go1.26.0 test/arch",
				"staticcheck": "staticcheck 2026.1.1 (0.8.1)",
			},
			"profiles": resultProfiles,
			"comparators": comparators,
		}
		if err := os.WriteFile(
			filepath.Join(root, repository.ID, "result.json"),
			marshalJSON(t, result),
			0o600,
		);
			err != nil {
			t.Fatal(err)
		}
	}
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func replaceFileText(t *testing.T, path, old, replacement string) {
	t.Helper()
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(input), old, replacement, 1)
	if updated == string(input) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
}
