package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidatesThePinnedManifestWithoutExecutingRepositories(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{
			"--manifest",
			filepath.Join("..", "..", "corpus", "manifest.json"),
			"--validate-only",
		},
		&stdout,
		&stderr,
	)
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("run() = exit %d, stderr %q", exitCode, stderr.String())
	}
	if stdout.String() != "valid corpus manifest: 17 repositories\n" {
		t.Fatalf("run() stdout = %q", stdout.String())
	}
}

func TestRunRejectsMissingExecutionInputs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"--manifest", filepath.Join("..", "..", "corpus", "manifest.json")},
		&stdout,
		&stderr,
	)
	if exitCode != 2 ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "--checkouts is required") {
		t.Fatalf(
			"run() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}
