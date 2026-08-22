package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

const useOctalFileModeFix = "use-octal-file-mode"

type nonOctalFileModeRule struct{}

// NewNonOctalFileModeRule constructs the suspicious decimal file-mode rule.
func NewNonOctalFileModeRule() Rule {
	return nonOctalFileModeRule{}
}

func (nonOctalFileModeRule) Metadata() Metadata {
	return Metadata{
		ID: "non-octal-file-mode",
		Summary: "detects decimal file modes that look unintentionally non-octal",
		Documentation: "File permissions are conventionally written in octal. A three-digit decimal os.FileMode made entirely of octal digits usually means the prefix was omitted, so 644 evaluates to mode 0o1204 rather than 0o644.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategorySafety, CategorySuspicious},
		Fixes: []FixMetadata{
			{
				Name: useOctalFileModeFix,
				Description: "spell the file mode as an octal literal",
				Safety: FixSuggestion,
			},
		},
		KnownLimitations: []string{
			"Only direct three-digit decimal integer literals whose digits are all valid in octal are reported.",
			"The argument must resolve to the exact standard os.FileMode or io/fs.FileMode type; distinct defined modes, constants, variables, and calculations remain conservative.",
			"The suggestion changes runtime permissions and therefore requires explicit suggestion-fix authorization.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Write permissions in octal",
				Incorrect: "err := os.WriteFile(path, data, 644)",
				Correct: "err := os.WriteFile(path, data, 0o644)",
			},
		},
	}
}

func (nonOctalFileModeRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"non-octal-file-mode requires a call expression and type information",
		)
	}
	findings := make([]Finding, 0)
	for _, argument := range call.Args {
		literal, _ := ast.Unparen(argument).(*ast.BasicLit)
		if literal == nil ||
			literal.Kind != token.INT ||
			!isSuspiciousDecimalFileMode(literal.Value) ||
			!isStandardFileMode(ctx.Info().TypeOf(literal)) {
			continue
		}
		value, err := strconv.ParseUint(literal.Value, 10, 16)
		if err != nil {
			continue
		}
		range_, err := ctx.Range(literal)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "decimal-file-mode",
				Message: fmt.Sprintf(
					"decimal file mode %s evaluates to 0o%o; did you mean 0o%s?",
					literal.Value,
					value,
					literal.Value,
				),
				Range: range_,
				Help: "prefix the intended permission bits with 0o",
				Fixes: []Fix{
					{
						Name: useOctalFileModeFix,
						Safety: FixSuggestion,
						Edits: []Edit{
							{
								Range: range_,
								NewText: "0o" + literal.Value,
							},
						},
					},
				},
			},
		)
	}
	return findings, nil
}

func isSuspiciousDecimalFileMode(value string) bool {
	if len(value) != 3 || value[0] == '0' {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '7' {
			return false
		}
	}
	return true
}

func isStandardFileMode(type_ types.Type) bool {
	named, _ := types.Unalias(type_).(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "io/fs" &&
		named.Obj().Name() == "FileMode"
}
