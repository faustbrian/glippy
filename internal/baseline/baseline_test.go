package baseline_test

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/baseline"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

func TestParseRejectsUnknownOrNoncanonicalBaselineData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		want string
	}{
		{
			name: "unknown field",
			input: `{"schema_version":1,"entries":[],"extra":true}`,
			want: "unknown field",
		},
		{
			name: "version",
			input: `{"schema_version":2,"entries":[]}`,
			want: "unsupported baseline schema version",
		},
		{
			name: "unknown rule",
			input: baselineJSON("missing", "file.go", strings.Repeat("a", 64), 1),
			want: "unknown rule",
		},
		{
			name: "absolute path",
			input: baselineJSON("known", "/file.go", strings.Repeat("a", 64), 1),
			want: "portable relative path",
		},
		{
			name: "parent path",
			input: baselineJSON("known", "../file.go", strings.Repeat("a", 64), 1),
			want: "portable relative path",
		},
		{
			name: "windows volume path",
			input: baselineJSON("known", "C:/file.go", strings.Repeat("a", 64), 1),
			want: "portable relative path",
		},
		{
			name: "uppercase fingerprint",
			input: baselineJSON("known", "file.go", strings.Repeat("A", 64), 1),
			want: "lowercase SHA-256",
		},
		{
			name: "zero count",
			input: baselineJSON("known", "file.go", strings.Repeat("a", 64), 0),
			want: "must be positive",
		},
		{
			name: "invalid expiry",
			input: strings.Replace(
				baselineJSON("known", "file.go", strings.Repeat("a", 64), 1),
				`"count":1`,
				`"count":1,"expires_on":"2026-02-30"`,
				1,
			),
			want: "valid YYYY-MM-DD",
		},
		{
			name: "duplicate",
			input: `{"schema_version":1,"entries":[` +
				baselineEntryJSON("known", "file.go", strings.Repeat("a", 64), 1) +
				`,` +
				baselineEntryJSON("known", "file.go", strings.Repeat("a", 64), 1) +
				`]}`,
			want: "duplicate baseline entry",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				_, err := baseline.Parse(
					".gox-baseline.json",
					[]byte(test.input),
					baseline.ParseOptions{KnownRules: []string{"known"}},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"Parse() error = %v, want containing %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}

func TestGenerateAggregatesPortableStructuralFingerprintsDeterministically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := loadBaselineFile(t, filepath.Join(root, "pkg", "file.go"))
	diagnostic := baselineDiagnostic(
		file,
		"call-rule",
		"call",
		source.Range{Start: 26, End: 34},
	)
	document, err := baseline.Generate(
		root,
		[]baseline.InputFile{
			{File: file, Diagnostics: []rules.Diagnostic{diagnostic, diagnostic}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != baseline.SchemaVersion || len(document.Entries) != 1 {
		t.Fatalf("Generate() = %#v", document)
	}
	entry := document.Entries[0]
	if entry.RuleID != "call-rule" ||
		entry.Path != "pkg/file.go" ||
		entry.MessageKey != "call" ||
		len(entry.SourceFingerprint) != 64 ||
		entry.Count != 2 {
		t.Fatalf("Generate() entry = %#v", entry)
	}
	encoded, err := baseline.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("target()")) || !bytes.HasSuffix(encoded, []byte("\n")) {
		t.Fatalf("Encode() leaked source or omitted final newline: %q", encoded)
	}
	parsed, err := baseline.Parse(
		".gox-baseline.json",
		encoded,
		baseline.ParseOptions{KnownRules: []string{"call-rule"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := baseline.Encode(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("baseline encoding is not deterministic:\n%s\n%s", encoded, reencoded)
	}
}

func TestApplyConsumesExactCountsAndReportsStaleEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := loadBaselineFile(t, filepath.Join(root, "pkg", "file.go"))
	diagnostic := baselineDiagnostic(
		file,
		"call-rule",
		"call",
		source.Range{Start: 26, End: 34},
	)
	document, err := baseline.Generate(
		root,
		[]baseline.InputFile{
			{File: file, Diagnostics: []rules.Diagnostic{diagnostic, diagnostic}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	document.Entries[0].Count = 2
	result, err := baseline.Apply(
		root,
		document,
		[]baseline.InputFile{{File: file, Diagnostics: []rules.Diagnostic{diagnostic}}},
		baseline.ApplyOptions{ReportStale: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 ||
		len(result.Files[0].Diagnostics) != 0 ||
		len(result.Files[0].Baselined) != 1 ||
		len(result.Problems) != 1 ||
		result.Problems[0].Kind != baseline.ProblemStale ||
		result.Problems[0].Remaining != 1 {
		t.Fatalf("Apply() = %#v", result)
	}
}

func TestApplyDoesNotHideChangedOrExpiredDiagnostics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := loadBaselineFile(t, filepath.Join(root, "pkg", "file.go"))
	diagnostic := baselineDiagnostic(
		file,
		"call-rule",
		"call",
		source.Range{Start: 26, End: 34},
	)
	document, err := baseline.Generate(
		root,
		[]baseline.InputFile{{File: file, Diagnostics: []rules.Diagnostic{diagnostic}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	document.Entries[0].ExpiresOn = "2026-08-13"
	result, err := baseline.Apply(
		root,
		document,
		[]baseline.InputFile{{File: file, Diagnostics: []rules.Diagnostic{diagnostic}}},
		baseline.ApplyOptions{ReportStale: true, ExpiryCutoff: "2026-08-13"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files[0].Diagnostics) != 1 ||
		len(result.Files[0].Baselined) != 0 ||
		len(result.Problems) != 1 ||
		result.Problems[0].Kind != baseline.ProblemExpired {
		t.Fatalf("Apply() expired = %#v", result)
	}

	other := loadBaselineFileWithSource(
		t,
		filepath.Join(root, "pkg", "file.go"),
		"package sample\nfunc run(){changed()}\n",
	)
	changedDiagnostic := baselineDiagnostic(
		other,
		"call-rule",
		"call",
		source.Range{Start: 26, End: 35},
	)
	result, err = baseline.Apply(
		root,
		document,
		[]baseline.InputFile{
			{File: other, Diagnostics: []rules.Diagnostic{changedDiagnostic}},
		},
		baseline.ApplyOptions{ReportStale: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files[0].Diagnostics) != 1 ||
		len(result.Problems) != 1 ||
		result.Problems[0].Kind != baseline.ProblemStale {
		t.Fatalf("Apply() changed = %#v", result)
	}
}

func TestApplyDoesNotReportEntriesOutsideAnalyzedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := loadBaselineFile(t, filepath.Join(root, "pkg", "file.go"))
	diagnostic := baselineDiagnostic(
		file,
		"call-rule",
		"call",
		source.Range{Start: 26, End: 34},
	)
	document, err := baseline.Generate(
		root,
		[]baseline.InputFile{{File: file, Diagnostics: []rules.Diagnostic{diagnostic}}},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := baseline.Apply(
		root,
		document,
		nil,
		baseline.ApplyOptions{ReportStale: true, ExpiryCutoff: "2026-08-13"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Problems) != 0 {
		t.Fatalf(
			"Apply() problems = %#v, want none outside analyzed files",
			result.Problems,
		)
	}
}

func FuzzParseCanonicalRoundTrip(f *testing.F) {
	f.Add([]byte(`{"schema_version":1,"entries":[]}`))
	f.Add([]byte(`{"schema_version":2,"entries":null}`))
	f.Fuzz(
		func(t *testing.T, input []byte) {
			document, err := baseline.Parse(
				".gox-baseline.json",
				input,
				baseline.ParseOptions{KnownRules: []string{"known"}},
			)
			if err != nil {
				return
			}
			encoded, err := baseline.Encode(document)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := baseline.Parse(
				".gox-baseline.json",
				encoded,
				baseline.ParseOptions{KnownRules: []string{"known"}},
			);
				err != nil {
				t.Fatalf("canonical baseline failed to parse: %v", err)
			}
		},
	)
}

func baselineJSON(ruleID, path, fingerprint string, count int) string {
	return `{"schema_version":1,"entries":[` +
		baselineEntryJSON(ruleID, path, fingerprint, count) +
		`]}`
}

func baselineEntryJSON(ruleID, path, fingerprint string, count int) string {
	return `{"rule_id":"` +
		ruleID +
		`","path":"` +
		path +
		`","message_key":"call","source_fingerprint":"` +
		fingerprint +
		`","count":` +
		strconv.Itoa(count) +
		`}`
}

func loadBaselineFile(t *testing.T, path string) *source.File {
	t.Helper()
	return loadBaselineFileWithSource(t, path, "package sample\nfunc run(){target()}\n")
}

func loadBaselineFileWithSource(t *testing.T, path, input string) *source.File {
	t.Helper()
	file, err := source.Load(path, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func baselineDiagnostic(
	file *source.File,
	ruleID, messageKey string,
	range_ source.Range,
) rules.Diagnostic {
	return rules.Diagnostic{
		RuleID: ruleID,
		Severity: rules.SeverityWarn,
		MessageKey: messageKey,
		Message: "call requires review",
		Path: file.Path(),
		Digest: file.Digest(),
		Range: range_,
	}
}
