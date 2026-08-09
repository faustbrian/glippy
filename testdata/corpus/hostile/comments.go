// Package hostile contains owned hostile-valid-Go fixtures.
package hostile

//go:generate go run example.invalid/generator

func comments(a, b int) int { return a /* left operand */ + /* right operand */ b }

func literal() []int { return []int{1 /* first */, 2 /* second */, 3} }
