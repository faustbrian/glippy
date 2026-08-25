package corpus

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

const (
	ResultSchemaVersion = 3
	corpusCommandMemoryLimit = "4GiB"
	maximumCommandOutput = 256 << 20
	maximumModuleDownloadArguments = 128
	maximumModuleDownloadArgumentBytes = 32 << 10
)

var (
	corpusProfiles = []string{"default", "recommended", "strict", "pedantic"}
	staticcheckOutputVersionPattern = regexp.MustCompile(`\((v?[0-9]+\.[0-9]+\.[0-9]+)\)`)
	runIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// Command is one bounded external corpus operation.
type Command struct {
	Path string
	Args []string
	Dir string
	Env []string
}

// CommandResult retains the complete bounded process output and exit category.
type CommandResult struct {
	Stdout []byte
	Stderr []byte
	ExitCode int
}

// Executor runs one corpus command. Tests replace it with an inert executor.
type Executor interface {
	Run(context.Context, Command) (CommandResult, error)
}

// RunOptions binds pre-existing clean checkouts to task-owned output and cache roots.
type RunOptions struct {
	RunID string
	CheckoutRoot string
	OutputRoot string
	CacheRoot string
	GlippyPath string
	StaticcheckPath string
	RepositoryIDs []string
	Environment []string
	Executor Executor
	downloadEnvironment []string
}

type repositoryResult struct {
	SchemaVersion int `json:"schema_version"`
	RunID string `json:"run_id"`
	Repository Repository `json:"repository"`
	StaticcheckVersion string `json:"staticcheck_version"`
	Tools toolVersions `json:"tools"`
	Format formatResult `json:"format"`
	FixPreview fixPreviewResult `json:"fix_preview"`
	Profiles []profileResult `json:"profiles"`
	Comparators []comparatorResult `json:"comparators"`
}

type formatResult struct {
	ExitCode int `json:"exit_code"`
	Report artifactResult `json:"report"`
	FileCount int `json:"file_count"`
	DifferenceCount int `json:"difference_count"`
	Complete bool `json:"complete"`
}

type fixPreviewResult struct {
	ExitCode int `json:"exit_code"`
	Output artifactResult `json:"output"`
	Complete bool `json:"complete"`
}

type toolVersions struct {
	Glippy string `json:"glippy"`
	Go string `json:"go"`
	Staticcheck string `json:"staticcheck"`
}

type profileResult struct {
	Profile string `json:"profile"`
	ExitCode int `json:"exit_code"`
	Diagnostics artifactResult `json:"diagnostics"`
	Statistics measurementArtifactResult `json:"statistics"`
	Findings artifactResult `json:"findings"`
	DiagnosticCount int `json:"diagnostic_count"`
	Complete bool `json:"complete"`
}

type comparatorResult struct {
	Name string `json:"name"`
	ExitCode int `json:"exit_code"`
	Output artifactResult `json:"output"`
}

type comparatorCommand struct {
	name string
	path string
	arguments []string
}

type artifactResult struct {
	File string `json:"file"`
	SHA256 string `json:"sha256"`
	ValidJSON bool `json:"valid_json,omitempty"`
}

type measurementArtifactResult struct {
	File string `json:"file"`
	ValidJSON bool `json:"valid_json,omitempty"`
}

type pathNormalization struct {
	Source string
	Replacement string
}

type artifactNormalizer struct {
	SourceRoot string
	Paths []pathNormalization
}

type findingInventory struct {
	SchemaVersion int `json:"schema_version"`
	Repository string `json:"repository"`
	Revision string `json:"revision"`
	Profile string `json:"profile"`
	Diagnostics []finding `json:"diagnostics"`
}

type finding struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	RuleID string `json:"rule_id"`
	Severity string `json:"severity"`
	MessageKey string `json:"message_key"`
	Message string `json:"message"`
	Path string `json:"path"`
	Range findingRange `json:"range"`
}

type findingRange struct {
	Start int `json:"start"`
	End int `json:"end"`
}

type normalizedLintResult struct {
	Outcome struct {
		Category string `json:"category"`
		ExitCode int `json:"exit_code"`
	} `json:"outcome"`
	Summary struct {
		Diagnostics int `json:"diagnostics"`
		PackageDiagnostics int `json:"package_diagnostics"`
		SourceProblems int `json:"source_problems"`
		Complete bool `json:"complete"`
	} `json:"summary"`
	Diagnostics []finding `json:"diagnostics"`
	PackageDiagnostics []json.RawMessage `json:"package_diagnostics"`
	SourceProblems []json.RawMessage `json:"source_problems"`
}

type normalizedFormatResult struct {
	SchemaVersion int `json:"schema_version"`
	Command string `json:"command"`
	Mode string `json:"mode"`
	Outcome struct {
		ExitCode int `json:"exit_code"`
	} `json:"outcome"`
	Summary struct {
		Files int `json:"files"`
		Changed int `json:"changed"`
		Complete bool `json:"complete"`
	} `json:"summary"`
}

// Run validates and audits every selected pinned checkout without writing to it.
func Run(ctx context.Context, manifest Manifest, options RunOptions) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	resolved, err := resolveRunOptions(options)
	if err != nil {
		return err
	}
	repositories, err := selectRepositories(manifest, resolved.RepositoryIDs)
	if err != nil {
		return err
	}
	if err := prepareOutputRoot(resolved.OutputRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(resolved.CacheRoot, 0o755); err != nil {
		return fmt.Errorf("create corpus cache root: %w", err)
	}
	resolved.downloadEnvironment = slices.Clone(resolved.Environment)
	resolved.Environment, err = isolatedEnvironment(resolved, "tools", false, "off")
	if err != nil {
		return err
	}
	versions, err := inspectToolVersions(ctx, resolved)
	if err != nil {
		return err
	}
	if staticcheckModuleVersion(versions.Staticcheck) != manifest.StaticcheckVersion {
		return fmt.Errorf(
			"Staticcheck version output %q does not identify %s",
			versions.Staticcheck,
			manifest.StaticcheckVersion,
		)
	}
	for _, repository := range repositories {
		if err := runRepository(ctx, manifest, repository, versions, resolved); err != nil {
			return err
		}
	}
	return nil
}

func runRepository(
	ctx context.Context,
	manifest Manifest,
	repository Repository,
	versions toolVersions,
	options RunOptions,
) (runErr error) {
	checkout := filepath.Join(options.CheckoutRoot, repository.ID)
	if err := validateCheckoutFiles(checkout, repository); err != nil {
		return err
	}
	if err := verifyGitCheckout(
		ctx,
		options.Executor,
		checkout,
		repository,
		options.Environment,
	);
		err != nil {
		return err
	}
	defer func() {
		if err := verifyGitCheckout(
			context.WithoutCancel(ctx),
			options.Executor,
			checkout,
			repository,
			options.Environment,
		);
			err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("post-run checkout verification: %w", err),
			)
		}
	}()
	executionRoot, err := os.MkdirTemp(options.CacheRoot, repository.ID + "-source-")
	if err != nil {
		return fmt.Errorf("create execution root for %q: %w", repository.ID, err)
	}
	defer func() {
		if err := removeReadOnlyTree(executionRoot); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("remove execution root: %w", err))
		}
	}()
	executionCheckout := filepath.Join(executionRoot, "source")
	if err := copyReadOnlyCheckout(checkout, executionCheckout); err != nil {
		return fmt.Errorf(
			"create read-only checkout snapshot for %q: %w",
			repository.ID,
			err,
		)
	}
	workspaceSumMutable, err := permitWorkspaceSumUpdate(executionCheckout)
	if err != nil {
		return fmt.Errorf("prepare workspace sum for %q: %w", repository.ID, err)
	}
	if err := prefetchRepositoryModules(ctx, options, repository, executionCheckout);
		err != nil {
		return err
	}
	if err := validateModuleGraphSnapshot(checkout, executionCheckout, workspaceSumMutable);
		err != nil {
		return fmt.Errorf(
			"module graph changed checkout snapshot outside go.work.sum for %q: %w",
			repository.ID,
			err,
		)
	}
	if err := makeTreeReadOnly(executionCheckout); err != nil {
		return fmt.Errorf(
			"restore read-only checkout snapshot for %q: %w",
			repository.ID,
			err,
		)
	}
	repositoryOutput := filepath.Join(options.OutputRoot, repository.ID)
	if err := os.MkdirAll(repositoryOutput, 0o755); err != nil {
		return fmt.Errorf("create output for %q: %w", repository.ID, err)
	}
	configurationRoot, err := os.MkdirTemp(options.CacheRoot, repository.ID + "-configs-")
	if err != nil {
		return fmt.Errorf("create configuration root for %q: %w", repository.ID, err)
	}
	defer os.RemoveAll(configurationRoot)
	environment, err := repositoryEnvironment(options, repository, executionCheckout)
	if err != nil {
		return err
	}
	preflightEnvironment, err := repositoryEnvironmentForScope(
		options,
		repository,
		executionCheckout,
		filepath.Join("repositories", repository.ID, "preflight"),
	)
	if err != nil {
		return err
	}
	formatEnvironment, err := repositoryEnvironmentForScope(
		options,
		repository,
		executionCheckout,
		filepath.Join("repositories", repository.ID, "format"),
	)
	if err != nil {
		return err
	}
	fixPreviewEnvironment, err := repositoryEnvironmentForScope(
		options,
		repository,
		executionCheckout,
		filepath.Join("repositories", repository.ID, "fix-preview"),
	)
	if err != nil {
		return err
	}
	formatConfigurationPath := filepath.Join(configurationRoot, "format.toml")
	if err := os.WriteFile(formatConfigurationPath, []byte("version = 1\n"), 0o600);
		err != nil {
		return fmt.Errorf("write formatter audit configuration: %w", err)
	}
	formatAudit, err := runFormatterAudit(
		ctx,
		options,
		executionCheckout,
		repositoryOutput,
		formatConfigurationPath,
		formatEnvironment,
	)
	if err != nil {
		return fmt.Errorf("run formatter audit for %q: %w", repository.ID, err)
	}
	if workspaceSumMutable {
		if err := permitExistingWorkspaceSumUpdate(executionCheckout); err != nil {
			return fmt.Errorf(
				"prepare workspace sum for offline preflight of %q: %w",
				repository.ID,
				err,
			)
		}
	}
	preflight, preflightErr := runComparator(
		ctx,
		options,
		repository,
		executionCheckout,
		repositoryOutput,
		preflightEnvironment,
		analysisPreflightCommand(repository),
	)
	var preflightSnapshotErr error
	if err := validateModuleGraphSnapshot(checkout, executionCheckout, workspaceSumMutable);
		err != nil {
		preflightSnapshotErr = fmt.Errorf(
			"offline preflight changed checkout snapshot outside go.work.sum for %q: %w",
			repository.ID,
			err,
		)
	}
	var preflightLockErr error
	if err := makeTreeReadOnly(executionCheckout); err != nil {
		preflightLockErr = fmt.Errorf(
			"restore read-only checkout snapshot after preflight for %q: %w",
			repository.ID,
			err,
		)
	}
	if err := errors.Join(preflightErr, preflightSnapshotErr, preflightLockErr); err != nil {
		return err
	}
	result := repositoryResult{
		SchemaVersion: ResultSchemaVersion,
		RunID: options.RunID,
		Repository: repository,
		StaticcheckVersion: manifest.StaticcheckVersion,
		Tools: versions,
		Format: formatAudit,
		Profiles: make([]profileResult, 0, len(corpusProfiles)),
		Comparators: []comparatorResult{preflight},
	}
	if preflight.ExitCode != 0 {
		fixPreview, writeErr := writeSkippedFixPreview(repositoryOutput, preflight.ExitCode)
		if writeErr != nil {
			return writeErr
		}
		result.FixPreview = fixPreview
		for _, profile := range corpusProfiles {
			profileResult, writeErr := writeIncompleteProfile(
				repository,
				repositoryOutput,
				profile,
				preflight.ExitCode,
			)
			if writeErr != nil {
				return writeErr
			}
			result.Profiles = append(result.Profiles, profileResult)
		}
		for _, name := range []string{"go-vet", "staticcheck"} {
			comparator, writeErr := writeSkippedComparator(
				repositoryOutput,
				name,
				preflight.ExitCode,
			)
			if writeErr != nil {
				return writeErr
			}
			result.Comparators = append(result.Comparators, comparator)
		}
		_, writeErr = writeJSON(filepath.Join(repositoryOutput, "result.json"), result)
		return writeErr
	}
	fixPreviewBaseline, err := checkoutSnapshotInventory(executionCheckout, false)
	if err != nil {
		return fmt.Errorf("capture pre-preview snapshot for %q: %w", repository.ID, err)
	}
	fixPreviewConfigurationPath := filepath.Join(configurationRoot, "fix-preview.toml")
	if err := os.WriteFile(
		fixPreviewConfigurationPath,
		[]byte(
			"version = 1\n\n[lint]\nprofile = \"recommended\"\n\n" +
				"[cache]\nenabled = false\n",
		),
		0o600,
	);
		err != nil {
		return fmt.Errorf("write safe-fix preview configuration: %w", err)
	}
	fixPreview, err := runSafeFixPreview(
		ctx,
		options,
		repository,
		executionCheckout,
		repositoryOutput,
		fixPreviewConfigurationPath,
		fixPreviewEnvironment,
	)
	if err != nil {
		return fmt.Errorf("run safe-fix preview for %q: %w", repository.ID, err)
	}
	result.FixPreview = fixPreview
	if err := validateExactCheckoutSnapshot(executionCheckout, fixPreviewBaseline); err != nil {
		return fmt.Errorf(
			"safe-fix preview changed checkout snapshot for %q: %w",
			repository.ID,
			err,
		)
	}
	for _, profile := range corpusProfiles {
		profileResult, err := runProfile(
			ctx,
			options,
			repository,
			executionCheckout,
			repositoryOutput,
			configurationRoot,
			environment,
			profile,
		)
		if err != nil {
			return err
		}
		result.Profiles = append(result.Profiles, profileResult)
	}
	comparators, err := runComparators(
		ctx,
		options,
		repository,
		executionCheckout,
		repositoryOutput,
		environment,
	)
	if err != nil {
		return err
	}
	result.Comparators = append(result.Comparators, comparators...)
	if _, err := writeJSON(filepath.Join(repositoryOutput, "result.json"), result); err != nil {
		return err
	}
	return nil
}

func runSafeFixPreview(
	ctx context.Context,
	options RunOptions,
	repository Repository,
	checkout, repositoryOutput, configurationPath string,
	environment []string,
) (fixPreviewResult, error) {
	arguments := []string{"lint", "--fix", "--diff", "--config", configurationPath}
	arguments = append(arguments, repository.Patterns...)
	execution, err := options.Executor.Run(
		ctx,
		Command{Path: options.GlippyPath, Args: arguments, Dir: checkout, Env: environment},
	)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fixPreviewResult{}, ctxErr
	}
	if err != nil {
		artifact, writeErr := writeArtifact(
			filepath.Join(repositoryOutput, "fix-preview.txt"),
			normalizeText(
				[]byte(fmt.Sprintf("safe-fix preview execution failed: %v\n", err)),
				newArtifactNormalizer(checkout, options),
			),
			false,
		)
		if writeErr != nil {
			return fixPreviewResult{}, writeErr
		}
		return fixPreviewResult{ExitCode: -1, Output: artifact}, nil
	}
	output := slices.Clone(execution.Stdout)
	if len(execution.Stderr) != 0 {
		if len(output) != 0 && output[len(output) - 1] != '\n' {
			output = append(output, '\n')
		}
		output = append(output, execution.Stderr...)
	}
	artifact, err := writeArtifact(
		filepath.Join(repositoryOutput, "fix-preview.txt"),
		normalizeText(output, newArtifactNormalizer(checkout, options)),
		false,
	)
	if err != nil {
		return fixPreviewResult{}, err
	}
	return fixPreviewResult{
		ExitCode: execution.ExitCode,
		Output: artifact,
		Complete: (execution.ExitCode == 0 || execution.ExitCode == 1) &&
			len(execution.Stderr) == 0,
	}, nil
}

func writeSkippedFixPreview(
	repositoryOutput string,
	preflightExitCode int,
) (fixPreviewResult, error) {
	output, err := writeArtifact(
		filepath.Join(repositoryOutput, "fix-preview.txt"),
		[]byte(
			fmt.Sprintf(
				"not run because analysis preflight exited with status %d\n",
				preflightExitCode,
			),
		),
		false,
	)
	if err != nil {
		return fixPreviewResult{}, err
	}
	return fixPreviewResult{ExitCode: preflightExitCode, Output: output}, nil
}

func runFormatterAudit(
	ctx context.Context,
	options RunOptions,
	checkout, repositoryOutput, configurationPath string,
	environment []string,
) (formatResult, error) {
	execution, err := options.Executor.Run(
		ctx,
		Command{
			Path: options.GlippyPath,
			Args: []string{
				"fmt",
				"--check",
				"--reporter=json",
				"--config",
				configurationPath,
				".",
			},
			Dir: checkout,
			Env: environment,
		},
	)
	if err != nil {
		return formatResult{}, err
	}
	reportInput := execution.Stdout
	if !json.Valid(reportInput) && len(execution.Stderr) != 0 {
		reportInput = append(slices.Clone(execution.Stdout), execution.Stderr...)
	}
	report, normalized, valid, err := writeNormalizedJSON(
		filepath.Join(repositoryOutput, "format.json"),
		reportInput,
		newArtifactNormalizer(checkout, options),
	)
	if err != nil {
		return formatResult{}, err
	}
	result := formatResult{ExitCode: execution.ExitCode, Report: report}
	if !valid {
		return result, nil
	}
	if err := validateJSONShape(normalized, formatterReportShape, "formatter report");
		err != nil {
		return writeInvalidFormatReport(repositoryOutput, normalized, execution.ExitCode)
	}
	var document normalizedFormatResult
	if err := json.Unmarshal(normalized, &document); err != nil {
		return formatResult{}, fmt.Errorf("decode normalized formatter report: %w", err)
	}
	if document.SchemaVersion != 1 ||
		document.Command != "fmt" ||
		document.Mode != "check" ||
		document.Outcome.ExitCode != execution.ExitCode ||
		document.Summary.Files < 0 ||
		document.Summary.Changed < 0 ||
		document.Summary.Changed > document.Summary.Files {
		return writeInvalidFormatReport(repositoryOutput, normalized, execution.ExitCode)
	}
	result.FileCount = document.Summary.Files
	result.DifferenceCount = document.Summary.Changed
	result.Complete = document.Summary.Complete
	return result, nil
}

func writeInvalidFormatReport(
	repositoryOutput string,
	input []byte,
	exitCode int,
) (formatResult, error) {
	if err := os.Remove(filepath.Join(repositoryOutput, "format.json")); err != nil {
		return formatResult{}, fmt.Errorf("remove invalid formatter JSON artifact: %w", err)
	}
	report, err := writeArtifact(filepath.Join(repositoryOutput, "format.txt"), input, false)
	if err != nil {
		return formatResult{}, err
	}
	return formatResult{ExitCode: exitCode, Report: report}, nil
}

func writeIncompleteProfile(
	repository Repository,
	repositoryOutput, profile string,
	preflightExitCode int,
) (profileResult, error) {
	profileDirectory := filepath.Join(repositoryOutput, profile)
	if err := os.MkdirAll(profileDirectory, 0o755); err != nil {
		return profileResult{}, fmt.Errorf("create %s profile output: %w", profile, err)
	}
	reason := []byte(
		fmt.Sprintf(
			"not run because analysis preflight exited with status %d\n",
			preflightExitCode,
		),
	)
	diagnostics, err := writeArtifact(
		filepath.Join(profileDirectory, "diagnostics.txt"),
		reason,
		false,
	)
	if err != nil {
		return profileResult{}, err
	}
	statistics, err := writeArtifact(
		filepath.Join(profileDirectory, "statistics.txt"),
		reason,
		false,
	)
	if err != nil {
		return profileResult{}, err
	}
	findings, err := writeJSON(
		filepath.Join(profileDirectory, "findings.json"),
		findingInventory{
			SchemaVersion: ResultSchemaVersion,
			Repository: repository.ID,
			Revision: repository.Revision,
			Profile: profile,
			Diagnostics: []finding{},
		},
	)
	if err != nil {
		return profileResult{}, err
	}
	return profileResult{
		Profile: profile,
		ExitCode: preflightExitCode,
		Diagnostics: diagnostics,
		Statistics: measurementArtifactResult{File: statistics.File},
		Findings: findings,
		DiagnosticCount: 0,
		Complete: false,
	}, nil
}

func writeSkippedComparator(
	repositoryOutput, name string,
	preflightExitCode int,
) (comparatorResult, error) {
	output, err := writeArtifact(
		filepath.Join(repositoryOutput, name + ".txt"),
		[]byte(
			fmt.Sprintf(
				"not run because analysis preflight exited with status %d\n",
				preflightExitCode,
			),
		),
		false,
	)
	if err != nil {
		return comparatorResult{}, err
	}
	return comparatorResult{Name: name, ExitCode: preflightExitCode, Output: output}, nil
}

func prefetchRepositoryModules(
	ctx context.Context,
	options RunOptions,
	repository Repository,
	checkout string,
) error {
	modules, err := repositoryModuleDirectories(checkout)
	if err != nil {
		return fmt.Errorf("discover modules for %q: %w", repository.ID, err)
	}
	if err := validateLocalModuleReplacements(checkout, modules); err != nil {
		return fmt.Errorf("validate modules for %q: %w", repository.ID, err)
	}
	environment, err := moduleDownloadEnvironment(options, repository)
	if err != nil {
		return err
	}
	graphRoots := modules
	graphWorkspace := "off"
	workspacePath := filepath.Join(checkout, "go.work")
	workspaceInfo, err := os.Stat(workspacePath)
	switch {
	case err == nil && !workspaceInfo.Mode().IsRegular():
		return fmt.Errorf(
			"resolve module graph for %q: workspace is not a regular file",
			repository.ID,
		)
	case err == nil:
		graphRoots = []string{checkout}
		graphWorkspace = workspacePath
	case !os.IsNotExist(err):
		return fmt.Errorf(
			"resolve module graph for %q: inspect workspace: %w",
			repository.ID,
			err,
		)
	}
	graphEnvironment := replaceEnvironment(
		environment,
		map[string]string{"GOWORK": graphWorkspace},
	)
	downloads := make(map[string]struct{})
	for _, module := range graphRoots {
		result, err := options.Executor.Run(
			ctx,
			Command{
				Path: "go",
				Args: []string{"list", "-mod=readonly", "-m", "-json", "all"},
				Dir: module,
				Env: graphEnvironment,
			},
		)
		if err != nil {
			return fmt.Errorf("resolve module graph for %q: %w", repository.ID, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf(
				"resolve module graph for %q: exit %d: %s",
				repository.ID,
				result.ExitCode,
				strings.TrimSpace(string(result.Stderr)),
			)
		}
		resolved, err := resolvedModuleDownloads(result.Stdout)
		if err != nil {
			return fmt.Errorf("resolve module graph for %q: %w", repository.ID, err)
		}
		for _, download := range resolved {
			downloads[download] = struct{}{}
		}
	}
	if len(downloads) == 0 {
		return nil
	}
	downloadDirectory, err := os.MkdirTemp(
		options.CacheRoot,
		repository.ID + "-module-download-",
	)
	if err != nil {
		return fmt.Errorf("create module download directory for %q: %w", repository.ID, err)
	}
	defer os.RemoveAll(downloadDirectory)
	if err := os.WriteFile(
		filepath.Join(downloadDirectory, "go.mod"),
		[]byte("module glippy.invalid/corpus-prefetch\n\ngo 1.25\n"),
		0o600,
	);
		err != nil {
		return fmt.Errorf("create module download metadata for %q: %w", repository.ID, err)
	}
	downloadEnvironment := replaceEnvironment(
		environment,
		map[string]string{"GOFLAGS": "", "GOWORK": "off"},
	)
	for _, arguments := range moduleDownloadBatches(downloads) {
		result, err := options.Executor.Run(
			ctx,
			Command{
				Path: "go",
				Args: append([]string{"mod", "download"}, arguments...),
				Dir: downloadDirectory,
				Env: downloadEnvironment,
			},
		)
		if err != nil {
			return fmt.Errorf("prefetch module graph for %q: %w", repository.ID, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf(
				"prefetch module graph for %q: exit %d: %s",
				repository.ID,
				result.ExitCode,
				strings.TrimSpace(string(result.Stderr)),
			)
		}
	}
	return nil
}

type listedModule struct {
	Path string
	Version string
	Main bool
	Replace *listedModule
}

func resolvedModuleDownloads(input []byte) ([]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(input))
	downloads := make(map[string]struct{})
	for {
		var listed listedModule
		if err := decoder.Decode(&listed); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode module graph: %w", err)
		}
		selected := &listed
		if listed.Replace != nil {
			selected = listed.Replace
		}
		if listed.Main || selected.Path == "" || selected.Version == "" {
			continue
		}
		downloads[selected.Path + "@" + selected.Version] = struct{}{}
	}
	result := make([]string, 0, len(downloads))
	for download := range downloads {
		result = append(result, download)
	}
	sort.Strings(result)
	return result, nil
}

func moduleDownloadBatches(downloads map[string]struct{}) [][]string {
	ordered := make([]string, 0, len(downloads))
	for download := range downloads {
		ordered = append(ordered, download)
	}
	sort.Strings(ordered)
	batches := make(
		[][]string,
		0,
		(len(ordered) + maximumModuleDownloadArguments - 1) /
			maximumModuleDownloadArguments,
	)
	for len(ordered) > 0 {
		count := 0
		bytes := 0
		for count < len(ordered) && count < maximumModuleDownloadArguments {
			argumentBytes := len(ordered[count]) + 1
			if count > 0 && bytes + argumentBytes > maximumModuleDownloadArgumentBytes {
				break
			}
			bytes += argumentBytes
			count++
		}
		batches = append(batches, slices.Clone(ordered[:count]))
		ordered = ordered[count:]
	}
	return batches
}

func repositoryModuleDirectories(checkout string) ([]string, error) {
	workspacePath := filepath.Join(checkout, "go.work")
	input, err := os.ReadFile(workspacePath)
	if os.IsNotExist(err) {
		return []string{checkout}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace: %w", err)
	}
	workspace, err := modfile.ParseWork(workspacePath, input, nil)
	if err != nil {
		return nil, fmt.Errorf("parse workspace: %w", err)
	}
	canonicalCheckout, err := canonicalAbsolute(checkout)
	if err != nil {
		return nil, fmt.Errorf("resolve checkout: %w", err)
	}
	if err := validateLocalReplacementTargets(
		canonicalCheckout,
		filepath.Dir(workspacePath),
		workspace.Replace,
		"workspace",
	);
		err != nil {
		return nil, err
	}
	modules := make([]string, 0, len(workspace.Use))
	seen := make(map[string]struct{}, len(workspace.Use))
	for _, use := range workspace.Use {
		module := filepath.FromSlash(use.Path)
		if !filepath.IsAbs(module) {
			module = filepath.Join(filepath.Dir(workspacePath), module)
		}
		module, err = canonicalAbsolute(module)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace module %q: %w", use.Path, err)
		}
		if !pathAtOrWithin(canonicalCheckout, module) {
			return nil, fmt.Errorf(
				"workspace module %q is outside the checkout",
				use.Path,
			)
		}
		info, statErr := os.Lstat(filepath.Join(module, "go.mod"))
		if statErr != nil {
			return nil, fmt.Errorf("inspect workspace module %q: %w", use.Path, statErr)
		}
		if !info.Mode().IsRegular() || info.Mode() & os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"workspace module %q has no regular go.mod",
				use.Path,
			)
		}
		if _, duplicate := seen[module]; duplicate {
			continue
		}
		seen[module] = struct{}{}
		modules = append(modules, module)
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("workspace does not select a module")
	}
	sort.Strings(modules)
	return modules, nil
}

func validateLocalModuleReplacements(checkout string, modules []string) error {
	canonicalCheckout, err := canonicalAbsolute(checkout)
	if err != nil {
		return fmt.Errorf("resolve checkout: %w", err)
	}
	for _, moduleDirectory := range modules {
		modulePath := filepath.Join(moduleDirectory, "go.mod")
		info, err := os.Lstat(modulePath)
		if err != nil {
			return fmt.Errorf("inspect module file %q: %w", modulePath, err)
		}
		if !info.Mode().IsRegular() || info.Mode() & os.ModeSymlink != 0 {
			return fmt.Errorf("module file %q is not a regular file", modulePath)
		}
		input, err := os.ReadFile(modulePath)
		if err != nil {
			return fmt.Errorf("read module file %q: %w", modulePath, err)
		}
		module, err := modfile.Parse(modulePath, input, nil)
		if err != nil {
			return fmt.Errorf("parse module file %q: %w", modulePath, err)
		}
		if err := validateLocalReplacementTargets(
			canonicalCheckout,
			moduleDirectory,
			module.Replace,
			"module",
		);
			err != nil {
			return fmt.Errorf("module file %q: %w", modulePath, err)
		}
	}
	return nil
}

func validateLocalReplacementTargets(
	checkout, baseDirectory string,
	replacements []*modfile.Replace,
	kind string,
) error {
	for _, replacement := range replacements {
		if replacement.New.Version != "" {
			continue
		}
		target := filepath.FromSlash(replacement.New.Path)
		if !filepath.IsAbs(target) {
			target = filepath.Join(baseDirectory, target)
		}
		canonicalTarget, err := canonicalAbsolute(target)
		if err != nil {
			return fmt.Errorf(
				"resolve local %s replacement %q: %w",
				kind,
				replacement.New.Path,
				err,
			)
		}
		if !pathAtOrWithin(checkout, canonicalTarget) {
			return fmt.Errorf(
				"local %s replacement %q is outside the checkout",
				kind,
				replacement.New.Path,
			)
		}
		targetInfo, err := os.Stat(canonicalTarget)
		if err != nil {
			return fmt.Errorf(
				"inspect local %s replacement %q: %w",
				kind,
				replacement.New.Path,
				err,
			)
		}
		if !targetInfo.IsDir() {
			return fmt.Errorf(
				"local %s replacement %q is not a directory",
				kind,
				replacement.New.Path,
			)
		}
	}
	return nil
}

func moduleDownloadEnvironment(options RunOptions, repository Repository) ([]string, error) {
	downloadOptions := options
	downloadOptions.Environment = options.downloadEnvironment
	environment, err := isolatedEnvironment(
		downloadOptions,
		repository.ID + "-download",
		repository.CGO,
		"off",
	)
	if err != nil {
		return nil, err
	}
	return replaceEnvironment(
		environment,
		map[string]string{
			"GOFLAGS": "-mod=readonly",
			"GONOSUMDB": "",
			"GOPRIVATE": "",
			"GOPROXY": "https://proxy.golang.org,direct",
			"GOSUMDB": "sum.golang.org",
			"GOVCS": "public:git|hg,private:off",
			"GOWORK": "off",
		},
	), nil
}

func runProfile(
	ctx context.Context,
	options RunOptions,
	repository Repository,
	checkout, repositoryOutput, configurationRoot string,
	environment []string,
	profile string,
) (profileResult, error) {
	configurationPath := filepath.Join(configurationRoot, profile + ".toml")
	configuration := "version = 1\n\n[lint]\nprofile = \"" +
		profile +
		"\"\n\n[cache]\nenabled = false\n"
	if err := os.WriteFile(configurationPath, []byte(configuration), 0o600); err != nil {
		return profileResult{}, fmt.Errorf(
			"write %s profile configuration: %w",
			profile,
			err,
		)
	}
	arguments := []string{
		"lint",
		"--reporter=json",
		"--stats=json",
		"--config",
		configurationPath,
	}
	arguments = append(arguments, repository.Patterns...)
	execution, err := options.Executor.Run(
		ctx,
		Command{Path: options.GlippyPath, Args: arguments, Dir: checkout, Env: environment},
	)
	if err != nil {
		return profileResult{}, fmt.Errorf(
			"run %s profile for %q: %w",
			profile,
			repository.ID,
			err,
		)
	}
	profileDirectory := filepath.Join(repositoryOutput, profile)
	if err := os.MkdirAll(profileDirectory, 0o755); err != nil {
		return profileResult{}, fmt.Errorf("create %s profile output: %w", profile, err)
	}
	diagnosticArtifact, normalizedDiagnostics, validDiagnostics, err := writeNormalizedJSON(
		filepath.Join(profileDirectory, "diagnostics.json"),
		execution.Stdout,
		newArtifactNormalizer(checkout, options),
	)
	if err != nil {
		return profileResult{}, err
	}
	statisticsArtifact, _, _, err := writeNormalizedJSON(
		filepath.Join(profileDirectory, "statistics.json"),
		execution.Stderr,
		newArtifactNormalizer(checkout, options),
	)
	if err != nil {
		return profileResult{}, err
	}
	inventory := findingInventory{
		SchemaVersion: ResultSchemaVersion,
		Repository: repository.ID,
		Revision: repository.Revision,
		Profile: profile,
		Diagnostics: []finding{},
	}
	complete := false
	if validDiagnostics {
		var lintResult normalizedLintResult
		if err := json.Unmarshal(normalizedDiagnostics, &lintResult); err != nil {
			return profileResult{}, fmt.Errorf("decode normalized diagnostics: %w", err)
		}
		inventory.Diagnostics = slices.Clone(lintResult.Diagnostics)
		for index := range inventory.Diagnostics {
			inventory.Diagnostics[index].Fingerprint = findingFingerprint(
				inventory.Diagnostics[index],
			)
		}
		sortFindings(inventory.Diagnostics)
		complete = lintResult.Summary.Complete
	}
	findingsArtifact, err := writeJSON(
		filepath.Join(profileDirectory, "findings.json"),
		inventory,
	)
	if err != nil {
		return profileResult{}, err
	}
	return profileResult{
		Profile: profile,
		ExitCode: execution.ExitCode,
		Diagnostics: diagnosticArtifact,
		Statistics: measurementArtifactResult{
			File: statisticsArtifact.File,
			ValidJSON: statisticsArtifact.ValidJSON,
		},
		Findings: findingsArtifact,
		DiagnosticCount: len(inventory.Diagnostics),
		Complete: complete,
	}, nil
}

func runComparators(
	ctx context.Context,
	options RunOptions,
	repository Repository,
	checkout, repositoryOutput string,
	environment []string,
) ([]comparatorResult, error) {
	commands := []comparatorCommand{
		{
			name: "go-vet",
			path: "go",
			arguments: append([]string{"vet"}, repository.Patterns...),
		},
		{
			name: "staticcheck",
			path: options.StaticcheckPath,
			arguments: slices.Clone(repository.Patterns),
		},
	}
	results := make([]comparatorResult, 0, len(commands))
	for _, comparator := range commands {
		result, err := runComparator(
			ctx,
			options,
			repository,
			checkout,
			repositoryOutput,
			environment,
			comparator,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func analysisPreflightCommand(repository Repository) comparatorCommand {
	return comparatorCommand{
		name: "analysis-preflight",
		path: "go",
		arguments: append(
			[]string{"list", "-deps", "-test", "-export"},
			repository.Patterns...,
		),
	}
}

func runComparator(
	ctx context.Context,
	options RunOptions,
	repository Repository,
	checkout, repositoryOutput string,
	environment []string,
	comparator comparatorCommand,
) (comparatorResult, error) {
	execution, err := options.Executor.Run(
		ctx,
		Command{
			Path: comparator.path,
			Args: comparator.arguments,
			Dir: checkout,
			Env: environment,
		},
	)
	if err != nil {
		return comparatorResult{}, fmt.Errorf(
			"run %s for %q: %w",
			comparator.name,
			repository.ID,
			err,
		)
	}
	output := append(slices.Clone(execution.Stdout), execution.Stderr...)
	output = normalizeText(output, newArtifactNormalizer(checkout, options))
	artifact, err := writeArtifact(
		filepath.Join(repositoryOutput, comparator.name + ".txt"),
		output,
		false,
	)
	if err != nil {
		return comparatorResult{}, err
	}
	return comparatorResult{
		Name: comparator.name,
		ExitCode: execution.ExitCode,
		Output: artifact,
	}, nil
}

func inspectToolVersions(ctx context.Context, options RunOptions) (toolVersions, error) {
	commands := []struct {
		name string
		path string
		arguments []string
		target *string
	}{
		{name: "Glippy", path: options.GlippyPath, arguments: []string{"version"}},
		{name: "Go", path: "go", arguments: []string{"version"}},
		{
			name: "Staticcheck",
			path: options.StaticcheckPath,
			arguments: []string{"-version"},
		},
	}
	versions := toolVersions{}
	commands[0].target = &versions.Glippy
	commands[1].target = &versions.Go
	commands[2].target = &versions.Staticcheck
	for _, command := range commands {
		result, err := options.Executor.Run(
			ctx,
			Command{
				Path: command.path,
				Args: command.arguments,
				Dir: filepath.Join(options.CacheRoot, "tools"),
				Env: options.Environment,
			},
		)
		if err != nil {
			return toolVersions{}, fmt.Errorf(
				"inspect %s version: %w",
				command.name,
				err,
			)
		}
		if result.ExitCode != 0 {
			return toolVersions{}, fmt.Errorf(
				"inspect %s version: exit %d: %s",
				command.name,
				result.ExitCode,
				strings.TrimSpace(string(result.Stderr)),
			)
		}
		*command.target = strings.TrimSpace(string(result.Stdout))
		if *command.target == "" {
			return toolVersions{}, fmt.Errorf(
				"inspect %s version: empty output",
				command.name,
			)
		}
	}
	return versions, nil
}

func verifyGitCheckout(
	ctx context.Context,
	executor Executor,
	checkout string,
	repository Repository,
	environment []string,
) error {
	for _, check := range
		[]struct {
			arguments []string
			want string
			label string
		}{
			{
				arguments: []string{"rev-parse", "HEAD"},
				want: repository.Revision,
				label: "revision",
			},
			{
				arguments: []string{"config", "--get", "remote.origin.url"},
				want: repository.Repository,
				label: "origin",
			},
		} {
		result, err := executor.Run(
			ctx,
			Command{
				Path: "git",
				Args: check.arguments,
				Dir: checkout,
				Env: environment,
			},
		)
		if err != nil {
			return fmt.Errorf("inspect %s for %q: %w", check.label, repository.ID, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf(
				"inspect %s for %q: exit %d",
				check.label,
				repository.ID,
				result.ExitCode,
			)
		}
		got := strings.TrimSpace(string(result.Stdout))
		matches := got == check.want
		if check.label == "origin" {
			matches = normalizeRepositoryURL(got) == normalizeRepositoryURL(check.want)
		}
		if !matches {
			return fmt.Errorf(
				"checkout %q %s = %q, want %q",
				repository.ID,
				check.label,
				got,
				check.want,
			)
		}
	}
	status, err := executor.Run(
		ctx,
		Command{
			Path: "git",
			Args: []string{
				"status",
				"--porcelain=v1",
				"--untracked-files=all",
				"--ignored=matching",
			},
			Dir: checkout,
			Env: environment,
		},
	)
	if err != nil {
		return fmt.Errorf("inspect checkout status for %q: %w", repository.ID, err)
	}
	if status.ExitCode != 0 {
		return fmt.Errorf(
			"inspect checkout status for %q: exit %d",
			repository.ID,
			status.ExitCode,
		)
	}
	if len(status.Stdout) != 0 {
		return fmt.Errorf("checkout %q is not clean", repository.ID)
	}
	return nil
}

func validateCheckoutFiles(checkout string, repository Repository) error {
	info, err := os.Lstat(checkout)
	if err != nil {
		return fmt.Errorf("inspect checkout %q: %w", repository.ID, err)
	}
	if !info.IsDir() || info.Mode() & os.ModeSymlink != 0 {
		return fmt.Errorf("checkout %q is not a real directory", repository.ID)
	}
	licensePath := filepath.Join(checkout, filepath.FromSlash(repository.LicensePath))
	licenseInfo, err := os.Lstat(licensePath)
	if err != nil {
		return fmt.Errorf("inspect license for %q: %w", repository.ID, err)
	}
	if !licenseInfo.Mode().IsRegular() || licenseInfo.Mode() & os.ModeSymlink != 0 {
		return fmt.Errorf("license for %q is not a regular file", repository.ID)
	}
	module, err := os.ReadFile(filepath.Join(checkout, "go.mod"))
	if err != nil {
		return fmt.Errorf("read go.mod for %q: %w", repository.ID, err)
	}
	if directive := goDirective(module); directive != repository.GoDirective {
		return fmt.Errorf(
			"checkout %q go directive = %q, want %q",
			repository.ID,
			directive,
			repository.GoDirective,
		)
	}
	return nil
}

func goDirective(module []byte) string {
	for _, line := range strings.Split(string(module), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	return ""
}

func repositoryEnvironment(
	options RunOptions,
	repository Repository,
	checkout string,
) ([]string, error) {
	return repositoryEnvironmentForScope(
		options,
		repository,
		checkout,
		filepath.Join("repositories", repository.ID, "analysis"),
	)
}

func repositoryEnvironmentForScope(
	options RunOptions,
	repository Repository,
	checkout, scope string,
) ([]string, error) {
	workspace := "off"
	workspacePath := filepath.Join(checkout, "go.work")
	if info, err := os.Stat(workspacePath); err == nil && info.Mode().IsRegular() {
		workspace = workspacePath
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect corpus workspace: %w", err)
	}
	return isolatedEnvironment(options, scope, repository.CGO, workspace)
}

func isolatedEnvironment(
	options RunOptions,
	scope string,
	cgoEnabled bool,
	workspace string,
) ([]string, error) {
	repositoryCache := filepath.Join(options.CacheRoot, scope)
	goCache := filepath.Join(repositoryCache, "gocache")
	goPath := filepath.Join(options.CacheRoot, "gopath")
	moduleCache := filepath.Join(options.CacheRoot, "gomodcache")
	temporary := filepath.Join(repositoryCache, "tmp")
	xdgCache := filepath.Join(repositoryCache, "xdg-cache")
	xdgConfig := filepath.Join(repositoryCache, "xdg-config")
	xdgData := filepath.Join(repositoryCache, "xdg-data")
	xdgState := filepath.Join(repositoryCache, "xdg-state")
	for _, directory := range
		[]string{
			goCache,
			goPath,
			moduleCache,
			temporary,
			xdgCache,
			xdgConfig,
			xdgData,
			xdgState,
		} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("create corpus cache directory: %w", err)
		}
	}
	telemetryDirectory := filepath.Join(xdgConfig, "go", "telemetry")
	if err := os.MkdirAll(telemetryDirectory, 0o755); err != nil {
		return nil, fmt.Errorf("create corpus telemetry directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(telemetryDirectory, "mode"), []byte("off\n"), 0o600);
		err != nil {
		return nil, fmt.Errorf("disable corpus telemetry: %w", err)
	}
	cgo := "0"
	if cgoEnabled {
		cgo = "1"
	}
	return replaceEnvironment(
		corpusBaseEnvironment(options.Environment),
		map[string]string{
			"CGO_ENABLED": cgo,
			"GIT_CONFIG_GLOBAL": "/dev/null",
			"GIT_CONFIG_NOSYSTEM": "1",
			"GIT_OPTIONAL_LOCKS": "0",
			"GIT_TERMINAL_PROMPT": "0",
			"GOCACHE": goCache,
			"GOCACHEPROG": "",
			"GOMEMLIMIT": corpusCommandMemoryLimit,
			"GOMODCACHE": moduleCache,
			"GOPATH": goPath,
			"GOENV": "off",
			"GOFLAGS": "",
			"GONOPROXY": "none",
			"GONOSUMDB": "*",
			"GOPRIVATE": "*",
			"GOPROXY": "off",
			"GOSUMDB": "off",
			"GOTOOLCHAIN": "local",
			"GOVCS": "*:off",
			"GOTMPDIR": temporary,
			"GOWORK": workspace,
			"TMPDIR": temporary,
			"XDG_CACHE_HOME": xdgCache,
			"XDG_CONFIG_HOME": xdgConfig,
			"XDG_DATA_HOME": xdgData,
			"XDG_STATE_HOME": xdgState,
		},
	), nil
}

func resolveRunOptions(options RunOptions) (RunOptions, error) {
	if !runIDPattern.MatchString(options.RunID) {
		return RunOptions{}, errors.New(
			"run ID is required and must contain only letters, digits, dot, colon, underscore, or hyphen",
		)
	}
	for name, value := range
		map[string]string{
			"checkout root": options.CheckoutRoot,
			"output root": options.OutputRoot,
			"cache root": options.CacheRoot,
			"Glippy path": options.GlippyPath,
			"Staticcheck path": options.StaticcheckPath,
		} {
		if value == "" {
			return RunOptions{}, fmt.Errorf("%s is required", name)
		}
	}
	var err error
	options.CheckoutRoot, err = canonicalAbsolute(options.CheckoutRoot)
	if err != nil {
		return RunOptions{}, fmt.Errorf("resolve checkout root: %w", err)
	}
	options.OutputRoot, err = canonicalAbsolute(options.OutputRoot)
	if err != nil {
		return RunOptions{}, fmt.Errorf("resolve output root: %w", err)
	}
	options.CacheRoot, err = canonicalAbsolute(options.CacheRoot)
	if err != nil {
		return RunOptions{}, fmt.Errorf("resolve cache root: %w", err)
	}
	info, err := os.Stat(options.CheckoutRoot)
	if err != nil || !info.IsDir() {
		return RunOptions{}, fmt.Errorf("checkout root is not a directory")
	}
	if pathAtOrWithin(options.CheckoutRoot, options.OutputRoot) ||
		pathAtOrWithin(options.CheckoutRoot, options.CacheRoot) {
		return RunOptions{}, errors.New("output and cache roots must be outside checkouts")
	}
	if pathAtOrWithin(options.OutputRoot, options.CacheRoot) ||
		pathAtOrWithin(options.CacheRoot, options.OutputRoot) {
		return RunOptions{}, errors.New("output and cache roots must be separate")
	}
	if options.Executor == nil {
		options.Executor = osExecutor{}
	}
	if options.Environment == nil {
		options.Environment = os.Environ()
	} else {
		options.Environment = slices.Clone(options.Environment)
	}
	return options, nil
}

func selectRepositories(manifest Manifest, selected []string) ([]Repository, error) {
	if len(selected) == 0 {
		return slices.Clone(manifest.Repositories), nil
	}
	if !slices.IsSorted(selected) {
		return nil, errors.New("selected repository IDs must be ordered")
	}
	wanted := make(map[string]struct{}, len(selected))
	for index, id := range selected {
		if index > 0 && selected[index - 1] == id {
			return nil, fmt.Errorf("duplicate selected repository ID %q", id)
		}
		wanted[id] = struct{}{}
	}
	result := make([]Repository, 0, len(selected))
	for _, repository := range manifest.Repositories {
		if _, found := wanted[repository.ID]; found {
			result = append(result, repository)
			delete(wanted, repository.ID)
		}
	}
	if len(wanted) != 0 {
		unknown := make([]string, 0, len(wanted))
		for id := range wanted {
			unknown = append(unknown, id)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf(
			"unknown selected repository IDs: %s",
			strings.Join(unknown, ", "),
		)
	}
	return result, nil
}

func prepareOutputRoot(root string) error {
	entries, err := os.ReadDir(root)
	switch {
	case err == nil && len(entries) != 0:
		return fmt.Errorf("corpus output root %q is not empty", root)
	case err == nil:
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("inspect corpus output root: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create corpus output root: %w", err)
	}
	return nil
}

func writeNormalizedJSON(
	path string,
	input []byte,
	normalizer artifactNormalizer,
) (artifactResult, []byte, bool, error) {
	var document any
	if err := json.Unmarshal(input, &document); err != nil {
		normalized := normalizeText(input, normalizer)
		fallbackPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".txt"
		artifact, writeErr := writeArtifact(fallbackPath, normalized, false)
		return artifact, normalized, false, writeErr
	}
	normalizeJSONStrings(document, normalizer)
	normalized, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return artifactResult{}, nil, false, fmt.Errorf("marshal normalized JSON: %w", err)
	}
	normalized = append(normalized, '\n')
	artifact, err := writeArtifact(path, normalized, true)
	return artifact, normalized, true, err
}

func normalizeJSONStrings(value any, normalizer artifactNormalizer) {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			normalizeJSONStrings(child, normalizer)
		}
	case map[string]any:
		for key, child := range typed {
			if text, ok := child.(string); ok {
				typed[key] = normalizeString(text, normalizer)
				continue
			}
			normalizeJSONStrings(child, normalizer)
		}
	}
}

func normalizeText(input []byte, normalizer artifactNormalizer) []byte {
	text := strings.ReplaceAll(string(input), "\r\n", "\n")
	for _, path := range normalizer.Paths {
		text = strings.ReplaceAll(
			text,
			path.Source + string(filepath.Separator),
			path.Replacement + "/",
		)
		text = strings.ReplaceAll(text, path.Source, path.Replacement)
	}
	return []byte(text)
}

func normalizeString(value string, normalizer artifactNormalizer) string {
	relative, err := filepath.Rel(normalizer.SourceRoot, value)
	if err == nil &&
		relative != "." &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".." + string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	if value == normalizer.SourceRoot {
		return "."
	}
	return string(normalizeText([]byte(value), normalizer))
}

func newArtifactNormalizer(checkout string, options RunOptions) artifactNormalizer {
	paths := []pathNormalization{
		{Source: checkout, Replacement: "<ROOT>"},
		{Source: options.CacheRoot, Replacement: "<CACHE>"},
		{Source: options.OutputRoot, Replacement: "<OUTPUT>"},
		{Source: options.CheckoutRoot, Replacement: "<CHECKOUTS>"},
	}
	for _, tool := range []string{options.GlippyPath, options.StaticcheckPath} {
		if filepath.IsAbs(tool) {
			paths = append(
				paths,
				pathNormalization{
					Source: filepath.Dir(tool),
					Replacement: "<TOOLS>",
				},
			)
		}
	}
	sort.Slice(
		paths,
		func(left, right int) bool {
			return len(paths[left].Source) > len(paths[right].Source)
		},
	)
	return artifactNormalizer{SourceRoot: checkout, Paths: paths}
}

func sortFindings(findings []finding) {
	sort.Slice(
		findings,
		func(left, right int) bool {
			leftFinding := findings[left]
			rightFinding := findings[right]
			return lessFinding(leftFinding, rightFinding)
		},
	)
}

func findingFingerprint(value finding) string {
	identity := struct {
		RuleID string `json:"rule_id"`
		Severity string `json:"severity"`
		MessageKey string `json:"message_key"`
		Message string `json:"message"`
		Path string `json:"path"`
		Range findingRange `json:"range"`
	}{
		RuleID: value.RuleID,
		Severity: value.Severity,
		MessageKey: value.MessageKey,
		Message: value.Message,
		Path: value.Path,
		Range: value.Range,
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func lessFinding(left, right finding) bool {
	for _, comparison := range
		[]int{
			strings.Compare(left.RuleID, right.RuleID),
			strings.Compare(left.Path, right.Path),
			cmp.Compare(left.Range.Start, right.Range.Start),
			cmp.Compare(left.Range.End, right.Range.End),
			strings.Compare(left.MessageKey, right.MessageKey),
			strings.Compare(left.Message, right.Message),
		} {
		if comparison != 0 {
			return comparison < 0
		}
	}
	return false
}

func writeJSON(path string, value any) (artifactResult, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return artifactResult{}, fmt.Errorf("encode %q: %w", path, err)
	}
	return writeArtifact(path, append(encoded, '\n'), true)
}

func writeArtifact(path string, input []byte, validJSON bool) (artifactResult, error) {
	if err := os.WriteFile(path, input, 0o644); err != nil {
		return artifactResult{}, fmt.Errorf("write corpus artifact %q: %w", path, err)
	}
	digest := sha256.Sum256(input)
	return artifactResult{
		File: filepath.ToSlash(filepath.Base(path)),
		SHA256: hex.EncodeToString(digest[:]),
		ValidJSON: validJSON,
	}, nil
}

func replaceEnvironment(
	environment []string,
	replacements map[string]string,
	removed ...string,
) []string {
	removedSet := make(map[string]struct{}, len(removed))
	for _, name := range removed {
		removedSet[name] = struct{}{}
	}
	result := make([]string, 0, len(environment) + len(replacements))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, remove := removedSet[name]; remove {
				continue
			}
			if _, replaced := replacements[name]; replaced {
				continue
			}
		}
		result = append(result, entry)
	}
	names := make([]string, 0, len(replacements))
	for name := range replacements {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name + "=" + replacements[name])
	}
	return result
}

func corpusBaseEnvironment(environment []string) []string {
	allowed := map[string]struct{}{
		"AR": {},
		"CC": {},
		"CXX": {},
		"LANG": {},
		"LC_ALL": {},
		"LC_CTYPE": {},
		"MACOSX_DEPLOYMENT_TARGET": {},
		"PATH": {},
		"PKG_CONFIG": {},
		"PKG_CONFIG_PATH": {},
		"SDKROOT": {},
	}
	result := make([]string, 0, len(allowed))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, keep := allowed[name]; keep {
				result = append(result, entry)
			}
		}
	}
	return result
}

func canonicalAbsolute(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	existing := absolute
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("no existing ancestor for %q", absolute)
		}
		suffix = append(suffix, filepath.Base(existing))
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for index := len(suffix) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, suffix[index])
	}
	return filepath.Clean(resolved), nil
}

func staticcheckModuleVersion(output string) string {
	if match := staticcheckOutputVersionPattern.FindStringSubmatch(output); len(match) == 2 {
		return "v" + strings.TrimPrefix(match[1], "v")
	}
	fields := strings.Fields(output)
	if len(fields) == 2 &&
		fields[0] == "staticcheck" &&
		staticcheckVersionPattern.MatchString(fields[1]) {
		return fields[1]
	}
	return ""
}

func copyReadOnlyCheckout(source, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	err := filepath.WalkDir(
		source,
		func(current string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source, current)
			if err != nil {
				return err
			}
			if relative == ".git" ||
				strings.HasPrefix(relative, ".git" + string(filepath.Separator)) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if relative == "." {
				return nil
			}
			target := filepath.Join(destination, relative)
			info, err := entry.Info()
			if err != nil {
				return err
			}
			switch {
			case entry.IsDir():
				return os.Mkdir(target, info.Mode().Perm() | 0o700)
			case info.Mode().IsRegular():
				return copyRegularFile(current, target, info.Mode().Perm())
			case info.Mode() & os.ModeSymlink != 0:
				link, err := os.Readlink(current)
				if err != nil {
					return err
				}
				resolved := filepath.Clean(
					filepath.Join(filepath.Dir(current), link),
				)
				if filepath.IsAbs(link) || !pathAtOrWithin(source, resolved) {
					return fmt.Errorf("symlink %q escapes checkout", relative)
				}
				return os.Symlink(link, target)
			default:
				return fmt.Errorf("unsupported file type at %q", relative)
			}
		},
	)
	if err != nil {
		return err
	}
	return makeTreeReadOnly(destination)
}

func makeTreeReadOnly(root string) error {
	return filepath.Walk(
		root,
		func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.Mode() & os.ModeSymlink != 0 {
				return nil
			}
			return os.Chmod(path, info.Mode().Perm() &^ 0o222)
		},
	)
}

func permitWorkspaceSumUpdate(root string) (bool, error) {
	workspaceInfo, err := os.Lstat(filepath.Join(root, "go.work"))
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, err
	case !workspaceInfo.Mode().IsRegular():
		return false, fmt.Errorf("workspace is not a regular file")
	}

	rootInfo, err := os.Stat(root)
	if err != nil {
		return false, err
	}
	if err := os.Chmod(root, rootInfo.Mode().Perm() | 0o200); err != nil {
		return false, err
	}

	sumPath := filepath.Join(root, "go.work.sum")
	sumInfo, err := os.Lstat(sumPath)
	switch {
	case err == nil && !sumInfo.Mode().IsRegular():
		return false, fmt.Errorf("workspace sum is not a regular file")
	case err == nil:
		if err := os.Chmod(sumPath, sumInfo.Mode().Perm() | 0o200); err != nil {
			return false, err
		}
	case os.IsNotExist(err):
		workspaceSum, createErr := os.OpenFile(
			sumPath,
			os.O_CREATE | os.O_EXCL | os.O_WRONLY,
			0o600,
		)
		if createErr != nil {
			return false, createErr
		}
		if closeErr := workspaceSum.Close(); closeErr != nil {
			return false, closeErr
		}
	case err != nil:
		return false, err
	}
	return true, nil
}

func permitExistingWorkspaceSumUpdate(root string) error {
	sumPath := filepath.Join(root, "go.work.sum")
	sumInfo, err := os.Lstat(sumPath)
	if err != nil {
		return err
	}
	if !sumInfo.Mode().IsRegular() {
		return fmt.Errorf("workspace sum is not a regular file")
	}
	return os.Chmod(sumPath, sumInfo.Mode().Perm() | 0o200)
}

type checkoutSnapshotEntry struct {
	kind string
	digest string
	link string
	mode os.FileMode
	size int64
	modifiedUnixNano int64
}

func validateModuleGraphSnapshot(source, snapshot string, allowWorkspaceSum bool) error {
	sourceEntries, err := checkoutSnapshotInventory(source, allowWorkspaceSum)
	if err != nil {
		return fmt.Errorf("inventory source checkout: %w", err)
	}
	snapshotEntries, err := checkoutSnapshotInventory(snapshot, allowWorkspaceSum)
	if err != nil {
		return fmt.Errorf("inventory execution snapshot: %w", err)
	}
	paths := make([]string, 0, len(sourceEntries) + len(snapshotEntries))
	for path := range sourceEntries {
		paths = append(paths, path)
	}
	for path := range snapshotEntries {
		if _, found := sourceEntries[path]; !found {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		sourceEntry, sourceFound := sourceEntries[path]
		snapshotEntry, snapshotFound := snapshotEntries[path]
		if !sourceFound ||
			!snapshotFound ||
			!sameCheckoutSnapshotContent(sourceEntry, snapshotEntry) {
			return fmt.Errorf("unexpected change at %q", path)
		}
	}
	if allowWorkspaceSum {
		sourceInfo, sourceErr := os.Lstat(filepath.Join(source, "go.work.sum"))
		if sourceErr != nil && !os.IsNotExist(sourceErr) {
			return fmt.Errorf("inspect source workspace sum: %w", sourceErr)
		}
		snapshotInfo, snapshotErr := os.Lstat(filepath.Join(snapshot, "go.work.sum"))
		if snapshotErr != nil && !os.IsNotExist(snapshotErr) {
			return fmt.Errorf("inspect snapshot workspace sum: %w", snapshotErr)
		}
		if sourceErr == nil && os.IsNotExist(snapshotErr) {
			return fmt.Errorf("workspace sum was removed")
		}
		if sourceErr == nil && !sourceInfo.Mode().IsRegular() {
			return fmt.Errorf("source workspace sum is not a regular file")
		}
		if snapshotErr == nil && !snapshotInfo.Mode().IsRegular() {
			return fmt.Errorf("workspace sum is not a regular file")
		}
	}
	return nil
}

func sameCheckoutSnapshotContent(left, right checkoutSnapshotEntry) bool {
	return left.kind == right.kind && left.digest == right.digest && left.link == right.link
}

func validateExactCheckoutSnapshot(root string, want map[string]checkoutSnapshotEntry) error {
	got, err := checkoutSnapshotInventory(root, false)
	if err != nil {
		return fmt.Errorf("inventory exact snapshot: %w", err)
	}
	paths := make([]string, 0, len(want) + len(got))
	for path := range want {
		paths = append(paths, path)
	}
	for path := range got {
		if _, found := want[path]; !found {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		wantEntry, wantFound := want[path]
		gotEntry, gotFound := got[path]
		if !wantFound || !gotFound || wantEntry != gotEntry {
			return fmt.Errorf("unexpected exact change at %q", path)
		}
	}
	return nil
}

func checkoutSnapshotInventory(
	root string,
	skipWorkspaceSum bool,
) (map[string]checkoutSnapshotEntry, error) {
	entries := make(map[string]checkoutSnapshotEntry)
	err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if relative == ".git" ||
				strings.HasPrefix(relative, ".git" + string(filepath.Separator)) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if skipWorkspaceSum && relative == "go.work.sum" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			fingerprint := checkoutSnapshotEntry{
				mode: info.Mode(),
				size: info.Size(),
				modifiedUnixNano: info.ModTime().UnixNano(),
			}
			switch {
			case entry.IsDir():
				fingerprint.kind = "directory"
			case info.Mode().IsRegular():
				fingerprint.kind = "regular"
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				hash := sha256.New()
				_, copyErr := io.Copy(hash, file)
				closeErr := file.Close()
				if err := errors.Join(copyErr, closeErr); err != nil {
					return err
				}
				fingerprint.digest = hex.EncodeToString(hash.Sum(nil))
			case info.Mode() & os.ModeSymlink != 0:
				fingerprint.kind = "symlink"
				link, err := os.Readlink(path)
				if err != nil {
					return err
				}
				fingerprint.link = link
			default:
				return fmt.Errorf("unsupported file type at %q", relative)
			}
			entries[relative] = fingerprint
			return nil
		},
	)
	return entries, err
}

func copyRegularFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE | os.O_EXCL | os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}

func removeReadOnlyTree(root string) error {
	if err := filepath.Walk(
		root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode() & os.ModeSymlink == 0 {
				return os.Chmod(path, info.Mode().Perm() | 0o700)
			}
			return nil
		},
	);
		err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(root)
}

func normalizeRepositoryURL(value string) string {
	value = strings.TrimSuffix(strings.TrimSpace(value), ".git")
	value = strings.TrimPrefix(value, "git@github.com:")
	value = strings.TrimPrefix(value, "ssh://git@github.com/")
	value = strings.TrimPrefix(value, "https://github.com/")
	return strings.TrimSuffix(value, "/")
}

func pathAtOrWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".." + string(filepath.Separator))
}

type osExecutor struct{}

func (osExecutor) Run(ctx context.Context, command Command) (CommandResult, error) {
	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.Dir = command.Dir
	process.Env = slices.Clone(command.Env)
	stdout := &limitedBuffer{maximum: maximumCommandOutput}
	stderr := &limitedBuffer{maximum: maximumCommandOutput}
	process.Stdout = stdout
	process.Stderr = stderr
	err := process.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if outputErr := errors.Join(stdout.Err(), stderr.Err()); outputErr != nil {
		return CommandResult{}, outputErr
	}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return CommandResult{}, err
}

type limitedBuffer struct {
	buffer bytes.Buffer
	maximum int
	err error
}

func (b *limitedBuffer) Write(input []byte) (int, error) {
	if len(input) > b.maximum - b.buffer.Len() {
		b.err = fmt.Errorf("command output exceeds %d bytes", b.maximum)
		return 0, b.err
	}
	return b.buffer.Write(input)
}

func (b *limitedBuffer) Bytes() []byte {
	return slices.Clone(b.buffer.Bytes())
}

func (b *limitedBuffer) Err() error {
	return b.err
}

var _ io.Writer = (*limitedBuffer)(nil)
