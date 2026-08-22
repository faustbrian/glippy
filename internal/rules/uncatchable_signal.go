package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type uncatchableSignalRule struct{}

// NewUncatchableSignalRule constructs the uncatchable os/signal argument rule
// for product registry composition.
func NewUncatchableSignalRule() Rule {
	return uncatchableSignalRule{}
}

func (uncatchableSignalRule) Metadata() Metadata {
	return Metadata{
		ID:               "uncatchable-signal",
		Summary:          "detects attempts to handle SIGKILL or SIGSTOP",
		Documentation:    "SIGKILL and SIGSTOP cannot be caught by a Go program, so passing either signal to os/signal notification or disposition functions cannot make the requested handling take effect.",
		DefaultSeverity:  SeverityWarn,
		Presets:          []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement:      RequireTypes,
		NodeInterests:    []NodeKind{NodeCallExpr},
		Categories:       []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only direct calls to os/signal.Ignore, Notify, NotifyContext, and Reset are recognized by typed object identity.",
			"Only direct references to os.Kill, syscall.SIGKILL, and syscall.SIGSTOP are recognized, including references wrapped in explicit type conversions; constants and values copied through local aliases remain conservative.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title:     "Register a signal the program can receive",
				Incorrect: "signal.Notify(signals, syscall.SIGKILL)",
				Correct:   "signal.Notify(signals, syscall.SIGTERM)",
			},
		},
	}
}

func (uncatchableSignalRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"uncatchable-signal requires a call expression and type information",
		)
	}
	function := directStandardFunction(ctx.Info(), call.Fun, "os/signal")
	if function == nil {
		return nil, nil
	}
	firstSignal, recognized := signalArgumentStart(function.Name())
	if !recognized || firstSignal >= len(call.Args) {
		return nil, nil
	}
	findings := make([]Finding, 0, len(call.Args)-firstSignal)
	for _, argument := range call.Args[firstSignal:] {
		if !isUncatchableSignal(ctx.Info(), argument) {
			continue
		}
		range_, err := ctx.Range(argument)
		if err != nil {
			return nil, err
		}
		findings = append(findings, Finding{
			MessageKey: "uncatchable-signal",
			Message:    "this signal cannot be caught or affected by os/signal",
			Range:      range_,
			Help:       "remove the uncatchable signal from this call",
		})
	}
	return findings, nil
}

func signalArgumentStart(name string) (int, bool) {
	switch name {
	case "Ignore", "Reset":
		return 0, true
	case "Notify", "NotifyContext":
		return 1, true
	default:
		return 0, false
	}
}

func isUncatchableSignal(info *types.Info, expression ast.Expr) bool {
	expression = ast.Unparen(expression)
	if conversion, ok := expression.(*ast.CallExpr); ok &&
		len(conversion.Args) == 1 &&
		info.Types[conversion.Fun].IsType() {
		return isUncatchableSignal(info, conversion.Args[0])
	}
	object := referencedObject(info, expression)
	if object == nil || object.Pkg() == nil {
		return false
	}
	switch object.Pkg().Path() {
	case "os":
		_, variable := object.(*types.Var)
		return variable && object.Name() == "Kill"
	case "syscall":
		_, constant := object.(*types.Const)
		return constant && (object.Name() == "SIGKILL" || object.Name() == "SIGSTOP")
	default:
		return false
	}
}
