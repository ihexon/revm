package revmcmd

import (
	"fmt"
	"io"
	"linuxvm/pkg/revm"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

func writeSessions(w io.Writer, views []revm.SessionView, now time.Time) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SESSION\tMODE\tSTATUS\tSTARTED\tDURATION\tEXIT\tCOMMAND"); err != nil {
		return err
	}
	for _, v := range views {
		mode := string(v.Config.RunMode)
		switch v.Config.RunMode {
		case revm.ModeRootfs:
			mode = "run"
		case revm.ModeContainer:
			mode = "podman"
		}
		started, duration, code := "-", "-", "-"
		if r := v.LastRun; r != nil {
			if !r.StartedAt.IsZero() {
				started = r.StartedAt.Format(time.RFC3339)
				end := now
				if r.FinishedAt != nil {
					end = *r.FinishedAt
				}
				if r.FinishedAt != nil || v.Active {
					duration = max(time.Duration(0), end.Sub(r.StartedAt)).Round(time.Second).String()
				}
			}
			if r.ExitCode != nil {
				code = strconv.Itoa(*r.ExitCode)
			}
		}
		command := make([]string, len(v.Config.Exec.Argv))
		for i, arg := range v.Config.Exec.Argv {
			if arg == "" || strings.ContainsAny(arg, "\"'\\") || strings.IndexFunc(arg, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
				arg = strconv.Quote(arg)
			}
			command[i] = arg
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", tableCell(v.ID), tableCell(mode), tableCell(v.Status), started, duration, code, strings.Join(command, " ")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeVolumes(w io.Writer, views []revm.VolumeView) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tIN USE\tCAPACITY\tALLOCATED\tPATH\tERROR"); err != nil {
		return err
	}
	for _, v := range views {
		if _, err := fmt.Fprintf(tw, "%s\t%t\t%s\t%s\t%s\t%s\n", tableCell(v.Name), v.InUse, formatBytes(v.SizeBytes), formatBytes(v.AllocatedBytes), tableCell(v.Path), tableCell(v.Error)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatBytes(n *int64) string {
	if n == nil {
		return "-"
	}
	value := float64(*n)
	for _, unit := range []string{"B", "KiB", "MiB", "GiB", "TiB"} {
		if value < 1024 || unit == "TiB" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
		value /= 1024
	}
	return "-"
}
func tableCell(s string) string {
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return strconv.Quote(s)
	}
	return s
}
