package report

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

const sarifSchema = "https://json.schemastore.org/sarif-2.1.0.json"

// IntegrationInput binds diagnostics and non-rule findings to exact source
// versions for GitHub workflow annotations and SARIF output.
type IntegrationInput struct {
	Files []LintTextInput
	Formats []CheckFormatOutcome
	PackageDiagnostics []analysis.PackageDiagnostic
	SourceProblems []analysis.PackageSourceProblem
	Errors []Error
	Registry *rules.Registry
}

type integrationFinding struct {
	ruleID string
	targets []string
	level string
	message string
	path string
	file *source.File
	range_ *source.Range
}

// RenderGitHub renders deterministic GitHub workflow-command annotations.
func RenderGitHub(input IntegrationInput) ([]byte, error) {
	findings, err := integrationFindings(input)
	if err != nil {
		return nil, err
	}
	for _, item := range input.Errors {
		findings = append(
			findings,
			integrationFinding{ruleID: "glippy", level: "error", message: item.Message},
		)
	}
	var output strings.Builder
	for _, finding := range findings {
		properties := make([]string, 0, 6)
		if finding.path != "" {
			properties = append(
				properties,
				"file=" + escapeGitHubProperty(filepath.ToSlash(finding.path)),
			)
		}
		if finding.range_ != nil {
			start, end, valid := physicalPositions(finding.file, *finding.range_)
			if !valid {
				return nil, fmt.Errorf(
					"%s: integration finding %q has invalid physical range",
					finding.path,
					finding.ruleID,
				)
			}
			properties = append(
				properties,
				fmt.Sprintf("line=%d", start.Line),
				fmt.Sprintf("col=%d", start.Column),
				fmt.Sprintf("endLine=%d", end.Line),
				fmt.Sprintf("endColumn=%d", end.Column),
			)
		}
		properties = append(
			properties,
			"title=" +
				escapeGitHubProperty(
					finding.ruleID + integrationTargetSuffix(finding.targets),
				),
		)
		fmt.Fprintf(
			&output,
			"::%s %s::%s\n",
			githubLevel(finding.level),
			strings.Join(properties, ","),
			escapeGitHubData(finding.message),
		)
	}
	return []byte(output.String()), nil
}

// RenderSARIF renders one deterministic SARIF 2.1.0 log.
func RenderSARIF(input IntegrationInput) ([]byte, error) {
	findings, err := integrationFindings(input)
	if err != nil {
		return nil, err
	}
	ruleIDs := make([]string, 0)
	seenRules := make(map[string]struct{})
	ruleLevels := make(map[string]string)
	for _, finding := range findings {
		if _, found := ruleLevels[finding.ruleID]; !found {
			ruleLevels[finding.ruleID] = finding.level
		}
		if _, found := seenRules[finding.ruleID]; found {
			continue
		}
		seenRules[finding.ruleID] = struct{}{}
		ruleIDs = append(ruleIDs, finding.ruleID)
	}
	sort.Strings(ruleIDs)
	descriptors := make([]sarifRule, len(ruleIDs))
	for index, ruleID := range ruleIDs {
		descriptors[index] = sarifRuleDescriptor(input.Registry, ruleID, ruleLevels[ruleID])
	}
	results := make([]sarifResult, len(findings))
	for index, finding := range findings {
		result := sarifResult{
			RuleID: finding.ruleID,
			Level: finding.level,
			Message: sarifMessage{Text: finding.message},
			Locations: []sarifLocation{},
		}
		if len(finding.targets) != 0 {
			result.Properties = &sarifProperties{Targets: slices.Clone(finding.targets)}
		}
		if finding.path != "" {
			physical := sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: fileURI(finding.path)},
			}
			if finding.range_ != nil {
				start, end, valid := physicalPositions(
					finding.file,
					*finding.range_,
				)
				if !valid {
					return nil, fmt.Errorf(
						"%s: integration finding %q has invalid physical range",
						finding.path,
						finding.ruleID,
					)
				}
				physical.Region = &sarifRegion{
					StartLine: start.Line,
					StartColumn: start.Column,
					EndLine: end.Line,
					EndColumn: end.Column,
				}
			}
			result.Locations = append(
				result.Locations,
				sarifLocation{PhysicalLocation: physical},
			)
		}
		results[index] = result
	}
	executionSuccessful := len(input.Errors) == 0
	notifications := make([]sarifNotification, len(input.Errors))
	for index, item := range input.Errors {
		notifications[index] = sarifNotification{
			Level: "error",
			Message: sarifMessage{Text: item.Message},
		}
	}
	return marshalJSON(
		sarifLog{
			Version: "2.1.0",
			Schema: sarifSchema,
			Runs: []sarifRun{
				{
					Tool: sarifTool{
						Driver: sarifDriver{
							Name: "Glippy",
							Rules: descriptors,
						},
					},
					Invocations: []sarifInvocation{
						{
							ExecutionSuccessful: executionSuccessful,
							ToolExecutionNotifications: notifications,
						},
					},
					Results: results,
				},
			},
		},
	)
}

func integrationFindings(input IntegrationInput) ([]integrationFinding, error) {
	files := slices.Clone(input.Files)
	sort.Slice(
		files,
		func(left, right int) bool {
			return files[left].Result.Path < files[right].Result.Path
		},
	)
	byPath := make(map[string]*source.File, len(files))
	for index, item := range files {
		if item.File == nil {
			return nil, fmt.Errorf("integration input %d has no source file", index)
		}
		if item.Result.Path != item.File.Path() ||
			item.Result.Digest != item.File.Digest() {
			return nil, fmt.Errorf(
				"integration source identity does not match %q",
				item.Result.Path,
			)
		}
		if _, duplicate := byPath[item.Result.Path]; duplicate {
			return nil, fmt.Errorf(
				"duplicate integration source path %q",
				item.Result.Path,
			)
		}
		byPath[item.Result.Path] = item.File
	}
	findings := make([]integrationFinding, 0)
	formats := slices.Clone(input.Formats)
	sort.Slice(
		formats,
		func(left, right int) bool {
			return formats[left].Path < formats[right].Path
		},
	)
	for _, format := range formats {
		file, found := byPath[format.Path]
		if !found || file.Digest() != format.Digest {
			return nil, fmt.Errorf(
				"format integration source identity does not match %q",
				format.Path,
			)
		}
		if format.Different && format.Preexisting {
			return nil, fmt.Errorf(
				"format integration outcome %q is both changed and pre-existing",
				format.Path,
			)
		}
		if format.Different {
			findings = append(
				findings,
				integrationFinding{
					ruleID: "format",
					level: "warning",
					message: "source is not canonically formatted",
					path: format.Path,
					file: file,
				},
			)
		}
	}
	for _, item := range files {
		for _, diagnostic := range analysis.OrderDiagnostics(item.Result.Diagnostics) {
			if diagnostic.Path != item.Result.Path ||
				diagnostic.Digest != item.Result.Digest {
				return nil, fmt.Errorf(
					"diagnostic source identity does not match %q",
					item.Result.Path,
				)
			}
			if err := validateTargets(diagnostic.Targets); err != nil {
				return nil, fmt.Errorf(
					"diagnostic %q targets: %w",
					diagnostic.RuleID,
					err,
				)
			}
			range_ := diagnostic.Range
			findings = append(
				findings,
				integrationFinding{
					ruleID: diagnostic.RuleID,
					targets: slices.Clone(diagnostic.Targets),
					level: sarifLevel(diagnostic.Severity),
					message: diagnostic.Message,
					path: item.Result.Path,
					file: item.File,
					range_: &range_,
				},
			)
		}
		for _, problem := range item.Result.SuppressionProblems {
			range_ := problem.Range
			findings = append(
				findings,
				integrationFinding{
					ruleID: "suppression:" + string(problem.Kind),
					level: "warning",
					message: problem.Message,
					path: item.Result.Path,
					file: item.File,
					range_: &range_,
				},
			)
		}
		for _, directive := range item.Result.UnusedSuppressions {
			range_ := directive.Range
			findings = append(
				findings,
				integrationFinding{
					ruleID: "suppression:unused",
					level: "warning",
					message: "unused suppression for " + directive.RuleID,
					path: item.Result.Path,
					file: item.File,
					range_: &range_,
				},
			)
		}
		for _, problem := range item.Result.BaselineProblems {
			findings = append(
				findings,
				integrationFinding{
					ruleID: "baseline:" + string(problem.Kind),
					level: "warning",
					message: fmt.Sprintf(
						"%s/%s has %d unmatched occurrence(s)",
						problem.Entry.RuleID,
						problem.Entry.MessageKey,
						problem.Remaining,
					),
					path: item.Result.Path,
					file: item.File,
				},
			)
		}
	}
	mappedPackages, err := mapPackageDiagnostics(input.PackageDiagnostics)
	if err != nil {
		return nil, err
	}
	for _, diagnostic := range mappedPackages {
		message := diagnostic.Message
		if diagnostic.Position != "" {
			message = diagnostic.Position + ": " + message
		}
		findings = append(
			findings,
			integrationFinding{
				ruleID: "package:" + diagnostic.Kind,
				targets: slices.Clone(diagnostic.Targets),
				level: "error",
				message: diagnostic.PackageID + ": " + message,
			},
		)
	}
	mappedSources, err := mapSourceProblems(input.SourceProblems)
	if err != nil {
		return nil, err
	}
	for _, problem := range mappedSources {
		findings = append(
			findings,
			integrationFinding{
				ruleID: "source",
				targets: slices.Clone(problem.Targets),
				level: "error",
				message: problem.Message,
				path: problem.Path,
				file: byPath[problem.Path],
			},
		)
	}
	return findings, nil
}

func integrationTargetSuffix(targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	return " [" + strings.Join(targets, ",") + "]"
}

func physicalPositions(
	file *source.File,
	range_ source.Range,
) (source.Position, source.Position, bool) {
	if file == nil || range_.End < range_.Start {
		return source.Position{}, source.Position{}, false
	}
	start, valid := file.Position(range_.Start)
	if !valid {
		return source.Position{}, source.Position{}, false
	}
	end, valid := file.Position(range_.End)
	return start, end, valid
}

func escapeGitHubData(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")
	return strings.ReplaceAll(value, "\n", "%0A")
}

func escapeGitHubProperty(value string) string {
	value = escapeGitHubData(value)
	value = strings.ReplaceAll(value, ":", "%3A")
	return strings.ReplaceAll(value, ",", "%2C")
}

func githubLevel(level string) string {
	if level == "error" {
		return "error"
	}
	return "warning"
}

func sarifLevel(severity rules.Severity) string {
	switch severity {
	case rules.SeverityOff:
		return "none"
	case rules.SeverityError:
		return "error"
	default:
		return "warning"
	}
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func sarifRuleDescriptor(registry *rules.Registry, ruleID string, defaultLevel string) sarifRule {
	if registry != nil {
		if metadata, found := registry.Metadata(ruleID); found {
			return sarifRule{
				ID: ruleID,
				ShortDescription: sarifMessage{Text: metadata.Summary},
				FullDescription: sarifMessage{Text: metadata.Documentation},
				DefaultConfiguration: sarifRuleConfiguration{
					Level: sarifLevel(metadata.DefaultSeverity),
				},
			}
		}
	}
	return sarifRule{
		ID: ruleID,
		ShortDescription: sarifMessage{Text: syntheticRuleSummary(ruleID)},
		DefaultConfiguration: sarifRuleConfiguration{Level: defaultLevel},
	}
}

func syntheticRuleSummary(ruleID string) string {
	switch ruleID {
	case "format":
		return "source differs from canonical formatting"
	case "source":
		return "source could not be analyzed"
	case "glippy":
		return "Glippy invocation failed"
	default:
		return strings.ReplaceAll(ruleID, ":", " ")
	}
}

type sarifLog struct {
	Version string `json:"version"`
	Schema string `json:"$schema"`
	Runs []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool sarifTool `json:"tool"`
	Invocations []sarifInvocation `json:"invocations"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name string `json:"name"`
	Rules []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID string `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
	FullDescription sarifMessage `json:"fullDescription,omitempty"`
	DefaultConfiguration sarifRuleConfiguration `json:"defaultConfiguration"`
}

type sarifRuleConfiguration struct {
	Level string `json:"level"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool `json:"executionSuccessful"`
	ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications"`
}

type sarifNotification struct {
	Level string `json:"level"`
	Message sarifMessage `json:"message"`
}

type sarifResult struct {
	RuleID string `json:"ruleId"`
	Level string `json:"level"`
	Message sarifMessage `json:"message"`
	Properties *sarifProperties `json:"properties,omitempty"`
	Locations []sarifLocation `json:"locations"`
}

type sarifProperties struct {
	Targets []string `json:"targets,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region *sarifRegion `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
	StartColumn int `json:"startColumn"`
	EndLine int `json:"endLine"`
	EndColumn int `json:"endColumn"`
}
