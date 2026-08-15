package rulecatalog

import (
	"golang.org/x/tools/go/analysis/passes/appends"
	"golang.org/x/tools/go/analysis/passes/defers"
	"golang.org/x/tools/go/analysis/passes/hostport"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/slog"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unusedresult"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

func oversizedShiftRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		shift.Analyzer,
		rules.Metadata{
			ID: "oversized-shift",
			Summary: "detects shifts that equal or exceed an integer's width",
			Documentation: "Shifting a non-constant integer by at least its bit width discards every bit and commonly indicates the wrong operand type, shift count, or architecture assumption. Glippy adapts the standard Go shift analyzer and excludes constant bit-width idioms handled by its upstream contract.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The standard analyzer checks compile-time shift counts against the width of non-constant integer operands.",
				"Constant operands and unreachable branches are treated according to the upstream analyzer's compatibility rules.",
			},
			Examples: []rules.Example{
				{
					Title: "Keep the shift below the operand width",
					Incorrect: "var value uint8\n_ = value << 8",
					Correct: "var value uint8\n_ = value << 7",
				},
			},
		},
		nil,
	)
}

func suspiciousStringConversionRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		stringintconv.Analyzer,
		rules.Metadata{
			ID: "suspicious-string-conversion",
			Summary: "detects integer-to-string conversions that may expect decimal digits",
			Documentation: "Converting an integer directly to string produces the UTF-8 encoding of one Unicode code point, not the decimal representation of the number. The conversion is often a mistaken substitute for formatting, but deliberate rune conversions remain possible, so Glippy places the standard analyzer in the suspicious preset.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetSuspicious},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"Byte, rune, and untyped-rune conversions are excluded because their code-point intent is explicit.",
				"Both upstream repairs are suggestions: decimal formatting and explicit rune conversion have different semantics.",
			},
			Examples: []rules.Example{
				{
					Title: "Choose digits or a code point explicitly",
					Incorrect: "text := string(number)",
					Correct: "text := fmt.Sprint(number)",
				},
			},
		},
		[]analysis.AnalyzerFixMapping{
			{
				Message: "Format the number as a decimal",
				Name: "format-number-decimal",
				Description: "format the integer as decimal text",
				Safety: rules.FixSuggestion,
			},
			{
				Message: "Convert a single rune to a string",
				Name: "convert-single-rune",
				Description: "make the intended code-point conversion explicit",
				Safety: rules.FixSuggestion,
			},
		},
	)
}

func nilFunctionComparisonRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		nilfunc.Analyzer,
		rules.Metadata{
			ID: "nil-function-comparison",
			Summary: "detects comparisons between declared functions and nil",
			Documentation: "A declared function is not a function-valued variable and cannot be nil. Comparing its identifier with nil therefore has a constant result and often confuses the function with its return value or an optional callback. Glippy adapts the standard Go nilfunc analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule reports declared functions and methods resolved as *types.Func objects; function-valued variables may legitimately be nil.",
			},
			Examples: []rules.Example{
				{
					Title: "Call the function before comparing its result",
					Incorrect: "if lookup == nil {}",
					Correct: "if lookup() == nil {}",
				},
			},
		},
		nil,
	)
}

func appendNoValuesRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		appends.Analyzer,
		rules.Metadata{
			ID: "append-no-values",
			Summary: "detects append calls with no values to append",
			Documentation: "Calling append with only its destination slice is an identity operation: it appends nothing and returns the same slice. Such a call is ineffective and usually means an argument was omitted during editing. Glippy adapts the standard Go appends analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"Only the built-in append function with exactly one argument is reported.",
			},
			Examples: []rules.Example{
				{
					Title: "Provide the value being appended",
					Incorrect: "items = append(items)",
					Correct: "items = append(items, item)",
				},
			},
		},
		nil,
	)
}

func invalidSlogArgumentsRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		slog.Analyzer,
		rules.Metadata{
			ID: "invalid-slog-arguments",
			Summary: "detects malformed structured logging argument lists",
			Documentation: "Structured slog calls require alternating string keys and values unless an argument is a slog.Attr. A non-key argument in key position or a final key without a value produces malformed log records and runtime diagnostics. Glippy adapts the standard Go slog analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The analyzer recognizes standard log/slog functions and methods plus statically inferred wrappers supported upstream.",
				"Dynamic key and value slices passed through other APIs are outside this direct-call contract.",
			},
			Examples: []rules.Example{
				{
					Title: "Pair each structured key with a value",
					Incorrect: `slog.Info("request", "method")`,
					Correct: `slog.Info("request", "method", method)`,
				},
			},
		},
		nil,
	)
}

func unusedResultRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		analyzerWithoutFlags(unusedresult.Analyzer),
		rules.Metadata{
			ID: "unused-result",
			Summary: "detects discarded results from side-effect-free operations",
			Documentation: "Some standard functions and String or Error methods return their only useful effect as a value. Calling them as standalone statements discards that result and cannot accomplish the apparent operation. Glippy adapts the standard Go unusedresult analyzer with its authoritative default function catalog.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The configurable upstream funcs and stringmethods flags are not exposed as Glippy rule options in this release.",
				"The rule uses the standard analyzer's curated default catalog rather than guessing purity for arbitrary functions.",
			},
			Examples: []rules.Example{
				{
					Title: "Use the returned value",
					Incorrect: `fmt.Sprintf("%s", value)`,
					Correct: `text := fmt.Sprintf("%s", value)`,
				},
			},
		},
		nil,
	)
}

func unreachableCodeRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		unreachable.Analyzer,
		rules.Metadata{
			ID: "unreachable-code",
			Summary: "detects statements that execution cannot reach",
			Documentation: "Statements following an unconditional return, panic, terminating branch, or infinite loop cannot execute. They often preserve stale work after a refactor or conceal control-flow mistakes. Glippy adapts the standard Go unreachable analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			RunDespiteTypeErrors: true,
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The upstream control-flow model reports the first unreachable statement in a contiguous region.",
				"Removal remains suggestion-only because comments and intentionally retained examples require review.",
			},
			Examples: []rules.Example{
				{
					Title: "Remove statements after a terminal return",
					Incorrect: "return\nwork()",
					Correct: "work()\nreturn",
				},
			},
		},
		[]analysis.AnalyzerFixMapping{
			{
				Message: "Remove",
				Name: "remove-unreachable-code",
				Description: "remove the unreachable statement",
				Safety: rules.FixSuggestion,
			},
		},
	)
}

func unsafeHostPortRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		hostport.Analyzer,
		rules.Metadata{
			ID: "unsafe-host-port",
			Summary: "detects host and port formatting that breaks IPv6 addresses",
			Documentation: "Formatting an address as host:port with fmt.Sprintf does not add the brackets required around IPv6 literals. The resulting address fails when passed to net.Dial and related APIs. net.JoinHostPort implements the grammar for IPv4, hostnames, and IPv6. Glippy adapts the standard Go hostport analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The analyzer follows fmt.Sprintf address values into recognized net dialing calls and direct dial arguments.",
				"The net.JoinHostPort rewrite is suggestion-only because it can add an import and changes formatting behavior for malformed input.",
			},
			Examples: []rules.Example{
				{
					Title: "Join network addresses with net.JoinHostPort",
					Incorrect: `net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))`,
					Correct: `net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))`,
				},
			},
		},
		[]analysis.AnalyzerFixMapping{
			{
				Message: "Replace fmt.Sprintf with net.JoinHostPort",
				Name: "use-net-join-host-port",
				Description: "construct the address with net.JoinHostPort",
				Safety: rules.FixSuggestion,
				RequiredImports: []rules.ImportRequirement{
					{Path: "net", Name: "net"},
				},
			},
		},
	)
}

func deferredTimeSinceRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		defers.Analyzer,
		rules.Metadata{
			ID: "deferred-time-since",
			Summary: "detects time.Since evaluated before a deferred call",
			Documentation: "Arguments to a deferred call are evaluated when the defer statement executes. Passing time.Since(start) directly therefore records the elapsed time at defer registration rather than when the surrounding function returns. Glippy adapts the standard Go defers analyzer.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The analyzer reports time.Since calls lexically evaluated inside a defer call and does not infer equivalent custom elapsed-time helpers.",
				"No automatic fix is offered because introducing a closure changes evaluation and comment boundaries.",
			},
			Examples: []rules.Example{
				{
					Title: "Evaluate elapsed time when the deferred function runs",
					Incorrect: "defer record(time.Since(start))",
					Correct: "defer func() { record(time.Since(start)) }()",
				},
			},
		},
		nil,
	)
}
