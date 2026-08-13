package benchmarks_test

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"testing"
)

type corpusImporter struct {
	base types.Importer
}

func (i corpusImporter) Import(path string) (*types.Package, error) {
	if path == "C" {
		pkg := types.NewPackage("C", "C")
		pkg.Scope().Insert(
			types.NewConst(
				token.NoPos,
				pkg,
				"GLIPPY_DIRECTIVE_CORPUS",
				types.Typ[types.UntypedInt],
				constant.MakeInt64(1),
			),
		)
		pkg.MarkComplete()
		return pkg, nil
	}
	return i.base.Import(path)
}

func TestInitialCorpusIsValidGo(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("../testdata/corpus/hostile/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("initial corpus is empty")
	}

	for _, path := range paths {
		path := path
		t.Run(
			filepath.Base(path),
			func(t *testing.T) {
				t.Parallel()
				if _, err := parser.ParseFile(
					token.NewFileSet(),
					path,
					nil,
					parser.ParseComments,
				);
					err != nil {
					t.Fatal(err)
				}
			},
		)
	}
}

func TestInitialCorpusTypeChecks(t *testing.T) {
	t.Parallel()

	files := token.NewFileSet()
	packages, err := parser.ParseDir(
		files,
		"../testdata/corpus/hostile",
		nil,
		parser.ParseComments,
	)
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages["hostile"]
	if pkg == nil {
		t.Fatal("hostile corpus package is missing")
	}

	names := make([]string, 0, len(pkg.Files))
	for name := range pkg.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	parsed := make([]*ast.File, 0, len(names))
	for _, name := range names {
		parsed = append(parsed, pkg.Files[name])
	}

	// import "C" is resolved by cgo rather than the ordinary source importer.
	// The corpus uses one inert marker symbol, so a package shell keeps the Go
	// declarations type-checked without invoking cgo or executing repository code.
	config := &types.Config{Importer: corpusImporter{base: importer.Default()}}
	if _, err := config.Check("example.com/glippy-corpus/hostile", files, parsed, nil);
		err != nil {
		t.Fatal(err)
	}
}
