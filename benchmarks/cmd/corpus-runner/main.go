package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/faustbrian/glippy/internal/corpus"
)

type stringList []string

func (l *stringList) String() string {
	return fmt.Sprint([]string(*l))
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("corpus-runner", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "path to the corpus manifest")
	checkoutRoot := flags.String("checkouts", "", "root containing pinned checkouts")
	outputRoot := flags.String("output", "", "empty task-owned artifact root")
	cacheRoot := flags.String("cache", "", "task-owned Go and tool cache root")
	glippyPath := flags.String("glippy", "", "Glippy binary to audit")
	staticcheckPath := flags.String("staticcheck", "", "Staticcheck binary to compare")
	runID := flags.String("run-id", "", "shared source and workflow run identity")
	resultsRoot := flags.String("results", "", "root containing corpus result artifacts")
	adjudicationTemplate := flags.Bool(
		"adjudication-template",
		false,
		"write an adjudication template for result artifacts",
	)
	adjudicationPath := flags.String(
		"adjudication",
		"",
		"validate an adjudication document against result artifacts",
	)
	adjudicationReportPath := flags.String(
		"adjudication-report",
		"",
		"write a deterministic report for an adjudication document",
	)
	validateOnly := flags.Bool(
		"validate-only",
		false,
		"validate the manifest without executing",
	)
	var repositories stringList
	flags.Var(&repositories, "repository", "repository ID to run; repeat in canonical order")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "corpus-runner: positional arguments are not accepted")
		return 2
	}
	if *manifestPath == "" {
		fmt.Fprintln(stderr, "corpus-runner: --manifest is required")
		return 2
	}
	input, err := os.ReadFile(*manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "corpus-runner: read manifest: %v\n", err)
		return 2
	}
	manifest, err := corpus.ParseManifest(input)
	if err != nil {
		fmt.Fprintf(stderr, "corpus-runner: %v\n", err)
		return 2
	}
	adjudicationMode := *adjudicationTemplate ||
		*adjudicationPath != "" ||
		*adjudicationReportPath != ""
	if *validateOnly && adjudicationMode {
		fmt.Fprintln(
			stderr,
			"corpus-runner: --validate-only cannot be combined with adjudication modes",
		)
		return 2
	}
	adjudicationModeCount := 0
	if *adjudicationTemplate {
		adjudicationModeCount++
	}
	if *adjudicationPath != "" {
		adjudicationModeCount++
	}
	if *adjudicationReportPath != "" {
		adjudicationModeCount++
	}
	if adjudicationModeCount > 1 {
		fmt.Fprintln(stderr, "corpus-runner: adjudication modes are mutually exclusive")
		return 2
	}
	if adjudicationMode {
		if *resultsRoot == "" {
			fmt.Fprintln(stderr, "corpus-runner: --results is required")
			return 2
		}
		if *checkoutRoot != "" ||
			*outputRoot != "" ||
			*cacheRoot != "" ||
			*glippyPath != "" ||
			*staticcheckPath != "" ||
			*runID != "" ||
			len(repositories) != 0 {
			fmt.Fprintln(
				stderr,
				"corpus-runner: adjudication modes cannot be combined with execution inputs",
			)
			return 2
		}
		if *adjudicationTemplate {
			template, templateErr := corpus.BuildAdjudicationTemplate(
				manifest,
				input,
				*resultsRoot,
			)
			if templateErr != nil {
				fmt.Fprintf(stderr, "corpus-runner: %v\n", templateErr)
				return 1
			}
			if _, writeErr := stdout.Write(template); writeErr != nil {
				fmt.Fprintf(
					stderr,
					"corpus-runner: write adjudication template: %v\n",
					writeErr,
				)
				return 1
			}
			return 0
		}
		if *adjudicationReportPath != "" {
			adjudicationInput, readErr := os.ReadFile(*adjudicationReportPath)
			if readErr != nil {
				fmt.Fprintf(
					stderr,
					"corpus-runner: read adjudication: %v\n",
					readErr,
				)
				return 2
			}
			report, reportErr := corpus.BuildAdjudicationReport(
				manifest,
				input,
				*resultsRoot,
				adjudicationInput,
			)
			if reportErr != nil {
				fmt.Fprintf(stderr, "corpus-runner: %v\n", reportErr)
				return 1
			}
			if _, writeErr := stdout.Write(report); writeErr != nil {
				fmt.Fprintf(
					stderr,
					"corpus-runner: write adjudication report: %v\n",
					writeErr,
				)
				return 1
			}
			return 0
		}
		adjudicationInput, readErr := os.ReadFile(*adjudicationPath)
		if readErr != nil {
			fmt.Fprintf(stderr, "corpus-runner: read adjudication: %v\n", readErr)
			return 2
		}
		summary, validationErr := corpus.ValidateAdjudication(
			manifest,
			input,
			*resultsRoot,
			adjudicationInput,
		)
		if validationErr != nil {
			fmt.Fprintf(stderr, "corpus-runner: %v\n", validationErr)
			return 1
		}
		status := "valid"
		if summary.Unresolved != 0 {
			status = "incomplete"
		}
		fmt.Fprintf(
			stdout,
			"%s corpus adjudication: %d repositories, %d findings, %d gaps, %d unresolved\n",
			status,
			summary.Repositories,
			summary.Findings,
			summary.Gaps,
			summary.Unresolved,
		)
		if summary.Unresolved != 0 {
			return 1
		}
		return 0
	}
	if *resultsRoot != "" {
		fmt.Fprintln(stderr, "corpus-runner: --results requires an adjudication mode")
		return 2
	}
	if *validateOnly {
		fmt.Fprintf(
			stdout,
			"valid corpus manifest: %d repositories\n",
			len(manifest.Repositories),
		)
		return 0
	}
	for _, required := range
		[]struct {
			name string
			value string
		}{
			{name: "--checkouts", value: *checkoutRoot},
			{name: "--output", value: *outputRoot},
			{name: "--cache", value: *cacheRoot},
			{name: "--glippy", value: *glippyPath},
			{name: "--staticcheck", value: *staticcheckPath},
			{name: "--run-id", value: *runID},
		} {
		if required.value == "" {
			fmt.Fprintf(stderr, "corpus-runner: %s is required\n", required.name)
			return 2
		}
	}
	slices.Sort(repositories)
	err = corpus.Run(
		ctx,
		manifest,
		corpus.RunOptions{
			RunID: *runID,
			CheckoutRoot: *checkoutRoot,
			OutputRoot: *outputRoot,
			CacheRoot: *cacheRoot,
			GlippyPath: *glippyPath,
			StaticcheckPath: *staticcheckPath,
			RepositoryIDs: []string(repositories),
		},
	)
	if err != nil {
		fmt.Fprintf(stderr, "corpus-runner: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "corpus results: %s\n", *outputRoot)
	return 0
}
