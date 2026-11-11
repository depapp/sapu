package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/safety"
	"github.com/depapp/sapu/internal/ui"
)

// RunHistory implements `sapu history`.
func RunHistory(jsonOut bool, limit int) error {
	rows, p, err := safety.ReadHistory()
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"log": p, "rows": rows})
	}
	if len(rows) == 0 {
		fmt.Println("No history yet. Sapu records every run in", p)
		return nil
	}
	type session struct {
		id, command, serial string
		rows                []safety.Row
	}
	var sessions []*session
	idx := map[string]*session{}
	for _, r := range rows {
		s, ok := idx[r.RunID]
		if !ok {
			s = &session{id: r.RunID}
			idx[r.RunID] = s
			sessions = append(sessions, s)
		}
		switch r.Action {
		case "SESSION-START":
			s.command, s.serial = r.Target, r.Reason
		case "SESSION-END":
		default:
			s.rows = append(s.rows, r)
		}
	}
	if limit > 0 && len(sessions) > limit {
		sessions = sessions[len(sessions)-limit:]
	}
	fmt.Println()
	fmt.Println(ui.Title.Render("Sapu History"), ui.Gray.Render(p))
	for _, s := range sessions {
		var bytes int64
		counts := map[string]int{}
		for _, r := range s.rows {
			counts[r.Action]++
			if !strings.HasPrefix(r.Action, "SKIPPED") {
				bytes += r.Bytes
			}
		}
		var parts []string
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], strings.ToLower(k)))
		}
		fmt.Printf("\n%s %s  %s  %s\n", ui.Blue.Render(ui.IconSection), ui.Bold.Render(s.id), s.command, ui.Gray.Render(strings.Join(parts, " · ")+" · "+ui.Bytes(bytes)))
		for _, r := range s.rows {
			line := fmt.Sprintf("%-16s %s", r.Action, shortPath(r.Target))
			if r.Bytes > 0 {
				line += " · " + ui.Bytes(r.Bytes)
			}
			if r.Reason != "" {
				line += " " + ui.Gray.Render(r.Reason)
			}
			fmt.Println("  " + line)
		}
	}
	return nil
}

// RunTrash implements `sapu trash [list|restore <run>|empty]` for the on-device trash.
func RunTrash(ctx context.Context, c *adb.Client, args []string, dryRun bool) error {
	q := adb.Quote
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		out, _ := c.Shell(ctx, "ls -1 "+q(safety.TrashRoot)+" 2>/dev/null")
		runs := strings.Fields(out)
		if len(runs) == 0 {
			fmt.Println("Trash is empty:", safety.TrashRoot)
			return nil
		}
		fmt.Println(ui.Title.Render("Sapu Trash"), ui.Gray.Render(safety.TrashRoot))
		for _, r := range runs {
			size := duBytes(ctx, c, safety.TrashRoot+"/"+r)
			n, _ := c.Shell(ctx, "ls -1 "+q(safety.TrashRoot+"/"+r)+" | grep -vc manifest.tsv")
			fmt.Printf("  %s  %s · %s items\n", r, ui.Bytes(size), strings.TrimSpace(n))
		}
		fmt.Println(ui.Gray.Render("sapu trash restore <run>  ·  sapu trash empty"))
	case "restore":
		if len(args) < 2 {
			return fmt.Errorf("usage: sapu trash restore <run-id>")
		}
		run := args[1]
		if strings.ContainsAny(run, "/ .") || strings.Contains(run, "..") {
			return fmt.Errorf("invalid run id")
		}
		man, err := c.Shell(ctx, "cat "+q(safety.TrashRoot+"/"+run+"/manifest.tsv"))
		if err != nil {
			return fmt.Errorf("no manifest for %s: %v", run, err)
		}
		runDir := safety.TrashRoot + "/" + run
		restored := 0
		var remaining []string
		for _, line := range strings.Split(strings.TrimSpace(man), "\n") {
			from, to, ok := strings.Cut(line, "\t")
			if !ok {
				continue
			}
			// The manifest lives on the phone, so only the destination side is
			// trusted after validation; the source must be a direct child of
			// this run's folder or the line is ignored.
			if rest, ok := strings.CutPrefix(from, runDir+"/"); !ok || rest == "" || rest == "manifest.tsv" || strings.ContainsAny(rest, "/\r\n") || rest == "." || rest == ".." {
				fmt.Println("  skip", shortPath(to), "· manifest source is not inside this run's trash")
				remaining = append(remaining, line)
				continue
			}
			if _, err := safety.Validate(to); err != nil {
				fmt.Println("  skip", to, "·", err)
				remaining = append(remaining, line)
				continue
			}
			if dryRun {
				fmt.Println("  would restore", shortPath(to))
				continue
			}
			out, err := c.Shell(ctx, fmt.Sprintf("mkdir -p %s && [ ! -e %s ] && mv %s %s && echo ok", q(parentDir(to)), q(to), q(from), q(to)))
			if err == nil && strings.Contains(out, "ok") {
				restored++
				fmt.Println("  "+ui.Green.Render(ui.IconOK), shortPath(to))
			} else {
				fmt.Println("  "+ui.Gray.Render(ui.IconSkip), shortPath(to), ui.Gray.Render("exists or failed"))
				remaining = append(remaining, line)
			}
		}
		if !dryRun && restored > 0 {
			manifest := runDir + "/manifest.tsv"
			if len(remaining) == 0 {
				_, _ = c.Shell(ctx, "rm -f "+q(manifest)+" && rmdir "+q(runDir)+" 2>/dev/null || true") // SAFE: only the run's own manifest and its then-empty folder under TrashRoot.
			} else {
				// Items that stayed behind keep their manifest lines so a later
				// restore can still find them.
				quoted := make([]string, 0, len(remaining))
				for _, l := range remaining {
					quoted = append(quoted, q(l))
				}
				_, _ = c.Shell(ctx, "printf '%s\\n' "+strings.Join(quoted, " ")+" > "+q(manifest))
			}
		}
		fmt.Printf("Restored %d items\n", restored)
		if !dryRun && len(remaining) > 0 {
			fmt.Printf("%d item(s) still in trash: %s\n", len(remaining), runDir)
		}
	case "empty":
		size := duBytes(ctx, c, safety.TrashRoot)
		if dryRun {
			fmt.Printf("Would permanently delete %s in %s\n", ui.Bytes(size), safety.TrashRoot)
			return nil
		}
		if !ui.Confirm(fmt.Sprintf("Permanently delete %s in %s?", ui.Bytes(size), safety.TrashRoot), false) {
			return nil
		}
		_, err := c.Shell(ctx, "rm -rf "+q(safety.TrashRoot)) // SAFE: TrashRoot is a constant Sapu-owned folder; nothing else is ever placed under it.
		if err != nil {
			return err
		}
		fmt.Println("Trash emptied, freed", ui.Bytes(size))
	default:
		return fmt.Errorf("unknown trash command %q (list, restore <run>, empty)", sub)
	}
	return nil
}

func parentDir(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "/sdcard"
}
