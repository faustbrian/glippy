package rulecatalog_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestReceiverConsistencyRulesReportMinorityReceiverForms(t *testing.T) {
	t.Parallel()

	input := `package sample

type State struct{}

func (s State) Value() {}
func (s *State) First() {}
func (state *State) Second() {}
func (s *State) Third() {}

type Stable struct{}

func (stable *Stable) First() {}
func (stable *Stable) Second() {}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/receiverconsistency\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	run := func(ruleID string) analysis.PackageResult {
		t.Helper()
		result, runErr := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{ruleID: rules.SeverityWarn},
				SourceGoVersion: "go1.25",
			},
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				ModuleMode: analysis.ModuleReadonly,
			},
		)
		if runErr != nil {
			t.Fatal(runErr)
		}
		return result
	}

	nameResult := run("inconsistent-receiver-name")
	assertSingleReceiverDiagnostic(
		t,
		input,
		nameResult,
		"inconsistent-receiver-name",
		"state *State",
		"state",
		"s State",
		"s",
	)
	pointerResult := run("mixed-receiver-type")
	assertSingleReceiverDiagnostic(
		t,
		input,
		pointerResult,
		"mixed-receiver-type",
		"s State",
		"State",
		"s *State",
		"*State",
	)
}

func TestReceiverConsistencyOmitsCrossFileRelatedRanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/receiverfiles\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "canonical.go"),
		"package sample\ntype State struct{}\nfunc (s *State) First() {}\nfunc (s *State) Second() {}\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "minority.go"),
		"package sample\nfunc (state State) Third() {}\n",
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, ruleID := range []string{"inconsistent-receiver-name", "mixed-receiver-type"} {
		result, runErr := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{ruleID: rules.SeverityWarn},
				SourceGoVersion: "go1.25",
			},
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				ModuleMode: analysis.ModuleReadonly,
			},
		)
		if runErr != nil {
			t.Fatalf("%s cross-file analysis: %v", ruleID, runErr)
		}
		if countPackageDiagnostics(result) != 1 {
			t.Fatalf("%s cross-file result = %#v", ruleID, result)
		}
		diagnostic := result.Files[1].Diagnostics[0]
		if diagnostic.RuleID != ruleID || len(diagnostic.Related) != 0 {
			t.Fatalf("%s cross-file diagnostic = %#v", ruleID, diagnostic)
		}
	}
}

func TestReceiverConsistencyMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"inconsistent-receiver-name", "mixed-receiver-type"} {
		metadata, found := registry.Metadata(id)
		if !found ||
			metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(
				metadata.Presets,
				[]rules.Preset{rules.PresetPedantic},
			) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != rules.RequireTypes ||
			len(metadata.NodeInterests) != 0 ||
			metadata.RunOnGenerated ||
			metadata.RunDespiteTypeErrors ||
			len(metadata.Fixes) != 0 {
			t.Fatalf("%s metadata = %#v, found = %v", id, metadata, found)
		}
	}
}

func BenchmarkInconsistentReceiverNamePackageAnalysis(b *testing.B) {
	benchmarkReceiverConsistencyRule(b, "inconsistent-receiver-name")
}

func BenchmarkMixedReceiverTypePackageAnalysis(b *testing.B) {
	benchmarkReceiverConsistencyRule(b, "mixed-receiver-type")
}

func benchmarkReceiverConsistencyRule(b *testing.B, ruleID string) {
	b.Helper()
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/receiverconsistencybenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\ntype State struct{}\nfunc (s State) First() {}\nfunc (s *State) Second() {}\nfunc (state *State) Third() {}\nfunc (s *State) Fourth() {}\n",
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	benchmarkPackageRuns(
		b,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{ruleID: rules.SeverityWarn},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		1,
	)
}

func assertSingleReceiverDiagnostic(
	t *testing.T,
	input string,
	result analysis.PackageResult,
	ruleID string,
	receiver string,
	wantText string,
	relatedReceiver string,
	wantRelatedText string,
) {
	t.Helper()
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("%s result = %#v", ruleID, result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	receiverStart := strings.Index(input, receiver)
	start := receiverStart + strings.Index(receiver, wantText)
	relatedReceiverStart := strings.Index(input, relatedReceiver)
	relatedStart := relatedReceiverStart + strings.Index(relatedReceiver, wantRelatedText)
	wantMessageKey := "receiver-name"
	if ruleID == "mixed-receiver-type" {
		wantMessageKey = "receiver-form"
	}
	if diagnostic.RuleID != ruleID ||
		diagnostic.MessageKey != wantMessageKey ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len(wantText) ||
		len(diagnostic.Related) != 1 ||
		diagnostic.Related[0].Range.Start != relatedStart ||
		diagnostic.Related[0].Range.End != relatedStart + len(wantRelatedText) ||
		len(diagnostic.Fixes) != 0 {
		t.Fatalf("%s diagnostic = %#v", ruleID, diagnostic)
	}
}
