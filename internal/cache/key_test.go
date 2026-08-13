package cache

import (
	"reflect"
	"slices"
	"testing"
)

func TestBuildKeyCanonicalizesUnorderedInputsWithoutMutation(t *testing.T) {
	t.Parallel()

	input := testKeyInput()
	originalTags := slices.Clone(input.BuildTags)
	originalRules := slices.Clone(input.Rules)
	originalComponents := slices.Clone(input.Components)

	canonical, err := BuildKey(input)
	if err != nil {
		t.Fatal(err)
	}
	reversed := cloneKeyInput(input)
	slices.Reverse(reversed.BuildTags)
	slices.Reverse(reversed.Rules)
	slices.Reverse(reversed.Components)
	reordered, err := BuildKey(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != reordered || canonical == (Key{}) || len(canonical.String()) != 64 {
		t.Fatalf("canonical keys = %q and %q", canonical, reordered)
	}
	if !reflect.DeepEqual(input.BuildTags, originalTags) ||
		!reflect.DeepEqual(input.Rules, originalRules) ||
		!reflect.DeepEqual(input.Components, originalComponents) {
		t.Fatalf("BuildKey() mutated input = %#v", input)
	}
}

func TestBuildKeySchemaV1Fingerprint(t *testing.T) {
	t.Parallel()

	key, err := BuildKey(testKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	const want = "3ce4ac484dc089b9b4323b4fb404b3f2687e56ecaa5ea1bb7e71cf18ebab4fcc"
	if key.String() != want {
		t.Fatalf("BuildKey() = %q, want %q", key, want)
	}
}

func TestBuildKeyChangesForEveryDeclaredInput(t *testing.T) {
	t.Parallel()

	base := testKeyInput()
	want, err := BuildKey(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		mutate func(*KeyInput)
	}{
		{
			name: "namespace",
			mutate: func(input *KeyInput) {
				input.Namespace = "formatter"
			},
		},
		{
			name: "tool version",
			mutate: func(input *KeyInput) {
				input.ToolVersion = "v0.2.0"
			},
		},
		{
			name: "build Go version",
			mutate: func(input *KeyInput) {
				input.BuildGoVersion = "go1.27.0"
			},
		},
		{
			name: "source Go version",
			mutate: func(input *KeyInput) {
				input.SourceGoVersion = "go1.25"
			},
		},
		{
			name: "configuration",
			mutate: func(input *KeyInput) {
				input.Configuration = DigestOf([]byte("other configuration"))
			},
		},
		{
			name: "rule ID",
			mutate: func(input *KeyInput) {
				input.Rules[0].ID = "other-rule"
			},
		},
		{
			name: "rule severity",
			mutate: func(input *KeyInput) {
				input.Rules[0].Severity = "error"
			},
		},
		{
			name: "rule options",
			mutate: func(input *KeyInput) {
				input.Rules[0].Options = DigestOf([]byte("options"))
			},
		},
		{
			name: "build tags",
			mutate: func(input *KeyInput) {
				input.BuildTags = append(input.BuildTags, "race")
			},
		},
		{
			name: "GOOS",
			mutate: func(input *KeyInput) {
				input.GOOS = "darwin"
			},
		},
		{
			name: "GOARCH",
			mutate: func(input *KeyInput) {
				input.GOARCH = "arm64"
			},
		},
		{
			name: "cgo",
			mutate: func(input *KeyInput) {
				input.CGOEnabled = false
			},
		},
		{
			name: "formatter mode",
			mutate: func(input *KeyInput) {
				input.FormatterMode = "gofmt-fixed-point"
			},
		},
		{
			name: "component identity",
			mutate: func(input *KeyInput) {
				input.Components[0].Identity = "/project/b.go"
			},
		},
	}
	for index, component := range base.Components {
		componentIndex := index
		tests = append(
			tests,
			struct {
				name string
				mutate func(*KeyInput)
			}{
				name: "component " + string(component.Kind),
				mutate: func(input *KeyInput) {
					input.Components[componentIndex].Digest = DigestOf(
						[]byte("changed"),
					)
				},
			},
		)
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				input := cloneKeyInput(base)
				test.mutate(&input)
				got, err := BuildKey(input)
				if err != nil {
					t.Fatal(err)
				}
				if got == want {
					t.Fatalf("BuildKey() did not change for %s", test.name)
				}
			},
		)
	}
}

func TestBuildKeyRejectsIncompleteOrAmbiguousInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mutate func(*KeyInput)
	}{
		{
			name: "namespace",
			mutate: func(input *KeyInput) {
				input.Namespace = ""
			},
		},
		{
			name: "tool version",
			mutate: func(input *KeyInput) {
				input.ToolVersion = ""
			},
		},
		{
			name: "build Go version",
			mutate: func(input *KeyInput) {
				input.BuildGoVersion = ""
			},
		},
		{
			name: "source Go version",
			mutate: func(input *KeyInput) {
				input.SourceGoVersion = ""
			},
		},
		{
			name: "configuration",
			mutate: func(input *KeyInput) {
				input.Configuration = Digest{}
			},
		},
		{
			name: "GOOS",
			mutate: func(input *KeyInput) {
				input.GOOS = ""
			},
		},
		{
			name: "GOARCH",
			mutate: func(input *KeyInput) {
				input.GOARCH = ""
			},
		},
		{
			name: "formatter mode",
			mutate: func(input *KeyInput) {
				input.FormatterMode = ""
			},
		},
		{
			name: "components",
			mutate: func(input *KeyInput) {
				input.Components = nil
			},
		},
		{
			name: "component kind",
			mutate: func(input *KeyInput) {
				input.Components[0].Kind = "unknown"
			},
		},
		{
			name: "component identity",
			mutate: func(input *KeyInput) {
				input.Components[0].Identity = ""
			},
		},
		{
			name: "component digest",
			mutate: func(input *KeyInput) {
				input.Components[0].Digest = Digest{}
			},
		},
		{
			name: "duplicate component",
			mutate: func(input *KeyInput) {
				input.Components = append(input.Components, input.Components[0])
			},
		},
		{
			name: "rule ID",
			mutate: func(input *KeyInput) {
				input.Rules[0].ID = ""
			},
		},
		{
			name: "rule severity",
			mutate: func(input *KeyInput) {
				input.Rules[0].Severity = ""
			},
		},
		{
			name: "rule options",
			mutate: func(input *KeyInput) {
				input.Rules[0].Options = Digest{}
			},
		},
		{
			name: "duplicate rule",
			mutate: func(input *KeyInput) {
				input.Rules = append(input.Rules, input.Rules[0])
			},
		},
		{
			name: "build tag",
			mutate: func(input *KeyInput) {
				input.BuildTags = append(input.BuildTags, "")
			},
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				input := cloneKeyInput(testKeyInput())
				test.mutate(&input)
				if _, err := BuildKey(input); err == nil {
					t.Fatalf("BuildKey() accepted invalid %s", test.name)
				}
			},
		)
	}
}

func testKeyInput() KeyInput {
	return KeyInput{
		Namespace: "typed-analysis",
		ToolVersion: "v0.1.0",
		BuildGoVersion: "go1.26.5",
		SourceGoVersion: "go1.26",
		Configuration: DigestOf([]byte("configuration")),
		BuildTags: []string{"integration", "linux"},
		GOOS: "linux",
		GOARCH: "amd64",
		CGOEnabled: true,
		FormatterMode: "glippy-v1",
		Rules: []RuleInput{
			{ID: "nilness", Severity: "warn", Options: DigestOf(nil)},
			{ID: "printf", Severity: "error", Options: DigestOf([]byte("strict"))},
		},
		Components: []Component{
			{
				Kind: ComponentSource,
				Identity: "/project/a.go",
				Digest: DigestOf([]byte("source")),
			},
			{
				Kind: ComponentModule,
				Identity: "/project/go.mod",
				Digest: DigestOf([]byte("module")),
			},
			{
				Kind: ComponentWorkspace,
				Identity: "/project/go.work",
				Digest: DigestOf([]byte("workspace")),
			},
			{
				Kind: ComponentOverlay,
				Identity: "/project/a.go",
				Digest: DigestOf([]byte("overlay")),
			},
			{
				Kind: ComponentBuildSelection,
				Identity: "package-patterns",
				Digest: DigestOf([]byte("./...;tests=true;mod=readonly")),
			},
			{
				Kind: ComponentEnvironment,
				Identity: "GOAMD64",
				Digest: DigestOf([]byte("v3")),
			},
			{
				Kind: ComponentDependencyExport,
				Identity: "example.com/dep",
				Digest: DigestOf([]byte("export")),
			},
			{
				Kind: ComponentFact,
				Identity: "example.com/dep:nilness",
				Digest: DigestOf([]byte("fact")),
			},
		},
	}
}

func cloneKeyInput(input KeyInput) KeyInput {
	input.BuildTags = slices.Clone(input.BuildTags)
	input.Rules = slices.Clone(input.Rules)
	input.Components = slices.Clone(input.Components)
	return input
}
