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

func TestDeferInLoopReportsIterationScopedDefers(t *testing.T) {
	t.Parallel()

	input := `package sample

func cleanup() {}

func counted(limit int) {
	for index := 0; index < limit; index++ {
		defer cleanup()
	}
}

func ranged(values []int) {
	for range values {
		defer cleanup()
	}
}

func nested(values []int) {
	for range values {
		func() { defer cleanup() }()
	}
}

func conditionless() {
	for {
		defer cleanup()
	}
}
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
			Overrides: map[string]rules.Severity{"defer-in-loop": rules.SeverityWarn},
			SourceGoVersion: "go1.25",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("defer-in-loop result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Diagnostics {
		relative := strings.Index(input[searchFrom:], "defer")
		if relative < 0 {
			t.Fatal("missing defer")
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "defer-in-loop" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("defer") ||
			!strings.Contains(diagnostic.Message, "function returns") {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len("defer")
	}
}

func TestDeferInLoopMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("defer-in-loop")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireSyntax ||
		!reflect.DeepEqual(
			metadata.NodeInterests,
			[]rules.NodeKind{rules.NodeForStmt, rules.NodeRangeStmt},
		) ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("defer-in-loop metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkDeferInLoopSyntaxAnalysis(b *testing.B) {
	file, err := source.Load(
		"sample.go",
		[]byte(
			"package sample\nfunc cleanup() {}\nfunc run(values []int) { for range values { defer cleanup() } }\n",
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
					"defer-in-loop": rules.SeverityWarn,
				},
				SourceGoVersion: "go1.25",
			},
		)
		if runErr != nil {
			b.Fatal(runErr)
		}
		if len(result.Diagnostics) != 1 {
			b.Fatalf("diagnostics = %#v", result.Diagnostics)
		}
	}
}
