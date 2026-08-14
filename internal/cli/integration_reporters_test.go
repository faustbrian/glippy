package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestParseLintAndCheckAcceptIntegrationReporters(t *testing.T) {
	t.Parallel()

	for _, reporter := range []glippyreport.Format{glippyreport.GitHub, glippyreport.SARIF} {
		lint, valid := parseLintInvocation(
			[]string{"lint", "--reporter=" + string(reporter), "source.go"},
		)
		if !valid || lint.reporter != reporter {
			t.Fatalf("parseLintInvocation(%s) = %#v, %t", reporter, lint, valid)
		}
		check, valid := parseCheckInvocation(
			[]string{"check", "--reporter", string(reporter), "source.go"},
		)
		if !valid || check.reporter != reporter {
			t.Fatalf("parseCheckInvocation(%s) = %#v, %t", reporter, check, valid)
		}
	}
	if _, valid := parseFormatInvocation([]string{"fmt", "--check", "--reporter=sarif", "."});
		valid {
		t.Fatal("format accepted a diagnostic-only reporter")
	}
}

func TestRunLintIntegrationReportersRespectChangedCodeFiltering(t *testing.T) {
	t.Parallel()

	root := initializeChangedCLIRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nfunc run() {\n\tfirst := 1\n\tsecond := 2\n\tfirst = first\n\tsecond = second\n\t_, _ = first, second\n}\n"
	current := "package sample\n\nfunc run() {\n\tfirst := 1\n\tsecond := 2\n\tfirst = first // changed\n\tsecond = second\n\t_, _ = first, second\n}\n"
	writeChangedCLIFile(t, path, baseline)
	commitChangedCLIBaseline(t, root)
	writeChangedCLIFile(t, path, current)

	var github bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lint", "--new-from=HEAD", "--reporter=github", root},
		strings.NewReader(""),
		&github,
		&stderr,
	)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		strings.Count(github.String(), "title=self-assignment") != 1 ||
		!strings.Contains(github.String(), "line=6,col=2") {
		t.Fatalf(
			"RunContext(github) = exit %d, stdout %q, stderr %q",
			exitCode,
			github.String(),
			stderr.String(),
		)
	}

	var sarif bytes.Buffer
	stderr.Reset()
	exitCode = RunContext(
		context.Background(),
		[]string{"lint", "--new-from=HEAD", "--reporter=sarif", root},
		strings.NewReader(""),
		&sarif,
		&stderr,
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext(sarif) = exit %d, stdout %q, stderr %q",
			exitCode,
			sarif.String(),
			stderr.String(),
		)
	}
	var decoded struct {
		Runs []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
				Locations []struct {
					PhysicalLocation struct {
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(sarif.Bytes(), &decoded); err != nil {
		t.Fatalf("decode SARIF: %v\n%s", err, sarif.Bytes())
	}
	if len(decoded.Runs) != 1 ||
		len(decoded.Runs[0].Results) != 1 ||
		decoded.Runs[0].Results[0].RuleID != "self-assignment" ||
		decoded.Runs[0].Results[0].Locations[0].PhysicalLocation.Region.StartLine != 6 {
		t.Fatalf("changed-code SARIF = %#v\n%s", decoded, sarif.Bytes())
	}
}

func TestRunCheckIntegrationReportersIncludeFormattingDifferences(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/integrationreport\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(t, filepath.Join(root, ".glippy.toml"), "version = 1\n")
	path := filepath.Join(root, "source.go")
	writeChangedCLIFile(t, path, "package sample\nfunc run(){println(\"x\")}\n")

	for _, reporter := range []string{"github", "sarif"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := RunContext(
			context.Background(),
			[]string{"check", "--reporter=" + reporter, path},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
		if exitCode != ExitFindings ||
			stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), "format") {
			t.Fatalf(
				"RunContext(check %s) = exit %d, stdout %q, stderr %q",
				reporter,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestRunLintLevelSeverityFlowsToIntegrationReporters(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/lintlevelreporters\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "source.go")
	writeChangedCLIFile(
		t,
		path,
		"package sample\n\nfunc run(ready bool) {\n\tif ready {\n\t\tprintln(\"first\")\n\t} else if ready {\n\t\tprintln(\"second\")\n\t}\n}\n",
	)

	var github bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lint", "-Dwarnings", "--reporter=github", path},
		strings.NewReader(""),
		&github,
		&stderr,
	)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(github.String(), "::error ") ||
		!strings.Contains(github.String(), "title=duplicate-condition") {
		t.Fatalf(
			"RunContext(lint-level github) = exit %d, stdout %q, stderr %q",
			exitCode,
			github.String(),
			stderr.String(),
		)
	}

	var sarif bytes.Buffer
	stderr.Reset()
	exitCode = RunContext(
		context.Background(),
		[]string{"lint", "-Dwarnings", "--reporter=sarif", path},
		strings.NewReader(""),
		&sarif,
		&stderr,
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext(lint-level sarif) = exit %d, stdout %q, stderr %q",
			exitCode,
			sarif.String(),
			stderr.String(),
		)
	}
	var decoded struct {
		Runs []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
				Level string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(sarif.Bytes(), &decoded); err != nil {
		t.Fatalf("decode lint-level SARIF: %v\n%s", err, sarif.Bytes())
	}
	if len(decoded.Runs) != 1 ||
		len(decoded.Runs[0].Results) != 1 ||
		decoded.Runs[0].Results[0].RuleID != "duplicate-condition" ||
		decoded.Runs[0].Results[0].Level != "error" {
		t.Fatalf("lint-level SARIF = %#v\n%s", decoded, sarif.Bytes())
	}
}

func TestInvalidLintAndCheckInvocationsUseIntegrationReporters(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"lint", "check"} {
		t.Run(
			command + "/github",
			func(t *testing.T) {
				t.Parallel()

				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := RunContext(
					context.Background(),
					[]string{command, "--reporter=github", "--invalid"},
					strings.NewReader(""),
					&stdout,
					&stderr,
				)
				if exitCode != ExitInvalidInvocation ||
					stderr.Len() != 0 ||
					!strings.HasPrefix(
						stdout.String(),
						"::error title=glippy::",
					) {
					t.Fatalf(
						"RunContext(%s github invalid) = exit %d, stdout %q, stderr %q",
						command,
						exitCode,
						stdout.String(),
						stderr.String(),
					)
				}
			},
		)

		t.Run(
			command + "/sarif",
			func(t *testing.T) {
				t.Parallel()

				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := RunContext(
					context.Background(),
					[]string{command, "--reporter", "sarif", "--invalid"},
					strings.NewReader(""),
					&stdout,
					&stderr,
				)
				var decoded struct {
					Runs []struct {
						Invocations []struct {
							ExecutionSuccessful bool `json:"executionSuccessful"`
							Notifications []struct {
								Message struct {
									Text string `json:"text"`
								} `json:"message"`
							} `json:"toolExecutionNotifications"`
						} `json:"invocations"`
						Results []json.RawMessage `json:"results"`
					} `json:"runs"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
					t.Fatalf(
						"decode %s invalid SARIF: %v\n%s",
						command,
						err,
						stdout.Bytes(),
					)
				}
				if exitCode != ExitInvalidInvocation ||
					stderr.Len() != 0 ||
					len(decoded.Runs) != 1 ||
					len(decoded.Runs[0].Invocations) != 1 ||
					decoded.Runs[0].Invocations[0].ExecutionSuccessful ||
					len(decoded.Runs[0].Invocations[0].Notifications) != 1 ||
					len(decoded.Runs[0].Results) != 0 {
					t.Fatalf(
						"RunContext(%s sarif invalid) = exit %d, decoded %#v, stderr %q",
						command,
						exitCode,
						decoded,
						stderr.String(),
					)
				}
			},
		)
	}
}

func TestRuntimeFailuresUseIntegrationReporters(t *testing.T) {
	t.Parallel()

	for _, reporter := range []glippyreport.Format{glippyreport.GitHub, glippyreport.SARIF} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runLintCheck(
			nil,
			lintInvocation{reporter: reporter},
			&stdout,
			&stderr,
			nil,
		)
		if exitCode != ExitInternalError || stderr.Len() != 0 {
			t.Fatalf(
				"runLintCheck(nil, %s) = exit %d, stdout %q, stderr %q",
				reporter,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
		if reporter == glippyreport.GitHub &&
			stdout.String() != "::error title=glippy::context is required\n" {
			t.Fatalf("GitHub failure = %q", stdout.String())
		}
		if reporter == glippyreport.SARIF &&
			(!strings.Contains(stdout.String(), `"executionSuccessful": false`) ||
				!strings.Contains(stdout.String(), "context is required")) {
			t.Fatalf("SARIF failure = %q", stdout.String())
		}
	}
}

func TestLintFailureIntegrationReporterRetainsCompletedDiagnostics(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	input := glippyreport.LintTextInput{
		File: file,
		Result: analysis.Result{
			Path: file.Path(),
			Digest: file.Digest(),
			Diagnostics: []rules.Diagnostic{
				{
					RuleID: "completed-rule",
					Severity: rules.SeverityWarn,
					MessageKey: "completed",
					Message: "completed diagnostic",
					Path: file.Path(),
					Digest: file.Digest(),
					Range: source.Range{Start: 0, End: len("package")},
				},
			},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := reportLintFailure(
		lintInvocation{reporter: glippyreport.GitHub},
		&stdout,
		&stderr,
		ExitFilesystemError,
		[]glippyreport.LintTextInput{input},
		os.ErrNotExist,
	)
	if exitCode != ExitFilesystemError ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "title=completed-rule") ||
		!strings.Contains(stdout.String(), "title=glippy") {
		t.Fatalf(
			"reportLintFailure() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestTypedCombinedCheckUsesIntegrationReportersWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/integrationtyped\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npreset = \"suspicious\"\n",
	)
	path := filepath.Join(root, "sample.go")
	input := "package sample\nfunc inspect(pointer *int){if pointer==nil{_ = *pointer}}\n"
	writeChangedCLIFile(t, path, input)

	for _, reporter := range []string{"github", "sarif"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := RunContext(
			context.Background(),
			[]string{"check", "--reporter=" + reporter, filepath.Join(root, "...")},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
		if exitCode != ExitFindings ||
			stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), "nilness") ||
			!strings.Contains(stdout.String(), "format") {
			t.Fatalf(
				"RunContext(typed check %s) = exit %d, stdout %q, stderr %q",
				reporter,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != input {
			t.Fatalf("typed check %s mutated source: %q", reporter, got)
		}
	}
}

func TestLintFixUsesIntegrationReporters(t *testing.T) {
	t.Parallel()

	for _, reporter := range []string{"github", "sarif"} {
		t.Run(
			reporter,
			func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				writeChangedCLIFile(
					t,
					filepath.Join(root, "go.mod"),
					"module example.com/integrationfix\n\ngo 1.26.0\n",
				)
				writeChangedCLIFile(
					t,
					filepath.Join(root, ".glippy.toml"),
					"version = 1\n[lint]\npreset = \"correctness\"\n",
				)
				path := filepath.Join(root, "sample.go")
				writeChangedCLIFile(
					t,
					path,
					"package sample\n\nfunc reset(value int) int {\n\tvalue = value\n\treturn value\n}\n",
				)

				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := RunContext(
					context.Background(),
					[]string{
						"lint",
						"--fix-suggestions",
						"--reporter=" + reporter,
						path,
					},
					strings.NewReader(""),
					&stdout,
					&stderr,
				)
				if exitCode != ExitSuccess || stderr.Len() != 0 {
					t.Fatalf(
						"RunContext(lint fix %s) = exit %d, stdout %q, stderr %q",
						reporter,
						exitCode,
						stdout.String(),
						stderr.String(),
					)
				}
				if reporter == "github" && stdout.Len() != 0 {
					t.Fatalf("clean GitHub fix output = %q", stdout.String())
				}
				if reporter == "sarif" &&
					(!strings.Contains(
						stdout.String(),
						`"executionSuccessful": true`,
					) ||
						!strings.Contains(
							stdout.String(),
							`"results": []`,
						)) {
					t.Fatalf("clean SARIF fix output = %q", stdout.String())
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				want := "package sample\n\nfunc reset(value int) int {\n\treturn value\n}\n"
				if string(got) != want {
					t.Fatalf("lint fix %s source = %q", reporter, got)
				}
			},
		)
	}
}
