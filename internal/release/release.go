// Package release builds deterministic prototype release artifacts.
package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

const (
	productName    = "gox"
	manifestSchema = 1
	linkedVersion  = "github.com/faustbrian/gox/internal/version.linked"
)

var defaultTargets = []Target{
	{GOOS: "darwin", GOARCH: "arm64"},
	{GOOS: "linux", GOARCH: "arm64"},
}

// Target identifies one release operating system and architecture.
type Target struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

// Options selects one deterministic release build.
type Options struct {
	Root           string
	Output         string
	Version        string
	SourceRevision string
	GoBinary       string
	GitBinary      string
	Targets        []Target
}

// Artifact records one archived binary and its content identity.
type Artifact struct {
	File   string `json:"file"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest binds release artifacts to source, version, and build toolchain.
type Manifest struct {
	SchemaVersion  int        `json:"schema_version"`
	Product        string     `json:"product"`
	Version        string     `json:"version"`
	SourceRevision string     `json:"source_revision"`
	GoVersion      string     `json:"go_version"`
	Artifacts      []Artifact `json:"artifacts"`
}

// DefaultTargets returns the platform/filesystem pairs with current runtime
// evidence for the prototype release.
func DefaultTargets() []Target {
	return append([]Target(nil), defaultTargets...)
}

// Build creates one new output directory containing archived binaries, a
// canonical manifest, and sorted SHA-256 checksums. It removes the directory if
// any step fails and never overwrites an existing path.
func Build(ctx context.Context, options Options) (manifest Manifest, resultErr error) {
	if ctx == nil {
		return manifest, errors.New("release build requires a context")
	}
	options, err := validateOptions(options)
	if err != nil {
		return manifest, err
	}
	if err := verifySource(ctx, options); err != nil {
		return manifest, err
	}
	sourceRoot, err := exportSource(ctx, options)
	if err != nil {
		return manifest, err
	}
	sourcePending := true
	defer func() {
		if sourcePending {
			if err := os.RemoveAll(sourceRoot); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("remove release source snapshot: %w", err))
			}
		}
	}()
	options.Root = sourceRoot
	if err := verifyModuleBoundary(sourceRoot); err != nil {
		return manifest, err
	}
	cacheRoot, err := os.MkdirTemp("", "gox-release-cache-")
	if err != nil {
		return manifest, fmt.Errorf("create release build cache: %w", err)
	}
	cachePending := true
	defer func() {
		if cachePending {
			if err := os.RemoveAll(cacheRoot); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("remove release build cache: %w", err))
			}
		}
	}()
	goVersion, err := queryGoVersion(ctx, options)
	if err != nil {
		return manifest, err
	}
	output, err := createOwnedOutput(options.Output)
	if err != nil {
		return manifest, err
	}
	complete := false
	defer func() {
		if !complete {
			if err := removeOwnedOutput(output); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("remove failed release output: %w", err))
			}
		}
	}()

	manifest = Manifest{
		SchemaVersion:  manifestSchema,
		Product:        productName,
		Version:        options.Version,
		SourceRevision: options.SourceRevision,
		GoVersion:      goVersion,
		Artifacts:      make([]Artifact, 0, len(options.Targets)),
	}
	for _, target := range options.Targets {
		artifact, err := buildTarget(ctx, options, output, target, cacheRoot)
		if err != nil {
			return Manifest{}, err
		}
		manifest.Artifacts = append(manifest.Artifacts, artifact)
	}
	manifestName := productName + "_" + options.Version + "_manifest.json"
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("encode release manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := writeExclusive(output.root, manifestName, manifestBytes, 0o644); err != nil {
		return Manifest{}, fmt.Errorf("write release manifest: %w", err)
	}
	checksums := make([]checksum, 0, len(manifest.Artifacts)+1)
	for _, artifact := range manifest.Artifacts {
		checksums = append(checksums, checksum{name: artifact.File, digest: artifact.SHA256})
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	checksums = append(checksums, checksum{name: manifestName, digest: hex.EncodeToString(manifestDigest[:])})
	sort.Slice(checksums, func(left, right int) bool { return checksums[left].name < checksums[right].name })
	var checksumText strings.Builder
	for _, item := range checksums {
		fmt.Fprintf(&checksumText, "%s  %s\n", item.digest, item.name)
	}
	checksumName := productName + "_" + options.Version + "_checksums.txt"
	if err := writeExclusive(
		output.root,
		checksumName,
		[]byte(checksumText.String()),
		0o644,
	); err != nil {
		return Manifest{}, fmt.Errorf("write release checksums: %w", err)
	}
	if err := output.close(); err != nil {
		return Manifest{}, fmt.Errorf("close release output before publication: %w", err)
	}
	if err := os.RemoveAll(cacheRoot); err != nil {
		return Manifest{}, fmt.Errorf("remove release build cache before publication: %w", err)
	}
	cachePending = false
	if err := os.RemoveAll(sourceRoot); err != nil {
		return Manifest{}, fmt.Errorf("remove release source snapshot before publication: %w", err)
	}
	sourcePending = false
	if err := output.publish(); err != nil {
		return Manifest{}, err
	}
	complete = true
	return manifest, nil
}

func exportSource(ctx context.Context, options Options) (result string, resultErr error) {
	root, err := os.MkdirTemp("", "gox-release-source-")
	if err != nil {
		return "", fmt.Errorf("create release source snapshot: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			if err := os.RemoveAll(root); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("remove failed release source snapshot: %w", err))
			}
		}
	}()
	command := exec.CommandContext(
		ctx,
		options.GitBinary,
		"-C",
		options.Root,
		"archive",
		"--format=tar",
		options.SourceRevision,
	)
	command.Env = gitEnvironment()
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("open release source export: %w", err)
	}
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("start release source export: %w", err)
	}
	extractErr := extractSourceArchive(root, output)
	if extractErr != nil {
		_ = output.Close()
	}
	waitErr := command.Wait()
	if extractErr != nil {
		return "", extractErr
	}
	if waitErr != nil {
		return "", fmt.Errorf("export release source: %w: %s", waitErr, stderr.Bytes())
	}
	keep = true
	return root, nil
}

func extractSourceArchive(root string, input io.Reader) error {
	archive := tar.NewReader(input)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read release source archive: %w", err)
		}
		name := filepath.Clean(header.Name)
		if name == "." || filepath.IsAbs(name) || name == ".." ||
			strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("release source archive path %q escapes snapshot", header.Name)
		}
		path := filepath.Join(root, name)
		switch header.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return fmt.Errorf("create release source directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create release source parent: %w", err)
			}
			mode := os.FileMode(header.Mode).Perm()
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return fmt.Errorf("create release source file: %w", err)
			}
			_, copyErr := io.CopyN(file, archive, header.Size)
			closeErr := file.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return fmt.Errorf("write release source file: %w", err)
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("release source symlink %q has an absolute target", header.Name)
			}
			target := filepath.Clean(filepath.Join(filepath.Dir(name), header.Linkname))
			if target == ".." || strings.HasPrefix(target, ".."+string(filepath.Separator)) {
				return fmt.Errorf("release source symlink %q escapes snapshot", header.Name)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create release source symlink parent: %w", err)
			}
			if err := os.Symlink(header.Linkname, path); err != nil {
				return fmt.Errorf("create release source symlink: %w", err)
			}
		default:
			return fmt.Errorf("release source archive entry %q has unsupported type %d", header.Name, header.Typeflag)
		}
	}
}

type ownedOutput struct {
	path      string
	finalPath string
	root      *os.Root
	identity  os.FileInfo
	closed    bool
	published bool
}

func createOwnedOutput(path string) (*ownedOutput, error) {
	temporary, err := os.MkdirTemp(filepath.Dir(path), ".gox-release-output-")
	if err != nil {
		return nil, fmt.Errorf("create private release output: %w", err)
	}
	root, err := os.OpenRoot(temporary)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("pin private release output: %w", err),
			os.Remove(temporary),
		)
	}
	if err := root.Chmod(".", 0o755); err != nil {
		return nil, errors.Join(
			fmt.Errorf("set release output permissions: %w", err),
			removePrivateOutput(temporary, root),
		)
	}
	identity, err := root.Stat(".")
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("inspect pinned release output: %w", err),
			removePrivateOutput(temporary, root),
		)
	}
	output := &ownedOutput{path: temporary, finalPath: path, root: root, identity: identity}
	if err := output.validateIdentity(); err != nil {
		return nil, errors.Join(err, removeOwnedOutput(output))
	}
	return output, nil
}

func (output *ownedOutput) validateIdentity() error {
	current, err := os.Lstat(output.path)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("release output identity changed; output path is missing")
	}
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(output.identity, current) {
		return errors.New("release output identity changed")
	}
	return nil
}

func (output *ownedOutput) publish() error {
	if err := output.validateIdentity(); err != nil {
		return err
	}
	if err := publishOutput(output.path, output.finalPath); err != nil {
		return fmt.Errorf("publish release output without replacement: %w", err)
	}
	output.published = true
	return nil
}

func (output *ownedOutput) close() error {
	if output.closed {
		return nil
	}
	output.closed = true
	return output.root.Close()
}

func removeOwnedOutput(output *ownedOutput) error {
	if output.published {
		return errors.New("refusing to remove published release output")
	}
	if output.closed {
		if err := output.validateIdentity(); err != nil {
			return err
		}
		return os.RemoveAll(output.path)
	}
	entries, readErr := fs.ReadDir(output.root.FS(), ".")
	var cleanupErr error
	for _, entry := range entries {
		cleanupErr = errors.Join(cleanupErr, output.root.RemoveAll(entry.Name()))
	}
	identityErr := output.validateIdentity()
	closeErr := output.close()
	if readErr != nil || cleanupErr != nil || identityErr != nil || closeErr != nil {
		return errors.Join(readErr, cleanupErr, identityErr, closeErr)
	}
	return os.Remove(output.path)
}

func removePrivateOutput(path string, root *os.Root) error {
	entries, readErr := fs.ReadDir(root.FS(), ".")
	var cleanupErr error
	for _, entry := range entries {
		cleanupErr = errors.Join(cleanupErr, root.RemoveAll(entry.Name()))
	}
	return errors.Join(readErr, cleanupErr, root.Close(), os.Remove(path))
}

type checksum struct {
	name   string
	digest string
}

func validateOptions(options Options) (Options, error) {
	if options.Root == "" || options.Output == "" {
		return Options{}, errors.New("release root and output are required")
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return Options{}, fmt.Errorf("resolve release root: %w", err)
	}
	information, err := os.Stat(root)
	if err != nil {
		return Options{}, fmt.Errorf("inspect release root: %w", err)
	}
	if !information.IsDir() {
		return Options{}, errors.New("release root is not a directory")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Options{}, fmt.Errorf("resolve release root symlinks: %w", err)
	}
	output, err := filepath.Abs(options.Output)
	if err != nil {
		return Options{}, fmt.Errorf("resolve release output: %w", err)
	}
	if _, err := os.Lstat(output); err == nil {
		return Options{}, fmt.Errorf("release output %q already exists", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Options{}, fmt.Errorf("inspect release output: %w", err)
	}
	if semver.Canonical(options.Version) != options.Version {
		return Options{}, fmt.Errorf("release version %q is not canonical semantic version", options.Version)
	}
	revision, err := hex.DecodeString(options.SourceRevision)
	if err != nil || len(revision) != 20 && len(revision) != 32 ||
		strings.ToLower(options.SourceRevision) != options.SourceRevision {
		return Options{}, errors.New("source revision must be a lowercase 40- or 64-character hexadecimal digest")
	}
	if options.GoBinary == "" {
		options.GoBinary = "go"
	}
	if options.GitBinary == "" {
		options.GitBinary = "git"
	}
	if len(options.Targets) == 0 {
		options.Targets = DefaultTargets()
	} else {
		options.Targets = append([]Target(nil), options.Targets...)
	}
	sort.Slice(options.Targets, func(left, right int) bool {
		if options.Targets[left].GOOS != options.Targets[right].GOOS {
			return options.Targets[left].GOOS < options.Targets[right].GOOS
		}
		return options.Targets[left].GOARCH < options.Targets[right].GOARCH
	})
	for index, target := range options.Targets {
		if !supportedTarget(target) {
			return Options{}, fmt.Errorf("unsupported release target %s/%s", target.GOOS, target.GOARCH)
		}
		if index > 0 && target == options.Targets[index-1] {
			return Options{}, fmt.Errorf("duplicate release target %s/%s", target.GOOS, target.GOARCH)
		}
	}
	options.Root = root
	options.Output = output
	return options, nil
}

func verifySource(ctx context.Context, options Options) error {
	revisionCommand := exec.CommandContext(
		ctx,
		options.GitBinary,
		"-C",
		options.Root,
		"rev-parse",
		"--verify",
		"HEAD",
	)
	revisionCommand.Env = gitEnvironment()
	revision, err := revisionCommand.Output()
	if err != nil {
		return fmt.Errorf("resolve release source revision: %w", err)
	}
	if strings.TrimSpace(string(revision)) != options.SourceRevision {
		return errors.New("release source revision does not match repository HEAD")
	}
	statusCommand := exec.CommandContext(
		ctx,
		options.GitBinary,
		"-C",
		options.Root,
		"status",
		"--porcelain=v1",
		"--untracked-files=all",
		"--ignored=matching",
	)
	statusCommand.Env = gitEnvironment()
	status, err := statusCommand.Output()
	if err != nil {
		return fmt.Errorf("inspect release source state: %w", err)
	}
	if len(status) != 0 {
		return errors.New("release source working tree is not clean")
	}
	return nil
}

func verifyModuleBoundary(root string) error {
	contents, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return fmt.Errorf("read release module: %w", err)
	}
	module, err := modfile.Parse("go.mod", contents, nil)
	if err != nil {
		return fmt.Errorf("parse release module: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve release module root: %w", err)
	}
	for _, replacement := range module.Replace {
		if replacement.New.Version != "" {
			continue
		}
		target := replacement.New.Path
		if !filepath.IsAbs(target) {
			target = filepath.Join(root, target)
		}
		resolvedTarget, err := filepath.EvalSymlinks(target)
		if err != nil {
			return fmt.Errorf("resolve local module replacement %q: %w", replacement.New.Path, err)
		}
		relative, err := filepath.Rel(resolvedRoot, resolvedTarget)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("local module replacement %q is outside release root", replacement.New.Path)
		}
	}
	return nil
}

func supportedTarget(target Target) bool {
	for _, supported := range defaultTargets {
		if target == supported {
			return true
		}
	}
	return false
}

func queryGoVersion(ctx context.Context, options Options) (string, error) {
	command := exec.CommandContext(ctx, options.GoBinary, "env", "GOVERSION")
	command.Dir = options.Root
	command.Env = buildEnvironment("", "")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("query release Go version: %w", err)
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", errors.New("release Go version is empty")
	}
	return version, nil
}

func buildTarget(
	ctx context.Context,
	options Options,
	output *ownedOutput,
	target Target,
	cacheRoot string,
) (artifact Artifact, resultErr error) {
	temporary, err := os.MkdirTemp("", "gox-release-build-")
	if err != nil {
		return Artifact{}, fmt.Errorf("create release build directory: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, os.RemoveAll(temporary)) }()
	binary := filepath.Join(temporary, productName)
	linkerFlags := "-s -w -X " + linkedVersion + "=" + options.Version
	command := exec.CommandContext(
		ctx,
		options.GoBinary,
		"build",
		"-mod=readonly",
		"-trimpath",
		"-buildvcs=false",
		"-ldflags="+linkerFlags,
		"-o",
		binary,
		"./cmd/gox",
	)
	command.Dir = options.Root
	command.Env = buildEnvironment(target.GOOS, target.GOARCH, cacheRoot)
	if output, err := command.CombinedOutput(); err != nil {
		return Artifact{}, fmt.Errorf("build release target %s/%s: %w: %s", target.GOOS, target.GOARCH, err, output)
	}
	name := fmt.Sprintf("%s_%s_%s_%s.tar.gz", productName, options.Version, target.GOOS, target.GOARCH)
	var encoded bytes.Buffer
	if err := archiveBinary(&encoded, binary); err != nil {
		return Artifact{}, fmt.Errorf("archive release target %s/%s: %w", target.GOOS, target.GOARCH, err)
	}
	if err := writeExclusive(output.root, name, encoded.Bytes(), 0o644); err != nil {
		return Artifact{}, fmt.Errorf("write release target %s/%s: %w", target.GOOS, target.GOARCH, err)
	}
	digest := sha256.Sum256(encoded.Bytes())
	return Artifact{
		File: name, GOOS: target.GOOS, GOARCH: target.GOARCH,
		SHA256: hex.EncodeToString(digest[:]), Size: int64(encoded.Len()),
	}, nil
}

func buildEnvironment(goos, goarch string, cacheRoot ...string) []string {
	values := make(map[string]string)
	for _, name := range []string{"HOME", "PATH", "TMPDIR"} {
		if value, found := os.LookupEnv(name); found {
			values[name] = value
		}
	}
	values["CGO_ENABLED"] = "0"
	values["GOCACHEPROG"] = ""
	values["GODEBUG"] = ""
	values["GOENV"] = "off"
	values["GOFLAGS"] = ""
	values["GOFIPS140"] = "off"
	values["GOTOOLCHAIN"] = "local"
	values["GOWORK"] = "off"
	values["GOEXPERIMENT"] = ""
	values["GOARM64"] = "v8.0"
	if len(cacheRoot) > 0 && cacheRoot[0] != "" {
		values["GOCACHE"] = cacheRoot[0]
	}
	if goos != "" {
		values["GOOS"] = goos
	}
	if goarch != "" {
		values["GOARCH"] = goarch
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}

func gitEnvironment() []string {
	values := make(map[string]string)
	for _, name := range []string{"HOME", "PATH", "TMPDIR"} {
		if value, found := os.LookupEnv(name); found {
			values[name] = value
		}
	}
	values["GIT_CONFIG_GLOBAL"] = os.DevNull
	values["GIT_CONFIG_NOSYSTEM"] = "1"
	values["LC_ALL"] = "C"
	values["TZ"] = "UTC"
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}

func archiveBinary(output io.Writer, binary string) (resultErr error) {
	input, err := os.Open(binary)
	if err != nil {
		return err
	}
	defer input.Close()
	information, err := input.Stat()
	if err != nil {
		return err
	}
	compressed, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		return err
	}
	compressed.Header.ModTime = time.Unix(0, 0).UTC()
	compressed.Header.OS = 255
	archiveWriter := tar.NewWriter(compressed)
	header := &tar.Header{
		Name: productName, Mode: 0o755, Size: information.Size(),
		ModTime: time.Unix(0, 0).UTC(), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR,
	}
	if err := archiveWriter.WriteHeader(header); err != nil {
		return err
	}
	if _, err := io.Copy(archiveWriter, input); err != nil {
		return err
	}
	if err := archiveWriter.Close(); err != nil {
		return err
	}
	if err := compressed.Close(); err != nil {
		return err
	}
	return nil
}

func writeExclusive(root *os.Root, path string, content []byte, mode os.FileMode) (resultErr error) {
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	if _, err := file.Write(content); err != nil {
		return err
	}
	return file.Sync()
}
