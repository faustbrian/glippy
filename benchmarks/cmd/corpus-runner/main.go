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
