package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestChannelUsedAfterCloseReportsDefiniteSendAndClose(t *testing.T) {
	t.Parallel()

	input := `package sample

func run() {
	channel := make(chan int)
	close(channel)
	channel <- 1
	close(channel)
}
`
	result := runChannelUsedAfterClose(t, input)
	want := []struct {
		expression string
		messageKey string
	}{
		{expression: "channel <- 1", messageKey: "send-after-close"},
		{expression: "close(channel)", messageKey: "close-after-close"},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("channel-used-after-close result = %#v", result)
	}
	for index, expected := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		occurrence := 0
		if expected.messageKey == "close-after-close" {
			occurrence = 1
		}
		start := nthIndex(input, expected.expression, occurrence)
		if diagnostic.RuleID != "channel-used-after-close" ||
			diagnostic.MessageKey != expected.messageKey ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expected.expression) ||
			!strings.Contains(diagnostic.Message, "after") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		closeStart := nthIndex(input, "close(channel)", 0)
		if len(diagnostic.Related) != 1 ||
			diagnostic.Related[0].Range.Start != closeStart ||
			diagnostic.Related[0].Range.End != closeStart + len("close(channel)") {
			t.Fatalf("diagnostic[%d] related = %#v", index, diagnostic.Related)
		}
	}
}

func TestChannelUsedAfterClosePropagatesDefiniteStateAcrossControlFlow(t *testing.T) {
	t.Parallel()

	input := `package sample

func branches(closeLeft bool) {
	channel := make(chan int)
	if closeLeft {
		close(channel)
	} else {
		close(channel)
	}
	channel <- 1
}

func loop() {
	channel := make(chan int)
	for {
		close(channel)
		break
	}
	close(channel)
}

func declaration() {
	var channel = make(chan int)
	close(channel)
	_ = len(channel)
	_ = cap(channel)
	println(channel)
	channel <- 1
}
`
	result := runChannelUsedAfterClose(t, input)
	want := []struct {
		expression string
		occurrence int
		messageKey string
		related bool
	}{
		{expression: "channel <- 1", messageKey: "send-after-close"},
		{
			expression: "close(channel)",
			occurrence: 3,
			messageKey: "close-after-close",
			related: true,
		},
		{
			expression: "channel <- 1",
			occurrence: 1,
			messageKey: "send-after-close",
			related: true,
		},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("control-flow channel-used-after-close result = %#v", result)
	}
	for index, expected := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		start := nthIndex(input, expected.expression, expected.occurrence)
		if diagnostic.RuleID != "channel-used-after-close" ||
			diagnostic.MessageKey != expected.messageKey ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expected.expression) {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		if expected.related != (len(diagnostic.Related) == 1) {
			t.Fatalf("diagnostic[%d] related = %#v", index, diagnostic.Related)
		}
	}
}

func TestChannelUsedAfterCloseHandlesChannelOperandsAndSelectSends(t *testing.T) {
	t.Parallel()

	input := `package sample

type recursiveChannel chan recursiveChannel

func channelValue() {
	channel := make(recursiveChannel)
	close(channel)
	channel <- channel
}

func selected() {
	channel := make(chan int)
	close(channel)
	select {
	case channel <- 1:
	default:
	}
}

func directional() {
	var channel chan<- int = make(chan int)
	close(channel)
	channel <- 1
}
`
	result := runChannelUsedAfterClose(t, input)
	want := []struct {
		expression string
		occurrence int
	}{
		{expression: "channel <- channel"},
		{expression: "channel <- 1"},
		{expression: "channel <- 1", occurrence: 1},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("channel operand result = %#v", result)
	}
	for index, expected := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		start := nthIndex(input, expected.expression, expected.occurrence)
		if diagnostic.RuleID != "channel-used-after-close" ||
			diagnostic.MessageKey != "send-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expected.expression) {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestChannelUsedAfterCloseKeepsObjectsAndFunctionsIsolated(t *testing.T) {
	t.Parallel()

	input := `package sample

func channels() {
	first := make(chan int)
	second := make(chan int)
	close(first)
	close(second)
	second <- 1
	first <- 1
}

func captured() {
	channel := make(chan int)
	close(channel)
	func() { channel <- 1 }()
	channel <- 2
}

func shadowedClose() {
	close := func(chan int) {}
	channel := make(chan int)
	close(channel)
	channel <- 1
}

func shadowedMake() {
	make := func() chan int { return nil }
	channel := make()
	close(channel)
	channel <- 1
}
`
	result := runChannelUsedAfterClose(t, input)
	want := []string{"second <- 1", "first <- 1"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("isolated channel-used-after-close result = %#v", result)
	}
	for index, expression := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, expression)
		if diagnostic.MessageKey != "send-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expression) {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestChannelUsedAfterCloseReestablishesClosedStateAfterUnknownUse(t *testing.T) {
	t.Parallel()

	input := `package sample

func use(chan int) {}

func escaped() {
	channel := make(chan int)
	use(channel)
	close(channel)
	channel <- 1
}

func aliased() {
	channel := make(chan int)
	other := channel
	_ = other
	close(channel)
	channel <- 1
}
`
	result := runChannelUsedAfterClose(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("reestablished channel-used-after-close result = %#v", result)
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		start := nthIndex(input, "channel <- 1", index)
		closeStart := nthIndex(input, "close(channel)", index)
		if diagnostic.MessageKey != "send-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("channel <- 1") ||
			len(diagnostic.Related) != 1 ||
			diagnostic.Related[0].Range.Start != closeStart {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestChannelUsedAfterCloseRemainsConservative(t *testing.T) {
	t.Parallel()

	input := `package sample

var global = make(chan int)

func use(chan int) {}

func conditional(closeNow bool) {
	channel := make(chan int)
	if closeNow { close(channel) }
	channel <- 1
}

func receive() {
	channel := make(chan int)
	close(channel)
	_, _ = <-channel
}

func deferred() {
	channel := make(chan int)
	defer close(channel)
	channel <- 1
}

func asynchronous() {
	channel := make(chan int)
	go close(channel)
	channel <- 1
}

func alias() {
	channel := make(chan int)
	other := channel
	close(other)
	channel <- 1
}

func declarationAlias() {
	channel := make(chan int)
	close(channel)
	var other = channel
	_ = other
	channel <- 1
}

func escaped() {
	channel := make(chan int)
	close(channel)
	use(channel)
	channel <- 1
}

func remade() {
	channel := make(chan int)
	close(channel)
	channel = make(chan int)
	channel <- 1
}

func captured() {
	channel := make(chan int)
	close(channel)
	closure := func() { _ = channel }
	_ = closure
	channel <- 1
}

func parameter(channel chan int) {
	close(channel)
	channel <- 1
}

func globalChannel() {
	close(global)
	global <- 1
}

func resetGlobal() {
	global = make(chan int)
	close(global)
	global <- 1
}

func indirectMake() {
	makeChannel := func() chan int { return make(chan int) }
	channel := makeChannel()
	close(channel)
	channel <- 1
}

func indirectClose() {
	channel := make(chan int)
	closeChannel := func(chan int) {}
	closeChannel(channel)
	channel <- 1
}
`
	result := runChannelUsedAfterClose(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("conservative channel-used-after-close result = %#v", result)
	}
}

func TestChannelUsedAfterCloseMetadataAndEligibility(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("channel-used-after-close")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		metadata.RequiresEffectFacts ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		metadata.MinimumGoVersion != "1.25" {
		t.Fatalf("channel-used-after-close metadata = %#v, found = %v", metadata, found)
	}

	suppressed := runChannelUsedAfterClose(
		t,
		`package sample
func run() {
	channel := make(chan int)
	close(channel)
	//glippy:ignore channel-used-after-close -- compatibility probe
	channel <- 1
}
`,
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 {
		t.Fatalf("suppressed channel-used-after-close result = %#v", suppressed)
	}

	for name, input := range
		map[string]string{
			"generated": `// Code generated by test. DO NOT EDIT.
package sample
func run() { channel := make(chan int); close(channel); channel <- 1 }
`,
			"type-error": `package sample
func run() { channel := make(chan int); missing(); close(channel); channel <- 1 }
`,
		} {
		result := runChannelUsedAfterClose(t, input)
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
			t.Fatalf("%s channel-used-after-close result = %#v", name, result)
		}
		if name == "type-error" && len(result.LoadDiagnostics) == 0 {
			t.Fatalf("type-error result has no load diagnostics: %#v", result)
		}
	}

	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"channel-used-after-close": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 {
		t.Fatalf("go1.24 channel-used-after-close selection = %#v", older)
	}
}

func BenchmarkChannelUsedAfterClosePackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/channelusebenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() { channel := make(chan int); close(channel); channel <- 1 }\n",
			index,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
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
				"channel-used-after-close": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		100,
	)
}

func runChannelUsedAfterClose(t testing.TB, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/channeluse\n\ngo 1.25.0\n",
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
				"channel-used-after-close": rules.SeverityWarn,
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
	return result
}
