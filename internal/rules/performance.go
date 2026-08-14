package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
)

type regexpCompileInLoopRule struct{}

type syncPoolNonPointerRule struct{}

type stringRangeRuneConversionRule struct{}

type inefficientIOStringWriteRule struct{}

// NewRegexpCompileInLoopRule constructs the repeated regexp compilation rule.
func NewRegexpCompileInLoopRule() Rule {
	return regexpCompileInLoopRule{}
}

// NewSyncPoolNonPointerRule constructs the sync.Pool boxing rule.
func NewSyncPoolNonPointerRule() Rule {
	return syncPoolNonPointerRule{}
}

// NewStringRangeRuneConversionRule constructs the redundant rune conversion rule.
func NewStringRangeRuneConversionRule() Rule {
	return stringRangeRuneConversionRule{}
}

// NewInefficientIOStringWriteRule constructs the byte-slice string conversion rule.
func NewInefficientIOStringWriteRule() Rule {
	return inefficientIOStringWriteRule{}
}

func (regexpCompileInLoopRule) Metadata() Metadata {
	return Metadata{
		ID: "regexp-compile-in-loop",
		Summary: "detects repeated compilation of constant regular expressions in loops",
		Documentation: "The regexp.Match, regexp.MatchReader, and regexp.MatchString helpers compile their pattern on every call. Calling them with a constant pattern in a loop repeats compilation work that can be moved outside the loop with regexp.Compile or regexp.MustCompile.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPerformance},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeForStmt, NodeRangeStmt},
		Categories: []Category{CategoryPerformance},
		KnownLimitations: []string{
			"Only direct calls to the standard regexp.Match, regexp.MatchReader, and regexp.MatchString functions with compile-time constant patterns are reported.",
			"Immediately invoked function literals are included; function literals stored or passed for later execution are not attributed to the surrounding loop.",
			"The rule offers no automatic fix because choosing declaration scope, error handling, and Compile versus MustCompile requires program intent.",
		},
		Examples: []Example{
			{
				Title: "Compile a constant pattern once",
				Incorrect: "for _, value := range values { regexp.MatchString(`^[a-z]+$`, value) }",
				Correct: "pattern := regexp.MustCompile(`^[a-z]+$`)\nfor _, value := range values { pattern.MatchString(value) }",
			},
		},
	}
}

func (regexpCompileInLoopRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("regexp-compile-in-loop requires complete type information")
	}
	var roots []ast.Node
	switch loop := node.(type) {
	case *ast.ForStmt:
		if loop.Cond != nil {
			roots = append(roots, loop.Cond)
		}
		if loop.Post != nil {
			roots = append(roots, loop.Post)
		}
		roots = append(roots, loop.Body)
	case *ast.RangeStmt:
		roots = append(roots, loop.Body)
	default:
		return nil, fmt.Errorf("regexp-compile-in-loop requires a for or range statement")
	}
	findings := make([]Finding, 0)
	for _, root := range roots {
		rootFindings, err := repeatedRegexpFindings(ctx, root)
		if err != nil {
			return nil, err
		}
		findings = append(findings, rootFindings...)
	}
	return findings, nil
}

func repeatedRegexpFindings(ctx *TypesContext, root ast.Node) ([]Finding, error) {
	findings := make([]Finding, 0)
	var inspectError error
	ast.Inspect(
		root,
		func(current ast.Node) bool {
			if current == nil || inspectError != nil {
				return false
			}
			switch current.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			}
			call, ok := current.(*ast.CallExpr)
			if !ok {
				return true
			}
			if literal, immediate := ast.Unparen(call.Fun).(*ast.FuncLit); immediate {
				literalFindings, err := repeatedRegexpFindings(ctx, literal.Body)
				if err != nil {
					inspectError = err
					return false
				}
				findings = append(findings, literalFindings...)
			}
			if !constantRegexpHelperCall(ctx.Info(), call) {
				return true
			}
			range_, err := ctx.Range(call)
			if err != nil {
				inspectError = err
				return false
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "regexp-compile-in-loop",
					Message: "constant regular expression is compiled on every loop iteration",
					Range: range_,
					Help: "compile the pattern once outside the loop",
				},
			)
			return true
		},
	)
	return findings, inspectError
}

func constantRegexpHelperCall(info *types.Info, call *ast.CallExpr) bool {
	if call == nil || len(call.Args) == 0 {
		return false
	}
	recognized := false
	for _, name := range []string{"Match", "MatchReader", "MatchString"} {
		if isStandardFunction(info, call.Fun, "regexp", name) {
			recognized = true
			break
		}
	}
	if !recognized {
		return false
	}
	value := info.Types[call.Args[0]].Value
	return value != nil && value.Kind() == constant.String
}

func (syncPoolNonPointerRule) Metadata() Metadata {
	return Metadata{
		ID: "sync-pool-non-pointer",
		Summary: "detects non-pointer values stored in sync.Pool",
		Documentation: "Storing a non-pointer value in sync.Pool generally allocates when the value is boxed into an interface. Pointer-like values avoid that allocation. Slices are intentionally reported because placing their multiword headers in an interface also requires allocation.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPerformance},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryPerformance},
		KnownLimitations: []string{
			"The rule reports only direct calls to (*sync.Pool).Put whose argument has a statically known non-interface type.",
			"Interfaces and type parameters are excluded because their dynamic representation is not known locally.",
			"No fix is offered because allocating, taking an address, or changing the pool's element contract may alter ownership and aliasing.",
		},
		Examples: []Example{
			{
				Title: "Pool pointers instead of values",
				Incorrect: "pool.Put(bytes.Buffer{})",
				Correct: "pool.Put(&bytes.Buffer{})",
			},
		},
	}
}

func (syncPoolNonPointerRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"sync-pool-non-pointer requires a call expression and type information",
		)
	}
	if len(call.Args) != 1 || !isSyncPoolPut(ctx.Info(), call) {
		return nil, nil
	}
	argumentType := ctx.Info().TypeOf(call.Args[0])
	if argumentType == nil || poolValueIsPointerLike(argumentType) {
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "sync-pool-non-pointer",
			Message: "storing this non-pointer value in sync.Pool requires interface boxing",
			Range: range_,
			Help: "store a pointer when the pool's ownership contract permits it",
		},
	}, nil
}

func isSyncPoolPut(info *types.Info, call *ast.CallExpr) bool {
	function, _ := calledFunctionObject(info, call).(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "sync" ||
		function.Name() != "Put" {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil &&
		signature.Recv() != nil &&
		namedReceiver(signature.Recv().Type(), "sync", "Pool")
}

func poolValueIsPointerLike(type_ types.Type) bool {
	if type_ == nil {
		return true
	}
	switch underlying := types.Unalias(type_).Underlying().(type) {
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	case *types.Basic:
		return underlying.Kind() == types.UnsafePointer ||
			underlying.Kind() == types.UntypedNil
	case *types.TypeParam:
		return true
	default:
		return false
	}
}

func (stringRangeRuneConversionRule) Metadata() Metadata {
	return Metadata{
		ID: "string-range-rune-conversion",
		Summary: "detects rune-slice conversions used only to range over a string",
		Documentation: "Ranging directly over a string produces the same decoded rune values as ranging over []rune(string) without allocating a rune slice. The byte index differs from the rune-slice index, so the conversion is reported only when the index is absent or discarded.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPerformance},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeRangeStmt},
		Categories: []Category{CategoryPerformance},
		KnownLimitations: []string{
			"The rule requires a direct conversion from a string-like value to a rune slice in a range statement.",
			"Ranges using a nonblank index are excluded because direct string iteration reports byte offsets instead of rune indexes.",
			"No automatic fix is offered until delimiter-comment ownership is proven for this transformation.",
		},
		Examples: []Example{
			{
				Title: "Range over the string directly",
				Incorrect: "for _, runeValue := range []rune(text) { use(runeValue) }",
				Correct: "for _, runeValue := range text { use(runeValue) }",
			},
		},
	}
}

func (stringRangeRuneConversionRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	rangeStatement, ok := node.(*ast.RangeStmt)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"string-range-rune-conversion requires a range statement and type information",
		)
	}
	if identifier, hasIndex := ast.Unparen(rangeStatement.Key).(*ast.Ident);
		hasIndex && identifier.Name != "_" {
		return nil, nil
	} else if rangeStatement.Key != nil && !hasIndex {
		return nil, nil
	}
	conversion, _ := ast.Unparen(rangeStatement.X).(*ast.CallExpr)
	if conversion == nil ||
		len(conversion.Args) != 1 ||
		!ctx.Info().Types[conversion.Fun].IsType() {
		return nil, nil
	}
	if !isRuneSlice(ctx.Info().TypeOf(conversion.Fun)) ||
		!isStringLike(ctx.Info().TypeOf(conversion.Args[0])) {
		return nil, nil
	}
	range_, err := ctx.Range(conversion)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "string-range-rune-conversion",
			Message: "converting this string to []rune allocates before ranging",
			Range: range_,
			Help: "range over the string directly",
		},
	}, nil
}

func isRuneSlice(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	slice, _ := types.Unalias(type_).Underlying().(*types.Slice)
	return slice != nil && types.Identical(slice.Elem(), types.Typ[types.Rune])
}

func isStringLike(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	basic, _ := types.Unalias(type_).Underlying().(*types.Basic)
	return basic != nil && basic.Kind() == types.String
}

func (inefficientIOStringWriteRule) Metadata() Metadata {
	return Metadata{
		ID: "inefficient-io-string-write",
		Summary: "detects byte-slice conversion passed to io.WriteString",
		Documentation: "Converting a byte slice to string solely for io.WriteString allocates a string representation. Writers already accept byte slices through Write, avoiding that conversion.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPerformance},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryPerformance},
		KnownLimitations: []string{
			"Only direct calls to the standard io.WriteString function with an explicit byte-slice-to-string conversion are reported.",
			"No fix is offered because replacing io.WriteString with Write can change method dispatch for writers implementing both io.Writer and io.StringWriter.",
		},
		Examples: []Example{
			{
				Title: "Write the byte slice directly",
				Incorrect: "io.WriteString(writer, string(data))",
				Correct: "writer.Write(data)",
			},
		},
	}
}

func (inefficientIOStringWriteRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"inefficient-io-string-write requires a call expression and type information",
		)
	}
	if len(call.Args) != 2 || !isStandardFunction(ctx.Info(), call.Fun, "io", "WriteString") {
		return nil, nil
	}
	conversion, _ := ast.Unparen(call.Args[1]).(*ast.CallExpr)
	if conversion == nil ||
		len(conversion.Args) != 1 ||
		!ctx.Info().Types[conversion.Fun].IsType() {
		return nil, nil
	}
	if !isStringLike(ctx.Info().TypeOf(conversion.Fun)) ||
		!isByteSlice(ctx.Info().TypeOf(conversion.Args[0])) {
		return nil, nil
	}
	range_, err := ctx.Range(conversion)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "inefficient-io-string-write",
			Message: "converting this byte slice to string allocates before writing",
			Range: range_,
			Help: "write the byte slice directly when method dispatch is equivalent",
		},
	}, nil
}

func isByteSlice(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	slice, _ := types.Unalias(type_).Underlying().(*types.Slice)
	return slice != nil && types.Identical(slice.Elem(), types.Typ[types.Byte])
}
