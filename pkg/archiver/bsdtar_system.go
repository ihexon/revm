package archiver

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// TarOpts contains configuration options for tar operations.
type TarOpts struct {
	members      []string // Files to extract (extract only)
	excludes     []string // Files to exclude
	transformPat []string // Path transformation patterns
	sparse       bool     // Handle sparse files
	verbose      bool     // verbose output
	removeOld    bool     // Remove old files before extraction
	append       bool     // append mode (archive only)

	toStdout  bool // Extract file contents to stdout instead of disk (extract only)
	fromStdin bool // Read from stdin instead of file
	fastRead bool

	stdout io.Writer // Output to writer instead of disk
	stderr io.Writer // stderr writer
	stdin  io.Reader // stdin reader
}

// Tar implements tar archive operations.
type Tar struct {
	tarBinPath string
	opts       TarOpts
}

func NewTarInstance() *Tar {
	tarInstance := &Tar{
		tarBinPath: "bsdtar",
		opts: TarOpts{
			sparse:    true,
			removeOld: true,
			stdout:    os.Stdout,
			stderr:    os.Stderr,
			toStdout:  false,
		},
	}
	return tarInstance
}

func (t *Tar) Members(members ...string) *Tar {
	t.opts.members = append(t.opts.members, members...)
	return t
}

func (t *Tar) Excludes(excludes ...string) *Tar {
	t.opts.excludes = append(t.opts.excludes, excludes...)
	return t
}

func (t *Tar) Transform(patterns ...string) *Tar {
	t.opts.transformPat = append(t.opts.transformPat, patterns...)
	return t
}

func (t *Tar) Sparse(b bool) *Tar {
	t.opts.sparse = b
	return t
}

func (t *Tar) Verbose(b bool) *Tar {
	t.opts.verbose = b
	return t
}

func (t *Tar) RemoveOld(b bool) *Tar {
	t.opts.removeOld = b
	return t
}

func (t *Tar) Append(b bool) *Tar {
	t.opts.append = b
	return t
}

func (t *Tar) ExtractToStdout(b bool) *Tar {
	t.opts.toStdout = b
	return t
}

func (t *Tar) Stdout(w io.Writer) *Tar {
	t.opts.stdout = w
	return t
}

func (t *Tar) Stderr(w io.Writer) *Tar {
	t.opts.stderr = w
	return t
}

func (t *Tar) Stdin(r io.Reader) *Tar {
	t.opts.fromStdin = true
	t.opts.stdin = r
	return t
}

func (t *Tar) Unarchive(ctx context.Context, src, dstDir string) error {
	if dstDir != "" && !t.opts.toStdout {
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return err
		}
	}

	tarCmd := exec.CommandContext(ctx, t.tarBinPath, t.buildExtractArgs(src, dstDir)...)
	if t.opts.stdin != nil {
		tarCmd.Stdin = t.opts.stdin
	}
	if t.opts.stdout != nil {
		tarCmd.Stdout = t.opts.stdout
	}
	if t.opts.stderr != nil {
		tarCmd.Stderr = t.opts.stderr
	}

	logrus.Debugf("tar cmdline: %q", tarCmd.Args)
	return tarCmd.Run()
}

// Archive creates a tar archive from source directory.
func (t *Tar) Archive(ctx context.Context, srcDir, dstFile string) error {
	args := t.buildArchiveArgs(srcDir, dstFile)
	tarCmd := exec.CommandContext(ctx, t.tarBinPath, args...)
	if t.opts.stdin != nil {
		tarCmd.Stdin = t.opts.stdin
	}
	if t.opts.stdout != nil {
		tarCmd.Stdout = t.opts.stdout
	}
	if t.opts.stderr != nil {
		tarCmd.Stderr = t.opts.stderr
	}

	logrus.Debugf("tar cmdline: %v", tarCmd.Args)
	return tarCmd.Run()
}

func (t *Tar) buildExtractArgs(src, dstDir string) []string {
	args := []string{"--extract"}

	if dstDir != "" {
		args = append(args, "--directory", dstDir)
	}

	// bsdtar uses -s |old|new| for path transformation (no long option available)
	for _, pat := range t.opts.transformPat {
		args = append(args, "-s", pat)
	}

	if t.opts.removeOld {
		args = append(args, "--unlink-first")
	}

	for _, ex := range t.opts.excludes {
		args = append(args, "--exclude", ex)
	}

	if t.opts.sparse {
		args = append(args, "-S")
		args = append(args, "--read-sparse")
	}

	if t.opts.verbose {
		args = append(args, "--verbose")
	}

	if src != "" && src != "-" {
		args = append(args, "--file", src)
	}

	if t.opts.fromStdin {
		args = append(args, "--file", "-")
	}

	if t.opts.toStdout {
		args = append(args, "--to-stdout")
	}

	args = append(args, t.opts.members...)
	return args
}

func (t *Tar) buildArchiveArgs(srcDir, dstFile string) []string {
	var args []string

	if t.opts.append {
		args = append(args, "--append")
	} else {
		args = append(args, "--create")
	}

	if t.opts.verbose {
		args = append(args, "--verbose")
	}

	args = append(args, "--directory", filepath.Dir(srcDir))
	args = append(args, "--file", dstFile)

	// bsdtar uses -s /old/new/[flags] for path transformation (no long option available)
	for _, pat := range t.opts.transformPat {
		args = append(args, "-s", pat)
	}

	args = append(args, filepath.Base(srcDir))
	return args
}
