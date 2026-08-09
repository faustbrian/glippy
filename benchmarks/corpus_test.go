package benchmarks_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"testing"
)

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
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			if _, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitialCorpusTypeChecks(t *testing.T) {
	t.Parallel()

	files := token.NewFileSet()
	packages, err := parser.ParseDir(files, "../testdata/corpus/hostile", nil, parser.ParseComments)
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

	config := &types.Config{Importer: importer.Default()}
	if _, err := config.Check("example.com/gox-corpus/hostile", files, parsed, nil); err != nil {
		t.Fatal(err)
	}
}
