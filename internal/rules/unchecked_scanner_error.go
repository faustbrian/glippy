package rules

const bufioPackagePath = "bufio"

type uncheckedScannerErrorRule struct{}

var uncheckedScannerErrorSpec = iterationErrorSpec{
	packagePath: bufioPackagePath,
	typeName: "Scanner",
	iterationMethod: "Scan",
	errorMethod: "Err",
}

// NewUncheckedScannerErrorRule constructs the scanner iteration lifecycle rule
// for product registry composition.
func NewUncheckedScannerErrorRule() Rule {
	return uncheckedScannerErrorRule{}
}

func (uncheckedScannerErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "unchecked-scanner-error",
		Summary: "detects scanner iteration without a checked terminal error",
		Documentation: "bufio.Scanner.Scan returns false both at end of input and when scanning stops because of an error. Code that returns normally after the loop must observe Scanner.Err or it can silently accept truncated input. This rule follows the shared control-flow graph from each direct Scanner.Scan loop and reports when any normally returning path can bypass observation of the matching Scanner.Err result.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"The initial contract recognizes direct identifier-backed bufio.Scanner.Scan and Scanner.Err calls; aliases stored in fields, containers, or other variables are not tracked.",
			"A direct assignment to the scanner variable invalidates later checks against a replacement value; writes through range targets and indirect aliases are not modeled.",
			"Passing Scanner.Err to another call counts as observing the result; the rule does not inspect the callee's behavior.",
			"The shared CFG recognizes predeclared panic as non-returning; imported and project-local termination helpers are treated as returning.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Check the terminal scanner error",
				Incorrect: "for scanner.Scan() { consume(scanner.Bytes()) }\nreturn nil",
				Correct: "for scanner.Scan() { consume(scanner.Bytes()) }\nreturn scanner.Err()",
			},
		},
	}
}

func (uncheckedScannerErrorRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	return runUncheckedIterationError(
		ctx,
		"unchecked-scanner-error",
		uncheckedScannerErrorSpec,
		"scanner-error-not-checked",
		"scanner.Err is not checked on every normally returning path after iteration",
		"check scanner.Err after the loop before returning the consumed input",
	)
}
