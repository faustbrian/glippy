package rules_test

import (
	"reflect"
	"testing"

	"github.com/faustbrian/gox/internal/rules"
)

func TestContextDoesNotExposeMutableState(t *testing.T) {
	t.Parallel()

	contextType := reflect.TypeFor[rules.Context]()
	for index := range contextType.NumField() {
		field := contextType.Field(index)
		if field.IsExported() {
			t.Fatalf("Context exposes mutable field %q", field.Name)
		}
	}
}
