package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

type invalidStrconvArgumentRule struct{}

type invalidBinaryWriteRule struct{}

// NewInvalidStrconvArgumentRule constructs the strconv contract rule for
// product registry composition.
func NewInvalidStrconvArgumentRule() Rule {
	return invalidStrconvArgumentRule{}
}

// NewInvalidBinaryWriteRule constructs the encoding/binary data-shape rule for
// product registry composition.
func NewInvalidBinaryWriteRule() Rule {
	return invalidBinaryWriteRule{}
}

func (invalidStrconvArgumentRule) Metadata() Metadata {
	return Metadata{
		ID: "invalid-strconv-argument",
		Summary: "detects invalid constant arguments passed to strconv",
		Documentation: "Strconv parsing and formatting functions accept only documented number bases, bit sizes, and floating-point format bytes. Invalid constants panic, return an error, or produce a placeholder instead of the intended number.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only direct calls to exact strconv package functions are recognized; function values remain conservative.",
			"Only compile-time constant bases, bit sizes, and format bytes are validated; value flow through variables is not inferred.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Use a supported integer base",
				Incorrect: "text := strconv.FormatInt(value, 0)",
				Correct: "text := strconv.FormatInt(value, 10)",
			},
		},
	}
}

func (invalidStrconvArgumentRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"invalid-strconv-argument requires a call expression and type information",
		)
	}
	function := directStandardFunction(ctx.Info(), call.Fun, "strconv")
	if function == nil {
		return nil, nil
	}
	constraints := strconvConstraints(function.Name(), len(call.Args))
	findings := make([]Finding, 0, len(constraints))
	for _, constraint := range constraints {
		if constraint.index >= len(call.Args) {
			continue
		}
		argument := ast.Unparen(call.Args[constraint.index])
		value := ctx.Info().Types[argument].Value
		if value == nil || value.Kind() != constant.Int || constraint.valid(value) {
			continue
		}
		range_, err := ctx.Range(call.Args[constraint.index])
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: constraint.messageKey,
				Message: "strconv." + function.Name() + " " + constraint.message,
				Range: range_,
				Help: constraint.help,
			},
		)
	}
	return findings, nil
}

type strconvConstraint struct {
	index int
	messageKey string
	message string
	help string
	valid func(constant.Value) bool
}

func strconvConstraints(name string, argumentCount int) []strconvConstraint {
	base := func(index int, allowZero bool) strconvConstraint {
		message := "base must be between 2 and 36"
		if allowZero {
			message = "base must be 0 or between 2 and 36"
		}
		return strconvConstraint{
			index: index,
			messageKey: "invalid-base",
			message: message,
			help: "pass a documented number base",
			valid: func(value constant.Value) bool {
				return allowZero && constant.Sign(value) == 0 ||
					constantInRange(value, 2, 36)
			},
		}
	}
	bitSize := func(index int, minimum int64, maximum int64, message string) strconvConstraint {
		return strconvConstraint{
			index: index,
			messageKey: "invalid-bit-size",
			message: message,
			help: "pass a supported bit size",
			valid: func(value constant.Value) bool {
				if minimum == 32 && maximum == 64 {
					return constantEquals(value, 32) ||
						constantEquals(value, 64)
				}
				if minimum == 64 && maximum == 128 {
					return constantEquals(value, 64) ||
						constantEquals(value, 128)
				}
				return constantInRange(value, minimum, maximum)
			},
		}
	}
	format := func(index int) strconvConstraint {
		return strconvConstraint{
			index: index,
			messageKey: "invalid-format",
			message: "format must be one of b, e, E, f, g, G, x, or X",
			help: "pass a documented floating-point format byte",
			valid: validStrconvFloatFormat,
		}
	}

	switch name {
	case "ParseComplex":
		if argumentCount == 2 {
			return []strconvConstraint{
				bitSize(1, 64, 128, "bit size must be 64 or 128"),
			}
		}
	case "ParseFloat":
		if argumentCount == 2 {
			return []strconvConstraint{bitSize(1, 32, 64, "bit size must be 32 or 64")}
		}
	case "ParseInt", "ParseUint":
		if argumentCount == 3 {
			return []strconvConstraint{
				base(1, true),
				bitSize(2, 0, 64, "bit size must be between 0 and 64"),
			}
		}
	case "FormatComplex":
		if argumentCount == 4 {
			return []strconvConstraint{
				format(1),
				bitSize(3, 64, 128, "bit size must be 64 or 128"),
			}
		}
	case "FormatFloat":
		if argumentCount == 4 {
			return []strconvConstraint{
				format(1),
				bitSize(3, 32, 64, "bit size must be 32 or 64"),
			}
		}
	case "FormatInt", "FormatUint":
		if argumentCount == 2 {
			return []strconvConstraint{base(1, false)}
		}
	case "AppendFloat":
		if argumentCount == 5 {
			return []strconvConstraint{
				format(2),
				bitSize(4, 32, 64, "bit size must be 32 or 64"),
			}
		}
	case "AppendInt", "AppendUint":
		if argumentCount == 3 {
			return []strconvConstraint{base(2, false)}
		}
	}
	return nil
}

func directStandardFunction(info *types.Info, expression ast.Expr, packagePath string) *types.Func {
	if info == nil {
		return nil
	}
	expression = ast.Unparen(expression)
	var object types.Object
	switch current := expression.(type) {
	case *ast.Ident:
		object = info.ObjectOf(current)
	case *ast.SelectorExpr:
		object = info.ObjectOf(current.Sel)
	default:
		return nil
	}
	function, _ := object.(*types.Func)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != packagePath {
		return nil
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() != nil {
		return nil
	}
	return function
}

func constantEquals(value constant.Value, target int64) bool {
	return constant.Compare(value, token.EQL, constant.MakeInt64(target))
}

func constantInRange(value constant.Value, minimum int64, maximum int64) bool {
	return !constant.Compare(value, token.LSS, constant.MakeInt64(minimum)) &&
		!constant.Compare(value, token.GTR, constant.MakeInt64(maximum))
}

func validStrconvFloatFormat(value constant.Value) bool {
	format, exact := constant.Int64Val(value)
	if !exact {
		return false
	}
	switch format {
	case 'b', 'e', 'E', 'f', 'g', 'G', 'x', 'X':
		return true
	default:
		return false
	}
}

func (invalidBinaryWriteRule) Metadata() Metadata {
	return Metadata{
		ID: "invalid-binary-write",
		Summary: "detects values encoding/binary.Write cannot encode",
		Documentation: "Encoding/binary.Write accepts fixed-size values, slices of fixed-size values, and pointers to those values. Architecture-sized integers and values containing strings, maps, channels, functions, interfaces, or pointers have no supported fixed-size binary representation.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only direct three-argument calls to the exact encoding/binary.Write package function are recognized; tuple arguments and function values remain conservative.",
			"Top-level interface and type-parameter data whose concrete runtime shape may be fixed-size remain conservative.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Write fixed-width data",
				Incorrect: `err := binary.Write(writer, binary.LittleEndian, "header")`,
				Correct: `err := binary.Write(writer, binary.LittleEndian, []byte("header"))`,
			},
		},
	}
}

func (invalidBinaryWriteRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"invalid-binary-write requires a call expression and type information",
		)
	}
	if len(call.Args) != 3 {
		return nil, nil
	}
	function := directStandardFunction(ctx.Info(), call.Fun, "encoding/binary")
	if function == nil || function.Name() != "Write" {
		return nil, nil
	}
	if binaryWriteTypeStatus(ctx.Info().TypeOf(call.Args[2]), true, true) != binaryTypeInvalid {
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[2])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "variable-size-data",
			Message: "binary.Write cannot encode a value whose type has variable size",
			Range: range_,
			Help: "use fixed-width numeric types or encode variable-length data explicitly",
		},
	}, nil
}

type binaryTypeStatus uint8

const (
	binaryTypeUnknown binaryTypeStatus = iota
	binaryTypeValid
	binaryTypeInvalid
)

func binaryWriteTypeStatus(
	type_ types.Type,
	allowPointer bool,
	allowDynamicInterface bool,
) binaryTypeStatus {
	if type_ == nil {
		return binaryTypeUnknown
	}
	type_ = types.Unalias(type_)
	if _, typeParameter := type_.(*types.TypeParam); typeParameter {
		return binaryTypeUnknown
	}
	if pointer, ok := type_.(*types.Pointer); ok {
		if !allowPointer {
			return binaryTypeInvalid
		}
		return binaryWriteTypeStatus(pointer.Elem(), false, false)
	}
	switch underlying := type_.Underlying().(type) {
	case *types.Pointer:
		if !allowPointer {
			return binaryTypeInvalid
		}
		return binaryWriteTypeStatus(underlying.Elem(), false, false)
	case *types.Basic:
		switch underlying.Kind() {
		case types.Bool,
			types.Int8,
			types.Int16,
			types.Int32,
			types.Int64,
			types.Uint8,
			types.Uint16,
			types.Uint32,
			types.Uint64,
			types.Float32,
			types.Float64,
			types.Complex64,
			types.Complex128:
			return binaryTypeValid
		default:
			return binaryTypeInvalid
		}
	case *types.Array:
		return binaryWriteTypeStatus(underlying.Elem(), false, false)
	case *types.Slice:
		return binaryWriteTypeStatus(underlying.Elem(), false, false)
	case *types.Struct:
		status := binaryTypeValid
		for index := range underlying.NumFields() {
			fieldStatus := binaryWriteTypeStatus(
				underlying.Field(index).Type(),
				false,
				false,
			)
			if fieldStatus == binaryTypeInvalid {
				return binaryTypeInvalid
			}
			if fieldStatus == binaryTypeUnknown {
				status = binaryTypeUnknown
			}
		}
		return status
	case *types.Interface:
		if allowDynamicInterface {
			return binaryTypeUnknown
		}
		return binaryTypeInvalid
	default:
		return binaryTypeInvalid
	}
}
