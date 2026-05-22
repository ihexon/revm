package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const assetsBase = "https://github.com/ihexon/revm-assets/releases/download/v2.0.23"

var defaultBuildTargets = []string{"chroot", "dockerd"}

// Edit this table when revm-assets changes.
var assetSHA256 = map[string]string{
	"alpine-rootfs-Linux-aarch64.tar.zst": "f821e14647ce0de425ab0fce7b27d69c9af854c1d59ac51efcb3e238624f4457",
	"alpine-rootfs-Linux-x86_64.tar.zst":  "4ded9e9e705d8c19e55830d2bbca2f0f1c238c69de46cfd89f6d6b5ae8eb579a",
	"busybox-Linux-aarch64.tar.zst":       "b4415e0a6efdc00c55990758519d9a98115576f8f362496a98219246d734695b",
	"busybox-Linux-x86_64.tar.zst":        "671cd6b34b8ed487a1d4a3ae38ba34b4aa13fd1d2e0bac9ef7d23eeb9664b2a0",
	"dropbear-Linux-aarch64.tar.zst":      "190e0244f3cf80467c0a07e36e6f74fa03cfd3ac49d4e9e6bab69be839afc32e",
	"dropbear-Linux-x86_64.tar.zst":       "19211ed2758dd4e41ff952cd02f1cbc290b541feeb218a51daba3e49211eb269",
	"libkrun-Darwin-arm64.tar.zst":        "3a13e2eee78cd9209deb46e89bbbf26ea07bbb3a520b8dee68723a61c4633062",
	"libkrun-Linux-aarch64.tar.zst":       "8aab48b188910543e66e696ecc1cf1b75e18745ec1605ed674478cef2f072658",
	"libkrun-Linux-x86_64.tar.zst":        "76c5b37385b379b43b3ea8ed43ad7405c0c8ebd25cc4e27fbbae285230b47acc",
	"libkrunfw-Darwin-arm64.tar.zst":      "3d4ef8ee124485f9541559efb6c816bcddb8b7a4aedfc8a35dd4946600fe2c99",
	"libkrunfw-Linux-aarch64.tar.zst":     "28b66fae9f32daa41253f1617ca88cccb9f95214d9ee8d24ceb6e269765d9f3c",
	"libkrunfw-Linux-x86_64.tar.zst":      "6c0aec4d878b166ee0b385edfd5a028b2d7a749712b10cc4005050beac4ab1b9",
}

type builder struct {
	goos    string
	goarch  string
	targets []string
	lint    bool

	workspace  string
	outDir     string
	depsDir    string
	archiveDir string

	staticDir  string
	serviceDir string
	agentPath  string
	pkgCfgPath string
	homebrew   string
}

func main() {
	verbose := flag.Bool("v", false, "enable debug logging")
	runLint := flag.Bool("lint", false, "run golangci-lint before building")
	buildTarget := flag.String("build", "all", "target to build: all, chroot, or dockerd")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run build.go [-v] [--lint] [--build all|chroot|dockerd]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	targets, err := parseBuildTargets(*buildTarget)
	if err != nil {
		logrus.Fatal(err)
	}

	b, err := newBuilder(targets, *runLint)
	if err != nil {
		logrus.Fatal(err)
	}

	var buildErr error
	if err := b.run(); err != nil {
		buildErr = err
	}
	if err := b.restorePlaceholders(); err != nil && buildErr == nil {
		buildErr = err
	}
	if buildErr != nil {
		logrus.Fatal(buildErr)
	}
}

func (b *builder) run() error {
	if err := b.prepareWorkspace(); err != nil {
		return err
	}
	if b.lint {
		if err := b.runLint(); err != nil {
			return err
		}
	}
	if err := b.removePlaceholders(); err != nil {
		return err
	}
	if err := b.prepareGuestAssets(); err != nil {
		return err
	}
	if err := b.prepareDeps(); err != nil {
		return err
	}
	if err := b.buildBundles(); err != nil {
		return err
	}
	return b.packageTargets()
}

func newBuilder(targets []string, lint bool) (*builder, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}

	arch := commandOutput(runtime.GOARCH, "uname", "-m")
	homebrew := os.Getenv("HOMEBREW_PREFIX")
	if homebrew == "" {
		homebrew = "/opt/homebrew"
	}

	b := &builder{
		goos:       runtime.GOOS,
		goarch:     arch,
		targets:    append([]string(nil), targets...),
		lint:       lint,
		workspace:  workspace,
		outDir:     filepath.Join(workspace, "out"),
		depsDir:    "/tmp/.deps",
		archiveDir: filepath.Join("/tmp/.deps", "archives"),
		staticDir:  filepath.Join(workspace, "pkg", "static_resources"),
		serviceDir: filepath.Join(workspace, "cmd", "guest-agent", "pkg", "service"),
		homebrew:   homebrew,
	}
	b.agentPath = filepath.Join(b.staticDir, "guest_agent", "guest-agent")

	if b.goos == "darwin" {
		libarchive := commandOutput("", "brew", "--prefix", "libarchive")
		e2fsprogs := commandOutput("", "brew", "--prefix", "e2fsprogs")
		b.pkgCfgPath = filepath.Join(libarchive, "lib", "pkgconfig") + ":" +
			filepath.Join(e2fsprogs, "lib", "pkgconfig")
	}

	return b, nil
}

func parseBuildTargets(target string) ([]string, error) {
	switch strings.TrimSpace(target) {
	case "", "all":
		return append([]string(nil), defaultBuildTargets...), nil
	case "chroot", "dockerd":
		return []string{target}, nil
	default:
		return nil, fmt.Errorf("unsupported build target %q: use all, chroot, or dockerd", target)
	}
}

func (b *builder) prepareWorkspace() error {
	logrus.Infof("targets=%s lint=%t os=%s arch=%s workspace=%s", strings.Join(b.targets, ","), b.lint, b.goos, b.goarch, b.workspace)
	if err := removeAll(b.outDir); err != nil {
		return err
	}
	for _, target := range b.targets {
		if err := mkdirAll(b.binDir(target)); err != nil {
			return err
		}
		if err := mkdirAll(b.libDir(target)); err != nil {
			return err
		}
	}
	if err := mkdirAll(b.archiveDir); err != nil {
		return err
	}
	if err := mkdirAll(filepath.Dir(b.agentPath)); err != nil {
		return err
	}
	return nil
}

func (b *builder) prepareGuestAssets() error {
	arch, err := b.linuxAssetArch()
	if err != nil {
		return err
	}
	for _, asset := range []struct {
		name    string
		srcRel  string
		dstPath string
	}{
		{
			name:    fmt.Sprintf("busybox-Linux-%s.tar.zst", arch),
			srcRel:  filepath.Join("usr", "bin", "busybox"),
			dstPath: filepath.Join(b.serviceDir, "busybox.static"),
		},
		{
			name:    fmt.Sprintf("dropbear-Linux-%s.tar.zst", arch),
			srcRel:  filepath.Join("bin", "dropbearmulti"),
			dstPath: filepath.Join(b.serviceDir, "dropbearmulti"),
		},
	} {
		cacheDir := filepath.Join(b.depsDir, strings.TrimSuffix(asset.name, ".tar.zst"))
		if err := b.extractArchive(asset.name, cacheDir); err != nil {
			return err
		}
		if err := copyWithCP(filepath.Join(cacheDir, asset.srcRel), asset.dstPath, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) prepareDeps() error {
	assetOS, assetArch, err := b.depAssetPlatform()
	if err != nil {
		return err
	}
	for _, name := range []string{
		fmt.Sprintf("libkrun-%s-%s.tar.zst", assetOS, assetArch),
		fmt.Sprintf("libkrunfw-%s-%s.tar.zst", assetOS, assetArch),
	} {
		extractDir := filepath.Join(b.depsDir, strings.TrimSuffix(name, ".tar.zst"))
		if err := b.extractArchive(name, extractDir); err != nil {
			return err
		}
	}

	for _, name := range []string{"libkrun", "libkrunfw"} {
		alias := filepath.Join(b.depsDir, name)
		if !exists(alias) {
			if err := symlink(fmt.Sprintf("%s-%s-%s", name, assetOS, assetArch), alias); err != nil {
				return err
			}
		}
	}

	if b.goos == "linux" {
		for _, name := range []string{"libkrun", "libkrunfw"} {
			lib64 := filepath.Join(b.depsDir, name, "lib64")
			lib := filepath.Join(b.depsDir, name, "lib")
			if exists(lib64) && !exists(lib) {
				if err := symlink("lib64", lib); err != nil {
					return err
				}
			}
		}
	}

	linuxArch, err := b.linuxAssetArch()
	if err != nil {
		return err
	}
	rootfs := fmt.Sprintf("alpine-rootfs-Linux-%s.tar.zst", linuxArch)
	rootfsCache := filepath.Join(b.depsDir, "rootfs", "rootfs.tar.zst")
	rootfsDest := filepath.Join(b.staticDir, "rootfs", "rootfs.tar.zst")
	if err := mkdirAll(filepath.Dir(rootfsCache)); err != nil {
		return err
	}
	rootfsArchive, err := b.archivePath(rootfs)
	if err != nil {
		return err
	}
	if err := copyWithCP(rootfsArchive, rootfsCache, nil); err != nil {
		return err
	}
	return copyWithCP(rootfsCache, rootfsDest, nil)
}

func (b *builder) buildBundles() error {
	version := commandOutput("unknown", "git", "-C", b.workspace, "describe", "--tags", "--abbrev=0")
	commit := commandOutput("unknown", "git", "-C", b.workspace, "rev-parse", "--short", "HEAD")
	buildDate := time.Now().UTC().Format("20060102T150405Z")
	ldflags := fmt.Sprintf(
		"-X linuxvm/pkg/define.Version=%s -X linuxvm/pkg/define.CommitID=%s -X linuxvm/pkg/define.BuildDate=%s",
		version, commit, buildDate,
	)
	if b.goos == "linux" {
		ldflags += ` -linkmode=external -extldflags "-static-libgcc -static-libstdc++"`
	}

	logrus.Infof("building targets (%s-%s-%s)", version, commit, buildDate)
	if err := b.buildGuestAgent(); err != nil {
		return err
	}
	for _, target := range b.targets {
		logrus.Infof("building bundle for %s in %s", target, b.targetRoot(target))

		env := []string{}
		if b.goos == "darwin" {
			env = append(env, "PKG_CONFIG_PATH="+b.pkgCfgPath)
		} else {
			env = append(env, "CGO_ENABLED=1")
		}
		if err := command(env, "go", "build", "-ldflags="+ldflags, "-o", b.executablePath(target), filepath.Join(b.workspace, "cmd", target)); err != nil {
			return err
		}
		if err := b.prepareRuntimeLibsFor(target); err != nil {
			return err
		}
		if b.goos == "linux" {
			if err := b.writeLinuxLauncher(filepath.Join(b.binDir(target), target), target); err != nil {
				return err
			}
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
	return commandIn(
		filepath.Join(b.workspace, "cmd", "guest-agent"),
		[]string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + guestArch},
		"go", "build", "-ldflags=-s -w", "-o", b.agentPath, "main.go",
	)
}

func (b *builder) prepareRuntimeLibsFor(target string) error {
	logrus.Infof("preparing shared libraries for %s", target)
	if b.goos == "darwin" {
		return b.prepareRuntimeLibsDarwin(target)
	}
	return b.prepareRuntimeLibsLinux(target)
}

func (b *builder) prepareRuntimeLibsDarwin(target string) error {
	libDir := b.libDir(target)
	libkrunfwLibDir := filepath.Join(b.depsDir, "libkrunfw", "lib")

	if err := copyWithCP(libkrunfwLibDir, libDir+"/", func(name string) bool {
		return strings.HasSuffix(name, ".dylib")
	}); err != nil {
		return err
	}

	entitlements := filepath.Join(b.workspace, "revm.entitlements")
	bin := b.executablePath(target)
	if err := command(nil, "install_name_tool", "-change", "libkrunfw.5.dylib", "@loader_path/../lib/libkrunfw.5.dylib", bin); err != nil {
		return err
	}
	return command(nil, "codesign", "--entitlements", entitlements, "--force", "-s", "-", bin)
}

func (b *builder) prepareRuntimeLibsLinux(target string) error {
	libDir := b.libDir(target)

	libkrunfwDir, err := b.depLibDir("libkrunfw")
	if err != nil {
		return err
	}
	if err := copyWithCP(libkrunfwDir, libDir+"/", func(name string) bool {
		return strings.Contains(name, ".so")
	}); err != nil {
		return err
	}

	deps, err := linuxRuntimeLibs(append([]string{b.executablePath(target)}, linuxSharedLibraries(libDir)...)...)
	if err != nil {
		return err
	}
	for _, path := range deps {
		if strings.HasPrefix(path, libDir+string(os.PathSeparator)) {
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

func linuxRuntimeLibs(paths ...string) ([]string, error) {
	seen := map[string]bool{}
	for _, path := range paths {
		out, err := exec.Command("ldd", path).CombinedOutput()
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

func (b *builder) runLint() error {
	logrus.Info("running golangci-lint")
	env := []string{}
	if b.goos == "darwin" {
		env = append(env, "PKG_CONFIG_PATH="+b.pkgCfgPath)
	}
	return command(env, "golangci-lint", "run", "./...")
}

func (b *builder) packageTargets() error {
	logrus.Info("packaging")
	for _, target := range b.targets {
		if err := b.packageTarget(target); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) packageTarget(target string) error {
	tarName := fmt.Sprintf("%s-%s-%s.tar.zst", target, b.goos, b.goarch)
	tarPath := filepath.Join(b.workspace, tarName)
	if err := command(nil, "bsdtar", "--zstd", "-cf", tarPath, "-C", b.targetRoot(target), "."); err != nil {
		return err
	}
	logrus.Infof("build complete: %s", tarPath)
	return nil
}

func (b *builder) writeLinuxLauncher(path, target string) error {
	realName := target + ".real"
	command := fmt.Sprintf("exec \"$DIR/../lib/%s\" --library-path \"$DIR/../lib\" \"$DIR/%s\" \"$@\"", b.linuxDynLinkerName(), realName)
	script := fmt.Sprintf("#!/bin/sh\nDIR=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n%s\n", command)
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

func (b *builder) executablePath(target string) string {
	name := target
	if b.goos == "linux" {
		name += ".real"
	}
	return filepath.Join(b.binDir(target), name)
}

func (b *builder) targetRoot(target string) string {
	return filepath.Join(b.outDir, target)
}

func (b *builder) binDir(target string) string {
	return filepath.Join(b.targetRoot(target), "bin")
}

func (b *builder) libDir(target string) string {
	return filepath.Join(b.targetRoot(target), "lib")
}

func (b *builder) placeholderPaths() []string {
	return []string{
		filepath.Join(b.staticDir, "rootfs", "rootfs.tar.zst"),
		b.agentPath,
		filepath.Join(b.serviceDir, "busybox.static"),
		filepath.Join(b.serviceDir, "dropbearmulti"),
	}
}

func (b *builder) removePlaceholders() error {
	for _, path := range b.placeholderPaths() {
		if err := removeExistingFile(path); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) restorePlaceholders() error {
	var restoreErr error
	for _, path := range b.placeholderPaths() {
		data := []byte(nil)
		if path == b.agentPath {
			data = []byte("placeholder\n")
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			logrus.Errorf("restore placeholder %s: %v", path, err)
			restoreErr = err
		}
	}
	return restoreErr
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
			return path, nil
		}
	}
	return "", fmt.Errorf("missing shared library directory for %s", name)
}

func (b *builder) archivePath(name string) (string, error) {
	archivePath := filepath.Join(b.archiveDir, name)
	expected := strings.TrimSpace(assetSHA256[name])
	if expected == "" {
		return "", fmt.Errorf("missing sha256 for asset %s", name)
	}

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

	tmpPath := archivePath + ".tmp"
	_ = os.Remove(tmpPath)

	logrus.Infof("downloading %s", name)
	if err := downloadFile(assetsBase+"/"+name, tmpPath); err != nil {
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

func (b *builder) extractArchive(name, dest string) error {
	archivePath, err := b.archivePath(name)
	if err != nil {
		return err
	}
	if err := removeAll(dest); err != nil {
		return err
	}
	if err := mkdirAll(dest); err != nil {
		return err
	}
	return command(nil, "bsdtar", "--zstd", "-xf", archivePath, "-C", dest)
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

func removeAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("rm -rf %s: %w", path, err)
	}
	return nil
}

func removeExistingFile(path string) error {
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		return nil
	} else {
		return fmt.Errorf("remove existing file %s: %w", path, err)
	}
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
