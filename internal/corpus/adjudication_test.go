package corpus_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/corpus"
)

func TestValidateAdjudicationBindsEveryRequiredFindingAndGap(t *testing.T) {
	t.Parallel()

	manifestInput := []byte(singleRepositoryManifestJSON())
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	_, options, _, _ := newRunFixture(t)
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	defaultFingerprint := readFindingFingerprint(
		t,
		filepath.Join(options.OutputRoot, "alpha", "default", "findings.json"),
	)
	recommendedFingerprint := readFindingFingerprint(
		t,
		filepath.Join(options.OutputRoot, "alpha", "recommended", "findings.json"),
	)
	resultDigest := fileDigest(t, filepath.Join(options.OutputRoot, "alpha", "result.json"))
	input := adjudicationJSON(
		manifestInput,
		resultDigest,
		defaultFingerprint,
		recommendedFingerprint,
	)

	summary, err := corpus.ValidateAdjudication(
		manifest,
		manifestInput,
		options.OutputRoot,
		[]byte(input),
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Repositories != 1 ||
		summary.Findings != 2 ||
		summary.Gaps != 1 ||
		summary.Unresolved != 0 {
		t.Fatalf("ValidateAdjudication() summary = %#v", summary)
	}
}

func TestBuildAdjudicationTemplateIncludesEveryRequiredFinding(t *testing.T) {
	t.Parallel()

	manifestInput := []byte(singleRepositoryManifestJSON())
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	_, options, _, _ := newRunFixture(t)
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	template, err := corpus.BuildAdjudicationTemplate(
		manifest,
		manifestInput,
		options.OutputRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := corpus.ValidateAdjudication(
		manifest,
		manifestInput,
		options.OutputRoot,
		template,
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Repositories != 1 ||
		summary.Findings != 2 ||
		summary.Gaps != 0 ||
		summary.Unresolved != 2 {
		t.Fatalf("template summary = %#v", summary)
	}
	if !strings.Contains(string(template), `"reason": "pending manual adjudication"`) {
		t.Fatalf("template = %s", template)
	}
}

func TestValidateAdjudicationRejectsChangedResultSet(t *testing.T) {
	t.Parallel()

	manifestInput := []byte(singleRepositoryManifestJSON())
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	_, options, _, _ := newRunFixture(t)
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	template, err := corpus.BuildAdjudicationTemplate(
		manifest,
		manifestInput,
		options.OutputRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	replaceFileText(
		t,
		filepath.Join(options.OutputRoot, "alpha", "result.json"),
		`"glippy": "glippy v0.6-dev"`,
		`"glippy": "glippy v0.6-other"`,
	)

	_, err = corpus.ValidateAdjudication(manifest, manifestInput, options.OutputRoot, template)
	if err == nil || !strings.Contains(err.Error(), "result digest mismatch") {
		t.Fatalf("ValidateAdjudication() error = %v, want result digest mismatch", err)
	}
}

func TestAdjudicationTracksIncompleteProfilesAsUnresolvedEvidence(t *testing.T) {
	t.Parallel()

	manifestInput := []byte(singleRepositoryManifestJSON())
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	_, options, _, _ := newRunFixture(t)
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	makeDefaultDiagnosticsNonJSON(t, options.OutputRoot)
	template, err := corpus.BuildAdjudicationTemplate(
		manifest,
		manifestInput,
		options.OutputRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		string(template),
		`"incomplete_profiles": [
        "default"
      ]`,
	) {
		t.Fatalf("template does not record incomplete default profile: %s", template)
	}
	if _, err := corpus.ValidateAdjudication(
		manifest,
		manifestInput,
		options.OutputRoot,
		template,
	);
		err == nil ||
			!strings.Contains(
				err.Error(),
				"require a crash or unsupported-construct gap",
			) {
		t.Fatalf(
			"ValidateAdjudication() error = %v, want required incomplete-profile gap",
			err,
		)
	}

	var document map[string]any
	if err := json.Unmarshal(template, &document); err != nil {
		t.Fatal(err)
	}
	document["gaps"] = []any{
		map[string]any{
			"id": "alpha-default-incomplete",
			"repository": "alpha",
			"source": "manual",
			"kind": "crash",
			"summary": "default analysis did not complete",
			"evidence": "alpha/result.json default profile",
			"disposition": "not-actionable",
			"rule_id": "",
			"reason": "requires a successful rerun before release",
		},
	}
	input, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := corpus.ValidateAdjudication(
		manifest,
		manifestInput,
		options.OutputRoot,
		input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Unresolved != 3 {
		t.Fatalf("ValidateAdjudication() unresolved = %d, want 3", summary.Unresolved)
	}
}

func TestAdjudicationTracksFailedComparatorsAsUnresolvedEvidence(t *testing.T) {
	t.Parallel()

	manifestInput := []byte(singleRepositoryManifestJSON())
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	_, options, _, _ := newRunFixture(t)
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(options.OutputRoot, "alpha", "result.json")
	resultInput, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(resultInput, &result); err != nil {
		t.Fatal(err)
	}
	comparators := result["comparators"].([]any)
	for _, comparator := range comparators {
		entry := comparator.(map[string]any)
		if entry["name"] == "go-vet" {
			entry["exit_code"] = float64(2)
		}
	}
	if err := os.WriteFile(resultPath, marshalJSONValue(t, result), 0o600); err != nil {
		t.Fatal(err)
	}

	template, err := corpus.BuildAdjudicationTemplate(
		manifest,
		manifestInput,
		options.OutputRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		string(template),
		`"incomplete_comparators": [
        "go-vet"
      ]`,
	) {
		t.Fatalf("template does not record failed go-vet comparator: %s", template)
	}
	if _, err := corpus.ValidateAdjudication(
		manifest,
		manifestInput,
		options.OutputRoot,
		template,
	);
		err == nil ||
			!strings.Contains(
				err.Error(),
				"require a crash or unsupported-construct gap",
			) {
		t.Fatalf("ValidateAdjudication() error = %v, want required comparator gap", err)
	}

	var document map[string]any
	if err := json.Unmarshal(template, &document); err != nil {
		t.Fatal(err)
	}
	document["gaps"] = []any{
		map[string]any{
			"id": "alpha-go-vet-failed",
			"repository": "alpha",
			"source": "vet",
			"kind": "crash",
			"summary": "go vet did not produce a findings exit",
			"evidence": "alpha/result.json go-vet comparator",
			"disposition": "not-actionable",
			"rule_id": "",
			"reason": "requires a successful rerun before release",
		},
	}
	summary, err := corpus.ValidateAdjudication(
		manifest,
		manifestInput,
		options.OutputRoot,
		marshalJSONValue(t, document),
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Unresolved != 3 {
		t.Fatalf("ValidateAdjudication() unresolved = %d, want 3", summary.Unresolved)
	}
}

func TestAdjudicationTreatsFailedAnalysisPreflightAsIncompleteComparators(t *testing.T) {
	t.Parallel()

	manifestInput := []byte(singleRepositoryManifestJSON())
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	_, options, _, _ := newRunFixture(t)
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(options.OutputRoot, "alpha", "result.json")
	resultInput, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(resultInput, &result); err != nil {
		t.Fatal(err)
	}
	for _, comparator := range result["comparators"].([]any) {
		entry := comparator.(map[string]any)
		if entry["name"] == "analysis-preflight" {
			entry["exit_code"] = float64(1)
		}
	}
	if err := os.WriteFile(resultPath, marshalJSONValue(t, result), 0o600); err != nil {
		t.Fatal(err)
	}

	template, err := corpus.BuildAdjudicationTemplate(
		manifest,
		manifestInput,
		options.OutputRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		string(template),
		`"incomplete_comparators": [
        "go-vet",
        "staticcheck"
      ]`,
	) {
		t.Fatalf("template does not mark both comparators incomplete: %s", template)
	}
}

func TestValidateAdjudicationRejectsIncompleteOrNonCanonicalEvidence(t *testing.T) {
	t.Parallel()

	manifestInput := []byte(singleRepositoryManifestJSON())
	manifest, err := corpus.ParseManifest(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	_, options, _, _ := newRunFixture(t)
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	defaultFingerprint := readFindingFingerprint(
		t,
		filepath.Join(options.OutputRoot, "alpha", "default", "findings.json"),
	)
	recommendedFingerprint := readFindingFingerprint(
		t,
		filepath.Join(options.OutputRoot, "alpha", "recommended", "findings.json"),
	)
	resultDigest := fileDigest(t, filepath.Join(options.OutputRoot, "alpha", "result.json"))
	valid := adjudicationJSON(
		manifestInput,
		resultDigest,
		defaultFingerprint,
		recommendedFingerprint,
	)

	tests := []struct {
		name string
		old string
		new string
		want string
	}{
		{
			name: "duplicate field",
			old: `"schema_version": 1,`,
			new: `"schema_version": 1, "schema_version": 1,`,
			want: "duplicate field",
		},
		{
			name: "case-folded field",
			old: `"schema_version": 1,`,
			new: `"SCHEMA_VERSION": 1,`,
			want: "unknown field",
		},
		{
			name: "manifest mismatch",
			old: manifestDigest(manifestInput),
			new: strings.Repeat("0", 64),
			want: "manifest digest",
		},
		{
			name: "unknown finding",
			old: defaultFingerprint,
			new: strings.Repeat("f", 64),
			want: "unknown finding",
		},
		{
			name: "missing finding",
			old: fmt.Sprintf(
				`{"fingerprint": %q, "classification": "true-positive", "reason": "confirmed defect"}`,
				defaultFingerprint,
			),
			new: "",
			want: "missing 1 finding",
		},
		{
			name: "invalid classification",
			old: `"classification": "true-positive"`,
			new: `"classification": "maybe"`,
			want: "classification",
		},
		{
			name: "missing reason",
			old: `"reason": "confirmed defect"`,
			new: `"reason": ""`,
			want: "reason",
		},
		{
			name: "wrong revision",
			old: strings.Repeat("a", 40),
			new: strings.Repeat("b", 40),
			want: "revision",
		},
		{
			name: "invalid gap disposition",
			old: `"disposition": "nursery"`,
			new: `"disposition": "ship"`,
			want: "disposition",
		},
		{
			name: "null gap rule ID",
			old: `"disposition": "nursery",
      "rule_id": "resource-not-closed"`,
			new: `"disposition": "not-actionable",
      "rule_id": null`,
			want: "want JSON string",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				input := strings.Replace(valid, test.old, test.new, 1)
				_, err := corpus.ValidateAdjudication(
					manifest,
					manifestInput,
					options.OutputRoot,
					[]byte(input),
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"ValidateAdjudication() error = %v, want containing %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}

func TestBuildAdjudicationTemplateRejectsUnboundResultArtifacts(t *testing.T) {
	t.Parallel()

	for _, test := range
		[]struct {
			name string
			mutate func(*testing.T, string)
			want string
		}{
			{
				name: "result schema",
				mutate: func(t *testing.T, root string) {
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						`"schema_version": 1`,
						`"schema_version": 2`,
					)
				},
				want: "result schema_version",
			},
			{
				name: "duplicate result field",
				mutate: func(t *testing.T, root string) {
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						`"schema_version": 1,`,
						`"schema_version": 1, "schema_version": 1,`,
					)
				},
				want: "duplicate field",
			},
			{
				name: "case-folded result field",
				mutate: func(t *testing.T, root string) {
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						`"schema_version": 1,`,
						`"SCHEMA_VERSION": 1,`,
					)
				},
				want: "unknown field",
			},
			{
				name: "staticcheck version",
				mutate: func(t *testing.T, root string) {
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						`"staticcheck_version": "v0.8.1"`,
						`"staticcheck_version": "v0.9.0"`,
					)
				},
				want: "staticcheck_version",
			},
			{
				name: "repository manifest metadata",
				mutate: func(t *testing.T, root string) {
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						`"go_directive": "1.26"`,
						`"go_directive": "1.25"`,
					)
				},
				want: "repository metadata",
			},
			{
				name: "finding schema",
				mutate: func(t *testing.T, root string) {
					findingsPath := filepath.Join(
						root,
						"alpha",
						"default",
						"findings.json",
					)
					before, err := os.ReadFile(findingsPath)
					if err != nil {
						t.Fatal(err)
					}
					beforeDigest := sha256.Sum256(before)
					replaceFileText(
						t,
						findingsPath,
						`"schema_version": 1`,
						`"schema_version": 2`,
					)
					after, err := os.ReadFile(findingsPath)
					if err != nil {
						t.Fatal(err)
					}
					afterDigest := sha256.Sum256(after)
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						hex.EncodeToString(beforeDigest[:]),
						hex.EncodeToString(afterDigest[:]),
					)
				},
				want: "finding schema_version",
			},
			{
				name: "duplicate finding field",
				mutate: func(t *testing.T, root string) {
					mutateFindingArtifact(
						t,
						root,
						`"schema_version": 1,`,
						`"schema_version": 1, "schema_version": 1,`,
					)
				},
				want: "duplicate field",
			},
			{
				name: "case-folded finding field",
				mutate: func(t *testing.T, root string) {
					mutateFindingArtifact(
						t,
						root,
						`"schema_version": 1,`,
						`"SCHEMA_VERSION": 1,`,
					)
				},
				want: "unknown field",
			},
			{
				name: "diagnostic count mismatch",
				mutate: func(t *testing.T, root string) {
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						`"diagnostic_count": 1`,
						`"diagnostic_count": 0`,
					)
				},
				want: "diagnostic_count",
			},
			{
				name: "diagnostic inventory mismatch",
				mutate: func(t *testing.T, root string) {
					mutateDiagnosticArtifact(
						t,
						root,
						`"message": "sample finding"`,
						`"message": "different finding"`,
					)
				},
				want: "diagnostic inventory mismatch",
			},
			{
				name: "invalid diagnostics",
				mutate: func(t *testing.T, root string) {
					replaceFileText(
						t,
						filepath.Join(root, "alpha", "result.json"),
						`"valid_json": true`,
						`"valid_json": false`,
					)
				},
				want: "diagnostic artifact",
			},
			{
				name: "comparator digest mismatch",
				mutate: func(t *testing.T, root string) {
					if err := os.WriteFile(
						filepath.Join(root, "alpha", "go-vet.txt"),
						[]byte("changed\n"),
						0o600,
					);
						err != nil {
						t.Fatal(err)
					}
				},
				want: "comparator digest mismatch",
			},
			{
				name: "duplicate profile",
				mutate: func(t *testing.T, root string) {
					path := filepath.Join(root, "alpha", "result.json")
					input, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					var result map[string]any
					if err := json.Unmarshal(input, &result); err != nil {
						t.Fatal(err)
					}
					profiles := result["profiles"].([]any)
					result["profiles"] = append(profiles, profiles[0])
					encoded, err := json.MarshalIndent(result, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, append(encoded, '\n'), 0o600);
						err != nil {
						t.Fatal(err)
					}
				},
				want: "duplicate profile",
			},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				manifestInput := []byte(singleRepositoryManifestJSON())
				manifest, err := corpus.ParseManifest(manifestInput)
				if err != nil {
					t.Fatal(err)
				}
				_, options, _, _ := newRunFixture(t)
				if err := corpus.Run(context.Background(), manifest, options);
					err != nil {
					t.Fatal(err)
				}
				test.mutate(t, options.OutputRoot)

				_, err = corpus.BuildAdjudicationTemplate(
					manifest,
					manifestInput,
					options.OutputRoot,
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"BuildAdjudicationTemplate() error = %v, want containing %q",
						err,
						test.want,
					)
				}
			},
		)
	}
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

func mutateFindingArtifact(t *testing.T, root, old, replacement string) {
	t.Helper()
	findingsPath := filepath.Join(root, "alpha", "default", "findings.json")
	beforeDigest := fileDigest(t, findingsPath)
	replaceFileText(t, findingsPath, old, replacement)
	afterDigest := fileDigest(t, findingsPath)
	replaceFileText(t, filepath.Join(root, "alpha", "result.json"), beforeDigest, afterDigest)
}

func mutateDiagnosticArtifact(t *testing.T, root, old, replacement string) {
	t.Helper()
	diagnosticsPath := filepath.Join(root, "alpha", "default", "diagnostics.json")
	beforeDigest := fileDigest(t, diagnosticsPath)
	replaceFileText(t, diagnosticsPath, old, replacement)
	afterDigest := fileDigest(t, diagnosticsPath)
	replaceFileText(t, filepath.Join(root, "alpha", "result.json"), beforeDigest, afterDigest)
}

func makeDefaultDiagnosticsNonJSON(t *testing.T, root string) {
	t.Helper()
	profileRoot := filepath.Join(root, "alpha", "default")
	diagnosticsPath := filepath.Join(profileRoot, "diagnostics.json")
	beforeDigest := fileDigest(t, diagnosticsPath)
	textPath := filepath.Join(profileRoot, "diagnostics.txt")
	if err := os.WriteFile(textPath, []byte("glippy crashed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterDigest := fileDigest(t, textPath)
	resultPath := filepath.Join(root, "alpha", "result.json")
	replaceFileText(t, resultPath, `"file": "diagnostics.json"`, `"file": "diagnostics.txt"`)
	replaceFileText(t, resultPath, beforeDigest, afterDigest)
	replaceFileText(t, resultPath, `"valid_json": true`, `"valid_json": false`)
	replaceFileText(t, resultPath, `"complete": true`, `"complete": false`)
}

func readFindingFingerprint(t *testing.T, path string) string {
	t.Helper()
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Diagnostics []struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(input, &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Diagnostics) != 1 || inventory.Diagnostics[0].Fingerprint == "" {
		t.Fatalf("finding inventory = %s", input)
	}
	return inventory.Diagnostics[0].Fingerprint
}

func adjudicationJSON(
	manifest []byte,
	resultDigest, defaultFingerprint, recommendedFingerprint string,
) string {
	return fmt.Sprintf(
		`{
  "schema_version": 1,
  "manifest_sha256": %q,
  "run": {
    "id": "source-aaaaaaaa-run-1",
    "glippy": "glippy v0.6-dev",
    "go": "go version go1.26.0 test/arch",
    "staticcheck": "staticcheck 2026.1.1 (0.8.1)"
  },
  "repositories": [
    {
      "id": "alpha",
      "revision": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "result_sha256": %q,
      "incomplete_profiles": [],
      "incomplete_comparators": [],
      "profiles": [
        {
          "profile": "default",
          "findings": [
            {"fingerprint": %q, "classification": "true-positive", "reason": "confirmed defect"}
          ]
        },
        {
          "profile": "recommended",
          "findings": [
            {"fingerprint": %q, "classification": "duplicate-staticcheck", "reason": "same defect and range"}
          ]
        }
      ]
    }
  ],
  "gaps": [
    {
      "id": "manual-missed-cleanup",
      "repository": "alpha",
      "source": "manual",
      "kind": "missed-defect",
      "summary": "resource cleanup is not diagnosed",
      "evidence": "sample.go:1",
      "disposition": "nursery",
      "rule_id": "resource-not-closed",
      "reason": "requires broader ownership evidence"
    }
  ]
}`,
		manifestDigest(manifest),
		resultDigest,
		defaultFingerprint,
		recommendedFingerprint,
	)
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

func marshalJSONValue(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func manifestDigest(input []byte) string {
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

func singleRepositoryManifestJSON() string {
	return strings.ReplaceAll(
		validManifestJSON,
		`,
    {
      "id": "beta",
      "repository": "https://github.com/example/beta.git",
      "revision": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "license": "MIT",
      "license_path": "COPYING",
      "roles": ["library"],
      "go_directive": "1.24",
      "source_version_policy": "unsupported",
      "cgo": true,
      "generated": false,
      "patterns": ["."]
    }`,
		"",
	)
}
