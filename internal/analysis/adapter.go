package analysis

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/url"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	goanalysis "golang.org/x/tools/go/analysis"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

// AnalyzerFixMapping binds one exact go/analysis suggestion to native metadata.
type AnalyzerFixMapping struct {
	Message string
	Name string
	Description string
	Safety rules.FixSafety
	Audited bool
}

// AnalyzerAdapterOptions supplies the native product contract for an analyzer.
type AnalyzerAdapterOptions struct {
	Metadata rules.Metadata
	SuggestedFixes []AnalyzerFixMapping
	FlagBindings []AnalyzerFlagBinding
	ReadOnlyAudited bool
}

// AnalyzerFactory constructs one independent analyzer graph for a single run.
type AnalyzerFactory func() *goanalysis.Analyzer

// AnalyzerFlagBinding maps one native typed option to one analyzer flag.
type AnalyzerFlagBinding struct {
	Option string
	Analyzer string
	Flag string
}

type analyzerFix struct {
	name string
	safety rules.FixSafety
}

type analyzerRule struct {
	analyzer goanalysis.Analyzer
	metadata rules.Metadata
	fixes map[string]analyzerFix
	factory AnalyzerFactory
	bindings []AnalyzerFlagBinding
	contract []analyzerContractStep
	admission map[*goanalysis.Analyzer]struct{}
}

type packageAnalyzerRule struct {
	analyzer goanalysis.Analyzer
	metadata rules.Metadata
	fixes map[string]analyzerFix
	steps []packageAnalyzerStep
	factory AnalyzerFactory
	bindings []AnalyzerFlagBinding
	contract []analyzerContractStep
	admission map[*goanalysis.Analyzer]struct{}
}

type packageAnalyzerStep struct {
	original *goanalysis.Analyzer
	analyzer goanalysis.Analyzer
}

// AdaptAnalyzer wraps one suitable go/analysis analyzer as a native rule.
func AdaptAnalyzer(
	analyzer *goanalysis.Analyzer,
	options AnalyzerAdapterOptions,
) (rules.Rule, error) {
	return adaptAnalyzer(analyzer, nil, nil, options)
}

// AdaptAnalyzerFactory wraps independently constructed analyzer graphs so
// native options can be bound without mutating shared flag state.
func AdaptAnalyzerFactory(
	factory AnalyzerFactory,
	options AnalyzerAdapterOptions,
) (rules.Rule, error) {
	if factory == nil {
		return nil, fmt.Errorf("adapt go/analysis factory: nil factory")
	}
	analyzer, err := callAnalyzerFactory(factory)
	if err != nil {
		return nil, fmt.Errorf("adapt go/analysis factory: %w", err)
	}
	if analyzer == nil {
		return nil, fmt.Errorf("adapt go/analysis factory: factory returned nil analyzer")
	}
	if err := goanalysis.Validate([]*goanalysis.Analyzer{analyzer}); err != nil {
		return nil, fmt.Errorf(
			"adapt go/analysis factory: validate analyzer graph: %w",
			err,
		)
	}
	comparison, err := callAnalyzerFactory(factory)
	if err != nil {
		return nil, fmt.Errorf("adapt go/analysis factory: %w", err)
	}
	if comparison == nil {
		return nil, fmt.Errorf("adapt go/analysis factory: factory returned nil analyzer")
	}
	if err := goanalysis.Validate([]*goanalysis.Analyzer{comparison}); err != nil {
		return nil, fmt.Errorf(
			"adapt go/analysis factory: validate comparison graph: %w",
			err,
		)
	}
	if !freshAnalyzerGraphs(analyzer, comparison) {
		return nil, fmt.Errorf(
			"adapt go/analysis factory: factory must return a fresh analyzer graph",
		)
	}
	contract, err := analyzerContract(analyzer)
	if err != nil {
		return nil, fmt.Errorf("adapt go/analysis factory: %w", err)
	}
	comparisonContract, err := analyzerContract(comparison)
	if err != nil {
		return nil, fmt.Errorf("adapt go/analysis factory: %w", err)
	}
	if !reflect.DeepEqual(contract, comparisonContract) {
		return nil, fmt.Errorf(
			"adapt go/analysis factory: factory contract changed between instances",
		)
	}
	return adaptAnalyzer(
		analyzer,
		factory,
		analyzerGraphIdentities(analyzer, comparison),
		options,
	)
}

func analyzerGraphIdentities(graphs ...*goanalysis.Analyzer) map[*goanalysis.Analyzer]struct{} {
	result := make(map[*goanalysis.Analyzer]struct{})
	for _, graph := range graphs {
		for _, analyzer := range analyzerExecutionPlan(graph) {
			result[analyzer] = struct{}{}
		}
	}
	return result
}

func callAnalyzerFactory(factory AnalyzerFactory) (analyzer *goanalysis.Analyzer, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("factory panicked: %v", recovered)
		}
	}()
	return factory(), nil
}

func freshAnalyzerGraphs(first, second *goanalysis.Analyzer) bool {
	firstPlan := analyzerExecutionPlan(first)
	secondPlan := analyzerExecutionPlan(second)
	if len(firstPlan) != len(secondPlan) {
		return false
	}
	for index := range firstPlan {
		if firstPlan[index] == secondPlan[index] ||
			firstPlan[index].Name != secondPlan[index].Name {
			return false
		}
	}
	return true
}

type analyzerContractStep struct {
	Name string
	Doc string
	URL string
	RunDespiteErrors bool
	ResultType reflect.Type
	Requires []string
	FactTypes []reflect.Type
	Flags []analyzerFlagContract
}

type analyzerFlagContract struct {
	Name string
	Default string
	Usage string
	ValueType reflect.Type
	GetterType reflect.Type
}

type analyzerFlagStorage struct {
	type_ reflect.Type
	pointer uintptr
}

func analyzerContract(root *goanalysis.Analyzer) ([]analyzerContractStep, error) {
	plan := analyzerExecutionPlan(root)
	result := make([]analyzerContractStep, len(plan))
	for index, analyzer := range plan {
		step := analyzerContractStep{
			Name: analyzer.Name,
			Doc: analyzer.Doc,
			URL: analyzer.URL,
			RunDespiteErrors: analyzer.RunDespiteErrors,
		}
		step.ResultType = analyzer.ResultType
		step.Requires = make([]string, len(analyzer.Requires))
		for requirementIndex, required := range analyzer.Requires {
			step.Requires[requirementIndex] = required.Name
		}
		sort.Strings(step.Requires)
		step.FactTypes = make([]reflect.Type, len(analyzer.FactTypes))
		for factIndex, fact := range analyzer.FactTypes {
			step.FactTypes[factIndex] = reflect.TypeOf(fact)
		}
		sort.Slice(
			step.FactTypes,
			func(left, right int) bool {
				return reflectTypeSortKey(step.FactTypes[left]) <
					reflectTypeSortKey(step.FactTypes[right])
			},
		)
		var contractErr error
		analyzer.Flags.VisitAll(
			func(setting *flag.Flag) {
				if contractErr != nil {
					return
				}
				var getterType reflect.Type
				if getter, ok := setting.Value.(flag.Getter); ok {
					value, err := analyzerFlagGetterValue(getter)
					if err != nil {
						contractErr = fmt.Errorf(
							"analyzer flag %q.%s: %w",
							analyzer.Name,
							setting.Name,
							err,
						)
						return
					}
					getterType = reflect.TypeOf(value)
				}
				step.Flags = append(
					step.Flags,
					analyzerFlagContract{
						Name: setting.Name,
						Default: setting.DefValue,
						Usage: setting.Usage,
						ValueType: reflect.TypeOf(setting.Value),
						GetterType: getterType,
					},
				)
			},
		)
		if contractErr != nil {
			return nil, contractErr
		}
		result[index] = step
	}
	return result, nil
}

func analyzerFlagGetterValue(getter flag.Getter) (value any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("flag getter panicked: %v", recovered)
		}
	}()
	return getter.Get(), nil
}

func reflectTypeSortKey(type_ reflect.Type) string {
	if type_ == nil {
		return ""
	}
	for type_.Kind() == reflect.Pointer {
		type_ = type_.Elem()
	}
	return type_.PkgPath() + "\x00" + type_.String()
}

func validateAnalyzerFactoryInstance(
	instance *goanalysis.Analyzer,
	name string,
	contract []analyzerContractStep,
	admission map[*goanalysis.Analyzer]struct{},
) error {
	if instance == nil {
		return fmt.Errorf("go/analysis factory returned nil analyzer")
	}
	if instance.Name != name {
		return fmt.Errorf(
			"go/analysis factory returned analyzer %q; want %q",
			instance.Name,
			name,
		)
	}
	if err := goanalysis.Validate([]*goanalysis.Analyzer{instance}); err != nil {
		return fmt.Errorf("validate analyzer factory result: %w", err)
	}
	for _, analyzer := range analyzerExecutionPlan(instance) {
		if _, reused := admission[analyzer]; reused {
			return fmt.Errorf(
				"go/analysis factory reused an admission analyzer %q",
				analyzer.Name,
			)
		}
	}
	instanceContract, err := analyzerContract(instance)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(contract, instanceContract) {
		return fmt.Errorf("go/analysis factory contract changed at runtime")
	}
	return nil
}

func adaptAnalyzer(
	analyzer *goanalysis.Analyzer,
	factory AnalyzerFactory,
	admission map[*goanalysis.Analyzer]struct{},
	options AnalyzerAdapterOptions,
) (rules.Rule, error) {
	if analyzer == nil {
		return nil, fmt.Errorf("adapt go/analysis: nil analyzer")
	}
	if err := goanalysis.Validate([]*goanalysis.Analyzer{analyzer}); err != nil {
		return nil, fmt.Errorf("adapt go/analysis: %w", err)
	}
	typed := options.Metadata.Requirement == rules.RequireTypes
	if len(analyzer.Requires) != 0 && !typed {
		return nil, fmt.Errorf(
			"adapt go/analysis %q: prerequisite analyzers are not supported",
			analyzer.Name,
		)
	}
	if analyzer.ResultType != nil && !typed {
		return nil, fmt.Errorf(
			"adapt go/analysis %q: analyzer results require prerequisite scheduling",
			analyzer.Name,
		)
	}
	plan := analyzerExecutionPlan(analyzer)
	for _, step := range plan {
		if len(step.FactTypes) != 0 && !typed {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: analyzer %q facts require typed package execution",
				analyzer.Name,
				step.Name,
			)
		}
		hasFlags := analyzerHasFlags(step)
		if hasFlags && factory == nil {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: analyzer %q flags require an isolated analyzer factory",
				analyzer.Name,
				step.Name,
			)
		}
	}
	if err := validateAnalyzerFlagBindings(
		analyzer,
		plan,
		options.Metadata,
		options.FlagBindings,
	);
		err != nil {
		return nil, err
	}
	if len(options.Metadata.Fixes) != 0 {
		return nil, fmt.Errorf(
			"adapt go/analysis %q: native fix metadata must come from suggested-fix mappings",
			analyzer.Name,
		)
	}

	metadata := cloneAnalyzerMetadata(options.Metadata)
	contract, err := analyzerContract(analyzer)
	if err != nil {
		return nil, fmt.Errorf("adapt go/analysis %q: %w", analyzer.Name, err)
	}
	fixes := make(map[string]analyzerFix, len(options.SuggestedFixes))
	fixNames := make(map[string]struct{}, len(options.SuggestedFixes))
	for index, mapping := range options.SuggestedFixes {
		if strings.TrimSpace(mapping.Message) == "" ||
			strings.TrimSpace(mapping.Name) == "" ||
			strings.TrimSpace(mapping.Description) == "" {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: suggested-fix mapping %d is incomplete",
				analyzer.Name,
				index,
			)
		}
		if _, duplicate := fixes[mapping.Message]; duplicate {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: duplicate suggested-fix message %q",
				analyzer.Name,
				mapping.Message,
			)
		}
		if _, duplicate := fixNames[mapping.Name]; duplicate {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: duplicate native fix name %q",
				analyzer.Name,
				mapping.Name,
			)
		}
		safety := mapping.Safety
		if safety == "" {
			safety = rules.FixSuggestion
		}
		if safety == rules.FixSafe && !mapping.Audited {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: safe fix %q requires an explicit safety audit",
				analyzer.Name,
				mapping.Name,
			)
		}
		if mapping.Audited && safety != rules.FixSafe {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: fix audit applies only to safe fixes",
				analyzer.Name,
			)
		}
		fixes[mapping.Message] = analyzerFix{name: mapping.Name, safety: safety}
		fixNames[mapping.Name] = struct{}{}
		metadata.Fixes = append(
			metadata.Fixes,
			rules.FixMetadata{
				Name: mapping.Name,
				Description: mapping.Description,
				Safety: safety,
			},
		)
	}

	snapshot := *analyzer
	snapshot.Requires = nil
	snapshot.FactTypes = nil
	if len(metadata.NodeInterests) != 1 || metadata.NodeInterests[0] != rules.NodeFile {
		return nil, fmt.Errorf(
			"adapt go/analysis %q: adapter metadata must declare only file interest",
			analyzer.Name,
		)
	}
	var adapted rules.Rule
	switch metadata.Requirement {
	case rules.RequireSyntax:
		adapted = &analyzerRule{
			analyzer: snapshot,
			metadata: metadata,
			fixes: fixes,
			factory: factory,
			bindings: slices.Clone(options.FlagBindings),
			contract: contract,
			admission: admission,
		}
	case rules.RequireTypes:
		if !options.ReadOnlyAudited {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: typed package execution requires a read-only analyzer audit",
				analyzer.Name,
			)
		}
		if metadata.RunDespiteTypeErrors {
			for _, step := range plan {
				if !step.RunDespiteErrors {
					return nil, fmt.Errorf(
						"adapt go/analysis %q: native type-error policy exceeds analyzer %q contract",
						analyzer.Name,
						step.Name,
					)
				}
			}
		}
		steps := make([]packageAnalyzerStep, len(plan))
		for index, step := range plan {
			steps[index] = packageAnalyzerStep{original: step, analyzer: *step}
		}
		adapted = &packageAnalyzerRule{
			analyzer: snapshot,
			metadata: metadata,
			fixes: fixes,
			steps: steps,
			factory: factory,
			bindings: slices.Clone(options.FlagBindings),
			contract: contract,
			admission: admission,
		}
	default:
		return nil, fmt.Errorf(
			"adapt go/analysis %q: adapter metadata must declare syntax or types requirement",
			analyzer.Name,
		)
	}
	if _, err := rules.NewRegistry(adapted); err != nil {
		return nil, fmt.Errorf("adapt go/analysis %q metadata: %w", analyzer.Name, err)
	}
	return adapted, nil
}

func analyzerExecutionPlan(root *goanalysis.Analyzer) []*goanalysis.Analyzer {
	visited := make(map[*goanalysis.Analyzer]struct{})
	plan := make([]*goanalysis.Analyzer, 0)
	var visit func(*goanalysis.Analyzer)
	visit = func(analyzer *goanalysis.Analyzer) {
		if _, found := visited[analyzer]; found {
			return
		}
		visited[analyzer] = struct{}{}
		requires := slices.Clone(analyzer.Requires)
		sort.Slice(
			requires,
			func(left, right int) bool {
				return requires[left].Name < requires[right].Name
			},
		)
		for _, required := range requires {
			visit(required)
		}
		plan = append(plan, analyzer)
	}
	visit(root)
	return plan
}

func analyzerHasFlags(analyzer *goanalysis.Analyzer) bool {
	hasFlags := false
	analyzer.Flags.VisitAll(
		func(*flag.Flag) {
			hasFlags = true
		},
	)
	return hasFlags
}

func validateAnalyzerFlagBindings(
	root *goanalysis.Analyzer,
	plan []*goanalysis.Analyzer,
	metadata rules.Metadata,
	bindings []AnalyzerFlagBinding,
) error {
	options := make(map[string]rules.OptionMetadata, len(metadata.Options))
	for _, option := range metadata.Options {
		options[option.Name] = option
		if option.Kind == rules.OptionStrings {
			return fmt.Errorf(
				"adapt go/analysis %q: string-list option %q cannot bind to an analyzer flag",
				root.Name,
				option.Name,
			)
		}
	}
	analyzers := make(map[string]*goanalysis.Analyzer, len(plan))
	storageOwners := make(map[analyzerFlagStorage]string)
	for _, analyzer := range plan {
		if _, duplicate := analyzers[analyzer.Name]; duplicate {
			return fmt.Errorf(
				"adapt go/analysis %q: duplicate analyzer name %q",
				root.Name,
				analyzer.Name,
			)
		}
		analyzers[analyzer.Name] = analyzer
		var storageErr error
		analyzer.Flags.VisitAll(
			func(setting *flag.Flag) {
				if storageErr != nil {
					return
				}
				identity, found := analyzerFlagStorageIdentity(setting)
				if !found {
					return
				}
				current := analyzer.Name + "." + setting.Name
				if owner, duplicate := storageOwners[identity]; duplicate {
					storageErr = fmt.Errorf(
						"adapt go/analysis %q: analyzer flags %q and %q share value storage",
						root.Name,
						owner,
						current,
					)
					return
				}
				storageOwners[identity] = current
			},
		)
		if storageErr != nil {
			return storageErr
		}
	}
	boundOptions := make(map[string]struct{}, len(bindings))
	boundFlags := make(map[string]struct{}, len(bindings))
	for index, binding := range bindings {
		if strings.TrimSpace(binding.Option) == "" ||
			strings.TrimSpace(binding.Analyzer) == "" ||
			strings.TrimSpace(binding.Flag) == "" {
			return fmt.Errorf(
				"adapt go/analysis %q: flag binding %d is incomplete",
				root.Name,
				index,
			)
		}
		option, found := options[binding.Option]
		if !found {
			return fmt.Errorf(
				"adapt go/analysis %q: flag binding references unknown option %q",
				root.Name,
				binding.Option,
			)
		}
		analyzer, found := analyzers[binding.Analyzer]
		if !found {
			return fmt.Errorf(
				"adapt go/analysis %q: flag binding references unknown analyzer %q",
				root.Name,
				binding.Analyzer,
			)
		}
		setting := analyzer.Flags.Lookup(binding.Flag)
		if setting == nil {
			return fmt.Errorf(
				"adapt go/analysis %q: analyzer %q has no flag %q",
				root.Name,
				binding.Analyzer,
				binding.Flag,
			)
		}
		kind, supported, err := analyzerFlagKind(setting)
		if err != nil {
			return fmt.Errorf(
				"adapt go/analysis %q: analyzer flag %q.%s: %w",
				root.Name,
				binding.Analyzer,
				binding.Flag,
				err,
			)
		}
		if !supported {
			return fmt.Errorf(
				"adapt go/analysis %q: analyzer flag %q.%s has unsupported value type",
				root.Name,
				binding.Analyzer,
				binding.Flag,
			)
		}
		if kind != option.Kind {
			return fmt.Errorf(
				"adapt go/analysis %q: analyzer flag %q.%s has flag kind %s; want %s",
				root.Name,
				binding.Analyzer,
				binding.Flag,
				kind,
				option.Kind,
			)
		}
		if _, duplicate := boundOptions[binding.Option]; duplicate {
			return fmt.Errorf(
				"adapt go/analysis %q: option %q is bound more than once",
				root.Name,
				binding.Option,
			)
		}
		flagID := binding.Analyzer + "\x00" + binding.Flag
		if _, duplicate := boundFlags[flagID]; duplicate {
			return fmt.Errorf(
				"adapt go/analysis %q: analyzer flag %q.%s is bound more than once",
				root.Name,
				binding.Analyzer,
				binding.Flag,
			)
		}
		boundOptions[binding.Option] = struct{}{}
		boundFlags[flagID] = struct{}{}
	}
	for _, option := range metadata.Options {
		if _, found := boundOptions[option.Name]; !found {
			return fmt.Errorf(
				"adapt go/analysis %q: native option %q has no analyzer flag binding",
				root.Name,
				option.Name,
			)
		}
	}
	for _, analyzer := range plan {
		var unbound string
		analyzer.Flags.VisitAll(
			func(flag *flag.Flag) {
				if _, found := boundFlags[analyzer.Name + "\x00" + flag.Name];
					!found && unbound == "" {
					unbound = flag.Name
				}
			},
		)
		if unbound != "" {
			return fmt.Errorf(
				"adapt go/analysis %q: analyzer flag %q.%s has no native option binding",
				root.Name,
				analyzer.Name,
				unbound,
			)
		}
	}
	return nil
}

func analyzerFlagStorageIdentity(setting *flag.Flag) (analyzerFlagStorage, bool) {
	value := reflect.ValueOf(setting.Value)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return analyzerFlagStorage{}, false
	}
	return analyzerFlagStorage{type_: value.Type(), pointer: value.Pointer()}, true
}

func analyzerFlagKind(setting *flag.Flag) (rules.OptionKind, bool, error) {
	getter, ok := setting.Value.(flag.Getter)
	if !ok {
		return "", false, nil
	}
	value, err := analyzerFlagGetterValue(getter)
	if err != nil {
		return "", false, err
	}
	switch value.(type) {
	case bool:
		return rules.OptionBoolean, true, nil
	case int, int8, int16, int32, int64:
		return rules.OptionInteger, true, nil
	case string:
		return rules.OptionString, true, nil
	default:
		return "", false, nil
	}
}

func bindAnalyzerFlags(
	root *goanalysis.Analyzer,
	metadata rules.Metadata,
	bindings []AnalyzerFlagBinding,
	options analyzerOptionLookup,
) error {
	plan := analyzerExecutionPlan(root)
	if err := validateAnalyzerFlagBindings(root, plan, metadata, bindings); err != nil {
		return err
	}
	analyzers := make(map[string]*goanalysis.Analyzer, len(plan))
	for _, analyzer := range plan {
		analyzers[analyzer.Name] = analyzer
	}
	schema := make(map[string]rules.OptionMetadata, len(metadata.Options))
	for _, option := range metadata.Options {
		schema[option.Name] = option
	}
	for _, binding := range bindings {
		option := schema[binding.Option]
		var value string
		var found bool
		switch option.Kind {
		case rules.OptionBoolean:
			var configured bool
			configured, found = options.BooleanOption(binding.Option)
			value = fmt.Sprint(configured)
		case rules.OptionInteger:
			var configured int64
			configured, found = options.IntegerOption(binding.Option)
			value = fmt.Sprint(configured)
		case rules.OptionString:
			value, found = options.StringOption(binding.Option)
		}
		if !found {
			return fmt.Errorf(
				"adapt go/analysis %q: resolved option %q is missing",
				root.Name,
				binding.Option,
			)
		}
		if err := setAnalyzerFlag(&analyzers[binding.Analyzer].Flags, binding.Flag, value);
			err != nil {
			return fmt.Errorf(
				"adapt go/analysis %q: bind option %q to flag %q.%s: %w",
				root.Name,
				binding.Option,
				binding.Analyzer,
				binding.Flag,
				err,
			)
		}
	}
	return nil
}

func setAnalyzerFlag(flags *flag.FlagSet, name, value string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("flag setter panicked: %v", recovered)
		}
	}()
	return flags.Set(name, value)
}

type analyzerOptionLookup interface {
	BooleanOption(string) (bool, bool)
	IntegerOption(string) (int64, bool)
	StringOption(string) (string, bool)
}

type analyzerOptionSetLookup struct {
	options rules.OptionSet
}

func (l analyzerOptionSetLookup) BooleanOption(name string) (bool, bool) {
	return l.options.Boolean(name)
}

func (l analyzerOptionSetLookup) IntegerOption(name string) (int64, bool) {
	return l.options.Integer(name)
}

func (l analyzerOptionSetLookup) StringOption(name string) (string, bool) {
	return l.options.String(name)
}

func (r *analyzerRule) Metadata() rules.Metadata {
	return cloneAnalyzerMetadata(r.metadata)
}

func (r *packageAnalyzerRule) Metadata() rules.Metadata {
	return cloneAnalyzerMetadata(r.metadata)
}

func (r *analyzerRule) RunSyntaxFile(ctx *rules.Context) ([]rules.Finding, error) {
	if ctx == nil || ctx.File() == nil {
		return nil, fmt.Errorf("go/analysis adapter requires a source file")
	}
	file := ctx.File()
	analyzer := r.analyzer
	if r.factory != nil {
		instance, err := callAnalyzerFactory(r.factory)
		if err != nil {
			return nil, err
		}
		if err := validateAnalyzerFactoryInstance(
			instance,
			r.analyzer.Name,
			r.contract,
			r.admission,
		);
			err != nil {
			return nil, err
		}
		if err := bindAnalyzerFlags(instance, r.metadata, r.bindings, ctx); err != nil {
			return nil, err
		}
		analyzer = *instance
	}
	findings := make([]rules.Finding, 0)
	err := file.ReadSyntaxView(
		func(fileSet *token.FileSet, syntax *ast.File) error {
			tokenFile := fileSet.File(syntax.Pos())
			if tokenFile == nil {
				return fmt.Errorf("isolated syntax view has no token file")
			}
			diagnostics := make([]goanalysis.Diagnostic, 0)
			pass := &goanalysis.Pass{
				Analyzer: &analyzer,
				Fset: fileSet,
				Files: []*ast.File{syntax},
				Pkg: types.NewPackage("command-line-arguments", syntax.Name.Name),
				Report: func(diagnostic goanalysis.Diagnostic) {
					diagnostics = append(
						diagnostics,
						cloneAnalyzerDiagnostic(diagnostic),
					)
				},
				ResultOf: make(map[*goanalysis.Analyzer]any),
				ReadFile: func(filename string) ([]byte, error) {
					if filepath.Clean(filename) != file.Path() {
						return nil, fmt.Errorf(
							"read file %q: outside the adapted source",
							filename,
						)
					}
					return file.Bytes(), nil
				},
			}
			result, err := runAnalyzer(&analyzer, pass)
			if err != nil {
				return err
			}
			if result != nil {
				return fmt.Errorf("analyzer returned an unexpected result")
			}
			for _, diagnostic := range diagnostics {
				finding, err := r.finding(file, fileSet, tokenFile, diagnostic)
				if err != nil {
					return err
				}
				findings = append(findings, finding)
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	return findings, nil
}

func (r *analyzerRule) finding(
	file *source.File,
	fileSet *token.FileSet,
	tokenFile *token.File,
	diagnostic goanalysis.Diagnostic,
) (rules.Finding, error) {
	primary, err := analyzerRange(file, fileSet, tokenFile, diagnostic.Pos, diagnostic.End)
	if err != nil {
		return rules.Finding{}, fmt.Errorf("diagnostic range: %w", err)
	}
	messageKey := diagnostic.Category
	if strings.TrimSpace(messageKey) == "" {
		messageKey = r.analyzer.Name
	}
	related := make([]rules.Related, len(diagnostic.Related))
	for index, item := range diagnostic.Related {
		sourceRange, err := analyzerRange(file, fileSet, tokenFile, item.Pos, item.End)
		if err != nil {
			return rules.Finding{}, fmt.Errorf("related range %d: %w", index, err)
		}
		related[index] = rules.Related{Range: sourceRange, Message: item.Message}
	}
	fixes := make([]rules.Fix, len(diagnostic.SuggestedFixes))
	for fixIndex, suggested := range diagnostic.SuggestedFixes {
		mapped, found := r.fixes[suggested.Message]
		if !found {
			return rules.Finding{}, fmt.Errorf(
				"undeclared suggested fix %q",
				suggested.Message,
			)
		}
		edits := make([]rules.Edit, len(suggested.TextEdits))
		for editIndex, edit := range suggested.TextEdits {
			sourceRange, err := analyzerRange(
				file,
				fileSet,
				tokenFile,
				edit.Pos,
				edit.End,
			)
			if err != nil {
				return rules.Finding{}, fmt.Errorf(
					"suggested fix %q edit %d: %w",
					suggested.Message,
					editIndex,
					err,
				)
			}
			edits[editIndex] = rules.Edit{
				Range: sourceRange,
				NewText: string(edit.NewText),
			}
		}
		fixes[fixIndex] = rules.Fix{Name: mapped.name, Safety: mapped.safety, Edits: edits}
	}
	help, err := analyzerDiagnosticURL(&r.analyzer, diagnostic)
	if err != nil {
		return rules.Finding{}, err
	}
	return rules.Finding{
		MessageKey: messageKey,
		Message: diagnostic.Message,
		Range: primary,
		Related: related,
		Help: help,
		Fixes: fixes,
	}, nil
}

func analyzerDiagnosticURL(
	analyzer *goanalysis.Analyzer,
	diagnostic goanalysis.Diagnostic,
) (string, error) {
	if analyzer.URL == "" && diagnostic.URL == "" && diagnostic.Category == "" {
		return "", nil
	}
	raw := diagnostic.URL
	if raw == "" && diagnostic.Category != "" {
		raw = "#" + diagnostic.Category
	}
	diagnosticURL, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid diagnostic URL %q: %w", raw, err)
	}
	baseURL, err := url.Parse(analyzer.URL)
	if err != nil {
		return "", fmt.Errorf("invalid analyzer URL %q: %w", analyzer.URL, err)
	}
	return baseURL.ResolveReference(diagnosticURL).String(), nil
}

func analyzerRange(
	file *source.File,
	fileSet *token.FileSet,
	tokenFile *token.File,
	start, end token.Pos,
) (source.Range, error) {
	if !start.IsValid() {
		return source.Range{}, fmt.Errorf("position is invalid")
	}
	if !end.IsValid() {
		end = start
	}
	if fileSet.File(start) != tokenFile || fileSet.File(end) != tokenFile {
		return source.Range{}, fmt.Errorf("position is outside the adapted source")
	}
	result := source.Range{Start: tokenFile.Offset(start), End: tokenFile.Offset(end)}
	if _, valid := file.Slice(result); !valid {
		return source.Range{}, fmt.Errorf("position maps to an invalid physical range")
	}
	return result, nil
}

func runAnalyzer(analyzer *goanalysis.Analyzer, pass *goanalysis.Pass) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("analyzer panicked: %v", recovered)
		}
	}()
	return analyzer.Run(pass)
}

func cloneAnalyzerDiagnostic(diagnostic goanalysis.Diagnostic) goanalysis.Diagnostic {
	result := diagnostic
	result.Related = slices.Clone(diagnostic.Related)
	result.SuggestedFixes = make([]goanalysis.SuggestedFix, len(diagnostic.SuggestedFixes))
	for index, fix := range diagnostic.SuggestedFixes {
		result.SuggestedFixes[index] = fix
		result.SuggestedFixes[index].TextEdits = make(
			[]goanalysis.TextEdit,
			len(fix.TextEdits),
		)
		for editIndex, edit := range fix.TextEdits {
			result.SuggestedFixes[index].TextEdits[editIndex] = edit
			result.SuggestedFixes[index].TextEdits[editIndex].NewText = slices.Clone(
				edit.NewText,
			)
		}
	}
	return result
}

func cloneAnalyzerMetadata(metadata rules.Metadata) rules.Metadata {
	return rules.CloneMetadata(metadata)
}

var _ rules.SyntaxFileRule = (*analyzerRule)(nil)
