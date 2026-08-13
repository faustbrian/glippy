package rulecatalog_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestLoopCaptureReportsOnlyReusedIterationVariables(t *testing.T) {
	t.Parallel()

	input := `package sample

func use(int) {}

func reusedRange(values []int) {
	var value int
	for _, value = range values {
		go func() { use(value) }()
	}
}

func reusedClassic(limit int) {
	var index int
	for index = 0; index < limit; index++ {
		defer func() { use(index) }()
	}
}

func perIteration(values []int) {
	for _, value := range values {
		go func() { use(value) }()
	}
	for index := 0; index < len(values); index++ {
		defer func() { use(index) }()
	}
}

func synchronized(values []int) {
	var value int
	for _, value = range values {
		go func() { use(value) }()
		use(value)
	}
}
`
	result := runLoopCapture(t, input, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("loop-capture result = %#v", result)
	}
	for index, needle := range []string{"use(value)", "use(index)"} {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, needle) + len("use(")
		name := strings.TrimSuffix(strings.TrimPrefix(needle, "use("), ")")
		if diagnostic.RuleID != "loop-capture" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(name) ||
			!strings.Contains(diagnostic.Message, "captured by func literal") {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestLoopCaptureRecognizesErrgroupAndParallelSubtests(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"testing"

	"golang.org/x/sync/errgroup"
)

func use(int) {}

func group(values []int) {
	var value int
	var tasks errgroup.Group
	for _, value = range values {
		tasks.Go(func() error { use(value); return nil })
	}
}

func subtests(t *testing.T, values []int) {
	var value int
	for _, value = range values {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			use(value)
		})
	}
	for _, value = range values {
		t.Run("serial", func(t *testing.T) { use(value) })
	}
}

type Group struct{}
func (*Group) Go(func() error) {}

func lookalike(values []int) {
	var value int
	var tasks Group
	for _, value = range values {
		tasks.Go(func() error { use(value); return nil })
	}
}
`
	result := runLoopCapture(
		t,
		input,
		func(root string) {
			writeFixture(
				t,
				filepath.Join(root, "go.mod"),
				"module example.com/loopcapturepatterns\n\ngo 1.25.0\n\n" +
					"require golang.org/x/sync v0.0.0\n" +
					"replace golang.org/x/sync => ./xsync\n",
			)
			writeFixture(
				t,
				filepath.Join(root, "xsync", "go.mod"),
				"module golang.org/x/sync\n\ngo 1.25.0\n",
			)
			writeFixture(
				t,
				filepath.Join(root, "xsync", "errgroup", "errgroup.go"),
				"package errgroup\ntype Group struct{}\nfunc (*Group) Go(func() error) {}\n",
			)
		},
	)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("loop-capture pattern result = %#v", result)
	}
	first := strings.Index(input, "use(value)") + len("use(")
	second := strings.Index(input[first + len("value"):], "use(value)") +
		first +
		len("value") +
		len("use(")
	for index, start := range []int{first, second} {
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "loop-capture" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("value") {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func runLoopCapture(t *testing.T, input string, setup func(string)) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	if setup == nil {
		writeFixture(
			t,
			filepath.Join(root, "go.mod"),
			"module example.com/loopcapture\n\ngo 1.25.0\n",
		)
	} else {
		setup(root)
	}
	path := filepath.Join(root, "sample.go")
	writeFixture(t, path, input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{"loop-capture": rules.SeverityWarn},
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
	return result
}
