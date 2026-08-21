package rules

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"
)

type restrictionPolicyKind uint8

const (
	restrictDirectPanic restrictionPolicyKind = iota
	restrictProcessExit
	restrictContextBackground
	restrictContextTODO
)

type restrictionPolicyRule struct {
	kind restrictionPolicyKind
}

// NewDirectPanicRule constructs the direct panic policy rule.
func NewDirectPanicRule() Rule {
	return restrictionPolicyRule{kind: restrictDirectPanic}
}

// NewProcessExitRule constructs the process termination policy rule.
func NewProcessExitRule() Rule {
	return restrictionPolicyRule{kind: restrictProcessExit}
}

// NewContextBackgroundRule constructs the root background context policy rule.
func NewContextBackgroundRule() Rule {
	return restrictionPolicyRule{kind: restrictContextBackground}
}

// NewContextTODORule constructs the placeholder context policy rule.
func NewContextTODORule() Rule {
	return restrictionPolicyRule{kind: restrictContextTODO}
}

func (rule restrictionPolicyRule) Metadata() Metadata {
	includeTestsDefault := BooleanOption(false)
	metadata := Metadata{
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetRestriction},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Options: []OptionMetadata{
			{
				Name: "include-tests",
				Summary: "report restricted calls in files whose base name ends in _test.go",
				Kind: OptionBoolean,
				Default: &includeTestsDefault,
			},
		},
	}
	switch rule.kind {
	case restrictDirectPanic:
		metadata.ID = "direct-panic"
		metadata.Summary = "forbids direct calls to the predeclared panic function"
		metadata.Documentation = "Libraries and long-running services may require failures to cross an explicit error boundary instead of unwinding the current goroutine through panic. This restriction rule reports direct calls to the exact predeclared panic function while leaving project-defined lookalikes alone."
		metadata.Categories = []Category{CategorySafety, CategoryMaintainability}
		metadata.KnownLimitations = []string{
			"The rule intentionally reports panic calls that are valid recovery or invariant-enforcement strategies; enable it only where project policy forbids direct panic.",
			"Panics raised indirectly by library APIs, runtime checks, or called functions are outside the direct-call contract.",
			"Test files are excluded by default because panic behavior is commonly exercised in tests; include-tests enables the same policy for tests.",
			"Generated files and packages with type errors are excluded.",
		}
		metadata.Examples = []Example{
			{
				Title: "Return failures from library code",
				Incorrect: "panic(err)",
				Correct: "return fmt.Errorf(\"load configuration: %w\", err)",
			},
		}
	case restrictProcessExit:
		metadata.ID = "process-exit"
		metadata.Summary = "forbids direct process termination through os and log APIs"
		metadata.Documentation = "Reusable packages and managed services should return control to their caller instead of terminating the process. This restriction rule reports exact os.Exit calls and exact log.Fatal, Fatalf, and Fatalln package or Logger method calls, all of which terminate the process and bypass deferred cleanup in the terminating goroutine."
		metadata.Categories = []Category{CategorySafety, CategoryMaintainability}
		metadata.KnownLimitations = []string{
			"The rule intentionally reports process termination in command entry points; enable it only where callers must own termination or use a narrow suppression at the executable boundary.",
			"Only direct exact os.Exit and log fatal-family calls are covered; aliases stored in variables, wrappers, and third-party logging APIs require separate contracts.",
			"Test files are excluded by default because subprocess fixtures may deliberately exercise termination; include-tests enables the same policy for tests.",
			"Generated files and packages with type errors are excluded.",
		}
		metadata.Examples = []Example{
			{
				Title: "Return termination decisions to the caller",
				Incorrect: "log.Fatalf(\"serve: %v\", err)",
				Correct: "return fmt.Errorf(\"serve: %w\", err)",
			},
		}
	case restrictContextBackground:
		metadata.ID = "context-background"
		metadata.Summary = "forbids creating root work with context.Background"
		metadata.Documentation = "Request and job code should normally propagate its caller's context so cancellation, deadlines, and tracing remain connected. This restriction rule reports direct calls to the exact context.Background function. Executable roots and deliberately detached work can use narrow suppressions when they own the lifetime boundary."
		metadata.Categories = []Category{CategorySafety, CategoryMaintainability}
		metadata.KnownLimitations = []string{
			"The rule cannot infer whether a call occurs at a legitimate process root or whether detached work is intentional; those sites require a narrow suppression.",
			"Only direct exact context.Background calls are covered; wrappers and values returned by helpers require a project semantic contract or deeper value flow.",
			"Test files are excluded by default because isolated fixtures commonly create root contexts; include-tests enables the same policy for tests.",
			"Generated files and packages with type errors are excluded.",
		}
		metadata.Examples = []Example{
			{
				Title: "Propagate the caller context",
				Incorrect: "result, err := load(context.Background())",
				Correct: "result, err := load(ctx)",
			},
		}
	case restrictContextTODO:
		metadata.ID = "context-todo"
		metadata.Summary = "forbids unresolved placeholder contexts"
		metadata.Documentation = "context.TODO documents that the correct context has not yet been determined. Projects that require complete cancellation propagation can enable this restriction rule to make every remaining placeholder explicit and suppress only deliberate compatibility boundaries."
		metadata.Categories = []Category{CategorySafety, CategoryMaintainability}
		metadata.KnownLimitations = []string{
			"The rule intentionally reports context.TODO at incomplete integration boundaries where the placeholder may be appropriate; use a reasoned suppression until the caller contract is available.",
			"Only direct exact context.TODO calls are covered; wrappers and values returned by helpers require deeper value flow.",
			"Test files are excluded by default because fixtures commonly use placeholder contexts; include-tests enables the same policy for tests.",
			"Generated files and packages with type errors are excluded.",
		}
		metadata.Examples = []Example{
			{
				Title: "Propagate an owned context",
				Incorrect: "result, err := load(context.TODO())",
				Correct: "result, err := load(ctx)",
			},
		}
	}
	return metadata
}

func (rule restrictionPolicyRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"%s requires a call expression and complete type information",
			rule.Metadata().ID,
		)
	}
	includeTests, found := ctx.BooleanOption("include-tests")
	if !found {
		return nil, fmt.Errorf("%s requires the include-tests option", rule.Metadata().ID)
	}
	if !includeTests && strings.HasSuffix(filepath.Base(ctx.File().Path()), "_test.go") {
		return nil, nil
	}
	messageKey, message, help := rule.restrictedCall(ctx.Info(), call)
	if messageKey == "" {
		return nil, nil
	}
	range_, err := ctx.Range(call.Fun)
	if err != nil {
		return nil, err
	}
	return []Finding{{MessageKey: messageKey, Message: message, Range: range_, Help: help}}, nil
}

func (rule restrictionPolicyRule) restrictedCall(
	info *types.Info,
	call *ast.CallExpr,
) (string, string, string) {
	object := restrictionCallObject(info, call)
	switch rule.kind {
	case restrictDirectPanic:
		if object == types.Universe.Lookup("panic") {
			return "direct-panic", "direct panic invocation is forbidden by project policy", "return the failure or suppress this deliberate panic with a reason"
		}
	case restrictProcessExit:
		function, _ := object.(*types.Func)
		if processExitFunction(function) {
			return "process-exit", "direct process termination is forbidden by project policy", "return the failure to the process boundary or suppress this deliberate exit with a reason"
		}
	case restrictContextBackground:
		if exactPackageFunction(object, "context", "Background") {
			return "context-background", "context.Background disconnects work from caller cancellation", "propagate the caller context or suppress this deliberate root context with a reason"
		}
	case restrictContextTODO:
		if exactPackageFunction(object, "context", "TODO") {
			return "context-todo", "context.TODO leaves the context propagation contract unresolved", "propagate an owned context or suppress this deliberate placeholder with a reason"
		}
	}
	return "", "", ""
}

func restrictionCallObject(info *types.Info, call *ast.CallExpr) types.Object {
	if info == nil || call == nil {
		return nil
	}
	switch function := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		return info.ObjectOf(function)
	case *ast.SelectorExpr:
		return info.ObjectOf(function.Sel)
	default:
		return nil
	}
}

func exactPackageFunction(object types.Object, packagePath string, name string) bool {
	function, _ := object.(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != packagePath ||
		function.Name() != name {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && signature.Recv() == nil
}

func processExitFunction(function *types.Func) bool {
	if function == nil || function.Pkg() == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return false
	}
	if function.Pkg().Path() == "os" && function.Name() == "Exit" && signature.Recv() == nil {
		return true
	}
	if function.Pkg().Path() != "log" {
		return false
	}
	switch function.Name() {
	case "Fatal", "Fatalf", "Fatalln":
		return signature.Recv() == nil ||
			isNamedReceiver(signature.Recv().Type(), "log", "Logger")
	default:
		return false
	}
}
