package benchmarks_test

import (
	"bytes"
	"errors"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
)

var workload = mustRead("testdata/workload/hostile.go")

func BenchmarkScan(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(workload)))

	for b.Loop() {
		files := token.NewFileSet()
		file := files.AddFile("hostile.go", -1, len(workload))
		var scan scanner.Scanner
		scan.Init(file, workload, nil, scanner.ScanComments)
		for {
			_, tok, _ := scan.Scan()
			if tok == token.EOF {
				break
			}
		}
	}
}

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(workload)))

	for b.Loop() {
		if _, err := parser.ParseFile(token.NewFileSet(), "hostile.go", workload, parser.ParseComments); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoFormat(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(workload)))

	for b.Loop() {
		if _, err := format.Source(workload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkASTInspect(b *testing.B) {
	file := parseWorkload(b)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		count := 0
		ast.Inspect(file, func(node ast.Node) bool {
			if node != nil {
				count++
			}
			return true
		})
		if count == 0 {
			b.Fatal("empty traversal")
		}
	}
}

func BenchmarkInspectorBuildAndFilter(b *testing.B) {
	file := parseWorkload(b)
	nodes := []ast.Node{(*ast.CallExpr)(nil), (*ast.BinaryExpr)(nil), (*ast.FuncDecl)(nil)}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		count := 0
		inspect := inspector.New([]*ast.File{file})
		inspect.Preorder(nodes, func(ast.Node) {
			count++
		})
		if count == 0 {
			b.Fatal("empty filtered traversal")
		}
	}
}

func BenchmarkTypeCheck(b *testing.B) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "hostile.go", workload, parser.ParseComments)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		info := &types.Info{
			Defs:  make(map[*ast.Ident]types.Object),
			Uses:  make(map[*ast.Ident]types.Object),
			Types: make(map[ast.Expr]types.TypeAndValue),
		}
		config := &types.Config{Importer: importer.Default()}
		if _, err := config.Check("example.com/workload", files, []*ast.File{file}, info); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPackagesLoadSyntaxColdBuildCache(b *testing.B) {
	dir, err := filepath.Abs("testdata/workload")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()
		cache, err := os.MkdirTemp("", "gox-package-load-cold-")
		if err != nil {
			b.Fatal(err)
		}
		config := packageLoadConfig(dir, cache)
		b.StartTimer()

		err = loadPackages(config)

		b.StopTimer()
		cleanupErr := os.RemoveAll(cache)
		b.StartTimer()
		if err != nil {
			b.Fatal(err)
		}
		if cleanupErr != nil {
			b.Fatal(cleanupErr)
		}
	}
}

func BenchmarkPackagesLoadSyntaxWarmBuildCache(b *testing.B) {
	dir, err := filepath.Abs("testdata/workload")
	if err != nil {
		b.Fatal(err)
	}
	config := packageLoadConfig(dir, b.TempDir())
	if err := loadPackages(config); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := loadPackages(config); err != nil {
			b.Fatal(err)
		}
	}
}

func TestBaselineWorkload(t *testing.T) {
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, "hostile.go", workload, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Comments) == 0 {
		t.Fatal("workload must exercise comment handling")
	}

	formatted, err := format.Source(workload)
	if err != nil {
		t.Fatal(err)
	}
	formattedAgain, err := format.Source(formatted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(formatted, formattedAgain) {
		t.Fatal("go/format baseline is not idempotent")
	}
}

func TestEnvironmentIsReported(t *testing.T) {
	t.Logf("go=%s os=%s arch=%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func parseWorkload(tb testing.TB) *ast.File {
	tb.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "hostile.go", workload, parser.ParseComments)
	if err != nil {
		tb.Fatal(err)
	}
	return file
}

func mustRead(name string) []byte {
	data, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

func packageLoadConfig(dir, cache string) *packages.Config {
	return &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Dir: dir,
		Env: replaceEnvironment(os.Environ(), map[string]string{
			"GOCACHE": cache,
			"GOWORK":  "off",
		}),
	}
}

func loadPackages(config *packages.Config) error {
	loaded, err := packages.Load(config, "./...")
	if err != nil {
		return err
	}
	if packages.PrintErrors(loaded) != 0 {
		return errors.New("package loading reported errors")
	}
	return nil
}

func replaceEnvironment(current []string, replacements map[string]string) []string {
	result := make([]string, 0, len(current)+len(replacements))
	for _, value := range current {
		name, _, found := strings.Cut(value, "=")
		if found {
			if _, replaced := replacements[name]; replaced {
				continue
			}
		}
		result = append(result, value)
	}
	for name, value := range replacements {
		result = append(result, name+"="+value)
	}
	sort.Strings(result)
	return result
}
