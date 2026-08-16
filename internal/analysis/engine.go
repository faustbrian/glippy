// Package analysis schedules native rules over shared run-owned representations.
package analysis

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"slices"
	"sort"
	"strings"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

type activeSyntaxRule struct {
	rule rules.SyntaxRule
	metadata rules.Metadata
	severity rules.Severity
	context *rules.Context
}

type activeSyntaxFileRule struct {
	rule rules.SyntaxFileRule
	metadata rules.Metadata
	severity rules.Severity
	context *rules.Context
}

// RunSyntax executes selected syntax rules through one shared AST traversal.
func RunSyntax(
	ctx context.Context,
	file *source.File,
	registry *rules.Registry,
	selection []rules.Selection,
) ([]rules.Diagnostic, error) {
	if ctx == nil {
		return nil, fmt.Errorf("syntax analysis requires a context")
	}
	if file == nil {
		return nil, fmt.Errorf("syntax analysis requires a source file")
	}
	if registry == nil {
		return nil, fmt.Errorf("syntax analysis requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ordered := slices.Clone(selection)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].ID < ordered[right].ID
		},
	)
	dispatch := make(map[rules.NodeKind][]activeSyntaxRule)
	fileRules := make([]activeSyntaxFileRule, 0)
	previousID := ""
	for _, selected := range ordered {
		if selected.ID == previousID {
			return nil, fmt.Errorf("selected rule %q more than once", selected.ID)
		}
		previousID = selected.ID
		switch selected.Severity {
		case rules.SeverityWarn, rules.SeverityError:
		case rules.SeverityOff:
			return nil, fmt.Errorf(
				"selected rule %q has disabled severity",
				selected.ID,
			)
		default:
			return nil, fmt.Errorf(
				"selected rule %q has invalid severity %q",
				selected.ID,
				selected.Severity,
			)
		}
		nativeRule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if selected.Requirement != metadata.Requirement {
			return nil, fmt.Errorf(
				"selected rule %q requirement does not match registry",
				selected.ID,
			)
		}
		if metadata.Requirement != rules.RequireSyntax {
			return nil, fmt.Errorf(
				"selected rule %q requires %s; syntax runner requires syntax rules",
				selected.ID,
				metadata.Requirement,
			)
		}
		if file.Metadata().Generated && !metadata.RunOnGenerated {
			continue
		}
		fileRule, fileRuleFound := nativeRule.(rules.SyntaxFileRule)
		syntaxRule, syntaxRuleFound := nativeRule.(rules.SyntaxRule)
		if fileRuleFound && syntaxRuleFound {
			return nil, fmt.Errorf(
				"selected rule %q implements ambiguous syntax execution",
				selected.ID,
			)
		}
		if fileRuleFound {
			if len(metadata.NodeInterests) != 1 ||
				metadata.NodeInterests[0] != rules.NodeFile {
				return nil, fmt.Errorf(
					"selected file rule %q must declare only file interest",
					selected.ID,
				)
			}
			fileRules = append(
				fileRules,
				activeSyntaxFileRule{
					rule: fileRule,
					metadata: metadata,
					severity: selected.Severity,
					context: rules.NewContext(file, selected.Options),
				},
			)
			continue
		}
		if !syntaxRuleFound {
			return nil, fmt.Errorf(
				"selected rule %q does not implement syntax execution",
				selected.ID,
			)
		}
		active := activeSyntaxRule{
			rule: syntaxRule,
			metadata: metadata,
			severity: selected.Severity,
			context: rules.NewContext(file, selected.Options),
		}
		for _, interest := range metadata.NodeInterests {
			dispatch[interest] = append(dispatch[interest], active)
		}
	}
	if len(dispatch) == 0 && len(fileRules) == 0 {
		return []rules.Diagnostic{}, nil
	}
	statistics := statisticsFromContext(ctx)
	tierStarted := beginStatisticsMeasurement(statistics)
	defer statistics.recordTier(rules.RequireSyntax, tierStarted)

	diagnostics := make([]rules.Diagnostic, 0)
	if len(dispatch) > 0 {
		err := file.ReadSyntax(
			func(syntax *ast.File) error {
				var runErr error
				ast.Inspect(
					syntax,
					func(node ast.Node) bool {
						if runErr != nil {
							return false
						}
						if node == nil {
							return true
						}
						if err := ctx.Err(); err != nil {
							runErr = err
							return false
						}
						kind, found := rules.KindOf(node)
						if !found {
							return true
						}
						for _, active := range dispatch[kind] {
							ruleStarted := beginStatisticsMeasurement(
								statistics,
							)
							findings, err := active.rule.RunSyntax(
								active.context,
								node,
							)
							statistics.recordRule(
								active.metadata.ID,
								active.metadata.Requirement,
								ruleStarted,
							)
							if contextErr := ctx.Err();
								contextErr != nil {
								runErr = contextErr
								return false
							}
							if err != nil {
								runErr = fmt.Errorf(
									"%s: %w",
									active.metadata.ID,
									err,
								)
								return false
							}
							for _, finding := range findings {
								diagnostic, err := diagnosticForFinding(
									file,
									active.metadata,
									active.severity,
									finding,
								)
								if err != nil {
									runErr = fmt.Errorf(
										"%s: %w",
										active.metadata.ID,
										err,
									)
									return false
								}
								diagnostics = append(
									diagnostics,
									diagnostic,
								)
							}
						}
						return true
					},
				)
				return runErr
			},
		)
		if err != nil {
			return nil, err
		}
	}

	for _, active := range fileRules {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ruleStarted := beginStatisticsMeasurement(statistics)
		findings, err := active.rule.RunSyntaxFile(active.context)
		statistics.recordRule(active.metadata.ID, active.metadata.Requirement, ruleStarted)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", active.metadata.ID, err)
		}
		for _, finding := range findings {
			diagnostic, err := diagnosticForFinding(
				file,
				active.metadata,
				active.severity,
				finding,
			)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", active.metadata.ID, err)
			}
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	return OrderDiagnostics(diagnostics), nil
}

// OrderDiagnostics returns one canonically ordered diagnostic slice.
func OrderDiagnostics(diagnostics []rules.Diagnostic) []rules.Diagnostic {
	ordered := slices.Clone(diagnostics)
	sortDiagnostics(ordered)
	return ordered
}

func diagnosticForFinding(
	file *source.File,
	metadata rules.Metadata,
	severity rules.Severity,
	finding rules.Finding,
) (rules.Diagnostic, error) {
	if strings.TrimSpace(finding.MessageKey) == "" {
		return rules.Diagnostic{}, fmt.Errorf("finding message key is required")
	}
	if strings.TrimSpace(finding.Message) == "" {
		return rules.Diagnostic{}, fmt.Errorf("finding message is required")
	}
	if _, valid := file.Slice(finding.Range); !valid {
		return rules.Diagnostic{}, fmt.Errorf("finding has invalid primary range")
	}
	for _, related := range finding.Related {
		if _, valid := file.Slice(related.Range); !valid {
			return rules.Diagnostic{}, fmt.Errorf("finding has invalid related range")
		}
	}
	fixMetadata := make(map[string]rules.FixMetadata, len(metadata.Fixes))
	for _, fix := range metadata.Fixes {
		fixMetadata[fix.Name] = fix
	}
	fixes := slices.Clone(finding.Fixes)
	seenFixes := make(map[string]struct{}, len(fixes))
	for fixIndex := range fixes {
		fix := &fixes[fixIndex]
		declared, found := fixMetadata[fix.Name]
		if !found {
			return rules.Diagnostic{}, fmt.Errorf(
				"finding uses undeclared fix %q",
				fix.Name,
			)
		}
		if fix.Safety != declared.Safety {
			return rules.Diagnostic{}, fmt.Errorf(
				"finding fix %q safety does not match metadata",
				fix.Name,
			)
		}
		if _, duplicate := seenFixes[fix.Name]; duplicate {
			return rules.Diagnostic{}, fmt.Errorf("finding repeats fix %q", fix.Name)
		}
		seenFixes[fix.Name] = struct{}{}
		fix.Edits = slices.Clone(fix.Edits)
		fix.RequiredImports = slices.Clone(fix.RequiredImports)
		seenImports := make(map[rules.ImportRequirement]struct{}, len(fix.RequiredImports))
		importNames := make(map[string]string, len(fix.RequiredImports))
		importPaths := make(map[string]string, len(fix.RequiredImports))
		for _, requirement := range fix.RequiredImports {
			if err := requirement.Validate(); err != nil {
				return rules.Diagnostic{}, fmt.Errorf(
					"finding fix %q has invalid import requirement: %w",
					fix.Name,
					err,
				)
			}
			if _, duplicate := seenImports[requirement]; duplicate {
				return rules.Diagnostic{}, fmt.Errorf(
					"finding fix %q repeats required import %q as %q",
					fix.Name,
					requirement.Path,
					requirement.Name,
				)
			}
			seenImports[requirement] = struct{}{}
			if path, found := importNames[requirement.Name];
				found && path != requirement.Path {
				return rules.Diagnostic{}, fmt.Errorf(
					"finding fix %q requires import name %q for incompatible paths",
					fix.Name,
					requirement.Name,
				)
			}
			if name, found := importPaths[requirement.Path];
				found && name != requirement.Name {
				return rules.Diagnostic{}, fmt.Errorf(
					"finding fix %q requires import path %q with incompatible names",
					fix.Name,
					requirement.Path,
				)
			}
			importNames[requirement.Name] = requirement.Path
			importPaths[requirement.Path] = requirement.Name
		}
		sort.Slice(
			fix.RequiredImports,
			func(left, right int) bool {
				if fix.RequiredImports[left].Path !=
					fix.RequiredImports[right].Path {
					return fix.RequiredImports[left].Path <
						fix.RequiredImports[right].Path
				}
				return fix.RequiredImports[left].Name <
					fix.RequiredImports[right].Name
			},
		)
		for _, edit := range fix.Edits {
			if _, valid := file.Slice(edit.Range); !valid {
				return rules.Diagnostic{}, fmt.Errorf(
					"finding fix %q has invalid edit range",
					fix.Name,
				)
			}
		}
	}
	sort.Slice(
		fixes,
		func(left, right int) bool {
			return fixes[left].Name < fixes[right].Name
		},
	)
	withheldFixes := slices.Clone(finding.WithheldFixes)
	seenWithheldFixes := make(map[string]struct{}, len(withheldFixes))
	for _, fix := range withheldFixes {
		if _, found := fixMetadata[fix.Name]; !found {
			return rules.Diagnostic{}, fmt.Errorf(
				"finding withholds undeclared fix %q",
				fix.Name,
			)
		}
		if _, offered := seenFixes[fix.Name]; offered {
			return rules.Diagnostic{}, fmt.Errorf(
				"finding both offers and withholds fix %q",
				fix.Name,
			)
		}
		if _, duplicate := seenWithheldFixes[fix.Name]; duplicate {
			return rules.Diagnostic{}, fmt.Errorf(
				"finding repeats withheld fix %q",
				fix.Name,
			)
		}
		seenWithheldFixes[fix.Name] = struct{}{}
		if fix.Reason != rules.FixWithheldComments {
			return rules.Diagnostic{}, fmt.Errorf(
				"finding withheld fix %q has invalid reason %q",
				fix.Name,
				fix.Reason,
			)
		}
		if strings.TrimSpace(fix.Message) == "" {
			return rules.Diagnostic{}, fmt.Errorf(
				"finding withheld fix %q has no message",
				fix.Name,
			)
		}
	}
	sort.Slice(
		withheldFixes,
		func(left, right int) bool {
			return withheldFixes[left].Name < withheldFixes[right].Name
		},
	)
	related := slices.Clone(finding.Related)
	notes := slices.Clone(finding.Notes)
	return rules.Diagnostic{
		RuleID: metadata.ID,
		Severity: severity,
		MessageKey: finding.MessageKey,
		Message: finding.Message,
		Path: file.Path(),
		Digest: file.Digest(),
		Range: finding.Range,
		Related: related,
		Notes: notes,
		Help: finding.Help,
		Fixes: fixes,
		WithheldFixes: withheldFixes,
	}, nil
}

func sortDiagnostics(diagnostics []rules.Diagnostic) {
	sort.SliceStable(
		diagnostics,
		func(left, right int) bool {
			first, second := diagnostics[left], diagnostics[right]
			if order := cmp.Compare(first.Path, second.Path); order != 0 {
				return order < 0
			}
			if order := bytes.Compare(first.Digest[:], second.Digest[:]); order != 0 {
				return order < 0
			}
			if first.Range.Start != second.Range.Start {
				return first.Range.Start < second.Range.Start
			}
			if first.Range.End != second.Range.End {
				return first.Range.End < second.Range.End
			}
			if order := cmp.Compare(first.RuleID, second.RuleID); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.Severity, second.Severity); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.MessageKey, second.MessageKey); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.Message, second.Message); order != 0 {
				return order < 0
			}
			if order := slices.Compare(first.Targets, second.Targets); order != 0 {
				return order < 0
			}
			if order := compareRelated(first.Related, second.Related); order != 0 {
				return order < 0
			}
			if order := slices.Compare(first.Notes, second.Notes); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.Help, second.Help); order != 0 {
				return order < 0
			}
			if order := compareFixes(first.Fixes, second.Fixes); order != 0 {
				return order < 0
			}
			return compareWithheldFixes(first.WithheldFixes, second.WithheldFixes) < 0
		},
	)
}

func compareRelated(left, right []rules.Related) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if order := cmp.Compare(left[index].Range.Start, right[index].Range.Start);
			order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Range.End, right[index].Range.End); order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Message, right[index].Message); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(left), len(right))
}

func compareFixes(left, right []rules.Fix) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if order := cmp.Compare(left[index].Name, right[index].Name); order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Safety, right[index].Safety); order != 0 {
			return order
		}
		leftEdits, rightEdits := left[index].Edits, right[index].Edits
		for edit := 0; edit < min(len(leftEdits), len(rightEdits)); edit++ {
			if order := cmp.Compare(
				leftEdits[edit].Range.Start,
				rightEdits[edit].Range.Start,
			);
				order != 0 {
				return order
			}
			if order := cmp.Compare(
				leftEdits[edit].Range.End,
				rightEdits[edit].Range.End,
			);
				order != 0 {
				return order
			}
			if order := cmp.Compare(leftEdits[edit].NewText, rightEdits[edit].NewText);
				order != 0 {
				return order
			}
		}
		if order := cmp.Compare(len(leftEdits), len(rightEdits)); order != 0 {
			return order
		}
		leftImports := left[index].RequiredImports
		rightImports := right[index].RequiredImports
		for required := 0; required < min(len(leftImports), len(rightImports)); required++ {
			if order := cmp.Compare(
				leftImports[required].Path,
				rightImports[required].Path,
			);
				order != 0 {
				return order
			}
			if order := cmp.Compare(
				leftImports[required].Name,
				rightImports[required].Name,
			);
				order != 0 {
				return order
			}
		}
		if order := cmp.Compare(len(leftImports), len(rightImports)); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(left), len(right))
}

func compareWithheldFixes(left, right []rules.WithheldFix) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if order := cmp.Compare(left[index].Name, right[index].Name); order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Reason, right[index].Reason); order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Message, right[index].Message); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(left), len(right))
}
