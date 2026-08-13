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

func TestLockHeldAcrossBlockingCallReportsKnownBlockingCalls(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"sync"
	"time"
)

func sleepLocked(mu *sync.Mutex) {
	mu.Lock()
	time.Sleep(time.Second)
	mu.Unlock()
}

func waitLocked(mu *sync.RWMutex, group *sync.WaitGroup) {
	mu.RLock()
	group.Wait()
	mu.RUnlock()
}

func good(mu *sync.Mutex) {
	mu.Lock()
	mu.Unlock()
	time.Sleep(time.Second)
}

type lookalike struct{}
func (*lookalike) Lock() {}
func (*lookalike) Unlock() {}
func (*lookalike) Wait() {}

func ignored(value *lookalike) {
	value.Lock()
	value.Wait()
	value.Unlock()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/lockblocking\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"lock-held-across-blocking-call": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"time.Sleep(time.Second)", "group.Wait()"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("lock-held-across-blocking-call result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing call %q", want[index])
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "lock-held-across-blocking-call" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index]) ||
			!strings.Contains(diagnostic.Message, "lock") ||
			len(diagnostic.Related) != 1 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}

func TestLockHeldAcrossBlockingCallMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("lock-held-across-blocking-call")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeBlockStmt}) ||
		len(metadata.Fixes) != 0 {
		t.Fatalf(
			"lock-held-across-blocking-call metadata = %#v, found = %v",
			metadata,
			found,
		)
	}
}

func BenchmarkLockHeldAcrossBlockingCallPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/lockblockingbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nimport (\"sync\"; \"time\")\nfunc run(mu *sync.Mutex) { mu.Lock(); time.Sleep(time.Second); mu.Unlock() }\n",
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
			Overrides: map[string]rules.Severity{
				"lock-held-across-blocking-call": rules.SeverityWarn,
			},
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
