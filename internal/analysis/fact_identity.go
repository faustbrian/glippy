package analysis

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/go/types/objectpath"
)

type factObjectIdentity struct {
	PackagePath string
	ObjectPath  string
}

type factObjectEncoder struct {
	encoder objectpath.Encoder
}

func newFactObjectEncoder() *factObjectEncoder { return new(factObjectEncoder) }

func (e *factObjectEncoder) Identity(object types.Object) (factObjectIdentity, error) {
	if e == nil {
		return factObjectIdentity{}, fmt.Errorf("fact object identity requires an encoder")
	}
	if object == nil {
		return factObjectIdentity{}, fmt.Errorf("fact object identity requires an object")
	}
	pkg := object.Pkg()
	if pkg == nil || pkg.Path() == "" {
		return factObjectIdentity{}, fmt.Errorf("fact object %s has no package identity", object.Name())
	}
	path, err := e.encoder.For(object)
	if err != nil {
		return factObjectIdentity{}, fmt.Errorf(
			"identify fact object %s in package %q: %w",
			object.Name(),
			pkg.Path(),
			err,
		)
	}
	return factObjectIdentity{PackagePath: pkg.Path(), ObjectPath: string(path)}, nil
}

func (i factObjectIdentity) Resolve(pkg *types.Package) (types.Object, error) {
	if pkg == nil {
		return nil, fmt.Errorf("resolve fact object requires a package")
	}
	if i.PackagePath == "" || i.ObjectPath == "" {
		return nil, fmt.Errorf("resolve fact object requires a complete identity")
	}
	if pkg.Path() != i.PackagePath {
		return nil, fmt.Errorf(
			"resolve fact object for package %q with package %q",
			i.PackagePath,
			pkg.Path(),
		)
	}
	object, err := objectpath.Object(pkg, objectpath.Path(i.ObjectPath))
	if err != nil {
		return nil, fmt.Errorf(
			"resolve fact object %q in package %q: %w",
			i.ObjectPath,
			i.PackagePath,
			err,
		)
	}
	canonical, err := objectpath.For(object)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve fact object %q in package %q without a stable path: %w",
			i.ObjectPath,
			i.PackagePath,
			err,
		)
	}
	if string(canonical) != i.ObjectPath {
		return nil, fmt.Errorf(
			"resolve fact object path %q in package %q is not canonical",
			i.ObjectPath,
			i.PackagePath,
		)
	}
	return object, nil
}
