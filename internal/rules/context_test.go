package rules_test

import (
	"reflect"
	"testing"

	"github.com/faustbrian/glippy/internal/rules"
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

func TestNewPackageDependencyForcesFilesToNonTargets(t *testing.T) {
	t.Parallel()

	dependency := rules.NewPackageDependency(
		nil,
		"example.com/dependency",
		nil,
		nil,
		nil,
		false,
		[]rules.PackageFile{rules.NewPackageFile(nil, nil, nil, true)},
	)
	files := dependency.Files()
	if len(files) != 1 || files[0].Target() {
		t.Fatalf("dependency files = %#v", files)
	}
}
