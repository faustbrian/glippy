package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/unitchecker"

	"github.com/faustbrian/glippy/internal/source"
)

const (
	// UnitcheckerModeEnvironment selects one hidden exact analyzer mode in the
	// Glippy binary when it is invoked by the Go command as a vet tool.
	UnitcheckerModeEnvironment = "GLIPPY_INTERNAL_UNITCHECKER"
	// UnitcheckerPrintfIdentity is the versioned exact printf analyzer mode.
	UnitcheckerPrintfIdentity = "printf-v1"
	defaultUnitcheckerParallelism = 2
	maximumUnitcheckerOutputBytes = 64 << 20
	maximumUnitcheckerErrorBytes = 1 << 20
)

// UnitcheckerFactAnalyzerRunnerOptions configure the bounded Go unitchecker
// subprocess used for exact dependency-fact propagation.
type UnitcheckerFactAnalyzerRunnerOptions struct {
	Executable string
	GoBinary string
	Parallelism int
}

type unitcheckerFactAnalyzerRunner struct {
	executable string
	goBinary string
	parallelism int
	semaphore chan struct{}
}

type unitcheckerJSONEdit struct {
	Filename string `json:"filename"`
	Start int `json:"start"`
	End int `json:"end"`
	New string `json:"new"`
}

type unitcheckerJSONSuggestedFix struct {
	Message string `json:"message"`
	Edits []unitcheckerJSONEdit `json:"edits"`
}

type unitcheckerJSONDiagnostic struct {
	Posn string `json:"posn"`
	End string `json:"end"`
	Message string `json:"message"`
	SuggestedFixes []unitcheckerJSONSuggestedFix `json:"suggested_fixes"`
}

type boundedCommandBuffer struct {
	limit int
	bytes.Buffer
}

// RunUnitcheckerMode dispatches one hidden exact analyzer mode. The selected
// unitchecker owns process termination under the Go vet protocol.
func RunUnitcheckerMode(mode string) bool {
	if mode != UnitcheckerPrintfIdentity {
		return false
	}
	unitchecker.Main(printf.Analyzer)
	return true
}

func (b *boundedCommandBuffer) Write(input []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		return 0, fmt.Errorf("command output exceeds %d-byte limit", b.limit)
	}
	if len(input) > remaining {
		_, _ = b.Buffer.Write(input[:remaining])
		return remaining, fmt.Errorf("command output exceeds %d-byte limit", b.limit)
	}
	return b.Buffer.Write(input)
}

// NewUnitcheckerFactAnalyzerRunner creates one process-wide bounded runner.
func NewUnitcheckerFactAnalyzerRunner(
	options UnitcheckerFactAnalyzerRunnerOptions,
) (PackageFactAnalyzerRunner, error) {
	executable := filepath.Clean(options.Executable)
	if options.Executable == "" || !filepath.IsAbs(executable) {
		return nil, fmt.Errorf(
			"unitchecker executable %q is not a normalized absolute path",
			options.Executable,
		)
	}
	goBinary := options.GoBinary
	if goBinary == "" {
		goBinary = "go"
	}
	if strings.TrimSpace(goBinary) != goBinary || goBinary == "" {
		return nil, fmt.Errorf("unitchecker Go binary is empty or not canonical")
	}
	parallelism := options.Parallelism
	if parallelism == 0 {
		parallelism = defaultUnitcheckerParallelism
	}
	if parallelism < 1 || parallelism > 8 {
		return nil, fmt.Errorf(
			"unitchecker parallelism %d is outside the supported range 1-8",
			parallelism,
		)
	}
	return &unitcheckerFactAnalyzerRunner{
		executable: executable,
		goBinary: goBinary,
		parallelism: parallelism,
		semaphore: make(chan struct{}, 1),
	}, nil
}

func (r *unitcheckerFactAnalyzerRunner) RunPackageFactAnalyzer(
	ctx context.Context,
	request PackageFactAnalyzerRequest,
) ([]PackageFactAnalyzerDiagnostic, error) {
	if ctx == nil {
		return nil, fmt.Errorf("unitchecker execution requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Identity != UnitcheckerPrintfIdentity || request.Analyzer != "printf" {
		return nil, fmt.Errorf(
			"unsupported external fact analyzer %q (%q)",
			request.Analyzer,
			request.Identity,
		)
	}
	if !request.LoadOptions.Tests {
		return nil, fmt.Errorf("unitchecker execution requires test variants")
	}
	if err := validatePackageOverlay(request.LoadOptions.Overlay); err != nil {
		return nil, err
	}
	select {
	case r.semaphore <- struct{}{}:
		defer func() {
			<-r.semaphore
		}()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	buildFlags, err := packageBuildFlags(request.LoadOptions)
	if err != nil {
		return nil, err
	}
	overlayPath, cleanup, err := writeUnitcheckerOverlay(request.LoadOptions.Overlay)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	arguments := []string{
		"vet",
		"-json",
		"-p=" + strconv.Itoa(r.parallelism),
		"-vettool=" + r.executable,
	}
	arguments = append(arguments, buildFlags...)
	if overlayPath != "" {
		arguments = append(arguments, "-overlay=" + overlayPath)
	}
	arguments = append(arguments, request.LoadOptions.Patterns...)
	command := exec.CommandContext(ctx, r.goBinary, arguments...)
	command.Dir = request.LoadOptions.Dir
	command.Env = unitcheckerEnvironment(request.LoadOptions)
	stdout := &boundedCommandBuffer{limit: maximumUnitcheckerOutputBytes}
	stderr := &boundedCommandBuffer{limit: maximumUnitcheckerErrorBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	diagnostics, decodeErr := decodeUnitcheckerDiagnostics(
		stdout.Bytes(),
		request.Analyzer,
		request.Sources,
	)
	if decodeErr != nil {
		return nil, decodeErr
	}
	if runErr == nil {
		return diagnostics, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 && len(diagnostics) > 0 {
		return diagnostics, nil
	}
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 && request.PackageErrors {
		return []PackageFactAnalyzerDiagnostic{}, nil
	}
	message := strings.TrimSpace(stderr.String())
	if message == "" {
		message = runErr.Error()
	}
	return nil, fmt.Errorf("run external %s analyzer: %s", request.Analyzer, message)
}

func unitcheckerEnvironment(options PackageLoadOptions) []string {
	environment := packageLoadEnvironment(options)
	values := make(map[string]string, len(environment) + 1)
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found && name != "" {
			values[name] = value
		}
	}
	values[UnitcheckerModeEnvironment] = UnitcheckerPrintfIdentity
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, len(names))
	for index, name := range names {
		result[index] = name + "=" + values[name]
	}
	return result
}

func writeUnitcheckerOverlay(overlay map[string][]byte) (string, func(), error) {
	if len(overlay) == 0 {
		return "", func() {}, nil
	}
	root, err := os.MkdirTemp("", "glippy-unitchecker-overlay-*")
	if err != nil {
		return "", nil, fmt.Errorf("create unitchecker overlay: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(root)
	}
	paths := make([]string, 0, len(overlay))
	for path := range overlay {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	replacements := make(map[string]string, len(paths))
	for index, path := range paths {
		replacement := filepath.Join(root, strconv.Itoa(index) + ".go")
		if err := os.WriteFile(replacement, overlay[path], 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("write unitchecker overlay source: %w", err)
		}
		replacements[path] = replacement
	}
	encoded, err := json.Marshal(replacements)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("encode unitchecker overlay: %w", err)
	}
	path := filepath.Join(root, "overlay.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write unitchecker overlay manifest: %w", err)
	}
	return path, cleanup, nil
}

func decodeUnitcheckerDiagnostics(
	input []byte,
	analyzer string,
	sources PackageSourceSet,
) ([]PackageFactAnalyzerDiagnostic, error) {
	if len(bytes.TrimSpace(input)) == 0 {
		return []PackageFactAnalyzerDiagnostic{}, nil
	}
	type packageOutput struct {
		path string
		raw json.RawMessage
	}
	outputs := make([]packageOutput, 0)
	decoder := json.NewDecoder(bytes.NewReader(input))
	for {
		packages := make(map[string]map[string]json.RawMessage)
		if err := decoder.Decode(&packages); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode external %s diagnostics: %w", analyzer, err)
		}
		for packagePath, packageAnalyzers := range packages {
			raw, found := packageAnalyzers[analyzer]
			if found {
				outputs = append(
					outputs,
					packageOutput{path: packagePath, raw: raw},
				)
			}
		}
	}
	sort.Slice(
		outputs,
		func(left, right int) bool {
			return outputs[left].path < outputs[right].path
		},
	)
	result := make([]PackageFactAnalyzerDiagnostic, 0)
	seen := make(map[string]struct{})
	for _, output := range outputs {
		var diagnostics []unitcheckerJSONDiagnostic
		if err := json.Unmarshal(output.raw, &diagnostics); err != nil {
			return nil, fmt.Errorf(
				"decode external %s diagnostics for %q: %w",
				analyzer,
				output.path,
				err,
			)
		}
		for index, diagnostic := range diagnostics {
			mapped, found, err := mapUnitcheckerDiagnostic(
				analyzer,
				diagnostic,
				sources,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"external %s diagnostic %d for %q: %w",
					analyzer,
					index,
					output.path,
					err,
				)
			}
			if !found {
				continue
			}
			encoded, err := json.Marshal(mapped)
			if err != nil {
				return nil, fmt.Errorf("identify external diagnostic: %w", err)
			}
			identity := string(encoded)
			if _, duplicate := seen[identity]; duplicate {
				continue
			}
			seen[identity] = struct{}{}
			result = append(result, mapped)
		}
	}
	sort.Slice(
		result,
		func(left, right int) bool {
			if result[left].Path != result[right].Path {
				return result[left].Path < result[right].Path
			}
			if result[left].Range.Start != result[right].Range.Start {
				return result[left].Range.Start < result[right].Range.Start
			}
			if result[left].Range.End != result[right].Range.End {
				return result[left].Range.End < result[right].Range.End
			}
			return result[left].Message < result[right].Message
		},
	)
	return result, nil
}

func mapUnitcheckerDiagnostic(
	analyzer string,
	diagnostic unitcheckerJSONDiagnostic,
	sources PackageSourceSet,
) (PackageFactAnalyzerDiagnostic, bool, error) {
	path, start, found, err := unitcheckerPosition(diagnostic.Posn, sources)
	if err != nil || !found {
		return PackageFactAnalyzerDiagnostic{}, found, err
	}
	end := start
	if diagnostic.End != "" {
		endPath, resolved, endFound, err := unitcheckerPosition(diagnostic.End, sources)
		if err != nil {
			return PackageFactAnalyzerDiagnostic{}, false, err
		}
		if !endFound || endPath != path {
			return PackageFactAnalyzerDiagnostic{}, false, fmt.Errorf(
				"diagnostic positions do not belong to one retained source",
			)
		}
		end = resolved
	}
	file, _ := sources.Lookup(path)
	range_ := source.Range{Start: start, End: end}
	if _, valid := file.Slice(range_); !valid {
		return PackageFactAnalyzerDiagnostic{}, false, fmt.Errorf(
			"diagnostic maps to an invalid physical range",
		)
	}
	fixes := make([]PackageFactAnalyzerSuggestedFix, len(diagnostic.SuggestedFixes))
	for fixIndex, suggested := range diagnostic.SuggestedFixes {
		edits := make([]PackageFactAnalyzerEdit, len(suggested.Edits))
		for editIndex, edit := range suggested.Edits {
			editPath := filepath.Clean(edit.Filename)
			if editPath != path {
				return PackageFactAnalyzerDiagnostic{}, false, fmt.Errorf(
					"suggested fix %q edit %d belongs to another source file",
					suggested.Message,
					editIndex,
				)
			}
			editRange := source.Range{Start: edit.Start, End: edit.End}
			if _, valid := file.Slice(editRange); !valid {
				return PackageFactAnalyzerDiagnostic{}, false, fmt.Errorf(
					"suggested fix %q edit %d maps to an invalid physical range",
					suggested.Message,
					editIndex,
				)
			}
			edits[editIndex] = PackageFactAnalyzerEdit{
				Path: editPath,
				Range: editRange,
				NewText: edit.New,
			}
		}
		fixes[fixIndex] = PackageFactAnalyzerSuggestedFix{
			Message: suggested.Message,
			Edits: edits,
		}
	}
	return PackageFactAnalyzerDiagnostic{
		Analyzer: analyzer,
		Path: path,
		Range: range_,
		Message: diagnostic.Message,
		SuggestedFixes: fixes,
	}, true, nil
}

func unitcheckerPosition(position string, sources PackageSourceSet) (string, int, bool, error) {
	lastColon := strings.LastIndexByte(position, ':')
	if lastColon <= 0 {
		return "", 0, false, fmt.Errorf("invalid position %q", position)
	}
	previousColon := strings.LastIndexByte(position[:lastColon], ':')
	if previousColon <= 0 {
		return "", 0, false, fmt.Errorf("invalid position %q", position)
	}
	line, err := strconv.Atoi(position[previousColon + 1:lastColon])
	if err != nil || line < 1 {
		return "", 0, false, fmt.Errorf("invalid position line in %q", position)
	}
	column, err := strconv.Atoi(position[lastColon + 1:])
	if err != nil || column < 1 {
		return "", 0, false, fmt.Errorf("invalid position column in %q", position)
	}
	path := filepath.Clean(position[:previousColon])
	file, found := sources.Lookup(path)
	if !found {
		return path, 0, false, nil
	}
	input := file.Bytes()
	fileSet := token.NewFileSet()
	tokenFile := fileSet.AddFile(path, -1, len(input))
	tokenFile.SetLinesForContent(input)
	if line > tokenFile.LineCount() {
		return "", 0, false, fmt.Errorf("position line is outside %q", path)
	}
	lineStart := tokenFile.Offset(tokenFile.LineStart(line))
	lineEnd := len(input)
	if newline := bytes.IndexByte(input[lineStart:], '\n'); newline >= 0 {
		lineEnd = lineStart + newline
		if lineEnd > lineStart && input[lineEnd - 1] == '\r' {
			lineEnd--
		}
	}
	offset := lineStart + column - 1
	if offset > lineEnd {
		return "", 0, false, fmt.Errorf("position column is outside %q", path)
	}
	if _, valid := file.Slice(source.Range{Start: offset, End: offset}); !valid {
		return "", 0, false, fmt.Errorf("position column is outside %q", path)
	}
	return path, offset, true, nil
}
