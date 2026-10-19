package execution

import (
	"fmt"
	"os"
	"syscall"
	"testing"
)

func TestExecFailuresUseShellStatus(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{{os.ErrNotExist, 127}, {os.ErrPermission, 126}, {syscall.ENOEXEC, 126}, {syscall.EISDIR, 126}} {
		result := FromProcess(fmt.Errorf("start: %w", tc.err))
		if result.ExitCode() != tc.code || result.Kind != "start" {
			t.Fatalf("%v: %+v", tc.err, result)
		}
	}
}
