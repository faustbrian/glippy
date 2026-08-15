package rules

import (
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"regexp"
	"regexp/syntax"
)

const maximumCheckedRegexpBytes = 64 << 10

type invalidRegexpRule struct{}

type zeroRegexpMatchLimitRule struct{}

// NewInvalidRegexpRule constructs the constant regexp validation rule for
// product registry composition.
func NewInvalidRegexpRule() Rule {
	return invalidRegexpRule{}
}

// NewZeroRegexpMatchLimitRule constructs the zero FindAll limit rule for
// product registry composition.
func NewZeroRegexpMatchLimitRule() Rule {
	return zeroRegexpMatchLimitRule{}
}

func (invalidRegexpRule) Metadata() Metadata {
	return Metadata{
		ID: "invalid-regexp",
		Summary: "detects invalid constant regular expressions",
		Documentation: "The regexp package parses patterns at runtime. An invalid constant pattern makes MustCompile panic and makes Compile or the Match helpers return an error instead of performing the intended match.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only direct calls to exact regexp compilation and Match helpers with compile-time constant patterns are recognized; function values remain conservative.",
			"Patterns larger than 64 KiB are skipped to bound analysis work.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Use a valid regular expression",
				Incorrect: `pattern := regexp.MustCompile("[")`,
				Correct: `pattern := regexp.MustCompile("[a-z]")`,
			},
		},
	}
}

func (invalidRegexpRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"invalid-regexp requires a call expression and type information",
		)
	}
	posix, recognized := regexpPatternCall(ctx.Info(), call)
	if !recognized {
		return nil, nil
	}
	argument := ast.Unparen(call.Args[0])
	value := ctx.Info().Types[argument].Value
	if value == nil || value.Kind() != constant.String {
		return nil, nil
	}
	pattern := constant.StringVal(value)
	if len(pattern) > maximumCheckedRegexpBytes {
		return nil, nil
	}
	var compileErr error
	if posix {
		_, compileErr = regexp.CompilePOSIX(pattern)
	} else {
		_, compileErr = regexp.Compile(pattern)
	}
	if compileErr == nil {
		return nil, nil
	}
	description := "invalid syntax"
	var syntaxErr *syntax.Error
	if errors.As(compileErr, &syntaxErr) {
		description = syntaxErr.Code.String()
	}
	range_, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "invalid-pattern",
			Message: "invalid constant regular expression: " + description,
			Range: range_,
			Help: "correct the regular expression syntax",
		},
	}, nil
}

func regexpPatternCall(info *types.Info, call *ast.CallExpr) (bool, bool) {
	if len(call.Args) == 0 {
		return false, false
	}
	for _, name := range
		[]string{"Compile", "MustCompile", "Match", "MatchReader", "MatchString"} {
		if isStandardFunction(info, call.Fun, "regexp", name) {
			return false, true
		}
	}
	for _, name := range []string{"CompilePOSIX", "MustCompilePOSIX"} {
		if isStandardFunction(info, call.Fun, "regexp", name) {
			return true, true
		}
	}
	return false, false
}

func (zeroRegexpMatchLimitRule) Metadata() Metadata {
	return Metadata{
		ID: "zero-regexp-match-limit",
		Summary: "detects regexp FindAll calls whose zero limit returns no matches",
		Documentation: "Regexp FindAll methods return at most n matches when n is nonnegative. A compile-time zero limit therefore always returns an empty result, while a negative limit requests every match.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only direct calls to exact *regexp.Regexp FindAll methods are recognized; method values and interface dispatch remain conservative.",
			"Only compile-time integer zero limits are reported; value flow through variables is not inferred.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Request all matches",
				Incorrect: `matches := pattern.FindAllString(input, 0)`,
				Correct: `matches := pattern.FindAllString(input, -1)`,
			},
		},
	}
}

func (zeroRegexpMatchLimitRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"zero-regexp-match-limit requires a call expression and type information",
		)
	}
	if len(call.Args) != 2 || !isRegexpFindAllCall(ctx.Info(), call) {
		return nil, nil
	}
	limit := ast.Unparen(call.Args[1])
	value := ctx.Info().Types[limit].Value
	if value == nil || value.Kind() != constant.Int || constant.Sign(value) != 0 {
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[1])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "zero-limit",
			Message: "regexp FindAll limit zero always returns no matches",
			Range: range_,
			Help: "pass a positive match limit or a negative limit for all matches",
		},
	}, nil
}

func isRegexpFindAllCall(info *types.Info, call *ast.CallExpr) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "regexp" {
		return false
	}
	switch function.Name() {
	case "FindAll",
		"FindAllIndex",
		"FindAllString",
		"FindAllStringIndex",
		"FindAllStringSubmatch",
		"FindAllStringSubmatchIndex",
		"FindAllSubmatch",
		"FindAllSubmatchIndex":
	default:
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil &&
		signature.Recv() != nil &&
		isNamedReceiver(signature.Recv().Type(), "regexp", "Regexp")
}
