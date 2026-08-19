package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/cli"
)

func main() {
	if analysis.RunUnitcheckerMode(os.Getenv(analysis.UnitcheckerModeEnvironment)) {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "glippy: identify executable: %v\n", err)
		os.Exit(cli.ExitInternalError)
	}
	runner, err := analysis.NewUnitcheckerFactAnalyzerRunner(
		analysis.UnitcheckerFactAnalyzerRunnerOptions{Executable: executable},
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "glippy: configure analyzer runner: %v\n", err)
		os.Exit(cli.ExitInternalError)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx = analysis.WithPackageFactAnalyzerRunner(ctx, runner)
	exitCode := cli.RunContext(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(exitCode)
}
