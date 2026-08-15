package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/filesystem"
	"github.com/faustbrian/glippy/internal/goversion"
	"github.com/faustbrian/glippy/internal/rules"
)

const initialConfiguration = `version = 1

[format]
line-width = 100
tab-width = 8

[lint]
presets = ["correctness"]
warnings-as-errors = false
`

type configInvocation struct {
	action string
	path string
	configPath string
}

func runInit(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) < 1 || len(arguments) > 2 || arguments[0] != "init" {
		return report(stderr, ExitInvalidInvocation, initUsage)
	}
	if ctx == nil {
		return report(stderr, ExitInternalError, "glippy init: context is required\n")
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "glippy init: %v\n", err)
	}
	directory := "."
	if len(arguments) == 2 {
		directory = arguments[1]
		if directory == "" || strings.HasPrefix(directory, "--") {
			return report(stderr, ExitInvalidInvocation, initUsage)
		}
	}
	absoluteDirectory, err := filepath.Abs(directory)
	if err != nil {
		return report(
			stderr,
			ExitFilesystemError,
			"glippy init: resolve directory: %v\n",
			err,
		)
	}
	info, err := os.Stat(absoluteDirectory)
	if err != nil {
		return report(
			stderr,
			ExitFilesystemError,
			"glippy init: inspect directory: %v\n",
			err,
		)
	}
	if !info.IsDir() {
		return report(
			stderr,
			ExitInvalidInvocation,
			"glippy init: %q is not a directory\n",
			absoluteDirectory,
		)
	}
	configurationPath := filepath.Join(absoluteDirectory, config.Filename)
	if err := filesystem.CreateWithin(
		absoluteDirectory,
		configurationPath,
		[]byte(initialConfiguration),
		0o600,
	);
		err != nil {
		if errors.Is(err, filesystem.ErrStale) {
			return report(
				stderr,
				ExitConflict,
				"glippy init: configuration %q already exists\n",
				configurationPath,
			)
		}
		return report(stderr, ExitFilesystemError, "glippy init: %v\n", err)
	}
	if err := ctx.Err(); err != nil {
		return report(
			stderr,
			ExitCanceled,
			"glippy init: created %s before cancellation: %v\n",
			configurationPath,
			err,
		)
	}
	if err := write(stdout, []byte("glippy init: created " + configurationPath + "\n"));
		err != nil {
		return report(
			stderr,
			ExitFilesystemError,
			"glippy init: created %s; write standard output: %v\n",
			configurationPath,
			err,
		)
	}
	return ExitSuccess
}

func runConfig(
	ctx context.Context,
	arguments []string,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	invocation, valid := parseConfigInvocation(arguments)
	if !valid {
		return report(stderr, ExitInvalidInvocation, configUsage)
	}
	if ctx == nil {
		return report(stderr, ExitInternalError, "glippy config: context is required\n")
	}
	if registry == nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy config: rule registry is required\n",
		)
	}
	if err := ctx.Err(); err != nil {
		return report(
			stderr,
			ExitCanceled,
			"glippy config %s: %v\n",
			invocation.action,
			err,
		)
	}
	selection, err := config.Discover(invocation.path, invocation.configPath)
	if err != nil {
		return report(
			stderr,
			configurationErrorExitCode(err),
			"glippy config %s: %v\n",
			invocation.action,
			err,
		)
	}
	loaded, err := config.Load(
		selection,
		config.ParseOptions{
			KnownRules: registry.IDs(),
			RuleOptions: registry.OptionSchemas(),
		},
	)
	if err != nil {
		return report(
			stderr,
			configurationErrorExitCode(err),
			"glippy config %s: %v\n",
			invocation.action,
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return report(
			stderr,
			ExitCanceled,
			"glippy config %s: %v\n",
			invocation.action,
			err,
		)
	}
	language, err := goversion.Resolve(invocation.path, selection.Root)
	if err != nil {
		exitCode := ExitSourceError
		if goversion.IsFilesystemError(err) {
			exitCode = ExitFilesystemError
		}
		return report(stderr, exitCode, "glippy config %s: %v\n", invocation.action, err)
	}
	executionConfiguration := loaded
	executionConfiguration.Lint = loaded.LintForExecution()
	_, err = resolveConfiguredRules(executionConfiguration, language.Language, registry)
	if err != nil {
		return report(
			stderr,
			ExitInvalidInvocation,
			"glippy config %s: %v\n",
			invocation.action,
			err,
		)
	}
	if invocation.action == "check" {
		label := "built-in defaults"
		if selection.Path != "" {
			label = selection.Path + " (" + configurationOrigin(selection) + ")"
		}
		if err := write(stdout, []byte("configuration valid: " + label + "\n"));
			err != nil {
			return report(
				stderr,
				ExitFilesystemError,
				"glippy config check: write standard output: %v\n",
				err,
			)
		}
		return ExitSuccess
	}
	effective, matches, relativePath, err := effectiveConfigurationForInput(
		invocation.path,
		selection,
		loaded,
	)
	if err != nil {
		return report(stderr, ExitInvalidInvocation, "glippy config show: %v\n", err)
	}
	resolved, err := resolveConfiguredRules(effective, language.Language, registry)
	if err != nil {
		return report(stderr, ExitInvalidInvocation, "glippy config show: %v\n", err)
	}
	output, err := renderEffectiveConfiguration(
		invocation.path,
		selection,
		effective,
		language,
		resolved,
		registry,
		len(loaded.Lint.Overrides),
		matches,
		relativePath,
	)
	if err != nil {
		return report(stderr, ExitInternalError, "glippy config show: %v\n", err)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "glippy config show: %v\n", err)
	}
	if err := write(stdout, output); err != nil {
		return report(
			stderr,
			ExitFilesystemError,
			"glippy config show: write standard output: %v\n",
			err,
		)
	}
	return ExitSuccess
}

func parseConfigInvocation(arguments []string) (configInvocation, bool) {
	if len(arguments) < 2 ||
		arguments[0] != "config" ||
		(arguments[1] != "check" && arguments[1] != "show") {
		return configInvocation{}, false
	}
	result := configInvocation{action: arguments[1], path: "."}
	pathSet := false
	for index := 2; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case strings.HasPrefix(argument, "--config=") && result.configPath == "":
			result.configPath = strings.TrimPrefix(argument, "--config=")
			if result.configPath == "" {
				return configInvocation{}, false
			}
		case argument == "--config" &&
			result.configPath == "" &&
			index + 1 < len(arguments):
			index++
			result.configPath = arguments[index]
			if result.configPath == "" || strings.HasPrefix(result.configPath, "--") {
				return configInvocation{}, false
			}
		case strings.HasPrefix(argument, "--") || pathSet:
			return configInvocation{}, false
		default:
			result.path = argument
			pathSet = true
		}
	}
	return result, true
}

func renderEffectiveConfiguration(
	inputPath string,
	selection config.Selection,
	loaded config.Config,
	language goversion.Selection,
	resolved []rules.Selection,
	registry *rules.Registry,
	configuredOverrides int,
	matchedOverrides []int,
	relativePath string,
) ([]byte, error) {
	var output bytes.Buffer
	root := selection.Root
	if root == "" {
		root = "unset"
	}
	fmt.Fprintf(&output, "project root: %s\n", root)
	if selection.Path == "" {
		output.WriteString("configuration: built-in defaults\n")
	} else {
		fmt.Fprintf(
			&output,
			"configuration: %s (%s)\n",
			selection.Path,
			configurationOrigin(selection),
		)
	}
	if language.Path == "" {
		fmt.Fprintf(&output, "source language: %s (default)\n", language.Language)
	} else {
		fmt.Fprintf(&output, "source language: %s (%s)\n", language.Language, language.Path)
	}
	output.WriteString("migration target: unset\n")
	fmt.Fprintf(
		&output,
		"format: line-width=%d tab-width=%d\n",
		loaded.Format.LineWidth,
		loaded.Format.TabWidth,
	)
	presets := make([]string, len(loaded.Lint.Presets))
	for index, preset := range loaded.Lint.Presets {
		presets[index] = string(preset)
	}
	fmt.Fprintf(&output, "presets: %s\n", strings.Join(presets, ","))
	fmt.Fprintf(&output, "warnings-as-errors: %t\n", loaded.Lint.WarningsAsErrors)
	fmt.Fprintf(
		&output,
		"path overrides: configured=%d matched=%s path=%s\n",
		configuredOverrides,
		formatOverrideMatches(matchedOverrides),
		relativePath,
	)
	maximumRequirement := rules.RequireLexical
	generatedEligible := 0
	typeErrorEligible := 0
	for _, selected := range resolved {
		metadata, found := registry.Metadata(selected.ID)
		if !found {
			return nil, fmt.Errorf("resolved rule %q is missing metadata", selected.ID)
		}
		maximumRequirement = max(maximumRequirement, selected.Requirement)
		if metadata.RunOnGenerated {
			generatedEligible++
		}
		if metadata.RunDespiteTypeErrors {
			typeErrorEligible++
		}
		fmt.Fprintf(
			&output,
			"rule %s: %s (%s)\n",
			selected.ID,
			selected.Severity,
			ruleEnablementReason(selected.ID, metadata, loaded, matchedOverrides),
		)
		for _, option := range metadata.Options {
			value, found := configuredOptionValue(selected.Options, option)
			if found {
				fmt.Fprintf(&output, "  option %s=%s\n", option.Name, value)
			}
		}
	}
	fmt.Fprintf(&output, "maximum analysis tier: %s\n", maximumRequirement)
	fmt.Fprintf(
		&output,
		"generated files: readable; writes refused; enabled rules eligible=%d/%d\n",
		generatedEligible,
		len(resolved),
	)
	fmt.Fprintf(
		&output,
		"type errors: enabled rules eligible=%d/%d\n",
		typeErrorEligible,
		len(resolved),
	)
	output.WriteString("test files: included through package selection\n")
	output.WriteString("testdata and fixtures: excluded unless explicitly selected\n")
	output.WriteString("vendor: excluded from recursive discovery\n")
	buildTags := strings.Join(loaded.Analysis.BuildTags, ",")
	if buildTags == "" {
		buildTags = "none"
	}
	fmt.Fprintf(
		&output,
		"analysis: goos=%s goarch=%s cgo=%t build-tags=%s\n",
		loaded.Analysis.GOOS,
		loaded.Analysis.GOARCH,
		loaded.Analysis.CGOEnabled,
		buildTags,
	)
	fmt.Fprintf(&output, "baseline: %s\n", baselineDescription(inputPath, selection, loaded))
	expiry := loaded.Lint.Suppressions.ExpiryCutoff
	if expiry == "" {
		expiry = "unset"
	}
	fmt.Fprintf(
		&output,
		"suppressions: require-reason=%t expiry-cutoff=%s\n",
		loaded.Lint.Suppressions.RequireReason,
		expiry,
	)
	fmt.Fprintf(
		&output,
		"cache: enabled=%t max-entries=%d max-bytes=%d\n",
		loaded.Cache.Enabled,
		loaded.Cache.MaxEntries,
		loaded.Cache.MaxBytes,
	)
	return output.Bytes(), nil
}

func resolveConfiguredRules(
	loaded config.Config,
	sourceGoVersion string,
	registry *rules.Registry,
) ([]rules.Selection, error) {
	return registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: loaded.Lint.Presets,
			Overrides: loaded.Lint.Rules,
			RuleOptions: loaded.Lint.RuleOptions,
			SourceGoVersion: sourceGoVersion,
			WarningsAsErrors: loaded.Lint.WarningsAsErrors,
		},
	)
}

func configurationOrigin(selection config.Selection) string {
	if selection.Explicit {
		return "explicit"
	}
	return "discovered"
}

func ruleEnablementReason(
	id string,
	metadata rules.Metadata,
	loaded config.Config,
	matchedOverrides []int,
) string {
	reasons := make([]string, 0, len(loaded.Lint.Presets) + 1)
	for index := len(matchedOverrides) - 1; index >= 0; index-- {
		number := matchedOverrides[index]
		if number <= 0 || number > len(loaded.Lint.Overrides) {
			continue
		}
		severity, explicit := loaded.Lint.Overrides[number - 1].Rules[id]
		if !explicit {
			continue
		}
		reasons = append(reasons, fmt.Sprintf("path override %d", number))
		if loaded.Lint.WarningsAsErrors && severity == rules.SeverityWarn {
			reasons = append(reasons, "warnings-as-errors")
		}
		return strings.Join(reasons, "; ")
	}
	if severity, explicit := loaded.Lint.Rules[id]; explicit {
		reasons = append(reasons, "explicit override")
		if loaded.Lint.WarningsAsErrors && severity == rules.SeverityWarn {
			reasons = append(reasons, "warnings-as-errors")
		}
		return strings.Join(reasons, "; ")
	}
	configured := make(map[rules.Preset]struct{}, len(loaded.Lint.Presets))
	for _, preset := range loaded.Lint.Presets {
		configured[preset] = struct{}{}
	}
	for _, preset := range metadata.Presets {
		if _, found := configured[preset]; found {
			reasons = append(reasons, "preset " + string(preset))
		}
	}
	if loaded.Lint.WarningsAsErrors && metadata.DefaultSeverity == rules.SeverityWarn {
		reasons = append(reasons, "warnings-as-errors")
	}
	return strings.Join(reasons, "; ")
}

func effectiveConfigurationForInput(
	inputPath string,
	selection config.Selection,
	loaded config.Config,
) (config.Config, []int, string, error) {
	absoluteInput, err := filepath.Abs(inputPath)
	if err != nil {
		return config.Config{}, nil, "", fmt.Errorf(
			"resolve input path %q: %w",
			inputPath,
			err,
		)
	}
	root := selection.Root
	if root == "" && selection.Path != "" {
		root = filepath.Dir(selection.Path)
	}
	if root == "" {
		root = filepath.Dir(absoluteInput)
	}
	relative, err := filepath.Rel(root, absoluteInput)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".." + string(filepath.Separator)) {
		return config.Config{}, nil, "", fmt.Errorf(
			"input path %q is outside path-override root %q",
			inputPath,
			root,
		)
	}
	portable := filepath.ToSlash(relative)
	info, err := os.Stat(absoluteInput)
	if err != nil {
		return config.Config{}, nil, "", fmt.Errorf(
			"inspect input path %q: %w",
			inputPath,
			err,
		)
	}
	if info.IsDir() || len(loaded.Lint.Overrides) == 0 {
		return loaded, nil, portable, nil
	}
	lint, matches, err := loaded.LintForPath(portable)
	if err != nil {
		return config.Config{}, nil, "", err
	}
	effective := loaded
	effective.Lint = lint
	return effective, matches, portable, nil
}

func formatOverrideMatches(matches []int) string {
	if len(matches) == 0 {
		return "none"
	}
	values := make([]string, len(matches))
	for index, match := range matches {
		values[index] = fmt.Sprintf("%d", match)
	}
	return strings.Join(values, ",")
}

func configuredOptionValue(set rules.OptionSet, metadata rules.OptionMetadata) (string, bool) {
	switch metadata.Kind {
	case rules.OptionBoolean:
		value, found := set.Boolean(metadata.Name)
		return fmt.Sprintf("%t", value), found
	case rules.OptionInteger:
		value, found := set.Integer(metadata.Name)
		return fmt.Sprintf("%d", value), found
	case rules.OptionString:
		value, found := set.String(metadata.Name)
		return fmt.Sprintf("%q", value), found
	case rules.OptionStrings:
		value, found := set.Strings(metadata.Name)
		if !found {
			return "", false
		}
		quoted := make([]string, len(value))
		for index, item := range value {
			quoted[index] = fmt.Sprintf("%q", item)
		}
		return "[" + strings.Join(quoted, ", ") + "]", true
	default:
		return "", false
	}
}

func baselineDescription(
	inputPath string,
	selection config.Selection,
	loaded config.Config,
) string {
	policy := loaded.Lint.Baseline
	if policy.Path == "" {
		return "unset"
	}
	root := selection.Root
	if root == "" {
		absoluteInput, err := filepath.Abs(inputPath)
		if err != nil {
			return policy.Path + " (unresolved)"
		}
		info, err := os.Stat(absoluteInput)
		if err == nil && info.IsDir() {
			root = absoluteInput
		} else {
			root = filepath.Dir(absoluteInput)
		}
	}
	status := "missing"
	if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(policy.Path)));
		err == nil && info.Mode().IsRegular() {
		status = "present"
	} else if err != nil && !os.IsNotExist(err) {
		status = "unreadable"
	}
	expiry := policy.ExpiryCutoff
	if expiry == "" {
		expiry = "unset"
	}
	return fmt.Sprintf(
		"%s (%s; report-stale=%t; expiry-cutoff=%s)",
		policy.Path,
		status,
		policy.ReportStale,
		expiry,
	)
}
