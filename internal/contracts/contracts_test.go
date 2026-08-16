package contracts

import (
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestParseFilesBuildsCanonicalSemanticContracts(t *testing.T) {
	t.Parallel()

	set, err := ParseFiles(
		[]File{
			{
				Path: "/project/z.toml",
				Bytes: []byte(
					`version = 1

[[functions]]
symbol = "example.com/project/api.Open"
must-use = [1, 0]
blocking = true
returns-alias = [{ result = 0, argument = 0 }]
nil-error = [{ value = 0, error = 1, when-error-nil = "non-nil", when-error-non-nil = "nil" }]
`,
				),
			},
			{
				Path: "/project/a.toml",
				Bytes: []byte(
					`version = 1

[[functions]]
symbol = "example.com/project/api.Resource.Finish"
noreturn = true
closes = [0]
takes-ownership = [1]
completes-transaction = [2]
invokes-cancellation = [3]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	declarations := set.Functions()
	if len(declarations) != 2 {
		t.Fatalf("Functions() count = %d, want 2", len(declarations))
	}
	open := declarations[0]
	if open.Symbol != "example.com/project/api.Open" ||
		!open.Blocking ||
		!slices.Equal(open.MustUse, []int{0, 1}) ||
		len(open.ReturnsAlias) != 1 ||
		open.ReturnsAlias[0] != (ReturnAlias{Result: 0, Argument: 0}) ||
		len(open.NilError) != 1 ||
		open.NilError[0] !=
			(NilErrorRelation{
				Value: 0,
				Error: 1,
				WhenErrorNil: NilStateNonNil,
				WhenErrorNonNil: NilStateNil,
			}) {
		t.Fatalf("Open contract = %#v", open)
	}
	finish := declarations[1]
	if finish.Symbol != "example.com/project/api.Resource.Finish" ||
		!finish.NoReturn ||
		!slices.Equal(finish.Closes, []int{0}) ||
		!slices.Equal(finish.TakesOwnership, []int{1}) ||
		!slices.Equal(finish.CompletesTransaction, []int{2}) ||
		!slices.Equal(finish.InvokesCancellation, []int{3}) {
		t.Fatalf("Finish contract = %#v", finish)
	}
	declarations[0].MustUse[0] = 99
	if set.Functions()[0].MustUse[0] != 0 {
		t.Fatal("Functions() exposed mutable contract storage")
	}

	reordered, err := ParseFiles(
		[]File{
			{
				Path: "/elsewhere/first.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/project/api.Resource.Finish"
invokes-cancellation = [3]
completes-transaction = [2]
takes-ownership = [1]
closes = [0]
noreturn = true
`,
				),
			},
			{
				Path: "/elsewhere/second.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/project/api.Open"
nil-error = [{ error = 1, value = 0, when-error-non-nil = "nil", when-error-nil = "non-nil" }]
returns-alias = [{ argument = 0, result = 0 }]
blocking = true
must-use = [0, 1]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(set.CanonicalBytes(), reordered.CanonicalBytes()) {
		t.Fatal("canonical identity depends on file, declaration, field, or index order")
	}
}

func TestParseFilesRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		files []File
		want string
	}{
		{
			name: "unknown field",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte("version = 1\nunknown = true\n"),
				},
			},
			want: "unknown field",
		},
		{
			name: "missing version",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"[[functions]]\nsymbol = \"example.com/p.F\"\nnoreturn = true\n",
					),
				},
			},
			want: "version is required",
		},
		{
			name: "unsupported version",
			files: []File{{Path: "contract.toml", Bytes: []byte("version = 2\n")}},
			want: "unsupported contract version 2",
		},
		{
			name: "duplicate path",
			files: []File{
				{Path: "contract.toml", Bytes: []byte("version = 1\n")},
				{Path: "contract.toml", Bytes: []byte("version = 1\n")},
			},
			want: "duplicate contract file",
		},
		{
			name: "duplicate symbol",
			files: []File{
				{
					Path: "a.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\nnoreturn = true\n",
					),
				},
				{
					Path: "b.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\nblocking = true\n",
					),
				},
			},
			want: "duplicate function contract",
		},
		{
			name: "invalid symbol",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"not a symbol\"\nnoreturn = true\n",
					),
				},
			},
			want: "invalid function symbol",
		},
		{
			name: "invalid package path",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com\\\\project.F\"\nnoreturn = true\n",
					),
				},
			},
			want: "invalid function symbol",
		},
		{
			name: "empty effects",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\n",
					),
				},
			},
			want: "does not declare an effect",
		},
		{
			name: "duplicate index",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\nmust-use = [0, 0]\n",
					),
				},
			},
			want: "must-use contains duplicate index 0",
		},
		{
			name: "negative index",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\ncloses = [-1]\n",
					),
				},
			},
			want: "closes contains negative index -1",
		},
		{
			name: "unknown nil state",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\nnil-error = [{ value = 0, error = 1, when-error-nil = \"maybe\" }]\n",
					),
				},
			},
			want: "when-error-nil must be nil or non-nil",
		},
		{
			name: "duplicate relation",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\nnil-error = [{ value = 0, error = 1, when-error-nil = \"nil\" }, { value = 0, error = 1, when-error-non-nil = \"nil\" }]\n",
					),
				},
			},
			want: "repeats nil-error relationship",
		},
		{
			name: "same value and error result",
			files: []File{
				{
					Path: "contract.toml",
					Bytes: []byte(
						"version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\nnil-error = [{ value = 0, error = 0, when-error-nil = \"nil\" }]\n",
					),
				},
			},
			want: "must use distinct value and error results",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				_, err := ParseFiles(test.files)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"ParseFiles() error = %v, want containing %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}

func TestParseFilesBoundsInput(t *testing.T) {
	t.Parallel()

	if _, err := ParseFiles(
		[]File{{Path: "large.toml", Bytes: make([]byte, MaxFileBytes + 1)}},
	);
		err == nil || !strings.Contains(err.Error(), "exceeds 1048576-byte limit") {
		t.Fatalf("ParseFiles() error = %v, want per-file byte bound", err)
	}
	total := make([]File, 0, 5)
	for index := 0; index < 4; index++ {
		total = append(
			total,
			File{
				Path: fmt.Sprintf("total-%d.toml", index),
				Bytes: make([]byte, MaxFileBytes),
			},
		)
	}
	total = append(total, File{Path: "total-overflow.toml", Bytes: []byte{'x'}})
	if _, err := ParseFiles(total);
		err == nil || !strings.Contains(err.Error(), "4194304-byte total limit") {
		t.Fatalf("ParseFiles() error = %v, want total byte bound", err)
	}

	files := make([]File, MaxFiles + 1)
	for index := range files {
		files[index] = File{
			Path: string(rune('a' + index)) + ".toml",
			Bytes: []byte("version = 1\n"),
		}
	}
	if _, err := ParseFiles(files);
		err == nil || !strings.Contains(err.Error(), "too many contract files") {
		t.Fatalf("ParseFiles() error = %v, want file bound", err)
	}

	var functions strings.Builder
	functions.WriteString("version = 1\n")
	for index := 0; index <= MaxFunctions; index++ {
		fmt.Fprintf(
			&functions,
			"[[functions]]\nsymbol = \"example.com/p.F%d\"\nnoreturn = true\n",
			index,
		)
	}
	if _, err := ParseFiles(
		[]File{{Path: "functions.toml", Bytes: []byte(functions.String())}},
	);
		err == nil || !strings.Contains(err.Error(), "function limit") {
		t.Fatalf("ParseFiles() error = %v, want function bound", err)
	}

	indices := make([]string, MaxIndices + 1)
	for index := range indices {
		indices[index] = strconv.Itoa(index)
	}
	input := "version = 1\n[[functions]]\nsymbol = \"example.com/p.F\"\nmust-use = [" +
		strings.Join(indices, ",") +
		"]\n"
	if _, err := ParseFiles([]File{{Path: "indices.toml", Bytes: []byte(input)}});
		err == nil || !strings.Contains(err.Error(), "more than 64 indices") {
		t.Fatalf("ParseFiles() error = %v, want index bound", err)
	}
}

func TestParseFilesLocatesSemanticFunctionErrors(t *testing.T) {
	t.Parallel()

	_, err := ParseFiles(
		[]File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					"version = 1\n\n  [[functions]] # contract\n  symbol = \"invalid symbol\"\n  noreturn = true\n",
				),
			},
		},
	)
	var diagnostic *Error
	if !errors.As(err, &diagnostic) ||
		diagnostic.Path != "contracts.toml" ||
		diagnostic.Line != 3 ||
		diagnostic.Column != 3 {
		t.Fatalf("ParseFiles() error = %#v, want contracts.toml:3:3", diagnostic)
	}
}

func TestResolveValidatesLoadedFunctionSignatures(t *testing.T) {
	t.Parallel()

	package_ := types.NewPackage("example.com/project/api", "api")
	errorType := types.Universe.Lookup("error").Type()
	resourceName := types.NewTypeName(token.NoPos, package_, "Resource", nil)
	resource := types.NewNamed(resourceName, types.NewStruct(nil, nil), nil)
	package_.Scope().Insert(resourceName)
	open := types.NewFunc(
		token.NoPos,
		package_,
		"Open",
		types.NewSignatureType(
			nil,
			nil,
			nil,
			types.NewTuple(
				types.NewVar(
					token.NoPos,
					package_,
					"resource",
					types.NewPointer(resource),
				),
			),
			types.NewTuple(
				types.NewVar(
					token.NoPos,
					package_,
					"resource",
					types.NewPointer(resource),
				),
				types.NewVar(token.NoPos, package_, "err", errorType),
			),
			false,
		),
	)
	package_.Scope().Insert(open)
	receiver := types.NewVar(token.NoPos, package_, "resource", types.NewPointer(resource))
	finish := types.NewFunc(
		token.NoPos,
		package_,
		"Finish",
		types.NewSignatureType(
			receiver,
			nil,
			nil,
			types.NewTuple(
				types.NewVar(
					token.NoPos,
					package_,
					"closer",
					types.NewPointer(resource),
				),
			),
			types.NewTuple(),
			false,
		),
	)
	resource.AddMethod(finish)

	set, err := ParseFiles(
		[]File{
			{
				Path: "contract.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/project/api.Open"
must-use = [0, 1]
returns-alias = [{ result = 0, argument = 0 }]
nil-error = [{ value = 0, error = 1, when-error-nil = "non-nil" }]

[[functions]]
symbol = "example.com/project/api.Resource.Finish"
closes = [0]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve(set, []*types.Package{package_})
	if err != nil {
		t.Fatal(err)
	}
	openContract, found := resolved.Lookup(open)
	if !found || !slices.Equal(openContract.MustUse, []int{0, 1}) {
		t.Fatalf("Lookup(Open) = %#v, %t", openContract, found)
	}
	finishContract, found := resolved.Lookup(finish)
	if !found || !slices.Equal(finishContract.Closes, []int{0}) {
		t.Fatalf("Lookup(Finish) = %#v, %t", finishContract, found)
	}

	invalid, err := ParseFiles(
		[]File{
			{
				Path: "invalid.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/project/api.Open"
must-use = [2]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(invalid, []*types.Package{package_});
		err == nil || !strings.Contains(err.Error(), "result index 2") {
		t.Fatalf("Resolve() error = %v, want result bound", err)
	}

	unresolved, err := ParseFiles(
		[]File{
			{
				Path: "unresolved.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/project/api.Missing"
noreturn = true
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(unresolved, []*types.Package{package_});
		err == nil || !strings.Contains(err.Error(), "does not resolve in loaded package") {
		t.Fatalf("Resolve() error = %v, want unresolved symbol", err)
	}

	external, err := ParseFiles(
		[]File{
			{
				Path: "external.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/external/api.Stop"
noreturn = true
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	deferred, err := Resolve(external, []*types.Package{package_})
	if err != nil || deferred.Len() != 0 {
		t.Fatalf(
			"Resolve(external) = %#v, %v, want deferred unloaded package",
			deferred,
			err,
		)
	}
}

func TestResolveValidatesPackageVariantsDeterministically(t *testing.T) {
	t.Parallel()

	newVariant := func(resultCount int) *types.Package {
		package_ := types.NewPackage("example.com/project/api", "api")
		results := make([]*types.Var, resultCount)
		for index := range results {
			results[index] = types.NewVar(
				token.NoPos,
				package_,
				"",
				types.Typ[types.Int],
			)
		}
		function := types.NewFunc(
			token.NoPos,
			package_,
			"Open",
			types.NewSignatureType(
				nil,
				nil,
				nil,
				types.NewTuple(),
				types.NewTuple(results...),
				false,
			),
		)
		package_.Scope().Insert(function)
		return package_
	}
	set, err := ParseFiles(
		[]File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					"version = 1\n[[functions]]\nsymbol = \"example.com/project/api.Open\"\nmust-use = [2]\n",
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	zero := newVariant(0)
	one := newVariant(1)
	_, firstErr := Resolve(set, []*types.Package{zero, one})
	_, secondErr := Resolve(set, []*types.Package{one, zero})
	if firstErr == nil || secondErr == nil || firstErr.Error() != secondErr.Error() {
		t.Fatalf(
			"Resolve() errors = %q and %q, want identical variant-independent failure",
			firstErr,
			secondErr,
		)
	}
}

func TestResolveBindsEveryCompatiblePackageVariant(t *testing.T) {
	t.Parallel()

	newVariant := func(resultCount int) (*types.Package, *types.Func) {
		package_ := types.NewPackage("example.com/project/api", "api")
		results := make([]*types.Var, resultCount)
		for index := range results {
			results[index] = types.NewVar(
				token.NoPos,
				package_,
				"",
				types.Typ[types.Int],
			)
		}
		function := types.NewFunc(
			token.NoPos,
			package_,
			"Open",
			types.NewSignatureType(
				nil,
				nil,
				nil,
				types.NewTuple(),
				types.NewTuple(results...),
				false,
			),
		)
		package_.Scope().Insert(function)
		return package_, function
	}
	set, err := ParseFiles(
		[]File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					"version = 1\n[[functions]]\nsymbol = \"example.com/project/api.Open\"\nmust-use = [0]\n",
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	firstPackage, first := newVariant(1)
	secondPackage, second := newVariant(2)
	resolved, err := Resolve(set, []*types.Package{firstPackage, secondPackage})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := resolved.Lookup(first); !found {
		t.Fatal("Resolve() omitted first compatible package variant")
	}
	if _, found := resolved.Lookup(second); !found {
		t.Fatal("Resolve() omitted second compatible package variant")
	}
}
