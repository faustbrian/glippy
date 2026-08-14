package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"github.com/faustbrian/glippy/internal/suppressions"
)

func TestRenderGitHubEmitsEscapedPhysicalAnnotations(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run() {\n\ttarget()\n}\n"
	file, err := source.Load("/project/a,b.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	targetStart := strings.Index(input, "target()")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "call-rule",
				Severity: rules.SeverityWarn,
				MessageKey: "call",
				Message: "review % result\nsoon",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{
					Start: targetStart,
					End: targetStart + len("target()"),
				},
			},
		},
		SuppressionProblems: []suppressions.Problem{
			{
				Kind: suppressions.ProblemMalformed,
				Range: source.Range{Start: 0, End: len("package")},
				Message: "malformed: suppression",
			},
		},
	}

	output, err := RenderGitHub(
		IntegrationInput{
			Files: []LintTextInput{{File: file, Result: result}},
			Formats: []CheckFormatOutcome{
				{Path: file.Path(), Digest: file.Digest(), Different: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "::warning file=/project/a%2Cb.go,title=format::source is not canonically formatted\n" +
		"::warning file=/project/a%2Cb.go,line=3,col=2,endLine=3,endColumn=10,title=call-rule::review %25 result%0Asoon\n" +
		"::warning file=/project/a%2Cb.go,line=1,col=1,endLine=1,endColumn=8,title=suppression%3Amalformed::malformed: suppression\n"
	if string(output) != want {
		t.Fatalf("RenderGitHub() =\n%s\nwant:\n%s", output, want)
	}
}

func TestRenderSARIFEmitsDeterministicRulesAndPhysicalLocations(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run() {\n\ttarget()\n}\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	targetStart := strings.Index(input, "target()")
	registry, err := rules.NewRegistry(
		integrationRule{
			metadata: rules.Metadata{
				ID: "call-rule",
				Summary: "reports calls requiring review",
				Documentation: "Review the selected call.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireSyntax,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "call-rule",
				Severity: rules.SeverityError,
				MessageKey: "call",
				Message: "call requires review",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{
					Start: targetStart,
					End: targetStart + len("target()"),
				},
			},
		},
	}
	integration := IntegrationInput{
		Files: []LintTextInput{{File: file, Result: result}},
		Registry: registry,
	}
	first, err := RenderSARIF(integration)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderSARIF(integration)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("RenderSARIF() is nondeterministic")
	}
	var decoded struct {
		Version string `json:"version"`
		Schema string `json:"$schema"`
		Runs []struct {
			Tool struct {
				Driver struct {
					Name string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level string `json:"level"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
							StartColumn int `json:"startColumn"`
							EndLine int `json:"endLine"`
							EndColumn int `json:"endColumn"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("decode SARIF: %v\n%s", err, first)
	}
	if decoded.Version != "2.1.0" ||
		decoded.Schema != "https://json.schemastore.org/sarif-2.1.0.json" ||
		len(decoded.Runs) != 1 ||
		decoded.Runs[0].Tool.Driver.Name != "Glippy" ||
		len(decoded.Runs[0].Tool.Driver.Rules) != 1 ||
		decoded.Runs[0].Tool.Driver.Rules[0].ID != "call-rule" ||
		len(decoded.Runs[0].Results) != 1 ||
		decoded.Runs[0].Results[0].RuleID != "call-rule" ||
		decoded.Runs[0].Results[0].Level != "error" ||
		decoded.Runs[0].Results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI !=
			"file:///project/source.go" ||
		decoded.Runs[0].Results[0].Locations[0].PhysicalLocation.Region.StartLine != 3 ||
		decoded.Runs[0].Results[0].Locations[0].PhysicalLocation.Region.StartColumn != 2 ||
		decoded.Runs[0].Results[0].Locations[0].PhysicalLocation.Region.EndLine != 3 ||
		decoded.Runs[0].Results[0].Locations[0].PhysicalLocation.Region.EndColumn != 10 {
		t.Fatalf("SARIF contract = %#v\n%s", decoded, first)
	}
}

func TestRenderSARIFReportsToolErrorsOnlyAsInvocationNotifications(t *testing.T) {
	t.Parallel()

	output, err := RenderSARIF(
		IntegrationInput{Errors: []Error{{Message: "configuration failed"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Runs []struct {
			Invocations []struct {
				ExecutionSuccessful bool `json:"executionSuccessful"`
				Notifications []struct {
					Message sarifMessage `json:"message"`
				} `json:"toolExecutionNotifications"`
			} `json:"invocations"`
			Results []sarifResult `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Runs) != 1 ||
		len(decoded.Runs[0].Invocations) != 1 ||
		decoded.Runs[0].Invocations[0].ExecutionSuccessful ||
		len(decoded.Runs[0].Invocations[0].Notifications) != 1 ||
		decoded.Runs[0].Invocations[0].Notifications[0].Message.Text !=
			"configuration failed" ||
		len(decoded.Runs[0].Results) != 0 {
		t.Fatalf("SARIF tool failure = %#v\n%s", decoded, output)
	}
}

func TestRenderSARIFUsesFindingSeverityForSyntheticRuleDefaults(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/broken.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := RenderSARIF(
		IntegrationInput{
			SourceProblems: []analysis.PackageSourceProblem{
				{
					Path: file.Path(),
					Digest: file.Digest(),
					Message: "source failed",
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID string `json:"id"`
						DefaultConfiguration struct {
							Level string `json:"level"`
						} `json:"defaultConfiguration"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Runs) != 1 ||
		len(decoded.Runs[0].Tool.Driver.Rules) != 1 ||
		decoded.Runs[0].Tool.Driver.Rules[0].ID != "source" ||
		decoded.Runs[0].Tool.Driver.Rules[0].DefaultConfiguration.Level != "error" {
		t.Fatalf("synthetic SARIF rule = %#v\n%s", decoded, output)
	}
}

type integrationRule struct {
	metadata rules.Metadata
}

func (r integrationRule) Metadata() rules.Metadata {
	return r.metadata
}
