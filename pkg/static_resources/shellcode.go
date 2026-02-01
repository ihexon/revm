package static_resources

import (
	"bytes"

	"mvdan.cc/sh/v3/syntax"
)

// IgnitionScript builds the ignition shell script with the given parameters.
// Returns a string ready to pass to `sh -c`.
func IgnitionScript(tag, mountPoint string) string {
	// mkdir -p "$mount_point"
	mkdir := &syntax.CallExpr{
		Args: []*syntax.Word{
			word(lit("mkdir")),
			word(lit("-p")),
			word(squoted(mountPoint)),
		},
	}

	// mount -t virtiofs "$tag" "$mount_point"
	mount := &syntax.CallExpr{
		Args: []*syntax.Word{
			word(lit("mount")),
			word(lit("-t")),
			word(lit("virtiofs")),
			word(squoted(tag)),
			word(squoted(mountPoint)),
		},
	}

	// exec "$mount_point/guest-agent"
	exec := &syntax.CallExpr{
		Args: []*syntax.Word{
			word(lit("exec")),
			word(squoted(mountPoint + "/guest-agent")),
		},
	}

	// Combine: mkdir && mount && exec
	script := &syntax.BinaryCmd{
		Op: syntax.AndStmt,
		X:  &syntax.Stmt{Cmd: mkdir},
		Y: &syntax.Stmt{Cmd: &syntax.BinaryCmd{
			Op: syntax.AndStmt,
			X:  &syntax.Stmt{Cmd: mount},
			Y:  &syntax.Stmt{Cmd: exec},
		}},
	}

	var buf bytes.Buffer
	syntax.NewPrinter().Print(&buf, &syntax.Stmt{Cmd: script})
	return buf.String()
}

func word(parts ...syntax.WordPart) *syntax.Word {
	return &syntax.Word{Parts: parts}
}

func lit(s string) *syntax.Lit {
	return &syntax.Lit{Value: s}
}

func squoted(s string) *syntax.SglQuoted {
	return &syntax.SglQuoted{Value: s}
}

