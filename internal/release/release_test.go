package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildProducesReproducibleVersionedArtifacts(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" ||
		runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("prototype release targets are Darwin and Linux amd64 and arm64")
	}

	parent := t.TempDir()
	root, revision := committedFixture(t, filepath.Clean(filepath.Join("..", "..")))
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "wrong-git-dir"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "wrong-index"))
	options := Options{Root: root, Version: "v0.0.0-test", SourceRevision: revision}
	outputs := []string{filepath.Join(parent, "first"), filepath.Join(parent, "second")}
	manifests := make([]Manifest, len(outputs))
	for index, output := range outputs {
		options.Output = output
		manifest, err := Build(context.Background(), options)
		if err != nil {
			t.Fatal(err)
		}
		manifests[index] = manifest
	}
	if len(manifests[0].Artifacts) != len(DefaultTargets()) ||
		!reflect.DeepEqual(manifests[0], manifests[1]) {
		t.Fatalf("manifests differ: %#v != %#v", manifests[0], manifests[1])
	}
	if manifests[0].SchemaVersion != 1 ||
		manifests[0].Product != "glippy" ||
		manifests[0].Version != "v0.0.0-test" ||
		manifests[0].SourceRevision != revision ||
		manifests[0].GoVersion == "" {
		t.Fatalf("manifest identity = %#v", manifests[0])
	}
	wantTargets := map[Target]bool{
		{GOOS: "darwin", GOARCH: "amd64"}: true,
		{GOOS: "darwin", GOARCH: "arm64"}: true,
		{GOOS: "linux", GOARCH: "amd64"}: true,
		{GOOS: "linux", GOARCH: "arm64"}: true,
	}
	for _, artifact := range manifests[0].Artifacts {
		target := Target{GOOS: artifact.GOOS, GOARCH: artifact.GOARCH}
		if !wantTargets[target] {
			t.Fatalf("manifest contains unexpected target %#v", target)
		}
		delete(wantTargets, target)
	}
	if len(wantTargets) != 0 {
		t.Fatalf("manifest is missing targets %#v", wantTargets)
	}
	notices, err := os.ReadFile(filepath.Join(root, thirdPartyNoticesName))
	if err != nil {
		t.Fatal(err)
	}
	license, err := os.ReadFile(filepath.Join(root, projectLicenseName))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"glippy_v0.0.0-test_manifest.json", "glippy_v0.0.0-test_checksums.txt"}
	for _, artifact := range manifests[0].Artifacts {
		names = append(names, artifact.File)
		verifyArchiveMaterials(
			t,
			filepath.Join(outputs[0], artifact.File),
			license,
			notices,
		)
	}
	for _, name := range names {
		first, err := os.ReadFile(filepath.Join(outputs[0], name))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(outputs[1], name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("artifact %q is not reproducible", name)
		}
	}
	entries, err := os.ReadDir(outputs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(names) {
		t.Fatalf("release output entries = %#v", entries)
	}
	verifyChecksums(t, outputs[0])
	manifestBytes, err := os.ReadFile(
		filepath.Join(outputs[0], "glippy_v0.0.0-test_manifest.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	if err := json.Unmarshal(manifestBytes, &decoded);
		err != nil || !reflect.DeepEqual(decoded, manifests[0]) {
		t.Fatalf("decode manifest = %#v, %v", decoded, err)
	}

	var current Artifact
	for _, artifact := range manifests[0].Artifacts {
		if artifact.GOOS == runtime.GOOS && artifact.GOARCH == runtime.GOARCH {
			current = artifact
		}
	}
	if current == (Artifact{}) {
		t.Fatal("manifest does not contain the current runtime target")
	}
	binary := extractBinary(t, filepath.Join(outputs[0], current.File), license, notices)
	command := exec.Command(binary, "version")
	got, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run released binary: %v: %s", err, got)
	}
	if string(got) != "glippy v0.0.0-test\n" {
		t.Fatalf("released version = %q", got)
	}
}

func TestBuildRejectsExistingOutputWithoutMutation(t *testing.T) {
	output := t.TempDir()
	sentinel := filepath.Join(output, "owned")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Build(
		context.Background(),
		Options{
			Root: filepath.Clean(filepath.Join("..", "..")),
			Output: output,
			Version: "v0.0.0-test",
			SourceRevision: strings.Repeat("a", 40),
			Targets: []Target{{GOOS: "darwin", GOARCH: "arm64"}},
		},
	)
	if err == nil {
		t.Fatal("Build() accepted an existing output directory")
	}
	got, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(got) != "keep" {
		t.Fatalf("existing output changed: %q, %v", got, readErr)
	}
}

func TestBuildRemovesOwnedOutputAfterBuildFailure(t *testing.T) {
	root, _ := committedFixture(t, filepath.Clean(filepath.Join("..", "..")))
	if err := os.Remove(filepath.Join(root, "cmd", "glippy", "main.go")); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, root, "add", ".")
	runFixtureGit(
		t,
		root,
		"-c",
		"user.name=Glippy Release Test",
		"-c",
		"user.email=glippy-release-test@example.invalid",
		"commit",
		"--quiet",
		"-m",
		"remove product command",
	)
	revisionCommand := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	revisionOutput, err := revisionCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "release")
	_, err = Build(
		context.Background(),
		Options{
			Root: root,
			Output: output,
			Version: "v0.0.0-test",
			SourceRevision: strings.TrimSpace(string(revisionOutput)),
			Targets: []Target{{GOOS: "darwin", GOARCH: "arm64"}},
		},
	)
	if err == nil {
		t.Fatal("Build() succeeded without the product command")
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		t.Fatalf("failed build retained output: %v", statErr)
	}
}

func TestBuildRejectsSourceWithoutThirdPartyNotices(t *testing.T) {
	root, revision := committedFixture(t, filepath.Clean(filepath.Join("..", "..")))
	notices := filepath.Join(root, "THIRD_PARTY_LICENSES.txt")
	if err := os.Remove(notices); err == nil {
		runFixtureGit(t, root, "add", "THIRD_PARTY_LICENSES.txt")
		runFixtureGit(
			t,
			root,
			"-c",
			"user.name=Glippy Release Test",
			"-c",
			"user.email=glippy-release-test@example.invalid",
			"commit",
			"--quiet",
			"-m",
			"remove third-party notices",
		)
		revisionCommand := exec.Command("git", "-C", root, "rev-parse", "HEAD")
		revisionOutput, revisionErr := revisionCommand.Output()
		if revisionErr != nil {
			t.Fatal(revisionErr)
		}
		revision = strings.TrimSpace(string(revisionOutput))
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "release")
	_, err := Build(
		context.Background(),
		Options{
			Root: root,
			Output: output,
			Version: "v0.0.0-test",
			SourceRevision: revision,
			Targets: []Target{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}},
		},
	)
	if err == nil {
		t.Fatal("Build() accepted a source snapshot without third-party notices")
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		t.Fatalf("rejected source created output: %v", statErr)
	}
}

func TestBuildRejectsSourceWithoutProjectLicense(t *testing.T) {
	root, revision := committedFixture(t, filepath.Clean(filepath.Join("..", "..")))
	license := filepath.Join(root, projectLicenseName)
	if err := os.Remove(license); err == nil {
		runFixtureGit(t, root, "add", projectLicenseName)
		runFixtureGit(
			t,
			root,
			"-c",
			"user.name=Glippy Release Test",
			"-c",
			"user.email=glippy-release-test@example.invalid",
			"commit",
			"--quiet",
			"-m",
			"remove project license",
		)
		revisionCommand := exec.Command("git", "-C", root, "rev-parse", "HEAD")
		revisionOutput, revisionErr := revisionCommand.Output()
		if revisionErr != nil {
			t.Fatal(revisionErr)
		}
		revision = strings.TrimSpace(string(revisionOutput))
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "release")
	_, err := Build(
		context.Background(),
		Options{
			Root: root,
			Output: output,
			Version: "v0.0.0-test",
			SourceRevision: revision,
			Targets: []Target{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}},
		},
	)
	if err == nil {
		t.Fatal("Build() accepted a source snapshot without a project license")
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		t.Fatalf("rejected source created output: %v", statErr)
	}
}

func TestBuildRejectsDirtyOrMismatchedSourceBeforeCreatingOutput(t *testing.T) {
	root, _ := committedFixture(t, filepath.Clean(filepath.Join("..", "..")))
	if err := os.WriteFile(
		filepath.Join(root, ".gitignore"),
		[]byte(".release-ignored\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, root, "add", ".gitignore")
	runFixtureGit(
		t,
		root,
		"-c",
		"user.name=Glippy Release Test",
		"-c",
		"user.email=glippy-release-test@example.invalid",
		"commit",
		"--quiet",
		"-m",
		"add release ignore fixture",
	)
	revisionCommand := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	revisionOutput, err := revisionCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.TrimSpace(string(revisionOutput))
	for _, test := range
		[]struct {
			name string
			revision string
			dirtyPath string
		}{
			{name: "mismatched revision", revision: strings.Repeat("b", 40)},
			{name: "untracked source", revision: revision, dirtyPath: "untracked"},
			{name: "ignored source", revision: revision, dirtyPath: ".release-ignored"},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				if test.dirtyPath != "" {
					path := filepath.Join(root, test.dirtyPath)
					if err := os.WriteFile(path, []byte("dirty"), 0o600);
						err != nil {
						t.Fatal(err)
					}
					defer os.Remove(path)
				}
				output := filepath.Join(t.TempDir(), "release")
				_, err := Build(
					context.Background(),
					Options{
						Root: root,
						Output: output,
						Version: "v0.0.0-test",
						SourceRevision: test.revision,
						Targets: []Target{
							{GOOS: "darwin", GOARCH: "arm64"},
						},
					},
				)
				if err == nil {
					t.Fatalf("Build() accepted %s", test.name)
				}
				if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
					t.Fatalf("rejected source created output: %v", statErr)
				}
			},
		)
	}
}

func TestDefaultTargetsAreAdmittedPrototypePlatforms(t *testing.T) {
	t.Parallel()

	want := []Target{
		{GOOS: "darwin", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "linux", GOARCH: "arm64"},
	}
	got := DefaultTargets()
	if len(got) != len(want) {
		t.Fatalf("DefaultTargets() = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf(
				"DefaultTargets()[%d] = %#v, want %#v",
				index,
				got[index],
				want[index],
			)
		}
	}
	got[0] = Target{}
	if DefaultTargets()[0] != want[0] {
		t.Fatal("DefaultTargets() returned mutable shared state")
	}
}

func TestVerifyModuleBoundaryRejectsExternalLocalReplacement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	writeModule := func(replacement string) {
		t.Helper()
		contents := []byte(
			"module example.com/release\n\ngo 1.26.0\n\nreplace example.com/dependency => " +
				replacement +
				"\n",
		)
		if err := os.WriteFile(filepath.Join(root, "go.mod"), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeModule("./inside")
	if err := verifyModuleBoundary(root); err != nil {
		t.Fatalf("internal replacement rejected: %v", err)
	}
	writeModule(t.TempDir())
	if err := verifyModuleBoundary(root); err == nil {
		t.Fatal("external local replacement was accepted")
	}
}

func TestExportSourcePinsCommittedBytes(t *testing.T) {
	root, revision := committedFixture(t, filepath.Clean(filepath.Join("..", "..")))
	snapshot, err := exportSource(
		context.Background(),
		Options{Root: root, SourceRevision: revision, GitBinary: "git"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(snapshot)
	want, err := os.ReadFile(filepath.Join(snapshot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("mutated after export\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(snapshot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("source snapshot followed a working-tree mutation")
	}
	if _, err := os.Lstat(filepath.Join(snapshot, ".git")); !os.IsNotExist(err) {
		t.Fatalf("source snapshot contains repository metadata: %v", err)
	}
}

func TestOwnedOutputPinsWritesAndRefusesReplacement(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "release")
	owned, err := createOwnedOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.close()
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(owned.path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(owned.path, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(owned.path, "owned-by-someone-else")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(owned.root, "artifact", []byte("pinned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(moved, "artifact"));
		err != nil || string(got) != "pinned" {
		t.Fatalf("pinned output write = %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(owned.path, "artifact")); !os.IsNotExist(err) {
		t.Fatalf("replacement received artifact: %v", err)
	}
	if err := removeOwnedOutput(owned); err == nil {
		t.Fatal("removeOwnedOutput() accepted a replacement directory")
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep" {
		t.Fatalf("replacement output changed: %q, %v", got, err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("original output disappeared: %v", err)
	}
}

func TestOwnedOutputPublishesWithoutReplacingExistingPath(t *testing.T) {
	output := filepath.Join(t.TempDir(), "release")
	owned, err := createOwnedOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.close()
	if err := writeExclusive(owned.root, "artifact", []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(output, "owned-by-someone-else")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := owned.publish(); err == nil {
		t.Fatal("publish replaced an existing output path")
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep" {
		t.Fatalf("existing output changed: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(owned.path, "artifact"));
		err != nil || string(got) != "release" {
		t.Fatalf("private output changed: %q, %v", got, err)
	}
	if err := removeOwnedOutput(owned); err != nil {
		t.Fatal(err)
	}
}

func TestOwnedOutputCleansPrivateDirectoryAfterClose(t *testing.T) {
	output := filepath.Join(t.TempDir(), "release")
	owned, err := createOwnedOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	private := owned.path
	if err := writeExclusive(owned.root, "artifact", []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := owned.close(); err != nil {
		t.Fatal(err)
	}
	if err := removeOwnedOutput(owned); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(private); !os.IsNotExist(err) {
		t.Fatalf("private output retained after cleanup: %v", err)
	}
}

func TestBuildEnvironmentPinsBuildAffectingSettings(t *testing.T) {
	t.Setenv("GOCACHEPROG", "ambient-cache --unsafe")
	t.Setenv("GOMODCACHE", "/tmp/release-module-cache")
	t.Setenv("GOFIPS140", "v1.0.0")
	t.Setenv("GLIPPY_RELEASE_UNRELATED", "ambient")

	values := make(map[string]string)
	for _, entry := range buildEnvironment("linux", "arm64", "/tmp/release-cache") {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			t.Fatalf("environment entry = %q", entry)
		}
		values[name] = value
	}
	if values["GOCACHE"] != "/tmp/release-cache" || values["GOCACHEPROG"] != "" {
		t.Fatalf("cache environment = %#v", values)
	}
	if values["GOMODCACHE"] != "/tmp/release-module-cache" {
		t.Fatalf("module cache environment = %#v", values)
	}
	if values["GOFIPS140"] != "off" {
		t.Fatalf("GOFIPS140 = %q", values["GOFIPS140"])
	}
	if values["GOAMD64"] != "v1" || values["GOARM64"] != "v8.0" {
		t.Fatalf("architecture environment = %#v", values)
	}
	if _, found := values["GLIPPY_RELEASE_UNRELATED"]; found {
		t.Fatalf("unrelated ambient setting leaked into build: %#v", values)
	}
}

func committedFixture(t *testing.T, root string) (string, string) {
	t.Helper()

	fixture := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	listed := exec.Command(
		"git",
		"-C",
		root,
		"ls-files",
		"--cached",
		"--others",
		"--exclude-standard",
		"-z",
	)
	output, err := listed.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, encoded := range bytes.Split(output, []byte{0}) {
		if len(encoded) == 0 {
			continue
		}
		name := string(encoded)
		source := filepath.Join(root, name)
		information, err := os.Stat(source)
		if err != nil {
			t.Fatal(err)
		}
		if !information.Mode().IsRegular() {
			continue
		}
		contents, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(fixture, name)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, contents, information.Mode().Perm());
			err != nil {
			t.Fatal(err)
		}
	}
	runFixtureGit(t, fixture, "init", "--quiet")
	runFixtureGit(t, fixture, "add", ".")
	runFixtureGit(
		t,
		fixture,
		"-c",
		"user.name=Glippy Release Test",
		"-c",
		"user.email=glippy-release-test@example.invalid",
		"commit",
		"--quiet",
		"-m",
		"test fixture",
	)
	revision := exec.Command("git", "-C", fixture, "rev-parse", "HEAD")
	revisionOutput, err := revision.Output()
	if err != nil {
		t.Fatal(err)
	}
	return fixture, strings.TrimSpace(string(revisionOutput))
}

func runFixtureGit(t *testing.T, root string, arguments ...string) {
	t.Helper()

	arguments = append([]string{"-C", root}, arguments...)
	command := exec.Command("git", arguments...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func verifyChecksums(t *testing.T, output string) {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(output, "glippy_v0.0.0-test_checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) != len(DefaultTargets()) + 1 {
		t.Fatalf("checksum lines = %#v", lines)
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("checksum line = %q", line)
		}
		artifact, err := os.ReadFile(filepath.Join(output, fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(artifact)
		if hex.EncodeToString(digest[:]) != fields[0] {
			t.Fatalf("checksum for %q is invalid", fields[1])
		}
	}
}

func verifyArchiveMaterials(t *testing.T, archivePath string, license, notices []byte) {
	t.Helper()

	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	if !compressed.ModTime.IsZero() || compressed.OS != 255 {
		t.Fatalf("gzip header = %#v", compressed.Header)
	}
	reader := tar.NewReader(compressed)
	want := []struct {
		name string
		mode int64
		content []byte
	}{
		{name: "glippy", mode: 0o755},
		{name: projectLicenseName, mode: 0o644, content: license},
		{name: thirdPartyNoticesName, mode: 0o644, content: notices},
	}
	for _, expected := range want {
		header, err := reader.Next()
		if err != nil {
			t.Fatalf("read archive entry %q: %v", expected.name, err)
		}
		if header.Name != expected.name ||
			header.Mode != expected.mode ||
			header.Typeflag != tar.TypeReg ||
			header.Format != tar.FormatUSTAR ||
			header.Uid != 0 ||
			header.Gid != 0 ||
			header.Uname != "" ||
			header.Gname != "" ||
			!header.ModTime.Equal(time.Unix(0, 0).UTC()) {
			t.Fatalf("archive header for %q = %#v", expected.name, header)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if expected.name == "glippy" {
			verifyBinaryNoticeCoverage(t, content, notices)
		}
		if expected.content != nil && !bytes.Equal(content, expected.content) {
			t.Fatalf(
				"archive content for %q differs from source notices",
				expected.name,
			)
		}
	}
	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("archive contains unexpected entry: %v", err)
	}
}

func verifyBinaryNoticeCoverage(t *testing.T, binary, notices []byte) {
	t.Helper()

	information, err := buildinfo.Read(bytes.NewReader(binary))
	if err != nil {
		t.Fatalf("read released binary build information: %v", err)
	}
	if !bytes.Contains(notices, []byte("The Go toolchain and standard library")) {
		t.Fatal("third-party notices do not identify the Go toolchain")
	}
	for _, dependency := range information.Deps {
		if dependency.Replace != nil {
			dependency = dependency.Replace
		}
		identity := dependency.Path + " " + dependency.Version
		if !bytes.Contains(notices, []byte(identity)) {
			t.Fatalf("third-party notices do not identify linked module %s", identity)
		}
	}
}

func extractBinary(t *testing.T, archive string, license, notices []byte) string {
	t.Helper()

	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	header, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "glippy" || header.Mode != 0o755 {
		t.Fatalf("archive header = %#v", header)
	}
	binary := filepath.Join(t.TempDir(), "glippy")
	contents, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	header, err = reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != projectLicenseName || header.Mode != 0o644 {
		t.Fatalf("license archive header = %#v", header)
	}
	gotLicense, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotLicense, license) {
		t.Fatal("released project license differs from source license")
	}
	header, err = reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != thirdPartyNoticesName || header.Mode != 0o644 {
		t.Fatalf("notice archive header = %#v", header)
	}
	gotNotices, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotNotices, notices) {
		t.Fatal("released notices differ from source notices")
	}
	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("archive contains unexpected entry: %v", err)
	}
	return binary
}
