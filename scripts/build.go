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
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const assetsBase = "https://github.com/ihexon/revm-assets/releases/download/v2.0.17"

var buildTargets = []string{"chroot", "dockerd"}

// Edit this table when revm-assets changes.
var assetSHA256 = map[string]string{
	"alpine-rootfs-Linux-aarch64.tar.zst": "3c7dd4013ea827c63744a4da014c6b78bded60ccac51bb9519812e8a33e5e162",
	"alpine-rootfs-Linux-x86_64.tar.zst":  "74af5c8ed3806aff04d07c6b9fce84116f3b41abe80451368a9c9c1a6077b91d",
	"busybox-Linux-aarch64.tar.zst":       "27b52e5236a41b924dfe7c830ee624ac4e986605d5de1658f70132e0419cafb4",
	"busybox-Linux-x86_64.tar.zst":        "0ae53e8df8fac38beee6e575a74927d0d4b4a5a6cafe035151a75eaef818e85c",
	"dropbear-Linux-aarch64.tar.zst":      "5036f4b78a275000d8941b78f7dbf7e7bb76f76e5f4a10e85d41e28a6d06380f",
	"dropbear-Linux-x86_64.tar.zst":       "85cfe7ba283203944873bcd66459f7675f8da5e847131e0cf09930abbfac3f6e",
	"libkrun-Darwin-arm64.tar.zst":        "1a0768853800e0c0e45f1f09f90200429a63b4ed0a2cfb8abe43ebb8884a66fa",
	"libkrun-Linux-aarch64.tar.zst":       "f94aab875b5ea5727f1ac9e64b1addba86fd4b99395d533c257aa85cdb1c27a9",
	"libkrun-Linux-x86_64.tar.zst":        "ab445bebfcca7e90535f053b68097a6b7f7aa7b6198fc487c9a588241c0fa48d",
	"libkrunfw-Darwin-arm64.tar.zst":      "8e164c13f83c3549133795e6796d5530beacfbf634d8692cc751ad119020ad4a",
	"libkrunfw-Linux-aarch64.tar.zst":     "96881a57e8b5391bc6e9ff19ddb7a245fc5c0c323dadbc6b3040a8535e331a3d",
	"libkrunfw-Linux-x86_64.tar.zst":      "54ed3c7fdf24e99350c3623bae188b4eb7e7f6c3622502e8e20a0adcdb83baca",
}

type archiveAsset struct {
	name string
	url  string
}

type guestAsset struct {
	archive archiveAsset
	srcRel  string
	dstPath string
}

type targetLayout struct {
	root      string
	binDir    string
	libDir    string
	helperDir string
	launcher  string
}

type builder struct {
	verbose bool
	goos    string
	goarch  string

	workspace  string
	outDir     string
	depsDir    string
	archiveDir string

	staticDir  string
	serviceDir string
	pkgCfgPath string
	homebrew   string
}

func main() {
	verbose := flag.Bool("v", false, "enable debug logging")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run build.go [-v]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	b := newBuilder(*verbose)
	b.prepareWorkspace()
	b.prepareGuestAssets()
	b.prepareDeps()
	b.runLint()
	b.buildBundles()
	b.packageTargets()
	b.restorePlaceholders()
}

func newBuilder(verbose bool) *builder {
	workspace, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}

	arch := commandOutput(runtime.GOARCH, "uname", "-m")
	homebrew := os.Getenv("HOMEBREW_PREFIX")
	if homebrew == "" {
		homebrew = "/opt/homebrew"
	}

	b := &builder{
		verbose:    verbose,
		goos:       runtime.GOOS,
		goarch:     arch,
		workspace:  workspace,
		outDir:     filepath.Join(workspace, "out"),
		depsDir:    "/tmp/.deps",
		archiveDir: filepath.Join("/tmp/.deps", "archives"),
		staticDir:  filepath.Join(workspace, "pkg", "static_resources"),
		serviceDir: filepath.Join(workspace, "cmd", "guest-agent", "pkg", "service"),
		homebrew:   homebrew,
	}

	if b.goos == "darwin" {
		libarchive := commandOutput("", "brew", "--prefix", "libarchive")
		e2fsprogs := commandOutput("", "brew", "--prefix", "e2fsprogs")
		b.pkgCfgPath = filepath.Join(libarchive, "lib", "pkgconfig") + ":" +
			filepath.Join(e2fsprogs, "lib", "pkgconfig")
	}

	return b
}

func (b *builder) prepareWorkspace() {
	logrus.Infof("targets=%s os=%s arch=%s workspace=%s", strings.Join(buildTargets, ","), b.goos, b.goarch, b.workspace)
	removeAll(b.outDir)
	for _, target := range buildTargets {
		layout := b.layout(target)
		mkdirAll(layout.binDir)
		mkdirAll(layout.libDir)
		mkdirAll(layout.helperDir)
	}
	mkdirAll(b.archiveDir)
}

func (b *builder) prepareGuestAssets() {
	for _, asset := range b.guestAssets() {
		cacheDir := filepath.Join(b.depsDir, strings.TrimSuffix(asset.archive.name, ".tar.zst"))
		b.extractArchive(asset.archive, cacheDir)
		copyWithCP(filepath.Join(cacheDir, asset.srcRel), asset.dstPath, nil)
	}
}

func (b *builder) guestAssets() []guestAsset {
	arch := b.linuxAssetArch()
	return []guestAsset{
		{
			archive: b.asset(fmt.Sprintf("busybox-Linux-%s.tar.zst", arch)),
			srcRel:  filepath.Join("usr", "bin", "busybox"),
			dstPath: filepath.Join(b.serviceDir, "busybox.static"),
		},
		{
			archive: b.asset(fmt.Sprintf("dropbear-Linux-%s.tar.zst", arch)),
			srcRel:  filepath.Join("bin", "dropbearmulti"),
			dstPath: filepath.Join(b.serviceDir, "dropbearmulti"),
		},
	}
}

func (b *builder) prepareDeps() {
	assetOS, assetArch := b.depAssetPlatform()
	for _, name := range []string{
		fmt.Sprintf("libkrun-%s-%s.tar.zst", assetOS, assetArch),
		fmt.Sprintf("libkrunfw-%s-%s.tar.zst", assetOS, assetArch),
	} {
		asset := b.asset(name)
		extractDir := filepath.Join(b.depsDir, strings.TrimSuffix(asset.name, ".tar.zst"))
		b.extractArchive(asset, extractDir)
	}

	for _, name := range []string{"libkrun", "libkrunfw"} {
		alias := filepath.Join(b.depsDir, name)
		if !exists(alias) {
			platform, arch := b.depAssetPlatform()
			symlink(fmt.Sprintf("%s-%s-%s", name, platform, arch), alias)
		}
	}

	if b.goos == "linux" {
		for _, name := range []string{"libkrun", "libkrunfw"} {
			lib64 := filepath.Join(b.depsDir, name, "lib64")
			lib := filepath.Join(b.depsDir, name, "lib")
			if exists(lib64) && !exists(lib) {
				symlink("lib64", lib)
			}
		}
	}

	rootfs := b.asset(fmt.Sprintf("alpine-rootfs-Linux-%s.tar.zst", b.linuxAssetArch()))
	rootfsCache := filepath.Join(b.depsDir, "rootfs", "rootfs.tar.zst")
	rootfsDest := filepath.Join(b.staticDir, "rootfs", "rootfs.tar.zst")
	mkdirAll(filepath.Dir(rootfsCache))
	copyWithCP(b.archivePath(rootfs), rootfsCache, nil)
	copyWithCP(rootfsCache, rootfsDest, nil)
}

func (b *builder) buildBundles() {
	version := commandOutput("unknown", "git", "-C", b.workspace, "describe", "--tags", "--abbrev=0")
	commit := commandOutput("unknown", "git", "-C", b.workspace, "rev-parse", "--short", "HEAD")
	buildDate := time.Now().UTC().Format("20060102T150405Z")
	ldflags := fmt.Sprintf(
		"-X linuxvm/pkg/define.Version=%s -X linuxvm/pkg/define.CommitID=%s -X linuxvm/pkg/define.BuildDate=%s",
		version, commit, buildDate,
	)

	logrus.Infof("building targets (%s-%s-%s)", version, commit, buildDate)
	for _, target := range buildTargets {
		layout := b.layout(target)
		logrus.Infof("building bundle for %s in %s", target, layout.root)

		commandIn(
			filepath.Join(b.workspace, "cmd", "guest-agent"),
			[]string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + b.guestGoArch()},
			"go", "build", "-ldflags=-s -w", "-o", filepath.Join(layout.helperDir, "guest-agent"), "main.go",
		)

		env := []string{}
		if b.goos == "darwin" {
			env = append(env, "PKG_CONFIG_PATH="+b.pkgCfgPath)
		} else {
			env = append(env, "CGO_ENABLED=1")
		}
		command(env, "go", "build", "-ldflags="+ldflags, "-o", filepath.Join(layout.binDir, target), filepath.Join(b.workspace, "cmd", target))
		b.prepareRuntimeLibsFor(target, layout)
		b.writeLauncher(layout.launcher, target)
	}
}

func (b *builder) prepareRuntimeLibsFor(target string, layout targetLayout) {
	logrus.Infof("preparing shared libraries for %s", target)
	if b.goos == "darwin" {
		b.prepareRuntimeLibsDarwin(target, layout)
		return
	}
	b.prepareRuntimeLibsLinux(target, layout)
}

func (b *builder) prepareRuntimeLibsDarwin(target string, layout targetLayout) {
	libkrunLibDir := filepath.Join(b.depsDir, "libkrun", "lib")
	libkrunfwLibDir := filepath.Join(b.depsDir, "libkrunfw", "lib")

	copyWithCP(libkrunLibDir, layout.libDir+"/", func(name string) bool {
		return strings.HasSuffix(name, ".dylib")
	})
	copyWithCP(libkrunfwLibDir, layout.libDir+"/", func(name string) bool {
		return strings.HasSuffix(name, ".dylib")
	})

	for _, extra := range []string{
		filepath.Join(b.homebrew, "opt", "libepoxy", "lib", "libepoxy.0.dylib"),
		filepath.Join(b.homebrew, "opt", "virglrenderer", "lib", "libvirglrenderer.1.dylib"),
		filepath.Join(b.homebrew, "opt", "molten-vk", "lib", "libMoltenVK.dylib"),
	} {
		copyWithCP(extra, layout.libDir+"/", nil)
	}

	type rewrite struct{ dylib, old, new string }
	for _, r := range []rewrite{
		{"libkrun.1.dylib", filepath.Join(b.homebrew, "opt", "libepoxy", "lib", "libepoxy.0.dylib"), "@loader_path/libepoxy.0.dylib"},
		{"libkrun.1.dylib", filepath.Join(b.homebrew, "opt", "virglrenderer", "lib", "libvirglrenderer.1.dylib"), "@loader_path/libvirglrenderer.1.dylib"},
		{"libkrun.1.dylib", filepath.Join(b.homebrew, "opt", "molten-vk", "lib", "libMoltenVK.dylib"), "@loader_path/libMoltenVK.dylib"},
		{"libkrunfw.5.dylib", filepath.Join(b.homebrew, "opt", "libepoxy", "lib", "libepoxy.0.dylib"), "@loader_path/libepoxy.0.dylib"},
		{"libkrunfw.5.dylib", filepath.Join(b.homebrew, "opt", "virglrenderer", "lib", "libvirglrenderer.1.dylib"), "@loader_path/libvirglrenderer.1.dylib"},
		{"libkrunfw.5.dylib", filepath.Join(b.homebrew, "opt", "molten-vk", "lib", "libMoltenVK.dylib"), "@loader_path/libMoltenVK.dylib"},
		{"libvirglrenderer.1.dylib", filepath.Join(b.homebrew, "opt", "libepoxy", "lib", "libepoxy.0.dylib"), "@loader_path/libepoxy.0.dylib"},
		{"libvirglrenderer.1.dylib", filepath.Join(b.homebrew, "opt", "molten-vk", "lib", "libMoltenVK.dylib"), "@loader_path/libMoltenVK.dylib"},
	} {
		command(nil, "install_name_tool", "-change", r.old, r.new, filepath.Join(layout.libDir, r.dylib))
	}

	for _, item := range []struct{ dylib, id string }{
		{"libepoxy.0.dylib", "@loader_path/libepoxy.0.dylib"},
		{"libvirglrenderer.1.dylib", "@loader_path/libvirglrenderer.1.dylib"},
		{"libMoltenVK.dylib", "@loader_path/libMoltenVK.dylib"},
	} {
		p := filepath.Join(layout.libDir, item.dylib)
		command(nil, "install_name_tool", "-id", item.id, p)
		command(nil, "codesign", "--force", "-s", "-", p)
	}

	entitlements := filepath.Join(b.workspace, "revm.entitlements")
	bin := filepath.Join(layout.binDir, target)
	command(nil, "install_name_tool", "-change", "libkrun.1.dylib", "@loader_path/../lib/libkrun.1.dylib", bin)
	command(nil, "install_name_tool", "-change", "libkrunfw.5.dylib", "@loader_path/../lib/libkrunfw.5.dylib", bin)
	command(nil, "codesign", "--entitlements", entitlements, "--force", "-s", "-", bin)
}

func (b *builder) prepareRuntimeLibsLinux(target string, layout targetLayout) {
	copyWithCP(b.depLibDir("libkrun"), layout.libDir+"/", func(name string) bool {
		return strings.Contains(name, ".so")
	})
	copyWithCP(b.depLibDir("libkrunfw"), layout.libDir+"/", func(name string) bool {
		return strings.Contains(name, ".so")
	})

	b.collectSharedLibDeps(filepath.Join(layout.binDir, target), layout.libDir)

	copyWithCP(b.linuxDynLinkerPath(), layout.libDir+"/", nil)

	command(nil, "patchelf", "--set-rpath", "$ORIGIN/../lib", filepath.Join(layout.binDir, target))

	entries := readDir(layout.libDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "libkrun") && strings.Contains(name, ".so.") {
			command(nil, "patchelf", "--set-rpath", "$ORIGIN", filepath.Join(layout.libDir, name))
		}
	}
}

func (b *builder) collectSharedLibDeps(binary, libDir string) {
	out, err := exec.Command("sh", "-c", fmt.Sprintf("LD_LIBRARY_PATH='%s' ldd '%s' | grep -o '/.* '", libDir, binary)).Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "ld-linux") {
			continue
		}
		dst := filepath.Join(libDir, base)
		if exists(dst) {
			continue
		}
		copyWithCP(path, dst, nil)
	}
}

func (b *builder) runLint() {
	logrus.Info("running golangci-lint")
	env := []string{}
	if b.goos == "darwin" {
		env = append(env, "PKG_CONFIG_PATH="+b.pkgCfgPath)
	}
	command(env, "golangci-lint", "run")
}

func (b *builder) packageTargets() {
	logrus.Info("packaging")
	for _, target := range buildTargets {
		b.packageTarget(target)
	}
}

func (b *builder) packageTarget(target string) {
	layout := b.layout(target)
	tarName := fmt.Sprintf("%s-%s-%s.tar.zst", target, b.goos, b.goarch)
	tarPath := filepath.Join(b.workspace, tarName)
	command(nil, "bsdtar", "--zstd", "-cf", tarPath, "-C", layout.root, ".")
	logrus.Infof("build complete: %s", tarPath)
}

func (b *builder) writeLauncher(path, target string) {
	script := fmt.Sprintf("#!/bin/sh\nDIR=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n%s\n", b.launcherCommand(target))
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		fatalf("write %s: %v", path, err)
	}
}

func (b *builder) launcherCommand(target string) string {
	if b.goos == "darwin" {
		return fmt.Sprintf("exec \"$DIR/bin/%s\" \"$@\"", target)
	}
	ldName := filepath.Base(b.linuxDynLinkerPath())
	return fmt.Sprintf("exec \"$DIR/lib/%s\" --library-path \"$DIR/lib\" \"$DIR/bin/%s\" \"$@\"", ldName, target)
}

func (b *builder) layout(target string) targetLayout {
	root := filepath.Join(b.outDir, target)
	return targetLayout{
		root:      root,
		binDir:    filepath.Join(root, "bin"),
		libDir:    filepath.Join(root, "lib"),
		helperDir: filepath.Join(root, "helper"),
		launcher:  filepath.Join(root, target),
	}
}

func (b *builder) restorePlaceholders() {
	for _, path := range []string{
		filepath.Join(b.staticDir, "rootfs", "rootfs.tar.zst"),
		filepath.Join(b.serviceDir, "busybox.static"),
		filepath.Join(b.serviceDir, "dropbearmulti"),
	} {
		if err := os.WriteFile(path, nil, 0644); err != nil {
			fatalf("restore placeholder %s: %v", path, err)
		}
	}
}

func (b *builder) asset(name string) archiveAsset {
	return archiveAsset{
		name: name,
		url:  assetsBase + "/" + name,
	}
}

func (b *builder) depAssetPlatform() (string, string) {
	if b.goos == "darwin" {
		return "Darwin", "arm64"
	}
	return "Linux", b.linuxAssetArch()
}

func (b *builder) guestGoArch() string {
	switch b.goarch {
	case "arm64", "aarch64":
		return "arm64"
	case "amd64", "x86_64":
		return "amd64"
	default:
		fatalf("unsupported host arch: %s", b.goarch)
		return ""
	}
}

func (b *builder) linuxAssetArch() string {
	switch b.goarch {
	case "arm64", "aarch64":
		return "aarch64"
	case "amd64", "x86_64":
		return "x86_64"
	default:
		fatalf("unsupported host arch: %s", b.goarch)
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
	fatalf("missing shared library directory for %s", name)
	return ""
}

func (b *builder) linuxDynLinkerPath() string {
	var candidates []string
	switch b.goarch {
	case "arm64", "aarch64":
		candidates = []string{
			"/lib/ld-linux-aarch64.so.1",
			"/lib64/ld-linux-aarch64.so.1",
			"/lib/aarch64-linux-gnu/ld-linux-aarch64.so.1",
		}
	case "amd64", "x86_64":
		candidates = []string{
			"/lib64/ld-linux-x86-64.so.2",
			"/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2",
			"/lib/ld-linux-x86-64.so.2",
		}
	default:
		fatalf("unsupported host arch: %s", b.goarch)
	}

	for _, path := range candidates {
		if exists(path) {
			return path
		}
	}

	fatalf("failed to find dynamic linker for linux/%s (tried: %s)", b.goarch, strings.Join(candidates, ", "))
	return ""
}

func (b *builder) archivePath(asset archiveAsset) string {
	archivePath := filepath.Join(b.archiveDir, asset.name)
	expected := strings.TrimSpace(assetSHA256[asset.name])
	if expected == "" {
		fatalf("missing sha256 for asset %s", asset.name)
	}

	if exists(archivePath) {
		actual := sha256File(archivePath)
		if actual == expected {
			logrus.Infof("reusing cached archive %s", asset.name)
			return archivePath
		}
		logrus.Infof("cached archive %s failed checksum validation, downloading again", asset.name)
	}

	tmpPath := archivePath + ".tmp"
	_ = os.Remove(tmpPath)

	logrus.Infof("downloading %s", asset.name)
	downloadFile(asset.url, tmpPath)

	actual := sha256File(tmpPath)
	if actual != expected {
		_ = os.Remove(tmpPath)
		fatalf("sha256 mismatch for %s: expected %s, got %s", asset.name, expected, actual)
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		fatalf("rename %s -> %s: %v", tmpPath, archivePath, err)
	}
	return archivePath
}

func (b *builder) extractArchive(asset archiveAsset, dest string) {
	archivePath := b.archivePath(asset)
	removeAll(dest)
	mkdirAll(dest)
	command(nil, "bsdtar", "--zstd", "-xf", archivePath, "-C", dest)
}

func command(env []string, args ...string) {
	if len(args) == 0 {
		fatalf("empty command")
	}
	logrus.Debugf("exec: %s", strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		fatalf("command failed: %s\n  %v", strings.Join(args, " "), err)
	}
}

func commandIn(dir string, env []string, args ...string) {
	if len(args) == 0 {
		fatalf("empty command")
	}
	logrus.Debugf("exec (in %s): %s", dir, strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		fatalf("command failed (in %s): %s\n  %v", dir, strings.Join(args, " "), err)
	}
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

func downloadFile(url, dst string) {
	resp, err := http.Get(url)
	if err != nil {
		fatalf("download %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fatalf("download %s: unexpected status %s", url, resp.Status)
	}

	mkdirAll(filepath.Dir(dst))
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fatalf("create %s: %v", dst, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		fatalf("write %s: %v", dst, err)
	}
}

func sha256File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		fatalf("hash %s: %v", path, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mkdirAll(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		fatalf("mkdir %s: %v", path, err)
	}
}

func removeAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		fatalf("rm -rf %s: %v", path, err)
	}
}

func copyWithCP(src, dst string, match func(name string) bool) {
	logrus.Debugf("cp -av %s %s", src, dst)
	if match == nil {
		command(nil, "cp", "-av", src, dst)
		return
	}

	srcPath := strings.TrimSuffix(src, string(os.PathSeparator))
	info, err := os.Stat(srcPath)
	if err != nil {
		fatalf("stat %s: %v", srcPath, err)
	}
	if !info.IsDir() {
		fatalf("copy filter requires directory source: %s", srcPath)
	}

	for _, entry := range readDir(srcPath) {
		if entry.IsDir() {
			continue
		}
		if !match(entry.Name()) {
			continue
		}
		command(nil, "cp", "-av", filepath.Join(srcPath, entry.Name()), dst)
	}
}

func readDir(path string) []os.DirEntry {
	entries, err := os.ReadDir(path)
	if err != nil {
		fatalf("readDir %s: %v", path, err)
	}
	return entries
}

func symlink(target, path string) {
	if err := os.Symlink(target, path); err != nil {
		fatalf("symlink %s -> %s: %v", path, target, err)
	}
}

func fatalf(format string, args ...any) {
	logrus.Fatalf(format, args...)
}
