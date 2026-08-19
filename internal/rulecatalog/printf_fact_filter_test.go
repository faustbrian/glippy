package rulecatalog

import (
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
)

func TestPrintfDependencyFactFilterRecognizesPossibleWrapperSignatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		files []string
		want bool
	}{
		{
			name: "no variadic declaration",
			files: []string{"package p\nfunc F(value any) {}\n"},
		},
		{
			name: "call expansion only",
			files: []string{"package p\nfunc F(values []any) { G(values...) }\n"},
		},
		{
			name: "concrete variadic type",
			files: []string{"package p\nfunc F(values ...string) {}\n"},
		},
		{
			name: "direct any",
			files: []string{"package p\nfunc F(values ...any) {}\n"},
			want: true,
		},
		{
			name: "empty interface",
			files: []string{"package p\nfunc F(values ...interface{}) {}\n"},
			want: true,
		},
		{
			name: "possible alias",
			files: []string{
				"package p\ntype Values = any\nfunc F(values ...Values) {}\n",
			},
			want: true,
		},
		{
			name: "cross-file shadowed predeclared type",
			files: []string{
				"package p\ntype string = any\n",
				"package p\nfunc F(values ...string) {}\n",
			},
			want: true,
		},
		{
			name: "interface method",
			files: []string{
				"package p\ntype Logger interface { Logf(string, ...any) }\n",
			},
			want: true,
		},
		{
			name: "malformed source is conservative",
			files: []string{"package p\nfunc F("},
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				sources := make(
					[]analysis.AnalyzerDependencyFactSource,
					len(test.files),
				)
				for index, input := range test.files {
					sources[index] = analysis.AnalyzerDependencyFactSource{
						Path: "/fixture/file.go",
						Bytes: []byte(input),
					}
				}
				got, err := printfDependencyMayExportFacts(sources)
				if err != nil {
					t.Fatal(err)
				}
				if got != test.want {
					t.Fatalf(
						"printfDependencyMayExportFacts() = %t, want %t",
						got,
						test.want,
					)
				}
			},
		)
	}
}
