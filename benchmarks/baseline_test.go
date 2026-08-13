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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/cli"
	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
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
		if _, err := parser.ParseFile(
			token.NewFileSet(),
			"hostile.go",
			workload,
			parser.ParseComments,
		);
			err != nil {
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

func BenchmarkGoxFormatManyClassicLoops(b *testing.B) {
	for _, loops := range []int{100, 1_000} {
		b.Run(
			strconv.Itoa(loops),
			func(b *testing.B) {
				input := []byte(
					"package benchmark\nfunc run(ready bool){" +
						strings.Repeat("for ;ready;{work()};", loops) +
						"}\n",
				)
				file, err := source.Load("many_loops.go", input)
				if err != nil {
					b.Fatal(err)
				}
				options := goxformat.Options{
					Width: 100,
					TabWidth: 8,
					FitBudget: 10_000,
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				b.ResetTimer()

				for b.Loop() {
					if _, err := goxformat.File(file, options); err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

func BenchmarkGoxEditorStdin(b *testing.B) {
	path, err := filepath.Abs("testdata/workload/hostile.go")
	if err != nil {
		b.Fatal(err)
	}
	arguments := []string{"fmt", "--stdin-filepath=" + path}
	if exitCode := cli.Run(arguments, bytes.NewReader(workload), io.Discard, io.Discard);
		exitCode != cli.ExitSuccess {
		b.Fatalf("editor workload exit code = %d, want %d", exitCode, cli.ExitSuccess)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(workload)))
	b.ResetTimer()

	for b.Loop() {
		if exitCode := cli.Run(
			arguments,
			bytes.NewReader(workload),
			io.Discard,
			io.Discard,
		);
			exitCode != cli.ExitSuccess {
			b.Fatalf(
				"editor workload exit code = %d, want %d",
				exitCode,
				cli.ExitSuccess,
			)
		}
	}
}

func BenchmarkASTInspect(b *testing.B) {
	file := parseWorkload(b)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		count := 0
		ast.Inspect(
			file,
			func(node ast.Node) bool {
				if node != nil {
					count++
				}
				return true
			},
		)
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
		inspect.Preorder(
			nodes,
			func(ast.Node) {
				count++
			},
		)
		if count == 0 {
			b.Fatal("empty filtered traversal")
		}
	}
}

func BenchmarkSyntaxRuleTraversalStrategies(b *testing.B) {
	file := parseWorkload(b)
	for _, ruleCount := range []int{1, 3, 5, 10, 25} {
		ruleSet := benchmarkSyntaxRules(ruleCount)
		wantVisits := runNaiveSyntaxRules(file, ruleSet)
		b.Run(
			strconv.Itoa(ruleCount),
			func(b *testing.B) {
				b.Run(
					"direct",
					func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							if visits := runDirectSyntaxRules(
								file,
								ruleSet,
							);
								visits != wantVisits {
								b.Fatalf(
									"direct visits = %d, want %d",
									visits,
									wantVisits,
								)
							}
						}
						b.ReportMetric(float64(wantVisits), "callbacks/op")
					},
				)
				b.Run(
					"inspector",
					func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							if visits := runInspectorSyntaxRules(
								file,
								ruleSet,
							);
								visits != wantVisits {
								b.Fatalf(
									"inspector visits = %d, want %d",
									visits,
									wantVisits,
								)
							}
						}
						b.ReportMetric(float64(wantVisits), "callbacks/op")
					},
				)
				b.Run(
					"naive",
					func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							if visits := runNaiveSyntaxRules(
								file,
								ruleSet,
							);
								visits != wantVisits {
								b.Fatalf(
									"naive visits = %d, want %d",
									visits,
									wantVisits,
								)
							}
						}
						b.ReportMetric(float64(wantVisits), "callbacks/op")
					},
				)
			},
		)
	}
}

func runDirectSyntaxRules(file *ast.File, ruleSet []benchmarkSyntaxRule) int {
	dispatch := make(map[rules.NodeKind][]benchmarkSyntaxRule, 3)
	for _, rule := range ruleSet {
		dispatch[rule.interest] = append(dispatch[rule.interest], rule)
	}
	visits := 0
	ast.Inspect(
		file,
		func(node ast.Node) bool {
			interest, found := rules.KindOf(node)
			if !found {
				return true
			}
			for _, rule := range dispatch[interest] {
				visits += rule.run(node)
			}
			return true
		},
	)
	return visits
}

var benchmarkSyntaxInterests = []rules.NodeKind{
	rules.NodeCallExpr,
	rules.NodeBinaryExpr,
	rules.NodeFuncDecl,
}

type benchmarkSyntaxRule struct {
	interest rules.NodeKind
	run func(ast.Node) int
}

func benchmarkSyntaxRules(count int) []benchmarkSyntaxRule {
	rules := make([]benchmarkSyntaxRule, count)
	for index := range rules {
		rules[index] = benchmarkSyntaxRule{
			interest: benchmarkSyntaxInterests[index % len(benchmarkSyntaxInterests)],
			run: func(ast.Node) int {
				return 1
			},
		}
	}
	return rules
}

func runInspectorSyntaxRules(file *ast.File, ruleSet []benchmarkSyntaxRule) int {
	dispatch := make(map[rules.NodeKind][]benchmarkSyntaxRule, 3)
	for _, rule := range ruleSet {
		dispatch[rule.interest] = append(dispatch[rule.interest], rule)
	}
	filter := make([]ast.Node, 0, len(dispatch))
	for _, interest := range benchmarkSyntaxInterests {
		if len(dispatch[interest]) == 0 {
			continue
		}
		prototype, found := rules.NodePrototype(interest)
		if !found {
			panic("benchmark syntax interest has no prototype")
		}
		filter = append(filter, prototype)
	}
	visits := 0
	inspect := inspector.New([]*ast.File{file})
	inspect.Preorder(
		filter,
		func(node ast.Node) {
			interest, found := rules.KindOf(node)
			if !found {
				panic("unexpected filtered syntax node")
			}
			for _, rule := range dispatch[interest] {
				visits += rule.run(node)
			}
		},
	)
	return visits
}

func runNaiveSyntaxRules(file *ast.File, ruleSet []benchmarkSyntaxRule) int {
	visits := 0
	for _, rule := range ruleSet {
		ast.Inspect(
			file,
			func(node ast.Node) bool {
				if benchmarkSyntaxNodeMatches(rule.interest, node) {
					visits += rule.run(node)
				}
				return true
			},
		)
	}
	return visits
}

func benchmarkSyntaxNodeMatches(interest rules.NodeKind, node ast.Node) bool {
	kind, matches := rules.KindOf(node)
	return matches && kind == interest
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
			Defs: make(map[*ast.Ident]types.Object),
			Uses: make(map[*ast.Ident]types.Object),
			Types: make(map[ast.Expr]types.TypeAndValue),
		}
		config := &types.Config{Importer: importer.Default()}
		if _, err := config.Check("example.com/workload", files, []*ast.File{file}, info);
			err != nil {
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
	file, err := parser.ParseFile(
		token.NewFileSet(),
		"hostile.go",
		workload,
		parser.ParseComments,
	)
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
		Env: replaceEnvironment(
			os.Environ(),
			map[string]string{"GOCACHE": cache, "GOWORK": "off"},
		),
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
	result := make([]string, 0, len(current) + len(replacements))
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
		result = append(result, name + "=" + value)
	}
	sort.Strings(result)
	return result
}
