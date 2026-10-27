package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type depsLock struct {
	Version string            `json:"version"`
	BaseURL string            `json:"base_url"`
	Assets  map[string]string `json:"assets"`
}

type builder struct {
	goos   string
	goarch string
	lint   bool

	sourceDir  string
	cacheDir   string
	test       bool
	workspace  string
	outDir     string
	depsDir    string
	archiveDir string
	localDeps  string

	staticDir  string
	agentPath  string
	pkgCfgPath string

	deps depsLock
}

func main() {
	verbose := flag.Bool("v", false, "enable debug logging")
	runTests := flag.Bool("test", false, "run unit and race tests with the pinned native dependencies")
	runLint := flag.Bool("lint", false, "run golangci-lint before building")
	localDeps := flag.String("deps-dir", "", "use locally built libkrun and libkrunfw archives from this directory")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run ./scripts [-v] [--lint] [--test]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	b, err := newBuilder(*runLint)
	if err != nil {
		logrus.Fatal(err)
	}
	b.test = *runTests
	if *localDeps != "" {
		b.localDeps, err = filepath.Abs(*localDeps)
		if err != nil {
			logrus.Fatal(err)
		}
	}

	if err := b.run(); err != nil {
		logrus.Fatal(err)
	}
}

func (b *builder) run() error {
	// One checkout has one output publisher. Other checkouts share only immutable
	// dependency objects; no build writes into another build's source or aliases.
	output := filepath.Join(b.sourceDir, "out")
	if err := mkdirAll(output); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(output, ".build.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("another build is using %s: %w", output, err)
	}
	stage, err := os.MkdirTemp(output, ".build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	b.workspace = stage
	b.outDir = filepath.Join(stage, "out")
	b.depsDir = filepath.Join(stage, ".deps")
	b.staticDir = filepath.Join(stage, "pkg", "static_resources")
	b.agentPath = filepath.Join(b.staticDir, "guest_agent", "guest-agent")
	for _, name := range []string{"cmd", "pkg", "include", "scripts", "docs", "tests", "go.mod", "go.sum", "deps.lock", "revm.entitlements", "README.md", "LICENSE", ".golangci.yml"} {
		src := filepath.Join(b.sourceDir, name)
		if !exists(src) {
			continue
		}
		if err := command(nil, "cp", "-a", src, filepath.Join(stage, name)); err != nil {
			return err
		}
	}
	if err := b.prepareWorkspace(); err != nil {
		return err
	}
	if err := b.prepareDeps(); err != nil {
		return err
	}
	if b.test {
		return b.runTests()
	}
	if b.lint {
		if err := b.runLint(); err != nil {
			return err
		}
	}
	if err := b.buildBundle(); err != nil {
		return err
	}
	if err := b.packageSupportFiles(); err != nil {
		return err
	}
	if err := b.packageBundle(); err != nil {
		return err
	}
	return b.publish()
}

func newBuilder(lint bool) (*builder, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}

	arch := commandOutput(runtime.GOARCH, "uname", "-m")
	deps, err := loadDepsLock(filepath.Join(workspace, "deps.lock"))
	if err != nil {
		return nil, err
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return nil, fmt.Errorf("XDG_CACHE_HOME must be absolute")
		}
		cache = xdg
	}
	cache = filepath.Join(cache, "revm-build", "v1")
	b := &builder{
		goos:      runtime.GOOS,
		goarch:    arch,
		lint:      lint,
		sourceDir: workspace,
		cacheDir:  cache,

		archiveDir: filepath.Join(cache, "archives"),
		deps:       deps,
	}
	if b.goos == "darwin" {
		libarchive := commandOutput("", "brew", "--prefix", "libarchive")
		e2fsprogs := commandOutput("", "brew", "--prefix", "e2fsprogs")
		b.pkgCfgPath = filepath.Join(libarchive, "lib", "pkgconfig") + ":" +
			filepath.Join(e2fsprogs, "lib", "pkgconfig")
	}

	return b, nil
}

func loadDepsLock(path string) (depsLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return depsLock{}, fmt.Errorf("read deps lock %s: %w", path, err)
	}

	var lock depsLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return depsLock{}, fmt.Errorf("parse deps lock %s: %w", path, err)
	}
	if strings.TrimSpace(lock.Version) == "" {
		return depsLock{}, fmt.Errorf("deps lock %s: missing version", path)
	}
	if strings.TrimSpace(lock.BaseURL) == "" {
		return depsLock{}, fmt.Errorf("deps lock %s: missing base_url", path)
	}
	if len(lock.Assets) == 0 {
		return depsLock{}, fmt.Errorf("deps lock %s: missing assets", path)
	}
	return lock, nil
}

func (b *builder) prepareWorkspace() error {
	logrus.Infof("lint=%t os=%s arch=%s workspace=%s", b.lint, b.goos, b.goarch, b.workspace)
	if err := mkdirAll(b.depsDir); err != nil {
		return err
	}
	if err := mkdirAll(b.binDir()); err != nil {
		return err
	}
	if err := mkdirAll(b.libDir()); err != nil {
		return err
	}

	if err := mkdirAll(b.archiveDir); err != nil {
		return err
	}
	if err := mkdirAll(filepath.Dir(b.agentPath)); err != nil {
		return err
	}
	return nil
}

func (b *builder) prepareDeps() error {
	assetOS, assetArch, err := b.depAssetPlatform()
	if err != nil {
		return err
	}
	for _, name := range []string{"libkrun", "libkrunfw"} {
		archive, err := b.archivePath(fmt.Sprintf("%s-%s-%s.tar.zst", name, assetOS, assetArch))
		if err != nil {
			return err
		}
		extracted, err := b.extractArchive(archive)
		if err != nil {
			return err
		}
		if err := symlink(extracted, filepath.Join(b.depsDir, name)); err != nil {
			return err
		}
	}
	if b.test {
		return nil
	}
	linuxArch, err := b.linuxAssetArch()
	if err != nil {
		return err
	}
	archive, err := b.archivePath(fmt.Sprintf("alpine-rootfs-Linux-%s.tar.zst", linuxArch))
	if err != nil {
		return err
	}
	return copyWithCP(archive, filepath.Join(b.staticDir, "rootfs", "rootfs.tar.zst"), nil)
}

func (b *builder) buildBundle() error {
	version := commandOutput("unknown", "git", "-C", b.sourceDir, "describe", "--tags", "--match", "v[0-9]*", "--abbrev=0")
	commit := commandOutput("unknown", "git", "-C", b.sourceDir, "rev-parse", "--short", "HEAD")
	buildDate := time.Now().UTC().Format("20060102T150405Z")
	ldflags := fmt.Sprintf(
		"-X linuxvm/pkg/define.Version=%s -X linuxvm/pkg/define.CommitID=%s -X linuxvm/pkg/define.BuildDate=%s",
		version, commit, buildDate,
	)
	if b.goos == "linux" {
		ldflags += ` -linkmode=external -extldflags "-static-libgcc -static-libstdc++"`
	}

	logrus.Infof("building revm (%s-%s-%s)", version, commit, buildDate)
	if err := b.buildGuestAgent(); err != nil {
		return err
	}
	logrus.Infof("building bundle in %s", b.bundleRoot())

	env, err := b.nativeEnv()
	if err != nil {
		return err
	}
	if err := commandIn(b.workspace, env, "go", "build", "-trimpath", "-ldflags="+ldflags, "-o", b.executablePath(), "./cmd/revm"); err != nil {
		return err
	}

	if err := b.prepareRuntimeLibs(); err != nil {
		return err
	}
	if b.goos == "linux" {
		if err := b.writeLinuxLauncher(); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) buildGuestAgent() error {
	guestArch, err := b.guestGoArch()
	if err != nil {
		return err
	}
	logrus.Infof("building embedded guest-agent: %s", b.agentPath)
	// Only the staged placeholder is replaced; the checkout stays untouched.
	if err := os.Remove(b.agentPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return commandIn(
		filepath.Join(b.workspace, "cmd", "guest-agent"),
		[]string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + guestArch},
		"go", "build", "-ldflags=-s -w", "-o", b.agentPath, "main.go",
	)
}

func (b *builder) prepareRuntimeLibs() error {
	logrus.Info("preparing shared libraries")
	for _, dependency := range []string{"libkrun", "libkrunfw"} {
		audit := filepath.Join(b.depsDir, dependency, "share", "revm")
		if exists(audit) {
			dest := filepath.Join(b.bundleRoot(), "share", dependency)
			if err := mkdirAll(dest); err != nil {
				return err
			}
			if err := copyWithCP(audit+"/.", dest+"/", nil); err != nil {
				return err
			}
		}
	}
	if b.goos == "darwin" {
		return b.prepareRuntimeLibsDarwin()
	}
	return b.prepareRuntimeLibsLinux()
}

func (b *builder) prepareRuntimeLibsDarwin() error {
	libDir := b.libDir()
	libkrunLibDir := filepath.Join(b.depsDir, "libkrun", "lib")
	libkrunfwLibDir := filepath.Join(b.depsDir, "libkrunfw", "lib")

	if err := copyWithCP(libkrunLibDir, libDir+"/", func(name string) bool {
		return strings.HasSuffix(name, ".dylib")
	}); err != nil {
		return err
	}
	if err := copyWithCP(libkrunfwLibDir, libDir+"/", func(name string) bool {
		return strings.HasSuffix(name, ".dylib")
	}); err != nil {
		return err
	}
	// libkrun loads the libkrunfw payload by soname at runtime. Keep that
	// lookup inside the relocatable bundle instead of relying on DYLD paths.
	// The dependency builder already adds this rpath to the libkrun dylib,
	// so make the application packaging step idempotent.
	libkrunPath := filepath.Join(libDir, "libkrun.2.0.0.dylib")
	if !strings.Contains(commandOutput("", "otool", "-l", libkrunPath), "@loader_path") {
		if err := command(nil, "install_name_tool", "-add_rpath", "@loader_path", libkrunPath); err != nil {
			return err
		}
	}

	entitlements := filepath.Join(b.workspace, "revm.entitlements")
	bin := b.executablePath()
	if err := command(nil, "install_name_tool", "-change", "libkrun.2.dylib", "@loader_path/../lib/libkrun.2.dylib", bin); err != nil {
		return err
	}
	if err := command(nil, "install_name_tool", "-change", "libkrun_init.0.dylib", "@loader_path/../lib/libkrun_init.0.dylib", bin); err != nil {
		return err
	}
	if err := command(nil, "install_name_tool", "-change", "libkrunfw.5.dylib", "@loader_path/../lib/libkrunfw.5.dylib", bin); err != nil {
		return err
	}
	return command(nil, "codesign", "--entitlements", entitlements, "--force", "-s", "-", bin)
}

func (b *builder) prepareRuntimeLibsLinux() error {
	libDir := filepath.Clean(b.libDir())

	libkrunDir, err := b.depLibDir("libkrun")
	if err != nil {
		return err
	}
	if err := copyWithCP(libkrunDir, libDir+"/", func(name string) bool {
		return strings.Contains(name, ".so")
	}); err != nil {
		return err
	}

	libkrunfwDir, err := b.depLibDir("libkrunfw")
	if err != nil {
		return err
	}
	if err := copyWithCP(libkrunfwDir, libDir+"/", func(name string) bool {
		return strings.Contains(name, ".so")
	}); err != nil {
		return err
	}

	deps, err := linuxRuntimeLibs(libDir, append([]string{b.executablePath()}, linuxSharedLibraries(libDir)...)...)
	if err != nil {
		return err
	}
	for _, path := range deps {
		cleanPath := filepath.Clean(path)
		if strings.HasPrefix(cleanPath, libDir+string(os.PathSeparator)) {
			continue
		}
		if err := copyResolvedFileWithCP(path, libDir+"/"); err != nil {
			return err
		}
	}
	return nil
}

func linuxSharedLibraries(dir string) []string {
	entries, err := readDir(dir)
	if err != nil {
		return nil
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.Contains(entry.Name(), ".so") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths
}

func linuxRuntimeLibs(libDir string, paths ...string) ([]string, error) {
	seen := map[string]bool{}
	for _, path := range paths {
		cmd := exec.Command("ldd", path)
		cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+libDir)
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil {
			return nil, fmt.Errorf("ldd %s: %w\n%s", path, err, text)
		}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "linux-vdso") {
				continue
			}
			if strings.Contains(line, "not found") {
				return nil, fmt.Errorf("ldd %s: unresolved dependency: %s", path, line)
			}
			libPath := lddLibraryPath(line)
			if libPath != "" {
				seen[libPath] = true
			}
		}
	}

	libs := make([]string, 0, len(seen))
	for path := range seen {
		libs = append(libs, path)
	}
	sort.Strings(libs)
	return libs, nil
}

func lddLibraryPath(line string) string {
	if idx := strings.Index(line, "=>"); idx >= 0 {
		fields := strings.Fields(strings.TrimSpace(line[idx+len("=>"):]))
		if len(fields) > 0 && strings.HasPrefix(fields[0], string(os.PathSeparator)) {
			return fields[0]
		}
		return ""
	}

	fields := strings.Fields(line)
	if len(fields) > 0 && strings.HasPrefix(fields[0], string(os.PathSeparator)) {
		return fields[0]
	}
	return ""
}

func (b *builder) nativeEnv() ([]string, error) {
	dirs := []string{}
	for _, name := range []string{"libkrun", "libkrunfw"} {
		dir, err := b.depLibDir(name)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	flags := os.Getenv("CGO_LDFLAGS")
	for _, dir := range dirs {
		flags += " " + strconv.Quote("-L"+dir)
	}
	env := []string{"CGO_ENABLED=1", "CGO_LDFLAGS=" + flags}
	libraryPath := strings.Join(dirs, string(os.PathListSeparator))
	if b.goos == "darwin" {
		env = append(env, "PKG_CONFIG_PATH="+b.pkgCfgPath, "DYLD_LIBRARY_PATH="+libraryPath)
	} else {
		env = append(env, "LD_LIBRARY_PATH="+libraryPath)
	}
	return env, nil
}

func (b *builder) runLint() error {
	env, err := b.nativeEnv()
	if err != nil {
		return err
	}
	return commandIn(b.workspace, env, "golangci-lint", "run", "./...")
}

func (b *builder) runTests() error {
	env, err := b.nativeEnv()
	if err != nil {
		return err
	}
	if err := commandIn(b.workspace, env, "go", "test", "-race", "./..."); err != nil {
		return err
	}
	guest := filepath.Join(b.workspace, "cmd", "guest-agent")
	if b.goos == "linux" {
		return commandIn(guest, nil, "go", "test", "-race", "./...")
	}
	if err := commandIn(guest, nil, "go", "test", "-race", "./pkg/supervisor"); err != nil {
		return err
	}
	// Compile every Linux-only guest package on macOS; VM checks execute the
	// service tests and workload regressions on a virtualization-capable host.
	logrus.Info("cross-compiling guest tests for Linux; use tests/guest.py to execute them in a VM")
	return commandIn(guest, []string{"CGO_ENABLED=0", "GOOS=linux"}, "go", "test", "-exec=true", "./...")
}

func (b *builder) publish() error {
	dest := filepath.Join(b.sourceDir, "out", "revm")
	backup := dest + ".previous"
	// Recover an interruption between the two renames without losing the last
	// usable bundle. A failed build never reaches this publication step.
	if !exists(dest) && exists(backup) {
		if err := os.Rename(backup, dest); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	hadOutput := exists(dest)
	if hadOutput {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(b.bundleRoot(), dest); err != nil {
		if hadOutput {
			err = errors.Join(err, os.Rename(backup, dest))
		}
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	archive := fmt.Sprintf("revm-%s-%s.tar.zst", b.goos, b.goarch)
	for _, name := range []string{archive, archive + ".sha256"} {
		if err := os.Rename(filepath.Join(b.workspace, name), filepath.Join(b.sourceDir, name)); err != nil {
			return err
		}
	}
	logrus.Infof("published %s", dest)

	return nil
}

func (b *builder) packageBundle() error {
	tarName := fmt.Sprintf("revm-%s-%s.tar.zst", b.goos, b.goarch)
	tarPath := filepath.Join(b.workspace, tarName)
	if err := command(nil, "bsdtar", "--zstd", "-cf", tarPath, "-C", b.bundleRoot(), "."); err != nil {
		return err
	}
	sum, err := sha256File(tarPath)
	if err != nil {
		return err
	}
	if err = os.WriteFile(tarPath+".sha256", []byte(sum+"  "+tarName+"\n"), 0644); err != nil {
		return err
	}
	logrus.Infof("build complete: %s", tarPath)
	return nil
}

func (b *builder) writeLinuxLauncher() error {
	path := filepath.Join(b.binDir(), "revm")
	script := fmt.Sprintf(`#!/bin/sh
DIR="$(cd "$(dirname "$0")" && pwd)"
REVM_BUNDLE_LAUNCHER="$DIR/revm"
export REVM_BUNDLE_LAUNCHER
exec "$DIR/../lib/%s" --library-path "$DIR/../lib" "$DIR/revm.real" "$@"
`, b.linuxDynLinkerName())
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func (b *builder) linuxDynLinkerName() string {
	switch b.goarch {
	case "arm64", "aarch64":
		return "ld-linux-aarch64.so.1"
	case "amd64", "x86_64":
		return "ld-linux-x86-64.so.2"
	default:
		return "ld-linux.so"
	}
}

func (b *builder) executablePath() string {
	name := "revm"
	if b.goos == "linux" {
		name += ".real"
	}
	return filepath.Join(b.binDir(), name)
}

func (b *builder) bundleRoot() string {
	return filepath.Join(b.outDir, "revm")
}

func (b *builder) binDir() string {
	return filepath.Join(b.bundleRoot(), "bin")
}

func (b *builder) libDir() string {
	return filepath.Join(b.bundleRoot(), "lib")
}

func (b *builder) depAssetPlatform() (string, string, error) {
	if b.goos == "darwin" {
		return "Darwin", "arm64", nil
	}
	arch, err := b.linuxAssetArch()
	if err != nil {
		return "", "", err
	}
	return "Linux", arch, nil
}

func (b *builder) guestGoArch() (string, error) {
	switch b.goarch {
	case "arm64", "aarch64":
		return "arm64", nil
	case "amd64", "x86_64":
		return "amd64", nil
	default:
		return "", fmt.Errorf("unsupported host arch: %s", b.goarch)
	}
}

func (b *builder) linuxAssetArch() (string, error) {
	switch b.goarch {
	case "arm64", "aarch64":
		return "aarch64", nil
	case "amd64", "x86_64":
		return "x86_64", nil
	default:
		return "", fmt.Errorf("unsupported host arch: %s", b.goarch)
	}
}

func (b *builder) depLibDir(name string) (string, error) {
	for _, dir := range []string{"lib64", "lib"} {
		path := filepath.Join(b.depsDir, name, dir)
		if exists(path) {
			return filepath.EvalSymlinks(path)
		}
	}
	return "", fmt.Errorf("missing shared library directory for %s", name)
}

func (b *builder) archivePath(name string) (string, error) {
	if b.localDeps != "" && (strings.HasPrefix(name, "libkrun-") || strings.HasPrefix(name, "libkrunfw-")) {
		path := filepath.Join(b.localDeps, name)
		sum, err := sha256File(path)
		if err != nil {
			return "", fmt.Errorf("local dependency: %w", err)
		}
		logrus.Infof("using local dependency %s (sha256=%s)", path, sum)
		return path, nil
	}
	expected := strings.TrimSpace(b.deps.Assets[name])
	if expected == "" {
		return "", fmt.Errorf("deps lock %s: missing sha256 for asset %s", b.deps.Version, name)
	}

	archivePath := filepath.Join(b.archiveDir, expected+"-"+name)
	if exists(archivePath) {
		actual, err := sha256File(archivePath)
		if err != nil {
			return "", err
		}
		if actual == expected {
			logrus.Infof("reusing cached archive %s", name)
			return archivePath, nil
		}
		logrus.Infof("cached archive %s failed checksum validation, downloading again", name)
	}

	tmp, err := os.CreateTemp(b.archiveDir, ".download-")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	logrus.Infof("downloading %s", name)
	if err := downloadFile(strings.TrimRight(b.deps.BaseURL, "/")+"/"+name, tmpPath); err != nil {
		return "", err
	}

	actual, err := sha256File(tmpPath)
	if err != nil {
		return "", err
	}
	if actual != expected {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", name, expected, actual)
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		return "", fmt.Errorf("rename %s -> %s: %w", tmpPath, archivePath, err)
	}
	return archivePath, nil
}

// Publish a fully extracted, content-addressed object. Concurrent builders may
// prepare the same object, but neither modifies a published directory.
func (b *builder) extractArchive(archive string) (string, error) {
	sum, err := sha256File(archive)
	if err != nil {
		return "", err
	}
	base := filepath.Join(b.cacheDir, "extracted")
	dest := filepath.Join(base, sum)
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return dest, nil
	}
	if err := mkdirAll(base); err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp(base, ".extract-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	if err := command(nil, "bsdtar", "--zstd", "-xf", archive, "-C", temp); err != nil {
		return "", err
	}
	if err := os.Rename(temp, dest); err != nil {
		if info, statErr := os.Stat(dest); statErr != nil || !info.IsDir() {
			return "", err
		}
	}
	return dest, nil
}

func command(env []string, args ...string) error {
	return runCommand("", env, args...)
}

func commandIn(dir string, env []string, args ...string) error {
	return runCommand(dir, env, args...)
}

func runCommand(dir string, env []string, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("empty command")
	}
	cmdline := strings.Join(args, " ")
	if dir == "" {
		logrus.Debugf("exec: %s", cmdline)
	} else {
		logrus.Debugf("exec (in %s): %s", dir, cmdline)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		if dir == "" {
			return fmt.Errorf("command failed: %s\n  %w", cmdline, err)
		}
		return fmt.Errorf("command failed (in %s): %s\n  %w", dir, cmdline, err)
	}
	return nil
}

func commandOutput(fallback string, args ...string) string {
	if len(args) == 0 {
		return fallback
	}
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(out))
}

func downloadFile(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	if err := mkdirAll(filepath.Dir(dst)); err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mkdirAll(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

func copyWithCP(src, dst string, match func(name string) bool) error {
	logrus.Debugf("cp -av %s %s", src, dst)
	if match == nil {
		return command(nil, "cp", "-av", src, dst)
	}

	srcPath := strings.TrimSuffix(src, string(os.PathSeparator))
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", srcPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("copy filter requires directory source: %s", srcPath)
	}

	entries, err := readDir(srcPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !match(entry.Name()) {
			continue
		}
		if err := command(nil, "cp", "-av", filepath.Join(srcPath, entry.Name()), dst); err != nil {
			return err
		}
	}
	return nil
}

func copyResolvedFileWithCP(src, dst string) error {
	logrus.Debugf("cp -avL %s %s", src, dst)
	return command(nil, "cp", "-avL", src, dst)
}

func readDir(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("readDir %s: %w", path, err)
	}
	return entries, nil
}

func symlink(target, path string) error {
	if err := os.Symlink(target, path); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", path, target, err)
	}
	return nil
}

func (b *builder) packageSupportFiles() error {
	for _, name := range []string{"README.md", "LICENSE"} {
		if !exists(filepath.Join(b.workspace, name)) {
			continue
		}
		if err := copyWithCP(filepath.Join(b.workspace, name), filepath.Join(b.bundleRoot(), name), nil); err != nil {
			return err
		}
	}
	doc := filepath.Join(b.bundleRoot(), "share", "doc", "revm")
	if err := mkdirAll(doc); err != nil {
		return err
	}
	if err := copyWithCP(filepath.Join(b.workspace, "docs")+"/.", doc+"/", nil); err != nil {
		return err
	}
	man := filepath.Join(b.bundleRoot(), "share", "man", "man1")
	if err := mkdirAll(man); err != nil {
		return err
	}
	if err := copyWithCP(filepath.Join(b.workspace, "docs", "revm.1"), filepath.Join(man, "revm.1"), nil); err != nil {
		return err
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		output, err := exec.Command(filepath.Join(b.binDir(), "revm"), "completion", shell).Output()
		if err != nil {
			return fmt.Errorf("generate %s completion: %w", shell, err)
		}
		path := filepath.Join(b.bundleRoot(), "share", "completions", "revm."+shell)
		if err = mkdirAll(filepath.Dir(path)); err != nil {
			return err
		}
		if err = os.WriteFile(path, output, 0644); err != nil {
			return err
		}
	}
	return nil
}
