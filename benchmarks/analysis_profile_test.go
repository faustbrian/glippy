package benchmarks_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

type heapPhaseSample struct {
	phase string
	heapAlloc uint64
	heapInuse uint64
	heapObjects uint64
}

type heapPhaseProfiler struct {
	directory string
	samples []heapPhaseSample
}

func (p *heapPhaseProfiler) Capture(phase string) error {
	if phase == "" || strings.ContainsAny(phase, `/\\`) {
		return fmt.Errorf("invalid heap profile phase %q", phase)
	}
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	path := filepath.Join(p.directory, fmt.Sprintf("%02d-%s.pprof", len(p.samples) + 1, phase))
	output, err := os.OpenFile(path, os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create heap profile: %w", err)
	}
	writeErr := pprof.WriteHeapProfile(output)
	closeErr := output.Close()
	if writeErr != nil {
		return fmt.Errorf("write heap profile: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close heap profile: %w", closeErr)
	}
	p.samples = append(
		p.samples,
		heapPhaseSample{
			phase: phase,
			heapAlloc: memory.HeapAlloc,
			heapInuse: memory.HeapInuse,
			heapObjects: memory.HeapObjects,
		},
	)
	return nil
}

func TestHeapPhaseProfilerWritesOneProfilePerCapture(t *testing.T) {
	directory := t.TempDir()
	profiler := &heapPhaseProfiler{directory: directory}
	if err := profiler.Capture("packages"); err != nil {
		t.Fatal(err)
	}
	if len(profiler.samples) != 1 || profiler.samples[0].phase != "packages" {
		t.Fatalf("heap samples = %#v", profiler.samples)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "01-packages.pprof" {
		t.Fatalf("heap profiles = %#v", entries)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("heap profile is empty")
	}
}

func TestProfileExternalTypedAnalysis(t *testing.T) {
	root := os.Getenv("GLIPPY_TYPED_PROFILE_ROOT")
	if root == "" {
		t.Skip("GLIPPY_TYPED_PROFILE_ROOT is not set")
	}
	directory := os.Getenv("GLIPPY_TYPED_PROFILE_DIR")
	if directory == "" {
		t.Fatal("GLIPPY_TYPED_PROFILE_DIR is required with GLIPPY_TYPED_PROFILE_ROOT")
	}
	for name, path := range map[string]string{"root": root, "profile directory": directory} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			t.Fatalf("%s %q is not a normalized absolute path", name, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("inspect %s: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s %q is not a directory", name, path)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("profile directory %q is not empty", directory)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	goVersion := os.Getenv("GLIPPY_TYPED_PROFILE_GO_VERSION")
	if goVersion == "" {
		goVersion = "go1.26"
	}
	profiler := &heapPhaseProfiler{directory: directory}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			SourceGoVersion: goVersion,
			Presets: []rules.Preset{rules.PresetCorrectness},
			PathRoot: root,
			LintLevels: []rules.LintLevelDirective{
				{Level: rules.LintWarn, Targets: []string{"suspicious"}},
			},
			Profiler: profiler,
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			Tests: true,
			ModuleMode: analysis.ModuleReadonly,
			Env: typedProfileEnvironment(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) == 0 {
		t.Fatal("typed profile produced no analyzed files")
	}
	for _, sample := range profiler.samples {
		t.Logf(
			"phase=%s heap_alloc=%d heap_inuse=%d heap_objects=%d",
			sample.phase,
			sample.heapAlloc,
			sample.heapInuse,
			sample.heapObjects,
		)
	}
}

func typedProfileEnvironment() []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if found && name != "" {
			values[name] = value
		}
	}
	values["CGO_ENABLED"] = "0"
	values["GOENV"] = "off"
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, len(names))
	for index, name := range names {
		result[index] = name + "=" + values[name]
	}
	return result
}
