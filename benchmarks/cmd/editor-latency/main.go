package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type probeOptions struct {
	warmups int
	runs    int
	budget  time.Duration
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("editor-latency", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	binary := flags.String("binary", "", "formatter binary")
	inputPath := flags.String("input", "", "formatter input")
	warmups := flags.Int("warmups", 5, "warmup invocations")
	runs := flags.Int("runs", 20, "measured invocations")
	budgetMS := flags.Int("budget-ms", 250, "maximum measured latency in milliseconds")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *binary == "" || *inputPath == "" {
		return errors.New("--binary and --input are required and operands are not accepted")
	}
	if *warmups < 0 || *runs <= 0 || *budgetMS <= 0 {
		return errors.New("--warmups must be non-negative and --runs and --budget-ms must be positive")
	}
	input, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("read formatter input: %w", err)
	}
	execute := func() (time.Duration, error) {
		command := exec.Command(*binary, "fmt", "--stdin-filepath="+*inputPath)
		command.Stdin = bytes.NewReader(input)
		command.Stdout = io.Discard
		var standardError bytes.Buffer
		command.Stderr = &standardError
		started := time.Now()
		err := command.Run()
		elapsed := time.Since(started)
		if err != nil {
			return 0, fmt.Errorf("formatter invocation failed: %w: %s", err, standardError.String())
		}
		return elapsed, nil
	}
	return probe(
		probeOptions{
			warmups: *warmups,
			runs:    *runs,
			budget:  time.Duration(*budgetMS) * time.Millisecond,
		},
		execute,
		output,
	)
}

func probe(options probeOptions, execute func() (time.Duration, error), output io.Writer) error {
	for range options.warmups {
		if _, err := execute(); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "sample,elapsed_ms"); err != nil {
		return err
	}
	var maximum time.Duration
	for sample := 1; sample <= options.runs; sample++ {
		elapsed, err := execute()
		if err != nil {
			return err
		}
		if elapsed > maximum {
			maximum = elapsed
		}
		if _, err := fmt.Fprintf(output, "%d,%.3f\n", sample, milliseconds(elapsed)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "maximum_ms,%.3f\n", milliseconds(maximum)); err != nil {
		return err
	}
	if maximum > options.budget {
		return fmt.Errorf(
			"editor latency budget exceeded: %.3f ms > %d ms",
			milliseconds(maximum),
			options.budget/time.Millisecond,
		)
	}
	return nil
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
