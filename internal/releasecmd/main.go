// Command releasecmd builds deterministic maintainer-only release artifacts.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/faustbrian/glippy/internal/release"
)

func main() {
	var options release.Options
	flag.StringVar(&options.Output, "output", "", "new artifact output directory")
	flag.StringVar(&options.Version, "version", "", "canonical release version")
	flag.StringVar(
		&options.SourceRevision,
		"revision",
		"",
		"40- or 64-character source revision",
	)
	flag.StringVar(&options.GoBinary, "go", "go", "Go toolchain binary")
	flag.StringVar(&options.GitBinary, "git", "git", "Git binary")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "releasecmd: positional arguments are not accepted")
		os.Exit(2)
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "releasecmd: resolve repository root: %v\n", err)
		os.Exit(1)
	}
	options.Root = root
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manifest, err := release.Build(ctx, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "releasecmd: %v\n", err)
		os.Exit(1)
	}
	for _, artifact := range manifest.Artifacts {
		fmt.Printf("%s  %s\n", artifact.SHA256, artifact.File)
	}
}
