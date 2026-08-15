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

func TestExecPipeRunReportsRunAfterOutputPipe(t *testing.T) {
	t.Parallel()

	input := `package sample

import "os/exec"

func stdout() error {
	command := exec.Command("true")
	output, err := command.StdoutPipe()
	if err != nil { return err }
	_ = output
	return command.Run()
}

func stderr() error {
	command := exec.Command("true")
	output, err := command.StderrPipe()
	if err != nil { return err }
	_ = output
	return command.Run()
}

func validStartWait() error {
	command := exec.Command("true")
	output, err := command.StdoutPipe()
	if err != nil { return err }
	if err := command.Start(); err != nil { return err }
	_ = output
	return command.Wait()
}

func runBeforePipe() error {
	command := exec.Command("true")
	if err := command.Run(); err != nil { return err }
	_, err := command.StdoutPipe()
	return err
}

func reassigned() error {
	command := exec.Command("true")
	if _, err := command.StdoutPipe(); err != nil { return err }
	command = exec.Command("false")
	return command.Run()
}

type command struct{}
func (*command) StdoutPipe() (any, error) { return nil, nil }
func (*command) Run() error { return nil }

func custom(value *command) error {
	_, err := value.StdoutPipe()
	if err != nil { return err }
	return value.Run()
}
`
	result := runOnePedanticRule(t, "exec-pipe-run", input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("exec-pipe-run result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], "command.Run()")
		if relative < 0 {
			t.Fatalf("missing Run call %d", index)
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "exec-pipe-run" ||
			diagnostic.MessageKey != "run-after-output-pipe" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("command.Run()") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("exec-pipe-run diagnostic %d = %#v", index, diagnostic)
		}
		searchFrom = start + len("command.Run()")
	}
}

func BenchmarkExecPipeRun(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/execpipebenchmark\n\ngo 1.26.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		`package sample

import "os/exec"

func run() error {
	command := exec.Command("true")
	output, err := command.StdoutPipe()
	if err != nil { return err }
	_ = output
	return command.Run()
}
`,
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	options := analysis.RunOptions{
		Presets: []rules.Preset{},
		Overrides: map[string]rules.Severity{"exec-pipe-run": rules.SeverityWarn},
		SourceGoVersion: "go1.26",
	}
	load := analysis.PackageLoadOptions{
		Dir: root,
		Patterns: []string{"."},
		ModuleMode: analysis.ModuleReadonly,
	}
	b.ResetTimer()
	for b.Loop() {
		result, err := analysis.RunPackages(context.Background(), registry, options, load)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
			b.Fatalf("exec-pipe-run benchmark result = %#v", result)
		}
	}
}
