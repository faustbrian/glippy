// Package rulecatalog composes native Glippy rules with admitted analyzers.
package rulecatalog

import (
	"fmt"
	"strings"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/assign"
	atomicanalyzer "golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"
	"golang.org/x/tools/go/analysis/passes/timeformat"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

// NewRegistry constructs the canonical product rule registry.
func NewRegistry() (*rules.Registry, error) {
	standard, err := standardAnalyzerRules()
	if err != nil {
		return nil, err
	}
	all := append(rules.DefaultRules(), standard...)
	all = append(
		all,
		rules.NewBlankErrorDiscardRule(),
		rules.NewBadBitMaskRule(),
		rules.NewBufferStringConversionRule(),
		rules.NewChannelUsedAfterCloseRule(),
		rules.NewContextCancelLeakRule(),
		rules.NewDeferInLoopRule(),
		rules.NewDiscardedErrorRule(),
		rules.NewEmptyBranchRule(),
		rules.NewExecPipeRunRule(),
		rules.NewHTTPCanonicalHeaderKeyRule(),
		rules.NewHTTPResponseBodyNotClosedRule(),
		rules.NewHTTPResponseBodyUsedAfterCloseRule(),
		rules.NewInconsistentReceiverNameRule(),
		rules.NewImpossibleComparisonRule(),
		rules.NewInvalidBinaryWriteRule(),
		rules.NewInvalidRandomBoundRule(),
		rules.NewInvalidRegexpRule(),
		rules.NewInvalidStrconvArgumentRule(),
		rules.NewZeroReplaceCountRule(),
		rules.NewZeroRegexpMatchLimitRule(),
		rules.NewLockHeldAcrossBlockingCallRule(),
		rules.NewLockNotReleasedRule(),
		rules.NewUnlockWithoutLockRule(),
		rules.NewLoopCaptureRule(),
		rules.NewManualMinMaxRule(),
		rules.NewMixedReceiverTypeRule(),
		rules.NewNeedlessBlankIdentifierRule(),
		rules.NewNilContextRule(),
		rules.NewNetIPBytesEqualRule(),
		rules.NewNonSliceSortRule(),
		rules.NewOverwrittenErrorRule(),
		rules.NewShadowedErrorRule(),
		rules.NewRedundantClosureRule(),
		rules.NewResourceNotClosedRule(),
		rules.NewResourceUsedAfterCloseRule(),
		rules.NewRedundantElseRule(),
		rules.NewRedundantNilCheckRule(),
		rules.NewRedundantTypeDeclarationRule(),
		rules.NewSubsumedConditionRule(),
		rules.NewSuspiciousRangeRule(),
		rules.NewTimeDurationUnitRule(),
		rules.NewTimeSinceRule(),
		rules.NewTimeUntilRule(),
		rules.NewTypedNilErrorReturnRule(),
		rules.NewUnnecessaryConversionRule(),
		rules.NewUnnecessaryFormatRule(),
		rules.NewUnnecessarySprintfRule(),
		rules.NewInefficientStringComparisonRule(),
		rules.NewIgnoredAppendResultRule(),
		rules.NewNilMapWriteRule(),
		rules.NewIneffectiveValueReceiverAssignmentRule(),
		rules.NewNaNComparisonRule(),
		rules.NewIntegerDivisionBeforeConversionRule(),
		rules.NewIneffectiveURLQueryMutationRule(),
		rules.NewDeferredLockRule(),
		rules.NewDeferBeforeErrorCheckRule(),
		rules.NewInfiniteRecursionRule(),
		rules.NewImpossibleInterfaceNilComparisonRule(),
		rules.NewRegexpCompileInLoopRule(),
		rules.NewSyncPoolNonPointerRule(),
		rules.NewStringRangeRuneConversionRule(),
		rules.NewInefficientIOStringWriteRule(),
		rules.NewExcessiveNestingRule(),
		rules.NewTooManyLinesRule(),
		rules.NewTooManyParametersRule(),
		rules.NewTooManyResultsRule(),
		rules.NewSQLTransactionNotCompletedRule(),
		rules.NewSQLTransactionUsedAfterCompletionRule(),
		rules.NewUncheckedRowsErrorRule(),
		rules.NewUncheckedScannerErrorRule(),
		rules.NewUnreachableCodeRule(),
		rules.NewWaitGroupNegativeCounterRule(),
	)
	all = append(all, rules.NewAlmostSwappedRule())
	registry, err := rules.NewRegistry(all...)
	if err != nil {
		return nil, fmt.Errorf("construct product rule registry: %w", err)
	}
	return registry, nil
}

func standardAnalyzerRules() ([]rules.Rule, error) {
	constructors := []func() (
		rules.Rule,
		error,
	){
		printfArgumentsRule,
		invalidStructTagRule,
		invalidUnmarshalTargetRule,
		waitgroupMisuseRule,
		testingGoroutineCallRule,
		standardMethodSignatureRule,
		oversizedShiftRule,
		suspiciousStringConversionRule,
		nilFunctionComparisonRule,
		appendNoValuesRule,
		invalidSlogArgumentsRule,
		unusedResultRule,
		unsafeHostPortRule,
		deferredTimeSinceRule,
		selfAssignmentRule,
		copiedLockRule,
		impossibleTypeAssertionRule,
		httpResponseBeforeErrorRule,
		atomicUpdateAssignmentRule,
		contradictoryConditionRule,
		errorsAsTargetRule,
		timeLayoutRule,
		invalidBuildConstraintRule,
		invalidDirectiveRule,
		invalidTestSignatureRule,
		unbufferedSignalChannelRule,
		standardLibraryVersionRule,
	}
	result := make([]rules.Rule, 0, len(constructors))
	for _, construct := range constructors {
		rule, err := construct()
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, nil
}

func errorsAsTargetRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		errorsas.Analyzer,
		rules.Metadata{
			ID: "errors-as-target",
			Summary: "detects invalid targets passed to errors.As",
			Documentation: "The second argument to errors.As must be a non-nil pointer to a type implementing error or to an interface. Invalid target shapes panic at runtime, while *error is ineffective because it matches any non-nil error. Glippy exposes the standard Go errorsas analyzer through its deterministic typed scheduler.",
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
				"The rule follows the standard errorsas analyzer and reports statically invalid direct targets.",
			},
			Examples: []rules.Example{
				{
					Title: "Pass a pointer target",
					Incorrect: "var target *os.PathError\nerrors.As(err, target)",
					Correct: "var target *os.PathError\nerrors.As(err, &target)",
				},
			},
		},
		nil,
	)
}

func timeLayoutRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		timeformat.Analyzer,
		rules.Metadata{
			ID: "time-layout",
			Summary: "detects time layouts that use 2006-02-01 instead of 2006-01-02",
			Documentation: "Go time layouts use the reference date January 2, 2006. Writing 2006-02-01 swaps the month and day tokens and parses or formats a different layout than intended. Glippy exposes the standard Go timeformat analyzer through its deterministic typed scheduler.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The standard analyzer targets the common 2006-02-01 reference-date transposition in time.Parse and Time.Format calls.",
			},
			Examples: []rules.Example{
				{
					Title: "Use Go's January 2 reference date",
					Incorrect: "time.Parse(\"2006-02-01\", value)",
					Correct: "time.Parse(\"2006-01-02\", value)",
				},
			},
		},
		[]analysis.AnalyzerFixMapping{
			{
				Message: "Replace 2006-02-01 with 2006-01-02",
				Name: "correct-reference-layout",
				Description: "replace the transposed reference date with 2006-01-02",
				Safety: rules.FixSuggestion,
			},
		},
	)
}

func contradictoryConditionRule() (rules.Rule, error) {
	analyzer := filterAnalyzerDiagnostics(
		"contradictorycondition",
		bools.Analyzer,
		func(diagnostic goanalysis.Diagnostic) bool {
			return strings.HasPrefix(diagnostic.Message, "suspect ")
		},
	)
	return adaptStandardAnalyzer(
		analyzer,
		rules.Metadata{
			ID: "contradictory-condition",
			Summary: "detects boolean chains that are always true or false",
			Documentation: "Comparing the same side-effect-free value to different constants with equality joined by && can never succeed; using inequality joined by || can never fail. Glippy exposes the contradictory subset of the standard Go bools analyzer through its deterministic rule contract.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The standard bools analyzer restricts reports to side-effect-free expression groups with a compile-time constant on one side of each comparison.",
				"Equivalent expressions written in syntactically different forms are not matched.",
			},
			Examples: []rules.Example{
				{
					Title: "Use a satisfiable comparison chain",
					Incorrect: "value == 1 && value == 2",
					Correct: "value == 1 || value == 2",
				},
			},
		},
		nil,
	)
}

func filterAnalyzerDiagnostics(
	name string,
	upstream *goanalysis.Analyzer,
	keep func(goanalysis.Diagnostic) bool,
) *goanalysis.Analyzer {
	return &goanalysis.Analyzer{
		Name: name,
		Doc: upstream.Doc,
		URL: upstream.URL,
		Requires: upstream.Requires,
		Run: func(pass *goanalysis.Pass) (any, error) {
			filtered := *pass
			filtered.Analyzer = upstream
			filtered.Report = func(diagnostic goanalysis.Diagnostic) {
				if keep(diagnostic) {
					pass.Report(diagnostic)
				}
			}
			return upstream.Run(&filtered)
		},
	}
}

func selfAssignmentRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		assign.Analyzer,
		rules.Metadata{
			ID: "self-assignment",
			Summary: "detects assignments that leave a value unchanged",
			Documentation: "A value assigned directly to itself has no effect and usually indicates a copied expression, a mistaken assignment target, or code left behind after a refactor. Glippy exposes the standard Go assign analyzer through its typed scheduler, deterministic diagnostics, suppressions, baselines, and fix-safety model.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule follows the standard Go assign analyzer and intentionally excludes assignments whose evaluation can have observable effects, including map index expressions.",
				"The removal fix is suggestion-only because deleting an assignment requires confirmation that the statement was not an intentional marker.",
				"Removing a statement that shares a physical line through an explicit semicolon can retain an empty statement after formatting; review suggestion output before accepting it.",
			},
			Examples: []rules.Example{
				{
					Title: "Assign the intended value",
					Incorrect: "value = value",
					Correct: "value = replacement",
				},
			},
		},
		[]analysis.AnalyzerFixMapping{
			{
				Message: "Remove self-assignment",
				Name: "remove-self-assignment",
				Description: "remove the ineffective self-assignment",
				Safety: rules.FixSuggestion,
			},
		},
	)
}

func copiedLockRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		copylock.Analyzer,
		rules.Metadata{
			ID: "copied-lock",
			Summary: "detects locks copied after first use",
			Documentation: "Copying a value that contains sync.Mutex, sync.RWMutex, or another lock-like noCopy value can split synchronization state and make callers believe they coordinate through the same lock. Glippy adapts the standard Go copylocks analyzer across assignments, declarations, returns, calls, ranges, and value receivers.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			RunDespiteTypeErrors: true,
			Categories: []rules.Category{
				rules.CategoryCorrectness,
				rules.CategorySafety,
			},
			KnownLimitations: []string{
				"The rule follows the standard Go copylocks analyzer's lock-path and noCopy conventions.",
				"Diagnostics do not prove that a particular copied lock has already been used at runtime; the structural copy is the reported hazard.",
			},
			Examples: []rules.Example{
				{
					Title: "Keep lock-bearing values behind pointers",
					Incorrect: "func clone(value *state) state { return *value }",
					Correct: "func share(value *state) *state { return value }",
				},
			},
		},
		nil,
	)
}

func impossibleTypeAssertionRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		ifaceassert.Analyzer,
		rules.Metadata{
			ID: "impossible-type-assertion",
			Summary: "detects interface assertions that can never succeed",
			Documentation: "An interface-to-interface assertion is impossible when no concrete type can implement both interfaces because they declare the same method with conflicting signatures. Such an assertion always panics in its single-result form and always fails in its comma-ok form.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The standard ifaceassert analyzer reports only conflicts it can prove from interface method sets.",
				"Assertions to concrete types are already checked by the Go type checker and are not duplicated here.",
			},
			Examples: []rules.Example{
				{
					Title: "Assert compatible interfaces",
					Incorrect: "value.(interface{ Read(int) })",
					Correct: "value.(interface{ Read() })",
				},
			},
		},
		nil,
	)
}

func httpResponseBeforeErrorRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		httpresponse.Analyzer,
		rules.Metadata{
			ID: "http-response-before-error",
			Summary: "detects HTTP responses used before their errors are checked",
			Documentation: "An HTTP request may return a nil response with a non-nil error. Deferring response.Body.Close before checking that error can therefore dereference nil on the failure path and hide the original request failure.",
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
				"The standard httpresponse analyzer recognizes net/http functions and Client methods returning (*http.Response, error).",
				"The rule targets the immediate defer-before-error-check pattern and does not attempt general response ownership analysis.",
			},
			Examples: []rules.Example{
				{
					Title: "Check the request error before using the response",
					Incorrect: "response, err := http.Get(url)\ndefer response.Body.Close()\nif err != nil { return err }",
					Correct: "response, err := http.Get(url)\nif err != nil { return err }\ndefer response.Body.Close()",
				},
			},
		},
		nil,
	)
}

func atomicUpdateAssignmentRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		atomicanalyzer.Analyzer,
		rules.Metadata{
			ID: "atomic-update-assignment",
			Summary: "detects atomic updates assigned back non-atomically",
			Documentation: "Functions such as atomic.AddUint64 already update the pointed-to value atomically and return the new value. Assigning that return value back through the same pointer performs a second non-atomic write, defeating the synchronization contract and introducing a race.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			RunDespiteTypeErrors: true,
			Categories: []rules.Category{
				rules.CategoryCorrectness,
				rules.CategorySafety,
			},
			KnownLimitations: []string{
				"The standard atomic analyzer covers the legacy sync/atomic AddInt32, AddInt64, AddUint32, AddUint64, and AddUintptr functions.",
				"Typed atomic wrapper methods and higher-level synchronization patterns are outside this analyzer's scope.",
			},
			Examples: []rules.Example{
				{
					Title: "Use the atomic update without a second write",
					Incorrect: "*value = atomic.AddUint64(value, 1)",
					Correct: "atomic.AddUint64(value, 1)",
				},
			},
		},
		nil,
	)
}

func adaptStandardAnalyzer(
	analyzer *goanalysis.Analyzer,
	metadata rules.Metadata,
	fixes []analysis.AnalyzerFixMapping,
) (rules.Rule, error) {
	return adaptStandardAnalyzerWithFactExecution(analyzer, metadata, fixes, nil, nil)
}

func adaptStandardAnalyzerWithDependencyFactFilter(
	analyzer *goanalysis.Analyzer,
	metadata rules.Metadata,
	fixes []analysis.AnalyzerFixMapping,
	filter *analysis.AnalyzerDependencyFactFilter,
) (rules.Rule, error) {
	return adaptStandardAnalyzerWithFactExecution(analyzer, metadata, fixes, filter, nil)
}

func adaptStandardAnalyzerWithFactExecution(
	analyzer *goanalysis.Analyzer,
	metadata rules.Metadata,
	fixes []analysis.AnalyzerFixMapping,
	filter *analysis.AnalyzerDependencyFactFilter,
	external *analysis.AnalyzerExternalFactExecution,
) (rules.Rule, error) {
	adapted, err := analysis.AdaptAnalyzer(
		analyzer,
		analysis.AnalyzerAdapterOptions{
			Metadata: metadata,
			SuggestedFixes: fixes,
			ReadOnlyAudited: true,
			DependencyFactFilter: filter,
			ExternalFactExecution: external,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("adapt %s analyzer: %w", metadata.ID, err)
	}
	return adapted, nil
}
