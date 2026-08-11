package doc

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestRenderCallUsesCanonicalFlatAndBrokenForms(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	call := arena.Group(arena.Concat(
		arena.Text("call("),
		arena.Indent(arena.Concat(
			arena.SoftLine(),
			arena.Text("first,"),
			arena.Line(),
			arena.Text("second"),
			arena.IfBreak(arena.Text(","), arena.Empty()),
		)),
		arena.SoftLine(),
		arena.Text(")"),
	))

	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "flat", width: 80, want: "call(first, second)"},
		{name: "broken", width: 12, want: "call(\n\tfirst,\n\tsecond,\n)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := arena.Render(call, Options{Width: test.width, TabWidth: 8, FitBudget: 1_000})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("Render() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderGroupWithIndependentTailSeparatesAdjacentLayoutDecisions(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	calleeBody := arena.Concat(
		arena.Text("client."),
		arena.Indent(arena.Concat(
			arena.SoftLine(),
			arena.Text("Call"),
		)),
	)
	arguments := arena.Group(arena.Concat(
		arena.Text("("),
		arena.Indent(arena.Concat(
			arena.SoftLine(),
			arena.Text("firstArgument,"),
			arena.Line(),
			arena.Text("secondArgument"),
			arena.IfBreak(arena.Text(","), arena.Empty()),
		)),
		arena.SoftLine(),
		arena.Text(")"),
	))
	document := arena.GroupWithIndependentTail(calleeBody, arena.Text("("), arguments)

	for _, test := range []struct {
		name  string
		width int
		want  string
	}{
		{
			name:  "callee and opening delimiter fit",
			width: 12,
			want:  "client.Call(\n\tfirstArgument,\n\tsecondArgument,\n)",
		},
		{
			name:  "opening delimiter exceeds width",
			width: 11,
			want:  "client.\n\tCall(\n\t\tfirstArgument,\n\t\tsecondArgument,\n\t)",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := arena.Render(document, Options{Width: test.width, TabWidth: 8, FitBudget: 1_000})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("Render() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderBinaryChainBreaksAfterOperators(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	condition := arena.Group(arena.Concat(
		arena.Text("if foo &&"),
		arena.Indent(arena.Concat(
			arena.Line(),
			arena.Text("bar &&"),
			arena.Line(),
			arena.Text("baz"),
		)),
		arena.Text(" {"),
	))

	got, err := arena.Render(condition, Options{Width: 12, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "if foo &&\n\tbar &&\n\tbaz {"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestRenderRepresentativeHostileStatementsWithHardLines(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	block := arena.Concat(
		arena.Text("if _, err := client.Discover(nil); !errors.Is(err, ErrContextRequired) {"),
		arena.Indent(arena.Concat(
			arena.HardLine(),
			arena.Text("t.Fatal(err)"),
		)),
		arena.HardLine(),
		arena.Text("}"),
	)
	statements := arena.Concat(
		arena.Text("ctx, cancel := context.WithCancel(t.Context())"),
		arena.HardLine(),
		arena.Text("cancel()"),
		arena.HardLine(),
		arena.Text("result := work(ctx)"),
	)

	tests := []struct {
		name string
		doc  ID
		want string
	}{
		{
			name: "compressed block",
			doc:  block,
			want: "if _, err := client.Discover(nil); !errors.Is(err, ErrContextRequired) {\n\tt.Fatal(err)\n}",
		},
		{
			name: "ordinary statement semicolons",
			doc:  statements,
			want: "ctx, cancel := context.WithCancel(t.Context())\ncancel()\nresult := work(ctx)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := arena.Render(test.doc, Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("Render() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderFitBudgetConservativelyBreaks(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	parts := make([]ID, 0, 2_000)
	for range 1_000 {
		parts = append(parts, arena.Text("\tx"), arena.SoftLine())
	}
	document := arena.Group(arena.Concat(parts...))

	got, err := arena.Render(document, Options{Width: 100_000, TabWidth: 8, FitBudget: 8})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\n") {
		t.Fatal("fit budget exhaustion must select the conservative broken form")
	}
}

func TestRenderMeasuresTabsFromTheCurrentColumn(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Group(arena.Concat(
		arena.Text("1234567"),
		arena.Text("\tX"),
		arena.Line(),
		arena.Text("Y"),
	))

	got, err := arena.Render(document, Options{Width: 11, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "1234567\tX Y" {
		t.Fatalf("Render() = %q, want the tab to advance from column 7 to column 8", got)
	}
}

func TestRenderMeasuresAGroupFromPendingIndentation(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Concat(
		arena.Text("{"),
		arena.Indent(arena.Concat(
			arena.HardLine(),
			arena.Group(arena.Concat(
				arena.Text("a"),
				arena.Line(),
				arena.Text("b"),
			)),
		)),
	)

	got, err := arena.Render(document, Options{Width: 9, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "{\n\ta\n\tb" {
		t.Fatalf("Render() = %q, want a broken group measured from the indented column", got)
	}
}

func TestRenderVerbatimPreservesMultilineContent(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Concat(
		arena.Verbatim("`first\nsecond`"),
		arena.HardLine(),
		arena.Text("after"),
	)

	got, err := arena.Render(document, Options{Width: 80, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "`first\nsecond`\nafter" {
		t.Fatalf("Render() = %q, want exact multiline verbatim content", got)
	}
}

func TestRenderRejectsNewlinesInText(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	_, err := arena.Render(arena.Text("first\nsecond"), Options{Width: 80, TabWidth: 8, FitBudget: 1_000})
	if err == nil {
		t.Fatal("Render() must require multiline content to use Verbatim")
	}
}

func TestRenderDeepNestingIsIterative(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Text("x")
	for range 20_000 {
		document = arena.Group(document)
	}

	got, err := arena.Render(document, Options{Width: 80, TabWidth: 8, FitBudget: 32})
	if err != nil {
		t.Fatal(err)
	}
	if got != "x" {
		t.Fatalf("Render() = %q, want x", got)
	}
}

func TestRenderLineSuffixBeforeBoundary(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Concat(
		arena.Text("value"),
		arena.LineSuffix(arena.Text(" // trailing")),
		arena.LineSuffixBoundary(),
		arena.Text("next"),
	)

	got, err := arena.Render(document, Options{Width: 80, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "value // trailing\nnext" {
		t.Fatalf("Render() = %q, want the suffix before its forced boundary", got)
	}
}

func TestRenderBreakParentForcesBrokenGroup(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Group(arena.Concat(
		arena.Text("first"),
		arena.Line(),
		arena.Text("second"),
		arena.BreakParent(),
	))

	got, err := arena.Render(document, Options{Width: 80, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "first\nsecond" {
		t.Fatalf("Render() = %q, want the parent group to break", got)
	}
}

func TestRenderBreakParentDoesNotBreakPrecedingSiblingGroup(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Concat(
		arena.Group(arena.Concat(
			arena.Text("a"),
			arena.Line(),
			arena.Text("b"),
		)),
		arena.Text(" "),
		arena.Group(arena.Concat(
			arena.Text("c"),
			arena.BreakParent(),
			arena.Line(),
			arena.Text("d"),
		)),
	)

	got, err := arena.Render(document, Options{Width: 80, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "a b c\nd" {
		t.Fatalf("Render() = %q, want only the group containing BreakParent to break", got)
	}
}

func TestRenderStillMeasuresAForcedBrokenSiblingFirstLine(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	document := arena.Concat(
		arena.Group(arena.Concat(
			arena.Text("aaaa"),
			arena.Line(),
			arena.Text("b"),
		)),
		arena.Text(" "),
		arena.Group(arena.Concat(
			arena.Text("cccc"),
			arena.BreakParent(),
			arena.Line(),
			arena.Text("d"),
		)),
	)

	got, err := arena.Render(document, Options{Width: 8, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != "aaaa\nb cccc\nd" {
		t.Fatalf("Render() = %q, want sibling first lines included in fit decisions", got)
	}
}

func TestRenderRecordsSourceMarkers(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	mark := SourceMark{Offset: 42}
	document := arena.Concat(
		arena.Text("a"),
		arena.SourceMarker(mark),
		arena.Text("bc"),
	)

	got, err := arena.RenderWithMarkers(document, Options{Width: 80, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "abc" {
		t.Fatalf("RenderWithMarkers().Text = %q, want abc", got.Text)
	}
	want := []RenderedMarker{{Source: mark, OutputOffset: 1}}
	if !slices.Equal(got.Markers, want) {
		t.Fatalf("RenderWithMarkers().Markers = %#v, want %#v", got.Markers, want)
	}
}

func TestRenderRecordsSourceMarkerAfterPendingIndentation(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	mark := SourceMark{Offset: 42}
	document := arena.Concat(
		arena.Text("{"),
		arena.Indent(arena.Concat(
			arena.HardLine(),
			arena.SourceMarker(mark),
			arena.Text("x"),
		)),
	)

	got, err := arena.RenderWithMarkers(document, Options{Width: 80, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := []RenderedMarker{{Source: mark, OutputOffset: 3}}
	if got.Text != "{\n\tx" || !slices.Equal(got.Markers, want) {
		t.Fatalf("RenderWithMarkers() = %#v, want indented text with marker at byte 3", got)
	}
}

func TestRenderRejectsForeignArenaReference(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	arena.Text("local")
	foreign := NewArena().Text("foreign")
	root := arena.Group(foreign)
	if _, err := arena.Render(root, Options{Width: 80, TabWidth: 8, FitBudget: 32}); err == nil {
		t.Fatal("Render() must reject document references owned by another arena")
	}
}

func TestRenderRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	arena := NewArena()
	_, err := arena.Render(arena.Text("x"), Options{})
	if err == nil {
		t.Fatal("Render() must reject a non-positive width, tab width, and fit budget")
	}
}

func TestRenderBoundsAdversarialDepthAndBreadthAllocations(t *testing.T) {
	const (
		depth   = 100_000
		breadth = 20_000
	)
	options := Options{Width: breadth + 1, TabWidth: 8, FitBudget: 32}

	deepArena := NewArena()
	deepDocument := deepArena.Text("x")
	for range depth {
		deepDocument = deepArena.Group(deepDocument)
	}
	var deepOutput string
	var deepErr error
	deepAllocations := testing.AllocsPerRun(5, func() {
		deepOutput, deepErr = deepArena.Render(deepDocument, options)
	})
	if deepErr != nil {
		t.Fatal(deepErr)
	}
	if deepOutput != "x" {
		t.Fatalf("deep Render() = %q, want x", deepOutput)
	}
	if deepAllocations > 2 {
		t.Fatalf("deep Render() allocations = %.0f, want at most 2", deepAllocations)
	}

	wideArena := NewArena()
	parts := make([]ID, 0, breadth)
	for range breadth {
		parts = append(parts, wideArena.Group(wideArena.Text("x")))
	}
	wideDocument := wideArena.Concat(parts...)
	var wideOutput string
	var wideErr error
	wideAllocations := testing.AllocsPerRun(5, func() {
		wideOutput, wideErr = wideArena.Render(wideDocument, options)
	})
	if wideErr != nil {
		t.Fatal(wideErr)
	}
	if len(wideOutput) != breadth || strings.Trim(wideOutput, "x") != "" {
		t.Fatalf("wide Render() produced %d unexpected bytes", len(wideOutput))
	}
	if wideAllocations > 64 {
		t.Fatalf("wide Render() allocations = %.0f, want at most 64", wideAllocations)
	}
}

func BenchmarkRenderAdversarialNesting(b *testing.B) {
	for _, depth := range []int{20_000, 100_000} {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			arena := NewArena()
			document := arena.Text("x")
			for range depth {
				document = arena.Group(document)
			}
			options := Options{Width: 80, TabWidth: 8, FitBudget: 32}
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				if _, err := arena.Render(document, options); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRenderAdversarialSiblings(b *testing.B) {
	for _, count := range []int{1_000, 2_000, 4_000, 8_000, 16_000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			arena := NewArena()
			parts := make([]ID, 0, count)
			for range count {
				parts = append(parts, arena.Group(arena.Text("x")))
			}
			document := arena.Concat(parts...)
			options := Options{Width: count + 1, TabWidth: 8, FitBudget: 32}
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				if _, err := arena.Render(document, options); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
