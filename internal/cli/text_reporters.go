package cli

import (
	"fmt"

	"github.com/faustbrian/glippy/internal/analysis"
	glippyreport "github.com/faustbrian/glippy/internal/report"
)

func renderLintText(
	reporter glippyreport.Format,
	inputs []glippyreport.LintTextInput,
) ([]byte, error) {
	switch reporter {
	case "", glippyreport.Text:
		return glippyreport.RenderLintText(inputs)
	case glippyreport.Short:
		return glippyreport.RenderLintShortText(inputs)
	default:
		return nil, fmt.Errorf("unsupported human reporter %q", reporter)
	}
}

func renderPackageLintText(
	reporter glippyreport.Format,
	inputs []glippyreport.LintTextInput,
	packageDiagnostics []analysis.PackageDiagnostic,
	sourceProblems []analysis.PackageSourceProblem,
) ([]byte, error) {
	switch reporter {
	case "", glippyreport.Text:
		return glippyreport.RenderPackageLintText(
			inputs,
			packageDiagnostics,
			sourceProblems,
		)
	case glippyreport.Short:
		return glippyreport.RenderPackageLintShortText(
			inputs,
			packageDiagnostics,
			sourceProblems,
		)
	default:
		return nil, fmt.Errorf("unsupported human reporter %q", reporter)
	}
}

func renderLintFixText(
	reporter glippyreport.Format,
	inputs []glippyreport.LintFixTextInput,
) ([]byte, error) {
	switch reporter {
	case "", glippyreport.Text:
		return glippyreport.RenderLintFixText(inputs)
	case glippyreport.Short:
		return glippyreport.RenderLintFixShortText(inputs)
	default:
		return nil, fmt.Errorf("unsupported human reporter %q", reporter)
	}
}
