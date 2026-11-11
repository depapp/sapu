package commands

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/device"
	"github.com/depapp/sapu/internal/rules"
	"github.com/depapp/sapu/internal/safety"
	"github.com/depapp/sapu/internal/ui"
)

// CleanOptions are the flags of `sapu clean`.
type CleanOptions struct {
	DryRun bool
	Yes    bool
}

type candidate struct {
	Path    string
	Bytes   int64
	Section string
	Note    string
	Owner   string
	Skip    string // non-empty: shown as skipped with this reason, never sent to the funnel
}

// RunClean implements `sapu clean`: known-safe targets only, every removal through the funnel.
func RunClean(ctx context.Context, c *adb.Client, db *rules.DB, opt CleanOptions) error {
	info, err := device.ReadInfo(ctx, c)
	if err != nil {
		return err
	}
	st, _ := device.ReadStorage(ctx, c)
	fmt.Println()
	fmt.Println(ui.Title.Render("Clean Your Android"))
	fmt.Println()
	fmt.Printf("%s %s %s · Android %s · Free space: %s\n", ui.Gray.Render("⚙"), info.Manufacturer, info.Model, info.Android, ui.Bytes(st.FreeBytes))
	if opt.DryRun {
		fmt.Println(ui.Yellow.Render("Dry run: nothing is removed."))
	}

	installed, err := device.InstalledSet(ctx, c)
	if err != nil {
		return err
	}
	runner, err := safety.New(c, db, "clean", opt.DryRun)
	if err != nil {
		return err
	}
	runner.Installed = installed
	defer runner.Close()

	var freed, skipped int64
	var items int
	apply := func(cands []candidate) {
		for _, cd := range cands {
			if cd.Skip != "" {
				skipped += cd.Bytes
				ui.Row(false, fmt.Sprintf("%s · %s %s", shortPath(cd.Path), ui.Bytes(cd.Bytes), ui.Gray.Render("skipped: "+cd.Skip)))
				continue
			}
			res := runner.Trash(ctx, cd.Path, cd.Bytes)
			switch res.Action {
			case "TRASHED", "WOULD-TRASH":
				freed += cd.Bytes
				items++
				label := "trashed"
				if res.Action == "WOULD-TRASH" {
					label = "would trash"
				}
				ui.Row(true, fmt.Sprintf("%s · %s %s", shortPath(cd.Path), ui.Bytes(cd.Bytes), ui.Gray.Render(label)))
			default:
				skipped += cd.Bytes
				ui.Row(false, fmt.Sprintf("%s · %s %s", shortPath(cd.Path), ui.Bytes(cd.Bytes), ui.Gray.Render("skipped: "+res.Reason)))
			}
		}
	}

	// Scan first so the plan can be shown and confirmed before anything moves.
	left := findLeftovers(ctx, c, db, installed)
	// A target nested under a leftover already listed is dropped so the total counts bytes once.
	bySection := map[string][]candidate{}
	var order []string
	for _, cd := range dropNested(findCleanable(ctx, c, db, installed), left) {
		if _, ok := bySection[cd.Section]; !ok {
			order = append(order, cd.Section)
		}
		bySection[cd.Section] = append(bySection[cd.Section], cd)
	}
	sizes, sizeErr := device.ReadSizes(ctx, c, true)
	before := sizes.TotalCache()

	const leftoversTitle = "Leftovers of uninstalled apps"
	const cachesTitle = "App caches (system eviction, permanent)"
	showSections := func(each func(cands []candidate)) {
		ui.Section(leftoversTitle)
		if len(left) == 0 {
			ui.Row(false, "none found")
		}
		each(left)
		for _, name := range order {
			ui.Section(name)
			each(bySection[name])
		}
		ui.Section(cachesTitle)
		if sizeErr != nil {
			ui.Row(false, "per-app cache sizes unavailable: "+sizeErr.Error())
		} else {
			fmt.Printf("  %s %d apps hold %s of cache\n", ui.Gray.Render("·"), len(sizes.Packages), ui.Bytes(before))
		}
	}

	if !opt.DryRun && !opt.Yes {
		var planned int64
		var count int
		showSections(func(cands []candidate) {
			for _, cd := range cands {
				skip := cd.Skip
				if skip == "" {
					_, skip = runner.CheckTrash(cd.Path)
				}
				if skip != "" {
					ui.Row(false, fmt.Sprintf("%s · %s %s", shortPath(cd.Path), ui.Bytes(cd.Bytes), ui.Gray.Render("skipped: "+skip)))
					continue
				}
				planned += cd.Bytes
				count++
				ui.Row(true, fmt.Sprintf("%s · %s %s", shortPath(cd.Path), ui.Bytes(cd.Bytes), ui.Gray.Render("will trash")))
			}
		})
		ui.Row(true, fmt.Sprintf("Will evict by Android (pm trim-caches) · up to %s %s", ui.Bytes(before), ui.Gray.Render("permanent, same as Settings > Storage > Clear cache")))
		fmt.Println()
		if !ui.Confirm(fmt.Sprintf("Move %d item(s) (%s) to the trash and evict app caches?", count, ui.Bytes(planned)), false) {
			fmt.Println(ui.Gray.Render("Cancelled. Nothing was changed; use --yes to skip this question."))
			return nil
		}
	}

	showSections(apply)
	res := runner.TrimCaches(ctx)
	switch res.Action {
	case "TRIMMED":
		after, err := device.ReadSizes(ctx, c, true)
		if err == nil && before > after.TotalCache() {
			d := before - after.TotalCache()
			freed += d
			items++
			ui.Row(true, fmt.Sprintf("Evicted by Android (pm trim-caches) · %s %s", ui.Bytes(d), ui.Gray.Render("not in trash")))
		} else {
			ui.Row(true, "Evicted by Android (pm trim-caches) · done "+ui.Gray.Render("not in trash"))
		}
	case "WOULD-TRIM":
		ui.Row(true, fmt.Sprintf("Would evict by Android (pm trim-caches) · up to %s %s", ui.Bytes(before), ui.Gray.Render("permanent, same as Settings > Storage > Clear cache")))
		freed += before
	default:
		ui.Row(false, "Cache eviction · skipped: "+res.Reason)
	}

	fmt.Println()
	ui.Rule()
	if opt.DryRun {
		fmt.Printf("%s\nPotential space: %s · %d items\n", ui.Bold.Render("Preview complete"), ui.Bytes(freed), items)
	} else {
		after, _ := device.ReadStorage(ctx, c)
		fmt.Printf("%s\nRemoved: %s · %d items\nFree space: %s (+%s)\n", ui.Bold.Render("Cleanup complete"), ui.Bytes(freed), items, ui.Bytes(after.FreeBytes), ui.Bytes(maxInt64(after.FreeBytes-st.FreeBytes, 0)))
		fmt.Println(ui.Gray.Render("Trashed items are in " + safety.TrashRoot + "/" + runner.RunID + " · restore with: sapu trash restore " + runner.RunID))
	}
	if skipped > 0 {
		fmt.Println(ui.Gray.Render(fmt.Sprintf("Skipped: %s (see reasons above)", ui.Bytes(skipped))))
	}
	ui.Rule()
	return nil
}

// findLeftovers lists Android/{data,obb,media}/<pkg> and known root folders whose package is gone.
func findLeftovers(ctx context.Context, c *adb.Client, db *rules.DB, installed map[string]bool) []candidate {
	var out []candidate
	for _, dir := range []string{"/sdcard/Android/data", "/sdcard/Android/obb", "/sdcard/Android/media"} {
		list, err := c.Shell(ctx, "ls -1 "+adb.Quote(dir)+" 2>/dev/null")
		if err != nil {
			continue
		}
		for _, name := range strings.Split(strings.TrimSpace(list), "\n") {
			name = strings.TrimSpace(name)
			if name == "" || installed[name] || !looksLikePackage(name) {
				continue
			}
			p := dir + "/" + name
			if db.IsDataProtected(name) && strings.HasSuffix(dir, "/media") {
				out = append(out, candidate{Path: p, Bytes: duBytes(ctx, c, p), Section: "Leftovers", Note: "personal media"})
				continue
			}
			out = append(out, candidate{Path: p, Bytes: duBytes(ctx, c, p), Section: "Leftovers", Owner: name})
		}
	}
	for i := range out {
		if out[i].Bytes == 0 && !readable(ctx, c, out[i].Path) {
			out[i].Skip = "not readable by adb shell"
		}
	}
	for _, l := range db.Leftovers {
		if l.Package == "__never__" || installed[l.Package] || db.IsDataProtected(l.Package) {
			continue
		}
		p := "/sdcard/" + l.Path
		exists, err := c.Shell(ctx, "[ -e "+adb.Quote(p)+" ] && echo yes")
		if err != nil || !strings.Contains(exists, "yes") {
			continue
		}
		out = append(out, candidate{Path: p, Bytes: duBytes(ctx, c, p), Section: "Leftovers", Owner: l.Package})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// findCleanable expands every catalog glob on the device with age filtering.
func findCleanable(ctx context.Context, c *adb.Client, db *rules.DB, installed map[string]bool) []candidate {
	var out []candidate
	seen := map[string]bool{}
	running := map[string]bool{}
	for _, rule := range db.Cleanable {
		var cmd string
		q := adb.Quote("/sdcard/" + rule.Glob)
		// The glob expands in the device shell; -maxdepth 0 keeps the match itself.
		age := ""
		if rule.MinAgeDays > 0 {
			age = fmt.Sprintf(" -mtime +%d", rule.MinAgeDays-1)
		}
		if rule.Kind == "dir" {
			cmd = "for p in " + strings.Trim(q, "'") + "; do [ -d \"$p\" ] && find \"$p\" -maxdepth 0 -type d" + age + " 2>/dev/null; done"
		} else {
			cmd = "for p in " + strings.Trim(q, "'") + "; do [ -f \"$p\" ] && find \"$p\" -maxdepth 0 -type f" + age + " 2>/dev/null; done"
		}
		list, err := c.ShellTimeout(ctx, 30*time.Second, cmd)
		if err != nil {
			continue
		}
		for _, p := range strings.Split(strings.TrimSpace(list), "\n") {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] || strings.Contains(p, "*") {
				continue
			}
			seen[p] = true
			cd := candidate{Path: p, Section: rule.Name, Note: rule.Note}
			if owner := safety.LeftoverOwner(p); owner != "" {
				cd.Owner = owner
				if rule.RunningGuard && installed[owner] {
					if _, ok := running[owner]; !ok {
						running[owner] = device.IsRunning(ctx, c, owner)
					}
					if running[owner] {
						out = append(out, candidate{Path: p, Bytes: duBytes(ctx, c, p), Section: rule.Name, Owner: owner, Skip: owner + " is running"})
						continue
					}
				}
			}
			cd.Bytes = duBytes(ctx, c, p)
			if cd.Bytes == 0 && rule.Kind == "dir" {
				continue
			}
			out = append(out, cd)
		}
	}
	return out
}

// dropNested removes candidates that live under another candidate, so a
// leftover folder and its cache subfolder are not both counted.
func dropNested(cands []candidate, parents []candidate) []candidate {
	all := append(append([]candidate{}, parents...), cands...)
	var out []candidate
	for _, cd := range cands {
		nested := false
		for _, p := range all {
			if p.Path != cd.Path && strings.HasPrefix(cd.Path, p.Path+"/") {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, cd)
		}
	}
	return out
}

func duBytes(ctx context.Context, c *adb.Client, p string) int64 {
	out, err := c.ShellTimeout(ctx, 60*time.Second, "du -sk "+adb.Quote(p)+" 2>/dev/null")
	if err != nil {
		return 0
	}
	f := strings.Fields(out)
	if len(f) == 0 {
		return 0
	}
	kb, _ := strconv.ParseInt(f[0], 10, 64)
	return kb * 1024
}

// readable reports whether the adb shell user can list the directory. A
// folder created by an app with restrictive permissions shows as 0 bytes
// and `mv` would fail on it, so the preview must not promise it.
func readable(ctx context.Context, c *adb.Client, p string) bool {
	out, err := c.ShellTimeout(ctx, 15*time.Second, "test -r "+adb.Quote(p)+" -a -x "+adb.Quote(p)+" && echo ok")
	return err == nil && strings.Contains(out, "ok")
}

func looksLikePackage(name string) bool {
	return strings.Contains(name, ".") && !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, " /")
}

func shortPath(p string) string {
	return strings.Replace(p, "/sdcard/", "~/", 1)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

var _ = path.Base
