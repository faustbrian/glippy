package doc

import (
	"slices"
	"testing"
)

func FuzzRenderDeterministic(f *testing.F) {
	f.Add([]byte{0, 1, 4, 0}, uint8(80), uint8(8), uint16(1_000))
	f.Add([]byte{0, 2, 0, 6, 5, 4}, uint8(20), uint8(4), uint16(32))
	f.Add([]byte{0, 8, 9, 0, 11}, uint8(1), uint8(1), uint16(1))

	f.Fuzz(func(t *testing.T, shape []byte, rawWidth, rawTabWidth uint8, rawBudget uint16) {
		if len(shape) > 4<<10 {
			t.Skip()
		}
		arena := NewArena()
		documents := []ID{arena.Empty()}
		for offset, operation := range shape {
			switch operation % 12 {
			case 0:
				documents = append(documents, arena.Text(string(rune('a'+operation%26))))
			case 1:
				documents = append(documents, arena.Line())
			case 2:
				documents = append(documents, arena.SoftLine())
			case 3:
				documents = append(documents, arena.HardLine())
			case 4:
				documents[len(documents)-1] = arena.Group(documents[len(documents)-1])
			case 5:
				documents[len(documents)-1] = arena.Indent(documents[len(documents)-1])
			case 6:
				if len(documents) >= 2 {
					last := len(documents) - 1
					documents[last-1] = arena.Concat(documents[last-1], documents[last])
					documents = documents[:last]
				}
			case 7:
				if len(documents) >= 2 {
					last := len(documents) - 1
					documents[last-1] = arena.IfBreak(documents[last-1], documents[last])
					documents = documents[:last]
				}
			case 8:
				documents = append(documents, arena.LineSuffix(arena.Text(" // suffix")))
			case 9:
				documents = append(documents, arena.LineSuffixBoundary())
			case 10:
				documents = append(documents, arena.BreakParent())
			case 11:
				documents = append(documents, arena.SourceMarker(SourceMark{Offset: offset}))
			}
		}
		root := arena.Concat(documents...)
		options := Options{
			Width:     1 + int(rawWidth%120),
			TabWidth:  1 + int(rawTabWidth%16),
			FitBudget: 1 + int(rawBudget%256),
		}
		first, firstErr := arena.RenderWithMarkers(root, options)
		if firstErr != nil {
			t.Fatalf("first render failed: %v", firstErr)
		}
		second, secondErr := arena.RenderWithMarkers(root, options)
		if secondErr != nil {
			t.Fatalf("second render failed: %v", secondErr)
		}
		if first.Text != second.Text || !slices.Equal(first.Markers, second.Markers) {
			t.Fatal("repeated rendering produced different output")
		}
	})
}
