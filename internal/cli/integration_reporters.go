package cli

import (
	"fmt"
	"io"

	glippyreport "github.com/faustbrian/glippy/internal/report"
)

func isIntegrationReporter(reporter glippyreport.Format) bool {
	return reporter == glippyreport.GitHub || reporter == glippyreport.SARIF
}

func reportIntegrationOutput(
	command string,
	reporter glippyreport.Format,
	stdout, stderr io.Writer,
	exitCode int,
	input glippyreport.IntegrationInput,
) int {
	var output []byte
	var err error
	switch reporter {
	case glippyreport.GitHub:
		output, err = glippyreport.RenderGitHub(input)
	case glippyreport.SARIF:
		output, err = glippyreport.RenderSARIF(input)
	default:
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"glippy %s: unsupported integration reporter %q\n",
			command,
			reporter,
		)
	}
	if err != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"glippy %s: render %s report: %v\n",
			command,
			reporter,
			err,
		)
	}
	if err := write(stdout, output); err != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitFilesystemError),
			"glippy %s: write %s report: %v\n",
			command,
			reporter,
			err,
		)
	}
	return exitCode
}

func integrationError(err error) []glippyreport.Error {
	if err == nil {
		return []glippyreport.Error{}
	}
	return []glippyreport.Error{{Message: fmt.Sprint(err)}}
}
