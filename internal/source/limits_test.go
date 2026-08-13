package source

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestReadAllLimitAcceptsBoundaryAndRejectsOverflow(t *testing.T) {
	input, err := readAllLimit(strings.NewReader("four"), 4)
	if err != nil || string(input) != "four" {
		t.Fatalf("readAllLimit(boundary) = %q, %v, want exact input", input, err)
	}
	input, err = readAllLimit(strings.NewReader("five!"), 4)
	if input != nil || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("readAllLimit(overflow) = %q, %v, want ErrTooLarge", input, err)
	}
	if got := err.Error(); got != "Go source exceeds 4-byte limit" {
		t.Fatalf("readAllLimit(overflow) error = %q", got)
	}
}

func TestReadAllLimitPreservesReaderFailure(t *testing.T) {
	readErr := errors.New("read failed")
	input, err := readAllLimit(failingReader{err: readErr}, 4)
	if input != nil || !errors.Is(err, readErr) || errors.Is(err, ErrTooLarge) {
		t.Fatalf("readAllLimit(reader failure) = %q, %v, want original error", input, err)
	}
}

func TestValidateSizeUsesTheSharedSourceLimit(t *testing.T) {
	if err := ValidateSize(MaxFileSize); err != nil {
		t.Fatalf("ValidateSize(boundary) = %v", err)
	}
	if err := ValidateSize(MaxFileSize + 1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ValidateSize(overflow) = %v, want ErrTooLarge", err)
	}
}

func TestSourceLoadersRejectOversizedPhysicalInputBeforeCloning(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates one source-size boundary buffer")
	}
	input := make([]byte, MaxFileSize + 1)

	file, err := Load("oversized.go", input)
	if file != nil || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Load() = %#v, %v, want ErrTooLarge", file, err)
	}
	fragment, err := LoadFragment("oversized.go", FragmentStatement, input)
	if fragment != nil || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("LoadFragment() = %#v, %v, want ErrTooLarge", fragment, err)
	}
	if got := err.Error(); got != "Go source exceeds 67108864-byte limit" {
		t.Fatalf("LoadFragment() error = %q", got)
	}
}

func TestReadFileRejectsOversizedSparseSourceBeforeAllocation(t *testing.T) {
	path := t.TempDir() + "/oversized.go"
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxFileSize + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	input, err := ReadFile(path)
	if input != nil || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadFile() = %d bytes, %v, want ErrTooLarge", len(input), err)
	}
}

var _ io.Reader = failingReader{}
