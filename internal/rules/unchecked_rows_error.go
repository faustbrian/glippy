package rules

const databaseSQLPackagePath = "database/sql"

type uncheckedRowsErrorRule struct{}

var uncheckedRowsErrorSpec = iterationErrorSpec{
	packagePath: databaseSQLPackagePath,
	typeName: "Rows",
	iterationMethod: "Next",
	errorMethod: "Err",
}

// NewUncheckedRowsErrorRule constructs the database row-iteration lifecycle
// rule for product registry composition.
func NewUncheckedRowsErrorRule() Rule {
	return uncheckedRowsErrorRule{}
}

func (uncheckedRowsErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "unchecked-rows-error",
		Summary: "detects database row iteration without a checked terminal error",
		Documentation: "database/sql.Rows.Next returns false both when iteration is complete and when iteration stops because of an error. Code that returns normally after the loop must observe Rows.Err or it can silently accept a partial result set. This rule follows the shared control-flow graph from each direct Rows.Next loop and reports when any normally returning path can bypass observation of the matching Rows.Err result.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"The initial contract recognizes direct identifier-backed database/sql.Rows.Next and Rows.Err calls; aliases stored in fields, containers, or other variables are not tracked.",
			"A direct assignment to the rows variable invalidates later checks against a replacement value; writes through range targets and indirect aliases are not modeled.",
			"Passing Rows.Err to another call counts as observing the result; the rule does not inspect the callee's behavior.",
			"The shared CFG propagates no-return behavior through selected local-source modules. Third-party helpers outside those modules remain conservatively returning unless they match an exact standard-library terminal API.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Check the terminal iteration error",
				Incorrect: "for rows.Next() { scan(rows) }\nreturn nil",
				Correct: "for rows.Next() { scan(rows) }\nreturn rows.Err()",
			},
		},
	}
}

func (uncheckedRowsErrorRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	return runUncheckedIterationError(
		ctx,
		"unchecked-rows-error",
		uncheckedRowsErrorSpec,
		"rows-error-not-checked",
		"rows.Err is not checked on every normally returning path after iteration",
		"check rows.Err after the loop before returning the accumulated results",
	)
}
