package rulecatalog

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/unmarshal"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

func printfArgumentsRule() (rules.Rule, error) {
	return adaptStandardAnalyzerWithFactExecution(
		analyzerWithoutFlags(printf.Analyzer),
		rules.Metadata{
			ID: "printf-arguments",
			Summary: "detects invalid Printf-style format strings and arguments",
			Documentation: "Printf-style calls can silently render malformed output when a directive is invalid, an argument is missing, or an argument has the wrong type. Glippy adapts the exact standard Go printf analyzer, including its wrapper facts, through an audited bounded execution mode and the deterministic diagnostic contract.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule follows the standard printf analyzer's recognized formatting functions and inferred wrappers.",
				"The CLI isolates the exact upstream fact graph in one serialized same-binary unitchecker process; internal callers without that runner use the bounded in-process fact scheduler.",
				"The configurable upstream funcs flag is not exposed as a Glippy rule option in this release.",
				"The non-constant-format repair is suggestion-only because adding %s may not reflect the caller's intended formatting contract.",
			},
			Examples: []rules.Example{
				{
					Title: "Match directives to argument types",
					Incorrect: `fmt.Printf("%d", "value")`,
					Correct: `fmt.Printf("%s", "value")`,
				},
			},
		},
		[]analysis.AnalyzerFixMapping{
			{
				Message: `Insert "%s" format string`,
				Name: "insert-string-format",
				Description: "insert an explicit %s format string before the non-constant argument",
				Safety: rules.FixSuggestion,
			},
		},
		&analysis.AnalyzerDependencyFactFilter{
			Identity: "printf-wrapper-signature-v1",
			PackageMayExport: printfDependencyMayExportFacts,
			Audited: true,
		},
		&analysis.AnalyzerExternalFactExecution{
			Identity: analysis.UnitcheckerPrintfIdentity,
			Analyzer: "printf",
			Audited: true,
		},
	)
}

func printfDependencyMayExportFacts(sources []analysis.AnalyzerDependencyFactSource) (bool, error) {
	files := make([]*ast.File, 0, len(sources))
	shadowed := make(map[string]struct{})
	fileSet := token.NewFileSet()
	for _, source := range sources {
		file, err := parser.ParseFile(
			fileSet,
			source.Path,
			source.Bytes,
			parser.SkipObjectResolution,
		)
		if err != nil {
			return true, nil
		}
		files = append(files, file)
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpecification, ok := specification.(*ast.TypeSpec)
				if ok {
					shadowed[typeSpecification.Name.Name] = struct{}{}
				}
			}
		}
	}
	mayExport := false
	for _, file := range files {
		ast.Inspect(
			file,
			func(node ast.Node) bool {
				function, ok := node.(*ast.FuncType)
				if !ok || function.Params == nil || len(function.Params.List) == 0 {
					return true
				}
				last := function.Params.List[len(function.Params.List) - 1]
				variadic, ok := last.Type.(*ast.Ellipsis)
				if ok && printfVariadicElementMayBeAny(variadic.Elt, shadowed) {
					mayExport = true
					return false
				}
				return true
			},
		)
		if mayExport {
			return true, nil
		}
	}
	return false, nil
}

func printfVariadicElementMayBeAny(expression ast.Expr, shadowed map[string]struct{}) bool {
	switch expression := expression.(type) {
	case *ast.Ident:
		if expression.Name == "any" {
			return true
		}
		if _, found := shadowed[expression.Name]; found {
			return true
		}
		return !printfConcretePredeclaredType(expression.Name)
	case *ast.InterfaceType:
		return expression.Methods == nil || len(expression.Methods.List) == 0
	case *ast.ParenExpr:
		return printfVariadicElementMayBeAny(expression.X, shadowed)
	case *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.BadExpr:
		return true
	case *ast.ArrayType,
		*ast.ChanType,
		*ast.FuncType,
		*ast.MapType,
		*ast.StarExpr,
		*ast.StructType:
		return false
	default:
		return true
	}
}

func printfConcretePredeclaredType(name string) bool {
	switch name {
	case "bool",
		"byte",
		"comparable",
		"complex64",
		"complex128",
		"error",
		"float32",
		"float64",
		"int",
		"int8",
		"int16",
		"int32",
		"int64",
		"rune",
		"string",
		"uint",
		"uint8",
		"uint16",
		"uint32",
		"uint64",
		"uintptr":
		return true
	default:
		return false
	}
}

func invalidStructTagRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		structtag.Analyzer,
		rules.Metadata{
			ID: "invalid-struct-tag",
			Summary: "detects malformed or ineffective struct field tags",
			Documentation: "Malformed struct tags are not interpreted consistently by reflection-based encoders, while json and xml tags on unexported fields cannot affect encoding. Duplicate serialization tags can also make field selection ambiguous. Glippy adapts the standard Go structtag analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			RunDespiteTypeErrors: true,
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule follows reflect.StructTag.Get syntax and the standard analyzer's json, xml, and duplicate-tag checks.",
				"Application-specific tag namespaces and encoder behavior are outside this analyzer's contract.",
			},
			Examples: []rules.Example{
				{
					Title: "Use a quoted struct tag value",
					Incorrect: "Field string `json:\"field`",
					Correct: "Field string `json:\"field\"`",
				},
			},
		},
		nil,
	)
}

func invalidUnmarshalTargetRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		unmarshal.Analyzer,
		rules.Metadata{
			ID: "invalid-unmarshal-target",
			Summary: "detects non-pointer values passed to unmarshalling APIs",
			Documentation: "Unmarshal functions need a pointer or interface destination so decoded data can reach the caller. Passing a non-pointer concrete value returns an error instead of populating that value and commonly leaves the intended result unchanged. Glippy adapts the standard Go unmarshal analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The standard analyzer recognizes unmarshalling functions through their typed signatures and known standard-library behavior.",
				"The rule does not determine whether the caller later handles the returned invalid-target error.",
			},
			Examples: []rules.Example{
				{
					Title: "Pass an addressable destination",
					Incorrect: "json.Unmarshal(data, value)",
					Correct: "json.Unmarshal(data, &value)",
				},
			},
		},
		nil,
	)
}

func waitgroupMisuseRule() (rules.Rule, error) {
	return rules.NewWaitGroupMisuseRule(), nil
}

func testingGoroutineCallRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		analyzerWithoutFlags(testinggoroutine.Analyzer),
		rules.Metadata{
			ID: "testing-goroutine-call",
			Summary: "detects test termination methods called from worker goroutines",
			Documentation: "Methods such as testing.T.Fatal, FailNow, and SkipNow terminate only the goroutine that calls them. Invoking them from a worker goroutine does not stop the test goroutine and can let the test continue with invalid state. Glippy adapts the standard Go testinggoroutine analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The experimental upstream subtest check is not enabled.",
				"The analyzer follows direct and statically resolved local goroutine calls from functions accepting *testing.T or *testing.B.",
			},
			Examples: []rules.Example{
				{
					Title: "Report worker failure to the test goroutine",
					Incorrect: "go func() { t.Fatal(\"failed\") }()",
					Correct: "if err := <-result; err != nil { t.Fatal(err) }",
				},
			},
		},
		nil,
	)
}

// analyzerWithoutFlags exposes an analyzer's authoritative default behavior
// without permitting its package-global flag values to become mutable product
// configuration. The returned analyzer owns an empty flag set; its Run and
// prerequisite graph remain the upstream implementation.
func analyzerWithoutFlags(analyzer *goanalysis.Analyzer) *goanalysis.Analyzer {
	clone := *analyzer
	clone.Flags = flag.FlagSet{}
	return &clone
}

func standardMethodSignatureRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		stdmethods.Analyzer,
		rules.Metadata{
			ID: "standard-method-signature",
			Summary: "detects incorrect signatures for conventional standard methods",
			Documentation: "A method named for a well-known standard interface can silently fail to implement that interface when its signature is slightly wrong. Dynamic interface checks then fail even though the method name communicates the opposite intent. Glippy adapts the standard Go stdmethods analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule covers the standard analyzer's fixed catalog of well-known interface method names and signatures.",
				"Methods whose names are deliberately unrelated to the standard contract may require a narrow suppression.",
			},
			Examples: []rules.Example{
				{
					Title: "Match io.WriterTo",
					Incorrect: "func (value) WriteTo(io.Writer) error",
					Correct: "func (value) WriteTo(io.Writer) (int64, error)",
				},
			},
		},
		nil,
	)
}
