// Package report owns stable machine-readable command result envelopes.
package report

import "encoding/json"

const SchemaVersion = 1

type Format string

const (
	Text Format = "text"
	JSON Format = "json"
)

type Outcome struct {
	Category string `json:"category"`
	ExitCode int `json:"exit_code"`
}

type Summary struct {
	Files int `json:"files"`
	Changed int `json:"changed"`
	Complete bool `json:"complete"`
}

type FileStatus string

const (
	FilePending FileStatus = "pending"
	FileUnchanged FileStatus = "unchanged"
	FileDifferent FileStatus = "different"
	FileFormatted FileStatus = "formatted"
	FileConflict FileStatus = "conflict"
	FileFailed FileStatus = "failed"
	FilePossiblyFormatted FileStatus = "possibly_formatted"
)

type File struct {
	Path string `json:"path"`
	Status FileStatus `json:"status"`
}

type Error struct {
	Message string `json:"message"`
}

type Result struct {
	SchemaVersion int `json:"schema_version"`
	Command string `json:"command"`
	Mode string `json:"mode"`
	Outcome Outcome `json:"outcome"`
	Summary Summary `json:"summary"`
	Files []File `json:"files"`
	Errors []Error `json:"errors"`
}

func NewFormatResult(
	mode, category string,
	exitCode, selected, changed int,
	complete bool,
	files []File,
	errs []Error,
) Result {
	if files == nil {
		files = []File{}
	}
	if errs == nil {
		errs = []Error{}
	}
	return Result{
		SchemaVersion: SchemaVersion,
		Command: "fmt",
		Mode: mode,
		Outcome: Outcome{Category: category, ExitCode: exitCode},
		Summary: Summary{Files: selected, Changed: changed, Complete: complete},
		Files: files,
		Errors: errs,
	}
}

func MarshalJSON(result Result) ([]byte, error) {
	return marshalJSON(result)
}

func marshalJSON(result any) ([]byte, error) {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
