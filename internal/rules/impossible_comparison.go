package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"sync"
)

const architectureTypeWorkBudget = 16_384

type impossibleComparisonRule struct{}

// NewImpossibleComparisonRule constructs the integer-boundary rule for product
// registry composition.
func NewImpossibleComparisonRule() Rule {
	return impossibleComparisonRule{}
}

func (impossibleComparisonRule) Metadata() Metadata {
	return Metadata{
		ID: "impossible-comparison",
		Summary: "detects comparisons outside an integer type's value range",
		Documentation: "Ordered comparisons against an integer type's minimum or maximum value can be constant regardless of the runtime operand. Such conditions commonly leave dead branches or accidentally invert a boundary check.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBinaryExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Constants whose package syntax depends on architecture-sized int, uint, or uintptr values are excluded because source selection may target an architecture different from the running Glippy binary; constants without available defining syntax remain conservative.",
			"Architecture-sensitive type provenance uses a fixed per-comparison work budget; comparisons whose proof exceeds it are conservatively left unreported.",
			"The rule reports only comparisons with a compile-time integer constant and does not infer ranges from preceding control flow.",
		},
		Examples: []Example{
			{
				Title: "Use a reachable unsigned boundary",
				Incorrect: "value < 0",
				Correct: "value == 0",
			},
		},
	}
}

func (impossibleComparisonRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	comparison, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, fmt.Errorf("impossible-comparison requires a binary expression")
	}
	if comparison.Op != token.LSS &&
		comparison.Op != token.LEQ &&
		comparison.Op != token.GTR &&
		comparison.Op != token.GEQ {
		return nil, nil
	}
	if constantMayBeArchitectureSized(ctx, comparison) {
		return nil, nil
	}
	variable, value, operator, found := normalizedIntegerConstantComparison(
		ctx.Info(),
		comparison,
	)
	if !found {
		return nil, nil
	}
	minimum, maximum, found := integerTypeExtremes(ctx.Info().TypeOf(variable))
	if !found {
		return nil, nil
	}
	always, found := extremeComparisonResult(operator, value, minimum, maximum)
	if !found {
		return nil, nil
	}
	range_, err := ctx.Range(comparison)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "impossible-comparison",
			Message: fmt.Sprintf(
				"comparison is always %t for type %s",
				always,
				ctx.Info().TypeOf(variable),
			),
			Range: range_,
			Help: "use a boundary comparison that can change with the runtime value",
		},
	}, nil
}

func constantMayBeArchitectureSized(ctx *TypesContext, comparison *ast.BinaryExpr) bool {
	if ctx == nil || ctx.Info() == nil || comparison == nil {
		return false
	}
	info := ctx.Info()
	leftValue := info.Types[comparison.X].Value
	rightValue := info.Types[comparison.Y].Value
	if (leftValue == nil) == (rightValue == nil) {
		return false
	}
	constantExpression := comparison.X
	if rightValue != nil {
		constantExpression = comparison.Y
	}
	return architectureSizedConstantExpression(info, ctx.PackageSyntax(), constantExpression)
}

const architectureConstantWorkBudget = 4_096

func architectureSizedConstantExpression(
	info *types.Info,
	files *PackageSyntax,
	expression ast.Expr,
) bool {
	if info == nil || expression == nil {
		return false
	}
	work := []ast.Expr{expression}
	seen := make(map[*types.Const]struct{})
	remaining := architectureConstantWorkBudget
	for len(work) != 0 {
		if remaining <= 0 {
			return true
		}
		currentExpression := work[len(work) - 1]
		work = work[:len(work) - 1]
		found := false
		exhausted := false
		queueConstant := func(constant *types.Const) bool {
			if constant == nil {
				return false
			}
			if _, visited := seen[constant]; visited {
				return true
			}
			initializer := constantInitializer(info, files, constant)
			if initializer == nil {
				found = true
				return true
			}
			seen[constant] = struct{}{}
			work = append(work, initializer)
			return true
		}
		ast.Inspect(
			currentExpression,
			func(current ast.Node) bool {
				if current == nil || found || exhausted {
					return false
				}
				remaining--
				if remaining < 0 {
					exhausted = true
					return false
				}
				candidate, ok := current.(ast.Expr)
				if !ok {
					return true
				}
				switch candidate := candidate.(type) {
				case *ast.Ident:
					if constant, _ := info.ObjectOf(candidate).(*types.Const);
						queueConstant(constant) {
						return false
					}
				case *ast.SelectorExpr:
					if constant, _ := info.ObjectOf(
						candidate.Sel,
					).(*types.Const);
						queueConstant(constant) {
						return false
					}
				}
				if info.Types[candidate].Value == nil {
					return true
				}
				if architectureSizedConstantValue(
					info.TypeOf(candidate),
					info.Types[candidate].Value,
				) {
					found = true
					return false
				}
				switch candidate := candidate.(type) {
				case *ast.CallExpr:
					if architectureSizedConstantOperation(
						info,
						files,
						candidate,
					) ||
						architectureSizedLengthOperation(
							info,
							files,
							candidate,
						) ||
						architectureSizedConstantConversion(
							info,
							candidate,
						) {
						found = true
						return false
					}
				case *ast.UnaryExpr:
					if candidate.Op == token.XOR &&
						architectureSizedBasic(info.TypeOf(candidate)) {
						found = true
						return false
					}
				}
				return true
			},
		)
		if found || exhausted {
			return true
		}
	}
	return false
}

func architectureSizedConstantConversion(info *types.Info, call *ast.CallExpr) bool {
	if info == nil || call == nil || len(call.Args) != 1 || !info.Types[call.Fun].IsType() {
		return false
	}
	argumentValue := info.Types[call.Args[0]].Value
	return architectureSizedConstantValue(info.TypeOf(call.Fun), argumentValue)
}

func architectureSizedConstantValue(type_ types.Type, value constant.Value) bool {
	if type_ == nil || value == nil {
		return false
	}
	target, _ := types.Unalias(type_).Underlying().(*types.Basic)
	if target == nil {
		return false
	}
	integer := constant.ToInt(value)
	if integer.Kind() != constant.Int {
		return false
	}
	switch target.Kind() {
	case types.Int:
		return constant.Compare(integer, token.LSS, constant.MakeInt64(math.MinInt32)) ||
			constant.Compare(integer, token.GTR, constant.MakeInt64(math.MaxInt32))
	case types.Uint, types.Uintptr:
		return constant.Sign(integer) < 0 ||
			constant.Compare(integer, token.GTR, constant.MakeUint64(math.MaxUint32))
	default:
		return false
	}
}

func architectureSizedConstantOperation(
	info *types.Info,
	files *PackageSyntax,
	call *ast.CallExpr,
) bool {
	if info == nil || call == nil {
		return false
	}
	name, found := unsafeConstantOperationName(info, call)
	if !found || len(call.Args) != 1 {
		return false
	}
	return unsafeOperationVariesByArchitecture(info, files, call.Args[0], name)
}

func architectureSizedLengthOperation(
	info *types.Info,
	files *PackageSyntax,
	call *ast.CallExpr,
) bool {
	if info == nil || call == nil || len(call.Args) != 1 {
		return false
	}
	identifier, _ := ast.Unparen(call.Fun).(*ast.Ident)
	builtin, _ := info.ObjectOf(identifier).(*types.Builtin)
	if builtin == nil || (builtin.Name() != "len" && builtin.Name() != "cap") {
		return false
	}
	return architectureSizedDeclaredExpressionType(info, files, call.Args[0])
}

func unsafeConstantOperationName(info *types.Info, call *ast.CallExpr) (string, bool) {
	function := ast.Unparen(call.Fun)
	if identifier, _ := function.(*ast.Ident); identifier != nil {
		object := info.ObjectOf(identifier)
		if object == nil || object.Pkg() == nil || object.Pkg().Path() != "unsafe" {
			return "", false
		}
		return object.Name(), unsafeConstantOperation(object.Name())
	}
	selector, _ := function.(*ast.SelectorExpr)
	if selector == nil {
		return "", false
	}
	identifier, _ := ast.Unparen(selector.X).(*ast.Ident)
	if identifier == nil {
		return "", false
	}
	packageName, _ := info.ObjectOf(identifier).(*types.PkgName)
	if packageName == nil ||
		packageName.Imported() == nil ||
		packageName.Imported().Path() != "unsafe" {
		return "", false
	}
	return selector.Sel.Name, unsafeConstantOperation(selector.Sel.Name)
}

func unsafeOperationVariesByArchitecture(
	info *types.Info,
	files *PackageSyntax,
	argument ast.Expr,
	operation string,
) bool {
	if info == nil || argument == nil {
		return true
	}
	sizes32 := types.SizesFor("gc", "386")
	sizes64 := types.SizesFor("gc", "amd64")
	if sizes32 == nil || sizes64 == nil {
		return true
	}
	provenance := newArchitectureTypeProvenance(info, files)
	if provenance.declaredExpression(argument) || provenance.expression(argument) {
		return true
	}
	switch operation {
	case "Sizeof":
		type_ := info.TypeOf(argument)
		return type_ == nil || sizes32.Sizeof(type_) != sizes64.Sizeof(type_)
	case "Alignof":
		type_ := info.TypeOf(argument)
		return type_ == nil || sizes32.Alignof(type_) != sizes64.Alignof(type_)
	case "Offsetof":
		selector, _ := ast.Unparen(argument).(*ast.SelectorExpr)
		selection := info.Selections[selector]
		if architectureSizedSelectionLayout(info, files, selection) {
			return true
		}
		offset32, valid32 := unsafeSelectionOffset(sizes32, selection)
		offset64, valid64 := unsafeSelectionOffset(sizes64, selection)
		return !valid32 || !valid64 || offset32 != offset64
	default:
		return true
	}
}

func architectureSizedDeclaredExpressionType(
	info *types.Info,
	files *PackageSyntax,
	expression ast.Expr,
) bool {
	return newArchitectureTypeProvenance(info, files).declaredExpression(expression)
}

func (provenance *architectureTypeProvenance) declaredExpression(expression ast.Expr) bool {
	if provenance == nil || provenance.info == nil || expression == nil {
		return false
	}
	info := provenance.info
	files := provenance.files
	if identifier, _ := ast.Unparen(expression).(*ast.Ident); identifier != nil {
		if variable, _ := info.ObjectOf(identifier).(*types.Var); variable != nil {
			if declaration := variableTypeExpression(info, files, variable);
				declaration != nil && provenance.expression(declaration) {
				return true
			}
		}
	}
	if selector, _ := ast.Unparen(expression).(*ast.SelectorExpr); selector != nil {
		selection := info.Selections[selector]
		if selection != nil && selection.Kind() == types.FieldVal {
			field, _ := selection.Obj().(*types.Var)
			declaration := fieldTypeExpression(info, files, field)
			if declaration != nil && provenance.expression(declaration) {
				return true
			}
		}
	}
	return provenance.namedType(info.TypeOf(expression))
}

type architectureTypeState uint8

const (
	architectureTypeUnknown architectureTypeState = iota
	architectureTypeVisiting
	architectureTypePortable
	architectureTypeSized
)

type architectureTypeProvenance struct {
	info *types.Info
	files *PackageSyntax
	index *architectureSyntaxIndex
	states map[*types.TypeName]architectureTypeState
	activeExpressions map[ast.Expr]struct{}
	remaining int
	exhausted bool
}

func newArchitectureTypeProvenance(
	info *types.Info,
	files *PackageSyntax,
) *architectureTypeProvenance {
	return &architectureTypeProvenance{
		info: info,
		files: files,
		index: architectureSyntaxIndexFor(info, files),
		states: make(map[*types.TypeName]architectureTypeState),
		activeExpressions: make(map[ast.Expr]struct{}),
		remaining: architectureTypeWorkBudget,
	}
}

func (provenance *architectureTypeProvenance) consume() bool {
	if provenance == nil || provenance.remaining <= 0 {
		if provenance != nil {
			provenance.exhausted = true
		}
		return false
	}
	provenance.remaining--
	return true
}

func (provenance *architectureTypeProvenance) namedType(type_ types.Type) bool {
	if provenance == nil || provenance.info == nil || type_ == nil {
		return false
	}
	type_ = types.Unalias(type_)
	if pointer, _ := type_.(*types.Pointer); pointer != nil {
		type_ = types.Unalias(pointer.Elem())
	}
	named, _ := type_.(*types.Named)
	if named == nil {
		return false
	}
	for index := 0; index < named.TypeArgs().Len(); index++ {
		if provenance.namedType(named.TypeArgs().At(index)) {
			return true
		}
	}
	typeName := named.Obj()
	if typeName == nil {
		return false
	}
	return provenance.typeName(typeName)
}

func (provenance *architectureTypeProvenance) typeName(typeName *types.TypeName) bool {
	if provenance == nil || typeName == nil {
		return false
	}
	if state := provenance.states[typeName]; state != architectureTypeUnknown {
		return state == architectureTypeSized
	}
	if state, found := provenance.index.typeState(typeName); found {
		provenance.states[typeName] = state
		return state == architectureTypeSized
	}
	if !provenance.consume() {
		return true
	}
	provenance.states[typeName] = architectureTypeVisiting
	declaration := typeNameExpression(provenance.info, provenance.files, typeName)
	var sized bool
	if declaration == nil {
		switch types.Unalias(typeName.Type()).Underlying().(type) {
		case *types.Array, *types.Struct:
			sized = true
		}
	} else {
		sized = provenance.expression(declaration)
	}
	if provenance.exhausted {
		delete(provenance.states, typeName)
		return true
	}
	state := architectureTypePortable
	if sized {
		state = architectureTypeSized
	}
	provenance.states[typeName] = state
	provenance.index.storeTypeState(typeName, state)
	return sized
}

type architectureSyntaxIndex struct {
	constants map[*types.Const]ast.Expr
	variables map[*types.Var]ast.Expr
	types map[*types.TypeName]ast.Expr
	fields map[*types.Var]ast.Expr
	fieldPositions map[token.Pos]ast.Expr
	typeStatesMu sync.RWMutex
	typeStates map[*types.TypeName]architectureTypeState
}

func (index *architectureSyntaxIndex) typeState(
	typeName *types.TypeName,
) (architectureTypeState, bool) {
	if index == nil || typeName == nil {
		return architectureTypeUnknown, false
	}
	index.typeStatesMu.RLock()
	defer index.typeStatesMu.RUnlock()
	state, found := index.typeStates[typeName]
	return state, found
}

func (index *architectureSyntaxIndex) storeTypeState(
	typeName *types.TypeName,
	state architectureTypeState,
) {
	if index == nil ||
		typeName == nil ||
		(state != architectureTypePortable && state != architectureTypeSized) {
		return
	}
	index.typeStatesMu.Lock()
	index.typeStates[typeName] = state
	index.typeStatesMu.Unlock()
}

func architectureSyntaxIndexFor(info *types.Info, files *PackageSyntax) *architectureSyntaxIndex {
	if files == nil {
		return newArchitectureSyntaxIndex(info, files)
	}
	value := files.memoized(
		"impossible-comparison/architecture-syntax-v1",
		func() any {
			return newArchitectureSyntaxIndex(info, files)
		},
	)
	index, _ := value.(*architectureSyntaxIndex)
	if index == nil {
		return newArchitectureSyntaxIndex(info, files)
	}
	return index
}

func newArchitectureSyntaxIndex(info *types.Info, files *PackageSyntax) *architectureSyntaxIndex {
	index := &architectureSyntaxIndex{
		constants: make(map[*types.Const]ast.Expr),
		variables: make(map[*types.Var]ast.Expr),
		types: make(map[*types.TypeName]ast.Expr),
		fields: make(map[*types.Var]ast.Expr),
		fieldPositions: make(map[token.Pos]ast.Expr),
		typeStates: make(map[*types.TypeName]architectureTypeState),
	}
	if info == nil || files == nil {
		return index
	}
	for fileIndex := 0; fileIndex < files.Len(); fileIndex++ {
		file := files.At(fileIndex)
		if file == nil {
			continue
		}
		ast.Inspect(
			file,
			func(current ast.Node) bool {
				switch current := current.(type) {
				case *ast.ValueSpec:
					for valueIndex, name := range current.Names {
						switch object := info.ObjectOf(name).(type) {
						case *types.Const:
							if valueIndex < len(current.Values) {
								index.constants[object] = current.Values[valueIndex]
							}
						case *types.Var:
							if current.Type != nil {
								index.variables[object] = current.Type
							} else if valueIndex < len(current.Values) {
								index.variables[object] = current.Values[valueIndex]
							}
						}
					}
				case *ast.AssignStmt:
					if current.Tok != token.DEFINE ||
						len(current.Lhs) != len(current.Rhs) {
						return true
					}
					for valueIndex, target := range current.Lhs {
						identifier, _ := ast.Unparen(target).(*ast.Ident)
						variable, _ := info.ObjectOf(
							identifier,
						).(*types.Var)
						if variable != nil {
							index.variables[variable] = current.Rhs[valueIndex]
						}
					}
				case *ast.Field:
					index.fieldPositions[current.Pos()] = current.Type
					for _, name := range current.Names {
						variable, _ := info.ObjectOf(name).(*types.Var)
						if variable != nil {
							index.variables[variable] = current.Type
							index.fields[variable] = current.Type
						}
					}
				case *ast.TypeSpec:
					typeName, _ := info.ObjectOf(current.Name).(*types.TypeName)
					if typeName != nil {
						index.types[typeName] = current.Type
					}
				}
				return true
			},
		)
	}
	return index
}

func variableTypeExpression(info *types.Info, files *PackageSyntax, variable *types.Var) ast.Expr {
	if info == nil || variable == nil || !variable.Pos().IsValid() {
		return nil
	}
	return architectureSyntaxIndexFor(info, files).variables[variable]
}

func (provenance *architectureTypeProvenance) expression(expression ast.Expr) bool {
	if provenance == nil || provenance.info == nil || expression == nil {
		return false
	}
	if _, active := provenance.activeExpressions[expression]; active {
		return false
	}
	if !provenance.consume() {
		return true
	}
	provenance.activeExpressions[expression] = struct{}{}
	defer delete(provenance.activeExpressions, expression)
	info := provenance.info
	files := provenance.files
	found := false
	ast.Inspect(
		expression,
		func(current ast.Node) bool {
			if current == nil || found {
				return false
			}
			if !provenance.consume() {
				found = true
				return false
			}
			switch current := current.(type) {
			case *ast.ArrayType:
				if current.Len != nil &&
					architectureSizedConstantExpression(
						info,
						files,
						current.Len,
					) {
					found = true
					return false
				}
			case *ast.Ident:
				if variable, _ := info.ObjectOf(current).(*types.Var);
					variable != nil {
					declaration := variableTypeExpression(info, files, variable)
					if declaration != nil && declaration != current {
						found = provenance.expression(declaration)
					}
					return !found
				}
				typeName, _ := info.ObjectOf(current).(*types.TypeName)
				if typeName == nil || typeName.Pkg() == nil {
					return true
				}
				found = provenance.typeName(typeName)
				return !found
			}
			return true
		},
	)
	return found
}

func typeNameExpression(info *types.Info, files *PackageSyntax, typeName *types.TypeName) ast.Expr {
	if info == nil || typeName == nil {
		return nil
	}
	return architectureSyntaxIndexFor(info, files).types[typeName]
}

func architectureSizedSelectionLayout(
	info *types.Info,
	files *PackageSyntax,
	selection *types.Selection,
) bool {
	if info == nil || selection == nil || selection.Kind() != types.FieldVal {
		return true
	}
	type_ := selection.Recv()
	provenance := newArchitectureTypeProvenance(info, files)
	if provenance.namedType(type_) {
		return true
	}
	for depth, selectedIndex := range selection.Index() {
		type_ = types.Unalias(type_)
		if pointer, ok := type_.(*types.Pointer); ok {
			if depth != 0 {
				return true
			}
			type_ = types.Unalias(pointer.Elem())
		}
		structure, _ := type_.Underlying().(*types.Struct)
		if structure == nil || selectedIndex < 0 || selectedIndex >= structure.NumFields() {
			return true
		}
		for index := 0; index <= selectedIndex; index++ {
			field := structure.Field(index)
			expression := fieldTypeExpression(info, files, field)
			if expression == nil || provenance.expression(expression) {
				return true
			}
		}
		type_ = structure.Field(selectedIndex).Type()
	}
	return false
}

func fieldTypeExpression(info *types.Info, files *PackageSyntax, field *types.Var) ast.Expr {
	if info == nil || field == nil || !field.Pos().IsValid() {
		return nil
	}
	index := architectureSyntaxIndexFor(info, files)
	if expression := index.fields[field]; expression != nil {
		return expression
	}
	return index.fieldPositions[field.Pos()]
}

func unsafeSelectionOffset(sizes types.Sizes, selection *types.Selection) (int64, bool) {
	if sizes == nil || selection == nil || selection.Kind() != types.FieldVal {
		return 0, false
	}
	type_ := selection.Recv()
	offset := int64(0)
	for depth, index := range selection.Index() {
		type_ = types.Unalias(type_)
		if pointer, ok := type_.(*types.Pointer); ok {
			if depth != 0 {
				return 0, false
			}
			type_ = types.Unalias(pointer.Elem())
		}
		structure, _ := type_.Underlying().(*types.Struct)
		if structure == nil || index < 0 || index >= structure.NumFields() {
			return 0, false
		}
		fields := make([]*types.Var, structure.NumFields())
		for fieldIndex := range structure.NumFields() {
			fields[fieldIndex] = structure.Field(fieldIndex)
		}
		offsets := sizes.Offsetsof(fields)
		if len(offsets) != len(fields) || offsets[index] < 0 {
			return 0, false
		}
		offset += offsets[index]
		type_ = structure.Field(index).Type()
	}
	return offset, true
}

func unsafeConstantOperation(name string) bool {
	switch name {
	case "Alignof", "Offsetof", "Sizeof":
		return true
	default:
		return false
	}
}

func architectureSizedBasic(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	basic, _ := types.Unalias(type_).Underlying().(*types.Basic)
	if basic == nil {
		return false
	}
	switch basic.Kind() {
	case types.Int, types.Uint, types.Uintptr:
		return true
	default:
		return false
	}
}

func constantInitializer(info *types.Info, files *PackageSyntax, constant *types.Const) ast.Expr {
	if info == nil || constant == nil {
		return nil
	}
	return architectureSyntaxIndexFor(info, files).constants[constant]
}

func normalizedIntegerConstantComparison(
	info *types.Info,
	comparison *ast.BinaryExpr,
) (ast.Expr, constant.Value, token.Token, bool) {
	if info == nil || comparison == nil {
		return nil, nil, token.ILLEGAL, false
	}
	leftValue := info.Types[comparison.X].Value
	rightValue := info.Types[comparison.Y].Value
	if (leftValue == nil) == (rightValue == nil) {
		return nil, nil, token.ILLEGAL, false
	}
	if rightValue != nil && rightValue.Kind() == constant.Int {
		return comparison.X, rightValue, comparison.Op, true
	}
	if leftValue == nil || leftValue.Kind() != constant.Int {
		return nil, nil, token.ILLEGAL, false
	}
	return comparison.Y, leftValue, reverseOrderedComparison(comparison.Op), true
}

func reverseOrderedComparison(operator token.Token) token.Token {
	switch operator {
	case token.EQL, token.NEQ:
		return operator
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	default:
		return token.ILLEGAL
	}
}

func integerTypeExtremes(type_ types.Type) (constant.Value, constant.Value, bool) {
	if type_ == nil {
		return nil, nil, false
	}
	basic, ok := types.Unalias(type_).Underlying().(*types.Basic)
	if !ok {
		return nil, nil, false
	}
	switch basic.Kind() {
	case types.Uint, types.Uintptr:
		return constant.MakeUint64(0), nil, true
	case types.Uint8:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint8), true
	case types.Uint16:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint16), true
	case types.Uint32:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint32), true
	case types.Uint64:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint64), true
	case types.Int8:
		return constant.MakeInt64(math.MinInt8), constant.MakeInt64(math.MaxInt8), true
	case types.Int16:
		return constant.MakeInt64(math.MinInt16), constant.MakeInt64(math.MaxInt16), true
	case types.Int32:
		return constant.MakeInt64(math.MinInt32), constant.MakeInt64(math.MaxInt32), true
	case types.Int64:
		return constant.MakeInt64(math.MinInt64), constant.MakeInt64(math.MaxInt64), true
	default:
		return nil, nil, false
	}
}

func extremeComparisonResult(
	operator token.Token,
	value constant.Value,
	minimum constant.Value,
	maximum constant.Value,
) (bool, bool) {
	if minimum != nil && constant.Compare(value, token.EQL, minimum) {
		switch operator {
		case token.LSS:
			return false, true
		case token.GEQ:
			return true, true
		}
	}
	if maximum != nil && constant.Compare(value, token.EQL, maximum) {
		switch operator {
		case token.GTR:
			return false, true
		case token.LEQ:
			return true, true
		}
	}
	return false, false
}
