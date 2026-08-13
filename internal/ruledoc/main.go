// Command ruledoc renders the built-in lint catalog from canonical metadata.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rulecatalog"
)

func main() {
	outputPath := flag.String("output", "", "path to the generated Markdown catalog")
	flag.Parse()
	if *outputPath == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: ruledoc -output <path>")
		os.Exit(2)
	}

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		fail("construct rule registry", err)
	}
	content, err := report.RenderRuleCatalogMarkdown(registry)
	if err != nil {
		fail("render rule catalog", err)
	}
	if err := replaceFile(*outputPath, content); err != nil {
		fail("write rule catalog", err)
	}
}

func replaceFile(path string, content []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".lint-rules.*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "ruledoc: %s: %v\n", action, err)
	os.Exit(1)
}
