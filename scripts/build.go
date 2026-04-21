package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const assetsBase = "https://github.com/ihexon/revm-assets/releases/download/v2.0.16"

// run executes a command, inheriting stdout/stderr. If env is non-nil,
// those vars are appended to the current environment.
func run(env []string, args ...string) {
	logrus.Debugf("exec: %s", strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		logrus.Fatalf("command failed: %s\n  %v", strings.Join(args, " "), err)
	}
}

// runIn is like run but sets the working directory.
func runIn(dir string, env []string, args ...string) {
	logrus.Debugf("exec (in %s): %s", dir, strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		logrus.Fatalf("command failed (in %s): %s\n  %v", dir, strings.Join(args, " "), err)
	}
}

// runQuiet runs a command and returns trimmed stdout. Returns fallback on error.
func runQuiet(fallback string, args ...string) string {
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(out))
}

// exists checks if path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mkdirAll(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		logrus.Fatalf("mkdir %s: %v", path, err)
	}
}

func removeAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		logrus.Fatalf("rm -rf %s: %v", path, err)
	}
}

type builder struct {
	dirty      bool
	goos       string // runtime.GOOS
	goarch     string // uname -m (arm64, aarch64, x86_64)
	workspace  string
	outDir     string
	binDir     string
	libDir     string
	helperDir  string
	depsDir    string
	staticDir  string
	serviceDir string
	pkgCfgDir  string // Darwin only
	homebrew   string
}

func (b *builder) isArm64() bool {
	return b.goarch == "aarch64" || b.goarch == "arm64"
}

func (b *builder) isX8664() bool {
	return b.goarch == "x86_64" || b.goarch == "amd64"
}

func (b *builder) guestGoArch() string {
	switch {
	case b.isArm64():
		return "arm64"
	case b.isX8664():
		return "amd64"
	default:
		logrus.Fatalf("unsupported host arch: %s", b.goarch)
		return ""
	}
}

func (b *builder) linuxAssetArch() string {
	switch {
	case b.isArm64():
		return "aarch64"
	case b.isX8664():
		return "x86_64"
	default:
		logrus.Fatalf("unsupported linux arch: %s", b.goarch)
		return ""
	}
}

func (b *builder) depLibDir(name string) string {
	for _, dir := range []string{"lib64", "lib"} {
		path := filepath.Join(b.depsDir, name, dir)
		if exists(path) {
			return path
		}
	}
	logrus.Fatalf("missing shared library directory for %s", name)
	return ""
}

func (b *builder) linuxDynLinkerPath() string {
	var candidates []string
	switch {
	case b.isArm64():
		candidates = []string{
			"/lib/ld-linux-aarch64.so.1",
			"/lib64/ld-linux-aarch64.so.1",
			"/lib/aarch64-linux-gnu/ld-linux-aarch64.so.1",
		}
	case b.isX8664():
		candidates = []string{
			"/lib64/ld-linux-x86-64.so.2",
			"/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2",
			"/lib/ld-linux-x86-64.so.2",
		}
	default:
		logrus.Fatalf("unsupported linux arch: %s", b.goarch)
	}

	for _, path := range candidates {
		if exists(path) {
			return path
		}
	}

	logrus.Fatalf("failed to find dynamic linker for linux/%s (tried: %s)", b.goarch, strings.Join(candidates, ", "))
	return ""
}

func newBuilder(dirty bool) *builder {
	workspace, err := os.Getwd()
	if err != nil {
		logrus.Fatalf("getwd: %v", err)
	}

	arch := runQuiet(runtime.GOARCH, "uname", "-m")
	homebrew := os.Getenv("HOMEBREW_PREFIX")
	if homebrew == "" {
		homebrew = "/opt/homebrew"
	}

	b := &builder{
		dirty:      dirty,
		goos:       runtime.GOOS,
		goarch:     arch,
		workspace:  workspace,
		outDir:     filepath.Join(workspace, "out"),
		depsDir:    "/tmp/.deps",
		staticDir:  filepath.Join(workspace, "pkg", "static_resources"),
		serviceDir: filepath.Join(workspace, "cmd", "guest-agent", "pkg", "service"),
		homebrew:   homebrew,
	}
	b.binDir = filepath.Join(b.outDir, "bin")
	b.libDir = filepath.Join(b.outDir, "lib")
	b.helperDir = filepath.Join(b.outDir, "helper")
	return b
}

func (b *builder) initEnv() {
	logrus.Infof("target=revm os=%s arch=%s workspace=%s", b.goos, b.goarch, b.workspace)

	removeAll(b.outDir)
	mkdirAll(b.binDir)
	mkdirAll(b.libDir)
	mkdirAll(b.helperDir)

	if b.goos == "darwin" {
		libarchive := runQuiet("", "brew", "--prefix", "libarchive")
		e2fsprogs := runQuiet("", "brew", "--prefix", "e2fsprogs")
		b.pkgCfgDir = filepath.Join(libarchive, "lib", "pkgconfig") + ":" +
			filepath.Join(e2fsprogs, "lib", "pkgconfig")
	}
}

func (b *builder) buildGuestAgent() {
	logrus.Info("building guest-agent")
	guestArch := b.guestGoArch()
	runIn(filepath.Join(b.workspace, "cmd", "guest-agent"),
		[]string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + guestArch},
		"go", "build", "-ldflags=-s -w",
		"-o", filepath.Join(b.helperDir, "guest-agent"), "main.go")
}

func (b *builder) fetchAsset(name, url, dest string) {
	logrus.Infof("fetching %s", name)
	mkdirAll(dest)
	run(nil, "sh", "-c", fmt.Sprintf("wget -qO- '%s' | bsdtar --zstd -x -f - -C '%s'", url, dest))
}

func (b *builder) fetchGuestEmbeddedBinaries() {
	assetArch := b.linuxAssetArch()
	type guestBinaryAsset struct {
		name       string
		archiveURL string
		sourceRel  string
		destFile   string
	}

	assets := []guestBinaryAsset{
		{
			name:       "busybox",
			archiveURL: fmt.Sprintf("%s/busybox-Linux-%s.tar.zst", assetsBase, assetArch),
			sourceRel:  filepath.Join("usr", "bin", "busybox"),
			destFile:   filepath.Join(b.serviceDir, "busybox.static"),
		},
		{
			name:       "dropbear",
			archiveURL: fmt.Sprintf("%s/dropbear-Linux-%s.tar.zst", assetsBase, assetArch),
			sourceRel:  filepath.Join("bin", "dropbearmulti"),
			destFile:   filepath.Join(b.serviceDir, "dropbearmulti"),
		},
	}

	for _, asset := range assets {
		cacheDir := filepath.Join(b.depsDir, asset.name)
		cacheFile := filepath.Join(cacheDir, asset.sourceRel)
		if b.dirty && exists(cacheFile) {
			logrus.Infof("dirty mode: reusing cached %s in %s", asset.name, cacheFile)
		} else {
			if b.dirty {
				logrus.Infof("dirty mode: %s not found, downloading anyway", cacheFile)
			}
			removeAll(cacheDir)
			mkdirAll(cacheDir)
			b.fetchAsset(asset.name, asset.archiveURL, cacheDir)
		}

		run(nil, "cp", "-av", cacheFile, asset.destFile)
	}
}

func (b *builder) fetchDeps() {
	assetOS, assetArch := "Linux", b.linuxAssetArch()
	if b.goos == "darwin" {
		assetOS, assetArch = "Darwin", "arm64"
	}

	if b.dirty && exists(b.depsDir) {
		logrus.Infof("dirty mode: reusing cached deps in %s", b.depsDir)
	} else {
		if b.dirty {
			logrus.Infof("dirty mode: %s not found, downloading anyway", b.depsDir)
		}
		removeAll(b.depsDir)
		mkdirAll(b.depsDir)
		b.fetchAsset("libkrun",
			fmt.Sprintf("%s/libkrun-%s-%s.tar.zst", assetsBase, assetOS, assetArch),
			filepath.Join(b.depsDir, "libkrun"))
		b.fetchAsset("libkrunfw",
			fmt.Sprintf("%s/libkrunfw-%s-%s.tar.zst", assetsBase, assetOS, assetArch),
			filepath.Join(b.depsDir, "libkrunfw"))
	}

	// lib64 → lib symlinks so CGo header search works on Linux
	if b.goos == "linux" {
		for _, name := range []string{"libkrun", "libkrunfw"} {
			lib64 := filepath.Join(b.depsDir, name, "lib64")
			lib := filepath.Join(b.depsDir, name, "lib")
			if exists(lib64) && !exists(lib) {
				os.Symlink("lib64", lib)
			}
		}
	}

	rootfsCache := filepath.Join(b.depsDir, "rootfs", "rootfs.tar.zst")
	if b.dirty && exists(rootfsCache) {
		logrus.Infof("dirty mode: reusing cached rootfs in %s", rootfsCache)
	} else {
		logrus.Info("fetching rootfs")
		mkdirAll(filepath.Dir(rootfsCache))
		run(nil, "wget", "-qO", rootfsCache,
			fmt.Sprintf("%s/alpine-rootfs-Linux-%s.tar.zst", assetsBase, b.linuxAssetArch()))
	}
	rootfsDest := filepath.Join(b.staticDir, "rootfs", "rootfs.tar.zst")
	run(nil, "cp", "-av", rootfsCache, rootfsDest)
}

func (b *builder) buildTarget() {
	version := runQuiet("unknown", "git", "-C", b.workspace, "describe", "--tags", "--abbrev=0")
	commit := runQuiet("unknown", "git", "-C", b.workspace, "rev-parse", "--short", "HEAD")
	buildDate := time.Now().UTC().Format("20060102T150405Z")
	logrus.Infof("building revm (%s-%s-%s)", version, commit, buildDate)

	ldflags := fmt.Sprintf(
		"-X linuxvm/pkg/define.Version=%s -X linuxvm/pkg/define.CommitID=%s -X linuxvm/pkg/define.BuildDate=%s",
		version,
		commit,
		buildDate,
	)
	out := filepath.Join(b.binDir, "revm")
	src := filepath.Join(b.workspace, "cmd", "revm")

	if b.goos == "darwin" {
		run([]string{"PKG_CONFIG_PATH=" + b.pkgCfgDir},
			"go", "build", "-ldflags="+ldflags, "-o", out, src)
	} else {
		run([]string{"CGO_ENABLED=1"},
			"go", "build", "-ldflags="+ldflags, "-o", out, src)
	}
}

func (b *builder) relocateLibsDarwin() {
	hp := b.homebrew
	lib := b.libDir

	// Copy dylibs
	run(nil, "sh", "-c", fmt.Sprintf(
		"cp -av %s/libkrun/lib/*.dylib %s/libkrunfw/lib/*.dylib '%s/'",
		b.depsDir, b.depsDir, lib))
	for _, l := range []string{
		hp + "/opt/libepoxy/lib/libepoxy.0.dylib",
		hp + "/opt/virglrenderer/lib/libvirglrenderer.1.dylib",
		hp + "/opt/molten-vk/lib/libMoltenVK.dylib",
	} {
		run(nil, "cp", "-av", l, lib+"/")
	}
	removeAll(filepath.Join(lib, "pkgconfig"))

	// install_name_tool rewrites
	type rewrite struct{ dylib, old, new string }
	rewrites := []rewrite{
		{"libkrun.1.dylib", hp + "/opt/libepoxy/lib/libepoxy.0.dylib", "@loader_path/libepoxy.0.dylib"},
		{"libkrun.1.dylib", hp + "/opt/virglrenderer/lib/libvirglrenderer.1.dylib", "@loader_path/libvirglrenderer.1.dylib"},
		{"libkrun.1.dylib", hp + "/opt/molten-vk/lib/libMoltenVK.dylib", "@loader_path/libMoltenVK.dylib"},
		{"libkrunfw.5.dylib", hp + "/opt/libepoxy/lib/libepoxy.0.dylib", "@loader_path/libepoxy.0.dylib"},
		{"libkrunfw.5.dylib", hp + "/opt/virglrenderer/lib/libvirglrenderer.1.dylib", "@loader_path/libvirglrenderer.1.dylib"},
		{"libkrunfw.5.dylib", hp + "/opt/molten-vk/lib/libMoltenVK.dylib", "@loader_path/libMoltenVK.dylib"},
		{"libvirglrenderer.1.dylib", hp + "/opt/libepoxy/lib/libepoxy.0.dylib", "@loader_path/libepoxy.0.dylib"},
		{"libvirglrenderer.1.dylib", hp + "/opt/molten-vk/lib/libMoltenVK.dylib", "@loader_path/libMoltenVK.dylib"},
	}
	for _, r := range rewrites {
		exec.Command("install_name_tool", "-change", r.old, r.new, filepath.Join(lib, r.dylib)).Run()
	}

	// Fix install names and re-sign
	type idSign struct{ dylib, id string }
	for _, is := range []idSign{
		{"libepoxy.0.dylib", "@loader_path/libepoxy.0.dylib"},
		{"libvirglrenderer.1.dylib", "@loader_path/libvirglrenderer.1.dylib"},
		{"libMoltenVK.dylib", "@loader_path/libMoltenVK.dylib"},
	} {
		p := filepath.Join(lib, is.dylib)
		run(nil, "install_name_tool", "-id", is.id, p)
		run(nil, "codesign", "--force", "-s", "-", p)
	}

	// Fix revm libkrun references (must happen before codesign)
	revm := filepath.Join(b.binDir, "revm")
	run(nil, "install_name_tool", "-change", "libkrun.1.dylib", "@loader_path/../lib/libkrun.1.dylib", revm)
	run(nil, "install_name_tool", "-change", "libkrunfw.5.dylib", "@loader_path/../lib/libkrunfw.5.dylib", revm)

	// Sign target binary
	ent := filepath.Join(b.workspace, "revm.entitlements")
	run(nil, "codesign", "--entitlements", ent, "--force", "-s", "-", revm)
}

func (b *builder) relocateLibsLinux() {
	lib := b.libDir
	bin := filepath.Join(b.binDir, "revm")

	// Copy shared libs
	run(nil, "sh", "-c", fmt.Sprintf("cp -av %s/*.so* '%s/'", b.depLibDir("libkrun"), lib))
	run(nil, "sh", "-c", fmt.Sprintf("cp -av %s/*.so* '%s/'", b.depLibDir("libkrunfw"), lib))

	// Collect .so deps from target binary
	b.collectSoDeps(bin)

	// Copy dynamic linker
	run(nil, "cp", "-Lv", b.linuxDynLinkerPath(), lib+"/")

	// Patch rpath
	run(nil, "patchelf", "--set-rpath", "$ORIGIN/../lib", bin)
	sofiles, _ := filepath.Glob(filepath.Join(lib, "libkrun*.so.*.*"))
	for _, sf := range sofiles {
		run(nil, "patchelf", "--set-rpath", "$ORIGIN", sf)
	}
}

func (b *builder) collectSoDeps(binary string) {
	out, err := exec.Command("sh", "-c",
		fmt.Sprintf("LD_LIBRARY_PATH='%s' ldd '%s' | grep -o '/.* '", b.libDir, binary)).Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		lib := strings.TrimSpace(line)
		if lib == "" {
			continue
		}
		base := filepath.Base(lib)
		if strings.HasPrefix(base, "ld-linux") {
			continue
		}
		dst := filepath.Join(b.libDir, base)
		if exists(dst) {
			continue
		}
		run(nil, "cp", "-Lv", lib, b.libDir+"/")
	}
}

func (b *builder) writeLinuxLauncher() {
	logrus.Info("writing revm.sh launcher")

	ldName := filepath.Base(b.linuxDynLinkerPath())

	script := fmt.Sprintf(`#!/bin/sh
DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$DIR/lib/%s" --library-path "$DIR/lib" "$DIR/bin/revm" "$@"
`, ldName)

	path := filepath.Join(b.outDir, "revm.sh")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		logrus.Fatalf("write revm.sh: %v", err)
	}
}

func (b *builder) relocateLibs() {
	logrus.Info("preparing shared libraries")
	if b.goos == "darwin" {
		b.relocateLibsDarwin()
	} else {
		b.relocateLibsLinux()
		b.writeLinuxLauncher()
	}
}

func (b *builder) lint() {
	logrus.Info("running golangci-lint")
	var env []string
	if b.goos == "darwin" {
		env = []string{"PKG_CONFIG_PATH=" + b.pkgCfgDir}
	}
	run(env, "golangci-lint", "run")
}

func (b *builder) packageTar() {
	logrus.Info("packaging")
	tarName := fmt.Sprintf("revm-%s-%s.tar.zst", b.goos, b.goarch)
	tarPath := filepath.Join(b.workspace, tarName)
	run(nil, "bsdtar", "--zstd", "-cf", tarPath, "-C", b.outDir, ".")

	b.restorePlaceholders()

	logrus.Infof("build complete: %s", tarPath)
}

func (b *builder) restorePlaceholders() {
	placeholders := []string{
		filepath.Join(b.staticDir, "rootfs", "rootfs.tar.zst"),
		filepath.Join(b.serviceDir, "busybox.static"),
		filepath.Join(b.serviceDir, "dropbearmulti"),
	}
	for _, placeholder := range placeholders {
		if err := os.WriteFile(placeholder, nil, 0644); err != nil {
			logrus.Fatalf("restore placeholder %s: %v", placeholder, err)
		}
	}
}

func main() {
	dirty := flag.Bool("dirty", false, "reuse cached deps")
	verbose := flag.Bool("v", false, "enable debug logging")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run build.go [-dirty] [-v]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	b := newBuilder(*dirty)
	b.initEnv()
	b.fetchGuestEmbeddedBinaries()
	b.buildGuestAgent()
	b.fetchDeps()
	b.buildTarget()
	b.relocateLibs()
	b.lint()
	b.packageTar()
}
