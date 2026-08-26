package format

import (
	"testing"

	"github.com/faustbrian/glippy/internal/source"
)

func TestCommentsBetweenOwnsOnlyCompleteCommentsInSourceOrder(t *testing.T) {
	t.Parallel()

	lowerer := lowerer{
		comments: []source.Comment{
			{ID: 1, Range: source.Range{Start: 2, End: 6}, Raw: "/*a*/"},
			{ID: 2, Range: source.Range{Start: 8, End: 12}, Raw: "/*b*/"},
			{ID: 3, Range: source.Range{Start: 14, End: 18}, Raw: "/*c*/"},
			{ID: 4, Range: source.Range{Start: 20, End: 24}, Raw: "/*d*/"},
			{ID: 5, Range: source.Range{Start: 26, End: 30}, Raw: "/*e*/"},
		},
		emittedComment: []bool{false, true, false, false, false},
	}

	got := lowerer.commentsBetween(6, 24)
	if len(got) != 2 || got[0].ID != 3 || got[1].ID != 4 {
		t.Fatalf("commentsBetween(6, 24) = %#v, want comments 3 then 4", got)
	}
	if !lowerer.emittedComment[2] || !lowerer.emittedComment[3] {
		t.Fatal("commentsBetween did not mark the owned comments emitted")
	}
	if lowerer.emittedComment[0] || lowerer.emittedComment[4] {
		t.Fatal("commentsBetween emitted a comment outside the requested range")
	}
}

func TestCommentsBetweenIncludesEqualBoundaries(t *testing.T) {
	t.Parallel()

	lowerer := lowerer{
		comments: []source.Comment{
			{ID: 1, Range: source.Range{Start: 4, End: 8}, Raw: "/*a*/"},
		},
		emittedComment: make([]bool, 1),
	}

	got := lowerer.commentsBetween(4, 8)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("commentsBetween(4, 8) = %#v, want comment 1", got)
	}
}

func TestCommentsBetweenReturnsNoPartialOrAbsentComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		start int
		end int
	}{
		{name: "empty range", start: 8, end: 8},
		{name: "starts inside comment", start: 6, end: 8},
		{name: "ends inside comment", start: 4, end: 6},
		{name: "after comment", start: 9, end: 12},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				lowerer := lowerer{
					comments: []source.Comment{
						{
							ID: 1,
							Range: source.Range{Start: 4, End: 8},
							Raw: "/*a*/",
						},
					},
					emittedComment: make([]bool, 1),
				}
				if got := lowerer.commentsBetween(test.start, test.end);
					len(got) != 0 {
					t.Fatalf(
						"commentsBetween(%d, %d) = %#v, want no comments",
						test.start,
						test.end,
						got,
					)
				}
			},
		)
	}
}

func BenchmarkCommentsBetweenLateRange(b *testing.B) {
	comments := make([]source.Comment, 100_000)
	for index := range comments {
		start := index * 8
		comments[index] = source.Comment{
			ID: source.CommentID(index + 1),
			Range: source.Range{Start: start, End: start + 4},
			Raw: "// x",
		}
	}
	start := comments[len(comments) - 1].Range.Start
	end := comments[len(comments) - 1].Range.End
	lowerer := lowerer{comments: comments, emittedComment: make([]bool, len(comments))}

	b.ReportAllocs()
	for range b.N {
		lowerer.emittedComment[len(comments) - 1] = false
		if got := lowerer.commentsBetween(start, end); len(got) != 1 {
			b.Fatalf(
				"commentsBetween(%d, %d) returned %d comments, want 1",
				start,
				end,
				len(got),
			)
		}
	}
}
