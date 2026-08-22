package corpus_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/corpus"
)

func TestParseManifestAcceptsCanonicalPinnedRepositories(t *testing.T) {
	t.Parallel()

	manifest, err := corpus.ParseManifest([]byte(validManifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.StaticcheckVersion != "v0.8.1" {
		t.Fatalf("ParseManifest() metadata = %#v", manifest)
	}
	if len(manifest.Repositories) != 2 ||
		manifest.Repositories[0].ID != "alpha" ||
		manifest.Repositories[1].ID != "beta" {
		t.Fatalf("ParseManifest() repositories = %#v", manifest.Repositories)
	}
}

func TestParseManifestRejectsNonCanonicalOrIncompleteInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		replace string
		with string
		want string
	}{
		{
			name: "unknown field",
			replace: `"schema_version": 1,`,
			with: `"schema_version": 1, "surprise": true,`,
			want: "unknown field",
		},
		{
			name: "duplicate top-level field",
			replace: `"schema_version": 1,`,
			with: `"schema_version": 1, "schema_version": 1,`,
			want: "duplicate field",
		},
		{
			name: "case-folded top-level field",
			replace: `"schema_version": 1,`,
			with: `"SCHEMA_VERSION": 1,`,
			want: "unknown field",
		},
		{
			name: "duplicate repository field",
			replace: `"id": "alpha",`,
			with: `"id": "alpha", "id": "alpha",`,
			want: "duplicate field",
		},
		{
			name: "case-folded repository field",
			replace: `"id": "alpha",`,
			with: `"ID": "alpha",`,
			want: "unknown field",
		},
		{
			name: "repository order",
			replace: `"id": "alpha"`,
			with: `"id": "zeta"`,
			want: "repositories must be ordered by ID",
		},
		{
			name: "abbreviated revision",
			replace: strings.Repeat("a", 40),
			with: "abc123",
			want: "full lowercase Git SHA",
		},
		{
			name: "missing license path",
			replace: `"license_path": "LICENSE"`,
			with: `"license_path": ""`,
			want: "license_path",
		},
		{
			name: "uncanonical roles",
			replace: `"roles": ["cli", "generated"]`,
			with: `"roles": ["generated", "cli"]`,
			want: "roles must be ordered",
		},
		{
			name: "escaping pattern",
			replace: `"patterns": ["./..."]`,
			with: `"patterns": ["../..."]`,
			want: "unsafe pattern",
		},
		{
			name: "unsupported policy",
			replace: `"source_version_policy": "supported"`,
			with: `"source_version_policy": "shim"`,
			want: "source_version_policy",
		},
		{
			name: "missing feature flag",
			replace: "      \"cgo\": false,\n",
			with: "",
			want: "cgo",
		},
		{
			name: "null feature flag",
			replace: `"generated": true`,
			with: `"generated": null`,
			want: "generated must be a JSON boolean",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				input := strings.Replace(
					validManifestJSON,
					test.replace,
					test.with,
					1,
				)
				_, err := corpus.ParseManifest([]byte(input))
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"ParseManifest() error = %v, want containing %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}

func TestPinnedManifestRecordsTheV06ValidationCorpus(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "benchmarks", "corpus", "manifest.json")
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := corpus.ParseManifest(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Repositories) != 17 {
		t.Fatalf("pinned repository count = %d, want 17", len(manifest.Repositories))
	}

	want := map[string]string{
		"kubernetes": "e81f39c0e03ce8ed8e2660c9147b391edd9e262b",
		"prometheus": "d15adb9ad7e5d9fbde3a9a8f30200593a5a14d86",
		"go-ethereum": "02b73d4ea7181464175e0a6cbecc0a3a2655a562",
		"go-sqlite3": "58c8e145308ceded07d1df2ac1b65999499e7055",
	}
	for _, repository := range manifest.Repositories {
		if revision, found := want[repository.ID]; found {
			if repository.Revision != revision {
				t.Fatalf(
					"repository %q revision = %q, want %q",
					repository.ID,
					repository.Revision,
					revision,
				)
			}
			delete(want, repository.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("pinned manifest is missing repositories: %v", want)
	}
}

const validManifestJSON = `{
  "schema_version": 1,
  "staticcheck_version": "v0.8.1",
  "repositories": [
    {
      "id": "alpha",
      "repository": "https://github.com/example/alpha.git",
      "revision": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "license": "Apache-2.0",
      "license_path": "LICENSE",
      "roles": ["cli", "generated"],
      "go_directive": "1.26",
      "source_version_policy": "supported",
      "cgo": false,
      "generated": true,
      "patterns": ["./..."]
    },
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
    }
  ]
}`
