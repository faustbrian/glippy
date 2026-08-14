package rulecatalog

import (
	"flag"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/waitgroup"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

func printfArgumentsRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		analyzerWithoutFlags(printf.Analyzer),
		rules.Metadata{
			ID: "printf-arguments",
			Summary: "detects invalid Printf-style format strings and arguments",
			Documentation: "Printf-style calls can silently render malformed output when a directive is invalid, an argument is missing, or an argument has the wrong type. Glippy adapts the standard Go printf analyzer, including its wrapper facts, through the shared typed scheduler and deterministic diagnostic contract.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule follows the standard printf analyzer's recognized formatting functions and inferred wrappers.",
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
	)
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
	return adaptStandardAnalyzer(
		waitgroup.Analyzer,
		rules.Metadata{
			ID: "waitgroup-misuse",
			Summary: "detects WaitGroup.Add calls made inside launched goroutines",
			Documentation: "Calling WaitGroup.Add from inside the goroutine being counted races with Wait: the waiting goroutine may observe a zero count and return before Add executes. The count must be incremented before launching the goroutine. Glippy adapts the standard Go waitgroup analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{
				rules.CategoryCorrectness,
				rules.CategorySafety,
			},
			KnownLimitations: []string{
				"The analyzer reports the direct pattern where WaitGroup.Add is the first statement of a newly launched function literal.",
				"More indirect counter ownership and synchronization protocols require separate concurrency analysis.",
			},
			Examples: []rules.Example{
				{
					Title: "Increment before launching",
					Incorrect: "go func() { wg.Add(1); defer wg.Done() }()",
					Correct: "wg.Add(1)\ngo func() { defer wg.Done() }()",
				},
			},
		},
		nil,
	)
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
