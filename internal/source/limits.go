package source

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// MaxFileSize is the largest complete file or physical fragment accepted by
// the source frontend.
const MaxFileSize int64 = 64 << 20

// ErrTooLarge reports source input that exceeds MaxFileSize.
var ErrTooLarge = errors.New("Go source is too large")

type sizeError struct {
	limit int64
}

func (e *sizeError) Error() string {
	return fmt.Sprintf("Go source exceeds %d-byte limit", e.limit)
}

func (e *sizeError) Unwrap() error {
	return ErrTooLarge
}

func checkSize(size, limit int64) error {
	if size > limit {
		return &sizeError{limit: limit}
	}
	return nil
}

// ValidateSize rejects a complete source file or physical fragment that
// exceeds the shared frontend limit.
func ValidateSize(size int64) error {
	return checkSize(size, MaxFileSize)
}

// ReadAll reads one bounded source stream.
func ReadAll(reader io.Reader) ([]byte, error) {
	return readAllLimit(reader, MaxFileSize)
}

func readAllLimit(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("Go source reader is required")
	}
	if limit < 0 {
		return nil, fmt.Errorf("Go source limit must not be negative")
	}
	input, err := io.ReadAll(io.LimitReader(reader, limit + 1))
	if err != nil {
		return nil, err
	}
	if err := checkSize(int64(len(input)), limit); err != nil {
		return nil, err
	}
	return input, nil
}

// ReadFile reads one bounded source file and detects growth beyond the limit.
func ReadFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := ValidateSize(info.Size()); err != nil {
		return nil, err
	}
	return ReadAll(file)
}
