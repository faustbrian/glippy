package rulecatalog_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestRedundantElseReportsAlternativeAfterTerminatingBranch(t *testing.T) {
	t.Parallel()

	input := `package sample

func returned(ok bool) int {
	if ok {
		return 1
	} else {
		return 2
	}
}

func continued(values []int) {
	for _, value := range values {
		if value < 0 {
			continue
		} else {
			use(value)
		}
	}
}

func nested(ok, ready bool) int {
	if ok {
		if ready { return 1 } else { return 2 }
	} else {
		return 3
	}
}

func nestedBlock(ok bool) {
	if ok {
		{ return }
	} else {
		use(1)
	}
}

func broken(values []int) {
	for _, value := range values {
		if value < 0 { break } else { use(value) }
	}
}

func jumped(value int) {
	if value < 0 { goto done } else { use(value) }
done:
}

func initialized(value int) int {
	if current := value; current > 0 { return current } else { return current + 1 }
}

func live(ok bool) int {
	if ok { use(1) } else { use(2) }
	return 0
}

func use(int) {}
`
	file, err := source.Load("sample.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{"redundant-else": rules.SeverityWarn},
			SourceGoVersion: "go1.25",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]int, 0, 7)
	searchFrom := 0
	for range 7 {
		relative := strings.Index(input[searchFrom:], "else")
		if relative < 0 {
			t.Fatal("missing expected else keyword")
		}
		start := searchFrom + relative
		want = append(want, start)
		searchFrom = start + len("else")
	}
	if len(result.Diagnostics) != len(want) {
		t.Fatalf("redundant-else result = %#v", result)
	}
	for index, diagnostic := range result.Diagnostics {
		if diagnostic.RuleID != "redundant-else" ||
			diagnostic.MessageKey != "terminating-if-branch" ||
			diagnostic.Range.Start != want[index] ||
			diagnostic.Range.End != want[index] + len("else") ||
			!strings.Contains(diagnostic.Message, "terminates") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("redundant-else diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestRedundantElseMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("redundant-else")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetPedantic}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSyntax ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeIfStmt}) ||
		metadata.RunOnGenerated ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("redundant-else metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkRedundantElseSyntaxAnalysis(b *testing.B) {
	file, err := source.Load(
		"sample.go",
		[]byte(
			"package sample\nfunc run(ok bool) int { if ok { return 1 } else { return 2 } }\n",
		),
	)
	if err != nil {
		b.Fatal(err)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, runErr := analysis.Run(
			context.Background(),
			file,
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{
					"redundant-else": rules.SeverityWarn,
				},
				SourceGoVersion: "go1.25",
			},
		)
		if runErr != nil {
			b.Fatal(runErr)
		}
		if len(result.Diagnostics) != 1 {
			b.Fatalf("benchmark result = %#v", result)
		}
	}
}
