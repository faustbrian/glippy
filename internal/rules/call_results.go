package rules

import (
	"go/ast"
	"go/types"
)

func callResultCount(info *types.Info, call *ast.CallExpr) int {
	if info == nil || call == nil {
		return 0
	}
	signature, _ := types.Unalias(info.TypeOf(call.Fun)).(*types.Signature)
	if signature == nil || signature.Results() == nil {
		return 0
	}
	return signature.Results().Len()
}
