package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

const encodingCSVPackagePath = "encoding/csv"

type uncheckedCSVWriterErrorRule struct{}

type csvWriterFlushCandidate struct {
	call *ast.CallExpr
	receiver types.Object
	start obligationStart
}

var uncheckedCSVWriterErrorSpec = iterationErrorSpec{
	packagePath: encodingCSVPackagePath,
	typeName: "Writer",
	iterationMethod: "Flush",
	errorMethod: "Error",
}

// NewUncheckedCSVWriterErrorRule constructs the CSV flush-error observation
// rule for product registry composition.
func NewUncheckedCSVWriterErrorRule() Rule {
	return uncheckedCSVWriterErrorRule{}
}

func (uncheckedCSVWriterErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "unchecked-csv-writer-error",
		Summary: "detects CSV flushing without observing the buffered write error",
		Documentation: "encoding/csv.Writer.Flush has no result even though its underlying buffered write can fail. The standard-library contract requires a later Writer.Error call to retrieve errors from Write or Flush. This rule follows direct writer values through the shared control-flow graph and reports when a normally returning path can leave the Flush error unobserved.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The initial contract recognizes direct identifier-backed encoding/csv.Writer.Flush and Writer.Error calls; fields, containers, and indirect aliases are not tracked.",
			"Passing the writer itself or one of its method values to another operation stops analysis because that operation may observe the stored error.",
			"Deferred and asynchronous Flush calls are excluded because proving a later Error observation requires execution-order or synchronization analysis.",
			"Passing Writer.Error to another call counts as observing the result; the rule does not inspect the callee's behavior.",
			"No fix is offered because propagation, logging, and partial-output policy depend on the surrounding function contract.",
		},
		Examples: []Example{
			{
				Title: "Check the CSV writer after flushing",
				Incorrect: "writer.Flush()\nreturn nil",
				Correct: "writer.Flush()\nreturn writer.Error()",
			},
		},
	}
}

func (uncheckedCSVWriterErrorRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"unchecked-csv-writer-error requires a complete control-flow context",
		)
	}
	findings := make([]Finding, 0)
	for _, candidate := range csvWriterFlushCandidates(ctx.Body(), ctx.Graph(), ctx.Info()) {
		if !obligationReachesOpenReturn(
			candidate.start,
			func(node ast.Node) obligationEffect {
				return csvWriterErrorEffect(ctx.Info(), node, candidate.receiver)
			},
		) {
			continue
		}
		range_, err := ctx.Range(candidate.call)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "csv-writer-error-not-checked",
				Message: "csv.Writer.Error is not checked on every normally returning path after Flush",
				Range: range_,
				Help: "check writer.Error after flushing before reporting successful output",
			},
		)
	}
	return findings, nil
}

func csvWriterFlushCandidates(
	body *ast.BlockStmt,
	graph *cfg.CFG,
	info *types.Info,
) []csvWriterFlushCandidate {
	result := make([]csvWriterFlushCandidate, 0)
	for _, block := range graph.Blocks {
		if block == nil || !block.Live {
			continue
		}
		for index, node := range block.Nodes {
			statement, _ := node.(*ast.ExprStmt)
			if statement == nil {
				continue
			}
			call, _ := ast.Unparen(statement.X).(*ast.CallExpr)
			receiver, matched := iterationMethodReceiver(
				info,
				call,
				uncheckedCSVWriterErrorSpec,
				uncheckedCSVWriterErrorSpec.iterationMethod,
			)
			if !matched {
				continue
			}
			if nodeInsideConstantUnreachableBranch(body, info, call) {
				continue
			}
			result = append(
				result,
				csvWriterFlushCandidate{
					call: call,
					receiver: receiver,
					start: obligationStart{block: block, offset: index + 1},
				},
			)
		}
	}
	return result
}

func csvWriterErrorEffect(info *types.Info, node ast.Node, receiver types.Object) obligationEffect {
	effect := obligationOpen
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, stack []ast.Node) bool {
			if effect != obligationOpen {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				if expressionUsesObject(info, literal.Body, receiver) {
					effect = obligationTransferred
				}
				return false
			}
			switch current := current.(type) {
			case *ast.CallExpr:
				callReceiver, matched := iterationMethodReceiver(
					info,
					current,
					uncheckedCSVWriterErrorSpec,
					uncheckedCSVWriterErrorSpec.errorMethod,
				)
				if matched && callReceiver == receiver {
					if iterationCallResultIsUsed(current, stack) {
						effect = obligationCompleted
					}
					return false
				}
				for _, argument := range current.Args {
					if directObject(info, argument) == receiver ||
						methodValueUsesObject(info, argument, receiver) {
						effect = obligationTransferred
						return false
					}
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directObject(info, expression) == receiver {
						effect = obligationTransferred
						return false
					}
				}
			case *ast.SendStmt:
				if directObject(info, current.Value) == receiver {
					effect = obligationTransferred
					return false
				}
			case *ast.AssignStmt:
				for _, target := range current.Lhs {
					if directObject(info, target) == receiver {
						effect = obligationLost
						return false
					}
				}
				for _, expression := range current.Rhs {
					if directObject(info, expression) != receiver &&
						!methodValueUsesObject(info, expression, receiver) {
						continue
					}
					for _, target := range current.Lhs {
						identifier, blank := target.(*ast.Ident)
						if blank && identifier.Name == "_" {
							continue
						}
						if directObject(info, target) != receiver {
							effect = obligationTransferred
							return false
						}
					}
				}
			case *ast.CompositeLit:
				for _, element := range current.Elts {
					if expressionUsesObject(info, element, receiver) {
						effect = obligationTransferred
						return false
					}
				}
			}
			return true
		},
	)
	return effect
}
