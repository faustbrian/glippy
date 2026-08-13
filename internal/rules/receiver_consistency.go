package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type inconsistentReceiverNameRule struct{}

type mixedReceiverTypeRule struct{}

type receiverMethod struct {
	file PackageFile
	name *ast.Ident
	typeExpression ast.Expr
	pointer bool
}

// NewInconsistentReceiverNameRule constructs the package receiver-name rule.
func NewInconsistentReceiverNameRule() Rule {
	return inconsistentReceiverNameRule{}
}

// NewMixedReceiverTypeRule constructs the package pointer/value receiver rule.
func NewMixedReceiverTypeRule() Rule {
	return mixedReceiverTypeRule{}
}

func (inconsistentReceiverNameRule) Metadata() Metadata {
	return Metadata{
		ID: "inconsistent-receiver-name",
		Summary: "detects different receiver names on methods of one type",
		Documentation: "Methods on one named type are easier to scan when they use one short, consistent receiver name. A one-off receiver spelling often survives a copied method or rename and makes examples and documentation inconsistent.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		KnownLimitations: []string{
			"The most frequent receiver name is treated as canonical; ties use the earliest method in canonical file and declaration order.",
			"Unnamed and blank receivers do not participate in receiver-name selection.",
			"No fix is offered because renaming must account for every receiver reference in the method body.",
		},
		Examples: []Example{
			{
				Title: "Use one receiver name per type",
				Incorrect: "func (client *Client) Open() {}\nfunc (c *Client) Close() {}",
				Correct: "func (c *Client) Open() {}\nfunc (c *Client) Close() {}",
			},
		},
	}
}

func (inconsistentReceiverNameRule) RunPackage(ctx *PackageContext) ([]PackageFinding, error) {
	groups, err := receiverMethods(ctx)
	if err != nil {
		return nil, err
	}
	findings := make([]PackageFinding, 0)
	for _, methods := range groups {
		counts := make(map[string]int)
		first := make(map[string]int)
		for index, method := range methods {
			if method.name == nil || method.name.Name == "_" {
				continue
			}
			counts[method.name.Name]++
			if _, found := first[method.name.Name]; !found {
				first[method.name.Name] = index
			}
		}
		canonical := dominantReceiverName(counts, first)
		if canonical == "" || len(counts) < 2 {
			continue
		}
		reference := methods[first[canonical]]
		referenceRange, rangeErr := reference.file.Range(reference.name)
		if rangeErr != nil {
			return nil, rangeErr
		}
		for _, method := range methods {
			if method.name == nil ||
				method.name.Name == "_" ||
				method.name.Name == canonical {
				continue
			}
			range_, rangeErr := method.file.Range(method.name)
			if rangeErr != nil {
				return nil, rangeErr
			}
			findings = append(
				findings,
				PackageFinding{
					File: method.file,
					Finding: Finding{
						MessageKey: "receiver-name",
						Message: fmt.Sprintf(
							"receiver name %q is inconsistent; use %q for methods on this type",
							method.name.Name,
							canonical,
						),
						Range: range_,
						Related: []Related{
							{
								Range: referenceRange,
								Message: "canonical receiver name",
							},
						},
						Help: "use one receiver name consistently across the type's methods",
					},
				},
			)
		}
	}
	return findings, nil
}

func (mixedReceiverTypeRule) Metadata() Metadata {
	return Metadata{
		ID: "mixed-receiver-type",
		Summary: "detects mixed pointer and value receivers on one type",
		Documentation: "Mixing pointer and value receivers on one named type can make mutation, copying, method sets, and interface satisfaction harder to reason about. A consistent receiver form communicates the type's ownership model.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		KnownLimitations: []string{
			"The majority receiver form is treated as canonical; ties use the earliest method in canonical file and declaration order.",
			"A deliberate value receiver for immutable inspection alongside pointer mutation may be valid and can be suppressed.",
			"No fix is offered because changing receiver form can alter method sets, interface satisfaction, copying, and mutation semantics.",
		},
		Examples: []Example{
			{
				Title: "Choose one receiver form",
				Incorrect: "func (v Value) Read() {}\nfunc (v *Value) Write() {}",
				Correct: "func (v *Value) Read() {}\nfunc (v *Value) Write() {}",
			},
		},
	}
}

func (mixedReceiverTypeRule) RunPackage(ctx *PackageContext) ([]PackageFinding, error) {
	groups, err := receiverMethods(ctx)
	if err != nil {
		return nil, err
	}
	findings := make([]PackageFinding, 0)
	for _, methods := range groups {
		pointerCount := 0
		for _, method := range methods {
			if method.pointer {
				pointerCount++
			}
		}
		valueCount := len(methods) - pointerCount
		if pointerCount == 0 || valueCount == 0 {
			continue
		}
		canonicalPointer := pointerCount > valueCount
		if pointerCount == valueCount {
			canonicalPointer = methods[0].pointer
		}
		var reference receiverMethod
		for _, method := range methods {
			if method.pointer == canonicalPointer {
				reference = method
				break
			}
		}
		referenceRange, rangeErr := reference.file.Range(reference.typeExpression)
		if rangeErr != nil {
			return nil, rangeErr
		}
		for _, method := range methods {
			if method.pointer == canonicalPointer {
				continue
			}
			range_, rangeErr := method.file.Range(method.typeExpression)
			if rangeErr != nil {
				return nil, rangeErr
			}
			canonical := "value"
			if canonicalPointer {
				canonical = "pointer"
			}
			findings = append(
				findings,
				PackageFinding{
					File: method.file,
					Finding: Finding{
						MessageKey: "receiver-form",
						Message: fmt.Sprintf(
							"receiver form is inconsistent; methods on this type predominantly use %s receivers",
							canonical,
						),
						Range: range_,
						Related: []Related{
							{
								Range: referenceRange,
								Message: "canonical receiver form",
							},
						},
						Help: "use one receiver form unless the method-set difference is intentional",
					},
				},
			)
		}
	}
	return findings, nil
}

func receiverMethods(ctx *PackageContext) (map[*types.TypeName][]receiverMethod, error) {
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"receiver consistency requires complete package type information",
		)
	}
	groups := make(map[*types.TypeName][]receiverMethod)
	for _, file := range ctx.Files() {
		if !file.Target() || file.Syntax() == nil {
			continue
		}
		for _, declaration := range file.Syntax().Decls {
			function, _ := declaration.(*ast.FuncDecl)
			if function == nil || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			object, _ := ctx.Info().ObjectOf(function.Name).(*types.Func)
			if object == nil {
				continue
			}
			signature, _ := object.Type().(*types.Signature)
			if signature == nil || signature.Recv() == nil {
				continue
			}
			receiverType := types.Unalias(signature.Recv().Type())
			pointer := false
			if pointerType, ok := receiverType.(*types.Pointer); ok {
				pointer = true
				receiverType = types.Unalias(pointerType.Elem())
			}
			named, _ := receiverType.(*types.Named)
			if named == nil || named.Obj() == nil {
				continue
			}
			field := function.Recv.List[0]
			var name *ast.Ident
			if len(field.Names) == 1 {
				name = field.Names[0]
			}
			groups[named.Obj()] = append(
				groups[named.Obj()],
				receiverMethod{
					file: file,
					name: name,
					typeExpression: field.Type,
					pointer: pointer,
				},
			)
		}
	}
	return groups, nil
}

func dominantReceiverName(counts map[string]int, first map[string]int) string {
	canonical := ""
	for name, count := range counts {
		if canonical == "" ||
			count > counts[canonical] ||
			count == counts[canonical] && first[name] < first[canonical] {
			canonical = name
		}
	}
	return canonical
}
