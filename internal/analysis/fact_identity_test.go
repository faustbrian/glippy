package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

const factIdentityFixture = `package fixture

type Box[T any] struct { Value T }

func (box Box[T]) Map(value T) (result T) {
	local := value
	return local
}

var Exported int
var hidden int
`

func TestFactObjectIdentitySurvivesIndependentTypeChecks(t *testing.T) {
	t.Parallel()

	first, _ := checkFactIdentityFixture(t)
	second, _ := checkFactIdentityFixture(t)
	firstObjects := persistentFixtureObjects(t, first)
	secondObjects := persistentFixtureObjects(t, second)
	encoder := newFactObjectEncoder()
	for name, firstObject := range firstObjects {
		t.Run(name, func(t *testing.T) {
			identity, err := encoder.Identity(firstObject)
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := identity.Resolve(second)
			if err != nil {
				t.Fatal(err)
			}
			if resolved != secondObjects[name] {
				t.Fatalf("Resolve() = %s, want %s", resolved, secondObjects[name])
			}
			reencoded, err := encoder.Identity(resolved)
			if err != nil {
				t.Fatal(err)
			}
			if reencoded != identity {
				t.Fatalf("identity after reload = %#v, want %#v", reencoded, identity)
			}
		})
	}
}

func TestFactObjectIdentityRejectsUnstableOrMismatchedObjects(t *testing.T) {
	t.Parallel()

	pkg, definitions := checkFactIdentityFixture(t)
	encoder := newFactObjectEncoder()
	tests := []struct {
		name   string
		object types.Object
	}{
		{name: "nil", object: nil},
		{name: "predeclared", object: types.Universe.Lookup("int")},
		{name: "unexported package variable", object: pkg.Scope().Lookup("hidden")},
		{name: "local variable", object: definitions["local"]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := encoder.Identity(test.object); err == nil {
				t.Fatalf("Identity(%v) succeeded", test.object)
			}
		})
	}

	identity, err := encoder.Identity(pkg.Scope().Lookup("Exported"))
	if err != nil {
		t.Fatal(err)
	}
	other := types.NewPackage("example.com/other", "other")
	if _, err := identity.Resolve(other); err == nil {
		t.Fatal("Resolve() accepted a different package")
	}
	identity.ObjectPath = "missing"
	if _, err := identity.Resolve(pkg); err == nil {
		t.Fatal("Resolve() accepted a missing object path")
	}
	if _, err := (factObjectIdentity{}).Resolve(pkg); err == nil {
		t.Fatal("Resolve() accepted an empty identity")
	}
	unstable := factObjectIdentity{PackagePath: pkg.Path(), ObjectPath: "hidden"}
	if _, err := unstable.Resolve(pkg); err == nil {
		t.Fatal("Resolve() accepted a noncanonical object path")
	}
	if _, err := identity.Resolve(nil); err == nil {
		t.Fatal("Resolve() accepted a nil package")
	}
}

func checkFactIdentityFixture(t *testing.T) (*types.Package, map[string]types.Object) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", factIdentityFixture, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Defs: make(map[*ast.Ident]types.Object)}
	pkg, err := new(types.Config).Check("example.com/fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	definitions := make(map[string]types.Object)
	for identifier, object := range info.Defs {
		if object != nil {
			definitions[identifier.Name] = object
		}
	}
	return pkg, definitions
}

func persistentFixtureObjects(t *testing.T, pkg *types.Package) map[string]types.Object {
	t.Helper()
	box := pkg.Scope().Lookup("Box").Type().(*types.Named)
	method, _, _ := types.LookupFieldOrMethod(box, true, pkg, "Map")
	signature := method.Type().(*types.Signature)
	return map[string]types.Object{
		"exported variable": pkg.Scope().Lookup("Exported"),
		"named type":        box.Obj(),
		"method":            method,
		"field":             box.Underlying().(*types.Struct).Field(0),
		"type parameter":    box.TypeParams().At(0).Obj(),
		"parameter":         signature.Params().At(0),
		"result":            signature.Results().At(0),
	}
}
