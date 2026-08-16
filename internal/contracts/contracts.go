// Package contracts owns strict, versioned project semantic contracts.
package contracts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/module"
)

const (
	// Version is the only accepted semantic-contract schema version.
	Version = 1
	// MaxFiles bounds one project contract snapshot.
	MaxFiles = 32
	// MaxFunctions bounds declarations across one project contract snapshot.
	MaxFunctions = 4096
	// MaxIndices bounds one indexed effect list.
	MaxIndices = 64
	// MaxFileBytes bounds one contract file before decoding.
	MaxFileBytes = 1 << 20
	// MaxTotalBytes bounds all contract files before decoding.
	MaxTotalBytes = 4 << 20
)

// NilState is one explicitly configured nilness state.
type NilState uint8

const (
	NilStateUnknown NilState = iota
	NilStateNil
	NilStateNonNil
)

// File is one exact contract-file version.
type File struct {
	Path string
	Bytes []byte
}

// Source identifies one declaration in a contract file.
type Source struct {
	Path string
	Function int
	Line int
	Column int
}

// NilErrorRelation describes a result state conditional on an error result.
type NilErrorRelation struct {
	Value int
	Error int
	WhenErrorNil NilState
	WhenErrorNonNil NilState
}

// ReturnAlias states that one result aliases one argument.
type ReturnAlias struct {
	Result int
	Argument int
}

// Function describes configured semantics for one exact function or method.
type Function struct {
	Symbol string
	NoReturn bool
	MustUse []int
	Closes []int
	TakesOwnership []int
	CompletesTransaction []int
	InvokesCancellation []int
	Blocking bool
	NilError []NilErrorRelation
	ReturnsAlias []ReturnAlias
	Source Source
}

// Set is one immutable, canonical collection of project contracts.
type Set struct {
	functions []Function
	canonical []byte
}

// Functions returns independently owned declarations in symbol order.
func (s Set) Functions() []Function {
	return cloneFunctions(s.functions)
}

// CanonicalBytes returns a declaration-order and file-order independent identity.
func (s Set) CanonicalBytes() []byte {
	return slices.Clone(s.canonical)
}

// Empty reports whether the snapshot contains no declarations.
func (s Set) Empty() bool {
	return len(s.functions) == 0
}

type document struct {
	Version *int `toml:"version"`
	Functions []functionDocument `toml:"functions"`
}

type functionDocument struct {
	Symbol string `toml:"symbol"`
	NoReturn bool `toml:"noreturn"`
	MustUse []int `toml:"must-use"`
	Closes []int `toml:"closes"`
	TakesOwnership []int `toml:"takes-ownership"`
	CompletesTransaction []int `toml:"completes-transaction"`
	InvokesCancellation []int `toml:"invokes-cancellation"`
	Blocking bool `toml:"blocking"`
	NilError []nilErrorDocument `toml:"nil-error"`
	ReturnsAlias []returnAliasDocument `toml:"returns-alias"`
}

type nilErrorDocument struct {
	Value int `toml:"value"`
	Error int `toml:"error"`
	WhenErrorNil string `toml:"when-error-nil"`
	WhenErrorNonNil string `toml:"when-error-non-nil"`
}

type returnAliasDocument struct {
	Result int `toml:"result"`
	Argument int `toml:"argument"`
}

// Error is one contract-file diagnostic.
type Error struct {
	Path string
	Line int
	Column int
	Message string
	cause error
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

func (e *Error) Unwrap() error {
	return e.cause
}

// ParseFiles strictly decodes and canonicalizes one complete contract snapshot.
func ParseFiles(files []File) (Set, error) {
	if len(files) > MaxFiles {
		return Set{}, fmt.Errorf(
			"too many contract files: got %d, maximum is %d",
			len(files),
			MaxFiles,
		)
	}
	ordered := make([]File, len(files))
	seenPaths := make(map[string]struct{}, len(files))
	var totalBytes int
	for index, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			return Set{}, fmt.Errorf("contract file %d has an empty path", index + 1)
		}
		if _, duplicate := seenPaths[file.Path]; duplicate {
			return Set{}, fmt.Errorf("duplicate contract file %q", file.Path)
		}
		seenPaths[file.Path] = struct{}{}
		if len(file.Bytes) > MaxFileBytes {
			return Set{}, fmt.Errorf(
				"contract file %q exceeds %d-byte limit",
				file.Path,
				MaxFileBytes,
			)
		}
		totalBytes += len(file.Bytes)
		if totalBytes > MaxTotalBytes {
			return Set{}, fmt.Errorf(
				"contract files exceed %d-byte total limit",
				MaxTotalBytes,
			)
		}
		ordered[index] = File{Path: file.Path, Bytes: slices.Clone(file.Bytes)}
	}
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].Path < ordered[right].Path
		},
	)

	functions := make([]Function, 0)
	seenSymbols := make(map[string]Source)
	for _, file := range ordered {
		var decoded document
		if err := toml.
			NewDecoder(bytes.NewReader(file.Bytes)).
			DisallowUnknownFields().
			Decode(&decoded);
			err != nil {
			return Set{}, decodeError(file.Path, err)
		}
		if decoded.Version == nil {
			return Set{}, contractError(file.Path, "version is required")
		}
		if *decoded.Version != Version {
			return Set{}, contractError(
				file.Path,
				"unsupported contract version %d",
				*decoded.Version,
			)
		}
		if len(functions) + len(decoded.Functions) > MaxFunctions {
			return Set{}, contractError(
				file.Path,
				"contract snapshot exceeds %d-function limit",
				MaxFunctions,
			)
		}
		locations := functionLocations(file.Bytes)
		for index, raw := range decoded.Functions {
			source := Source{Path: file.Path, Function: index + 1}
			if index < len(locations) {
				source.Line = locations[index].Line
				source.Column = locations[index].Column
			}
			function, err := normalizeFunction(raw, source)
			if err != nil {
				return Set{}, err
			}
			if previous, duplicate := seenSymbols[function.Symbol]; duplicate {
				return Set{}, sourceError(
					source,
					"duplicate function contract %q; first declared in %s function %d",
					function.Symbol,
					previous.Path,
					previous.Function,
				)
			}
			seenSymbols[function.Symbol] = source
			functions = append(functions, function)
		}
	}
	sort.Slice(
		functions,
		func(left, right int) bool {
			return functions[left].Symbol < functions[right].Symbol
		},
	)
	return Set{functions: functions, canonical: canonicalFunctions(functions)}, nil
}

func normalizeFunction(raw functionDocument, source Source) (Function, error) {
	if !validSymbol(raw.Symbol) {
		return Function{}, sourceError(source, "invalid function symbol %q", raw.Symbol)
	}
	function := Function{
		Symbol: raw.Symbol,
		NoReturn: raw.NoReturn,
		Blocking: raw.Blocking,
		Source: source,
	}
	var err error
	for _, indexed := range
		[]struct {
			name string
			input []int
		}{
			{name: "must-use", input: raw.MustUse},
			{name: "closes", input: raw.Closes},
			{name: "takes-ownership", input: raw.TakesOwnership},
			{name: "completes-transaction", input: raw.CompletesTransaction},
			{name: "invokes-cancellation", input: raw.InvokesCancellation},
		} {
		name := indexed.name
		input := indexed.input
		normalized, normalizeErr := normalizeIndices(name, input, source)
		if normalizeErr != nil {
			return Function{}, normalizeErr
		}
		switch name {
		case "must-use":
			function.MustUse = normalized
		case "closes":
			function.Closes = normalized
		case "takes-ownership":
			function.TakesOwnership = normalized
		case "completes-transaction":
			function.CompletesTransaction = normalized
		case "invokes-cancellation":
			function.InvokesCancellation = normalized
		}
	}
	function.NilError, err = normalizeNilError(raw.NilError, source)
	if err != nil {
		return Function{}, err
	}
	function.ReturnsAlias, err = normalizeAliases(raw.ReturnsAlias, source)
	if err != nil {
		return Function{}, err
	}
	if !function.NoReturn &&
		!function.Blocking &&
		len(function.MustUse) == 0 &&
		len(function.Closes) == 0 &&
		len(function.TakesOwnership) == 0 &&
		len(function.CompletesTransaction) == 0 &&
		len(function.InvokesCancellation) == 0 &&
		len(function.NilError) == 0 &&
		len(function.ReturnsAlias) == 0 {
		return Function{}, sourceError(
			source,
			"function contract %q does not declare an effect",
			function.Symbol,
		)
	}
	return function, nil
}

func normalizeIndices(name string, input []int, source Source) ([]int, error) {
	if len(input) > MaxIndices {
		return nil, sourceError(
			source,
			"%s contains more than %d indices",
			name,
			MaxIndices,
		)
	}
	result := slices.Clone(input)
	sort.Ints(result)
	for index, value := range result {
		if value < 0 {
			return nil, sourceError(
				source,
				"%s contains negative index %d",
				name,
				value,
			)
		}
		if index > 0 && result[index - 1] == value {
			return nil, sourceError(
				source,
				"%s contains duplicate index %d",
				name,
				value,
			)
		}
	}
	return result, nil
}

func normalizeNilError(input []nilErrorDocument, source Source) ([]NilErrorRelation, error) {
	if len(input) > MaxIndices {
		return nil, sourceError(
			source,
			"nil-error contains more than %d relationships",
			MaxIndices,
		)
	}
	result := make([]NilErrorRelation, len(input))
	for index, raw := range input {
		if raw.Value < 0 || raw.Error < 0 {
			return nil, sourceError(
				source,
				"nil-error relationship %d contains a negative result index",
				index + 1,
			)
		}
		if raw.Value == raw.Error {
			return nil, sourceError(
				source,
				"nil-error relationship %d must use distinct value and error results",
				index + 1,
			)
		}
		whenNil, err := parseNilState(raw.WhenErrorNil)
		if err != nil {
			return nil, sourceError(
				source,
				"nil-error relationship %d when-error-nil %s",
				index + 1,
				err,
			)
		}
		whenNonNil, err := parseNilState(raw.WhenErrorNonNil)
		if err != nil {
			return nil, sourceError(
				source,
				"nil-error relationship %d when-error-non-nil %s",
				index + 1,
				err,
			)
		}
		if whenNil == NilStateUnknown && whenNonNil == NilStateUnknown {
			return nil, sourceError(
				source,
				"nil-error relationship %d must declare at least one state",
				index + 1,
			)
		}
		result[index] = NilErrorRelation{
			Value: raw.Value,
			Error: raw.Error,
			WhenErrorNil: whenNil,
			WhenErrorNonNil: whenNonNil,
		}
	}
	sort.Slice(
		result,
		func(left, right int) bool {
			if result[left].Value != result[right].Value {
				return result[left].Value < result[right].Value
			}
			return result[left].Error < result[right].Error
		},
	)
	for index := 1; index < len(result); index++ {
		if result[index - 1].Value == result[index].Value &&
			result[index - 1].Error == result[index].Error {
			return nil, sourceError(
				source,
				"function contract repeats nil-error relationship for results %d and %d",
				result[index].Value,
				result[index].Error,
			)
		}
	}
	return result, nil
}

func normalizeAliases(input []returnAliasDocument, source Source) ([]ReturnAlias, error) {
	if len(input) > MaxIndices {
		return nil, sourceError(
			source,
			"returns-alias contains more than %d relationships",
			MaxIndices,
		)
	}
	result := make([]ReturnAlias, len(input))
	for index, raw := range input {
		if raw.Result < 0 || raw.Argument < 0 {
			return nil, sourceError(
				source,
				"returns-alias relationship %d contains a negative index",
				index + 1,
			)
		}
		result[index] = ReturnAlias{Result: raw.Result, Argument: raw.Argument}
	}
	sort.Slice(
		result,
		func(left, right int) bool {
			if result[left].Result != result[right].Result {
				return result[left].Result < result[right].Result
			}
			return result[left].Argument < result[right].Argument
		},
	)
	for index := 1; index < len(result); index++ {
		if result[index - 1] == result[index] {
			return nil, sourceError(
				source,
				"function contract repeats returns-alias relationship for result %d and argument %d",
				result[index].Result,
				result[index].Argument,
			)
		}
	}
	return result, nil
}

func parseNilState(value string) (NilState, error) {
	switch value {
	case "":
		return NilStateUnknown, nil
	case "nil":
		return NilStateNil, nil
	case "non-nil":
		return NilStateNonNil, nil
	default:
		return NilStateUnknown, fmt.Errorf("must be nil or non-nil")
	}
}

func validSymbol(symbol string) bool {
	if symbol == "" || symbol != strings.TrimSpace(symbol) || strings.Contains(symbol, "..") {
		return false
	}
	separator := strings.LastIndexByte(symbol, '.')
	if separator <= 0 ||
		separator == len(symbol) - 1 ||
		!token.IsIdentifier(symbol[separator + 1:]) {
		return false
	}
	prefix := symbol[:separator]
	if strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") {
		return false
	}
	return module.CheckImportPath(prefix) == nil
}

func canonicalFunctions(functions []Function) []byte {
	encoded := []byte("glippy-project-contracts-v1")
	encoded = binary.AppendUvarint(encoded, uint64(len(functions)))
	for _, function := range functions {
		encoded = appendString(encoded, function.Symbol)
		encoded = appendBool(encoded, function.NoReturn)
		encoded = appendIndices(encoded, function.MustUse)
		encoded = appendIndices(encoded, function.Closes)
		encoded = appendIndices(encoded, function.TakesOwnership)
		encoded = appendIndices(encoded, function.CompletesTransaction)
		encoded = appendIndices(encoded, function.InvokesCancellation)
		encoded = appendBool(encoded, function.Blocking)
		encoded = binary.AppendUvarint(encoded, uint64(len(function.NilError)))
		for _, relation := range function.NilError {
			encoded = binary.AppendUvarint(encoded, uint64(relation.Value))
			encoded = binary.AppendUvarint(encoded, uint64(relation.Error))
			encoded = append(
				encoded,
				byte(relation.WhenErrorNil),
				byte(relation.WhenErrorNonNil),
			)
		}
		encoded = binary.AppendUvarint(encoded, uint64(len(function.ReturnsAlias)))
		for _, relation := range function.ReturnsAlias {
			encoded = binary.AppendUvarint(encoded, uint64(relation.Result))
			encoded = binary.AppendUvarint(encoded, uint64(relation.Argument))
		}
	}
	return encoded
}

func appendString(encoded []byte, value string) []byte {
	encoded = binary.AppendUvarint(encoded, uint64(len(value)))
	return append(encoded, value...)
}

func appendBool(encoded []byte, value bool) []byte {
	if value {
		return append(encoded, 1)
	}
	return append(encoded, 0)
}

func appendIndices(encoded []byte, values []int) []byte {
	encoded = binary.AppendUvarint(encoded, uint64(len(values)))
	for _, value := range values {
		encoded = binary.AppendUvarint(encoded, uint64(value))
	}
	return encoded
}

func cloneFunctions(input []Function) []Function {
	result := make([]Function, len(input))
	for index, function := range input {
		result[index] = function
		result[index].MustUse = slices.Clone(function.MustUse)
		result[index].Closes = slices.Clone(function.Closes)
		result[index].TakesOwnership = slices.Clone(function.TakesOwnership)
		result[index].CompletesTransaction = slices.Clone(function.CompletesTransaction)
		result[index].InvokesCancellation = slices.Clone(function.InvokesCancellation)
		result[index].NilError = slices.Clone(function.NilError)
		result[index].ReturnsAlias = slices.Clone(function.ReturnsAlias)
	}
	return result
}

func sourceError(source Source, format string, arguments ...any) error {
	return &Error{
		Path: source.Path,
		Line: source.Line,
		Column: source.Column,
		Message: fmt.Sprintf(
			"function %d: %s",
			source.Function,
			fmt.Sprintf(format, arguments...),
		),
	}
}

func contractError(path, format string, arguments ...any) error {
	return &Error{Path: path, Message: fmt.Sprintf(format, arguments...)}
}

func decodeError(path string, cause error) error {
	result := &Error{Path: path, Message: cause.Error(), cause: cause}
	var decoded *toml.DecodeError
	if errors.As(cause, &decoded) {
		result.Line, result.Column = decoded.Position()
		result.Message = strings.TrimPrefix(decoded.Error(), "toml: ")
	}
	return result
}

func functionLocations(input []byte) []Source {
	lines := bytes.Split(input, []byte{'\n'})
	result := make([]Source, 0)
	for index, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if !bytes.HasPrefix(trimmed, []byte("[[functions]]")) {
			continue
		}
		remainder := bytes.TrimSpace(trimmed[len("[[functions]]"):])
		if len(remainder) != 0 && remainder[0] != '#' {
			continue
		}
		column := bytes.Index(line, []byte("[[functions]]")) + 1
		result = append(result, Source{Line: index + 1, Column: column})
	}
	return result
}

// Resolved is an immutable lookup keyed by stable package-qualified identity.
type Resolved struct {
	functions map[string]Function
	objects map[string]*types.Func
}

// Binding associates one loaded type object with its immutable contract.
type Binding struct {
	Function *types.Func
	Contract Function
}

// Len returns the number of contracts resolved against loaded packages.
func (r Resolved) Len() int {
	return len(r.functions)
}

// Lookup returns an independently owned contract for one exact function.
func (r Resolved) Lookup(function *types.Func) (Function, bool) {
	if function == nil {
		return Function{}, false
	}
	contract, found := r.functions[functionIdentity(function)]
	if !found {
		return Function{}, false
	}
	return cloneFunctions([]Function{contract})[0], true
}

// Bindings returns resolved functions in stable identity order.
func (r Resolved) Bindings() []Binding {
	identities := make([]string, 0, len(r.functions))
	for identity := range r.functions {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	result := make([]Binding, len(identities))
	for index, identity := range identities {
		result[index] = Binding{
			Function: r.objects[identity],
			Contract: cloneFunctions([]Function{r.functions[identity]})[0],
		}
	}
	return result
}

// Resolve binds declarations to exact functions available in a loaded type graph.
// Declarations for packages absent from the graph remain deferred. A declaration
// whose package is present but whose function or method is absent is invalid.
func Resolve(set Set, packages []*types.Package) (Resolved, error) {
	bySymbol := make(map[string][]*types.Func)
	packagePaths := make([]string, 0, len(packages))
	seenPackages := make(map[string]struct{})
	for _, package_ := range packages {
		if package_ == nil || package_.Path() == "" {
			continue
		}
		if _, seen := seenPackages[package_.Path()]; !seen {
			seenPackages[package_.Path()] = struct{}{}
			packagePaths = append(packagePaths, package_.Path())
		}
		for _, name := range package_.Scope().Names() {
			object := package_.Scope().Lookup(name)
			switch object := object.(type) {
			case *types.Func:
				symbol := package_.Path() + "." + object.Name()
				bySymbol[symbol] = append(bySymbol[symbol], object)
			case *types.TypeName:
				named, _ := object.Type().(*types.Named)
				if named == nil {
					continue
				}
				for methodIndex := 0;
					methodIndex < named.NumMethods();
					methodIndex++ {
					method := named.Method(methodIndex)
					symbol := package_.Path() +
						"." +
						object.Name() +
						"." +
						method.Name()
					bySymbol[symbol] = append(bySymbol[symbol], method)
				}
			}
		}
	}
	sort.Strings(packagePaths)
	resolved := Resolved{
		functions: make(map[string]Function),
		objects: make(map[string]*types.Func),
	}
	for _, contract := range set.functions {
		candidates := bySymbol[contract.Symbol]
		sort.Slice(
			candidates,
			func(left, right int) bool {
				return functionIdentity(candidates[left]) <
					functionIdentity(candidates[right])
			},
		)
		found := len(candidates) != 0
		if !found {
			loadedPackage := false
			for _, packagePath := range packagePaths {
				if strings.HasPrefix(contract.Symbol, packagePath + ".") {
					loadedPackage = true
					break
				}
			}
			if loadedPackage {
				return Resolved{}, sourceError(
					contract.Source,
					"function symbol %q does not resolve in loaded package",
					contract.Symbol,
				)
			}
			continue
		}
		for _, function := range candidates {
			if err := validateSignature(contract, function); err != nil {
				return Resolved{}, err
			}
		}
		for _, function := range candidates {
			identity := functionIdentity(function)
			resolved.functions[identity] = contract
			resolved.objects[identity] = function
		}
	}
	return resolved, nil
}

func validateSignature(contract Function, function *types.Func) error {
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return sourceError(
			contract.Source,
			"function symbol %q has no signature",
			contract.Symbol,
		)
	}
	resultCount := signature.Results().Len()
	parameterCount := signature.Params().Len()
	for _, index := range contract.MustUse {
		if index >= resultCount {
			return sourceError(
				contract.Source,
				"function symbol %q result index %d exceeds %d results",
				contract.Symbol,
				index,
				resultCount,
			)
		}
	}
	for _, indexed := range
		[]struct {
			name string
			indices []int
		}{
			{name: "closes", indices: contract.Closes},
			{name: "takes-ownership", indices: contract.TakesOwnership},
			{name: "completes-transaction", indices: contract.CompletesTransaction},
			{name: "invokes-cancellation", indices: contract.InvokesCancellation},
		} {
		name := indexed.name
		indices := indexed.indices
		for _, index := range indices {
			if index >= parameterCount {
				return sourceError(
					contract.Source,
					"function symbol %q %s parameter index %d exceeds %d parameters",
					contract.Symbol,
					name,
					index,
					parameterCount,
				)
			}
		}
	}
	errorType := types.Universe.Lookup("error").Type()
	for _, relation := range contract.NilError {
		if relation.Value >= resultCount || relation.Error >= resultCount {
			return sourceError(
				contract.Source,
				"function symbol %q nil-error result indexes %d and %d exceed %d results",
				contract.Symbol,
				relation.Value,
				relation.Error,
				resultCount,
			)
		}
		if !types.AssignableTo(signature.Results().At(relation.Error).Type(), errorType) {
			return sourceError(
				contract.Source,
				"function symbol %q result index %d is not assignable to error",
				contract.Symbol,
				relation.Error,
			)
		}
		if !nilCapable(signature.Results().At(relation.Value).Type()) {
			return sourceError(
				contract.Source,
				"function symbol %q result index %d is not nil-capable",
				contract.Symbol,
				relation.Value,
			)
		}
	}
	for _, relation := range contract.ReturnsAlias {
		if relation.Result >= resultCount {
			return sourceError(
				contract.Source,
				"function symbol %q result index %d exceeds %d results",
				contract.Symbol,
				relation.Result,
				resultCount,
			)
		}
		if relation.Argument >= parameterCount {
			return sourceError(
				contract.Source,
				"function symbol %q argument index %d exceeds %d parameters",
				contract.Symbol,
				relation.Argument,
				parameterCount,
			)
		}
		resultType := signature.Results().At(relation.Result).Type()
		argumentType := signature.Params().At(relation.Argument).Type()
		if !types.AssignableTo(resultType, argumentType) &&
			!types.AssignableTo(argumentType, resultType) {
			return sourceError(
				contract.Source,
				"function symbol %q alias result %d and argument %d have incompatible types",
				contract.Symbol,
				relation.Result,
				relation.Argument,
			)
		}
	}
	return nil
}

func nilCapable(type_ types.Type) bool {
	switch type_.Underlying().(type) {
	case *types.Chan,
		*types.Interface,
		*types.Map,
		*types.Pointer,
		*types.Signature,
		*types.Slice:
		return true
	default:
		return false
	}
}

func functionIdentity(function *types.Func) string {
	if function == nil || function.Pkg() == nil {
		return ""
	}
	return types.ObjectString(
		function,
		func(package_ *types.Package) string {
			if package_ == nil {
				return ""
			}
			return package_.Path()
		},
	)
}
