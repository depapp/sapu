package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/device"
	"github.com/depapp/sapu/internal/rules"
	"github.com/depapp/sapu/internal/safety"
	"github.com/depapp/sapu/internal/ui"
)

// UninstallOptions are the flags of `sapu uninstall`.
type UninstallOptions struct {
	List     bool
	JSON     bool
	DryRun   bool
	Yes      bool
	Bloat    bool // show preinstalled apps from the bloatware catalog instead of user apps
	Packages []string
}

type appRow struct {
	device.Package
	Label    string       `json:"name,omitempty"`
	Bloat    *rules.Bloat `json:"bloat,omitempty"`
	Leftover int64        `json:"leftover_bytes"`
	Running  bool         `json:"running"`
}

// loadApps builds the inventory: user apps, or system apps the catalog knows.
func loadApps(ctx context.Context, c *adb.Client, db *rules.DB, bloat bool) ([]appRow, error) {
	pkgs, err := device.ListPackages(ctx, c, bloat)
	if err != nil {
		return nil, err
	}
	sizes, _ := device.ReadSizes(ctx, c, false)
	usage, _ := device.ReadUsage(ctx, c)
	pkgs = device.Enrich(pkgs, sizes, usage)
	var rows []appRow
	for _, p := range pkgs {
		if db.IsProtected(p.Name) {
			continue
		}
		row := appRow{Package: p, Label: db.NameFor(p.Name)}
		if b, ok := db.BloatFor(p.Name); ok {
			bb := b
			row.Bloat = &bb
		}
		if bloat && row.Bloat == nil {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// RunUninstall implements `sapu uninstall`.
func RunUninstall(ctx context.Context, c *adb.Client, db *rules.DB, opt UninstallOptions) error {
	if opt.List || opt.JSON || !ui.IsTTY() && len(opt.Packages) == 0 {
		rows, err := loadApps(ctx, c, db, opt.Bloat)
		if err != nil {
			return err
		}
		if opt.JSON || !ui.IsTTY() {
			if rows == nil {
				rows = []appRow{}
			}
			return json.NewEncoder(os.Stdout).Encode(rows)
		}
		now := time.Now()
		fmt.Println()
		fmt.Printf("%-24s %-40s %10s %10s  %s\n", ui.Bold.Render("App"), "Package", "Size", "Cache", "Last used")
		for _, r := range rows {
			fmt.Printf("%-24s %-40s %10s %10s  %s\n", ui.Truncate(r.Label, 24), ui.Truncate(r.Name, 40), ui.Bytes(r.TotalBytes()), ui.Bytes(r.CacheBytes), device.Ago(r.LastUsed, now))
		}
		return nil
	}
	if len(opt.Packages) > 0 {
		return uninstallPackages(ctx, c, db, opt, opt.Packages)
	}
	rows, err := loadApps(ctx, c, db, opt.Bloat)
	if err != nil {
		return err
	}
	m := &selectModel{rows: rows, selected: map[string]bool{}, bloat: opt.Bloat, title: "Select Apps to Remove"}
	if opt.Bloat {
		m.title = "Select Preinstalled Apps to Remove"
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	if m.cancelled || len(m.selected) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}
	var picked []string
	for _, r := range rows {
		if m.selected[r.Name] {
			picked = append(picked, r.Name)
		}
	}
	return uninstallPackages(ctx, c, db, opt, picked)
}

func uninstallPackages(ctx context.Context, c *adb.Client, db *rules.DB, opt UninstallOptions, pkgs []string) error {
	installed, err := device.InstalledSet(ctx, c)
	if err != nil {
		return err
	}
	sizes, _ := device.ReadSizes(ctx, c, false)
	system := map[string]bool{}
	if out, err := c.Shell(ctx, "pm list packages -s --user 0"); err == nil {
		for _, p := range device.ParsePackageList(out, true) {
			system[p.Name] = true
		}
	}
	type plan struct {
		pkg       string
		size      int64
		leftovers []candidate
		kept      []string
		system    bool
		running   bool
	}
	var plans []plan
	var total int64
	fmt.Println()
	fmt.Println(ui.Bold.Render("Files to be removed:"))
	for _, pkg := range pkgs {
		if !installed[pkg] {
			fmt.Printf("  %s %s · not installed\n", ui.Gray.Render(ui.IconSkip), pkg)
			continue
		}
		if db.IsProtected(pkg) {
			fmt.Printf("  %s %s · protected system package\n", ui.Gray.Render(ui.IconSkip), pkg)
			continue
		}
		pl := plan{pkg: pkg, system: system[pkg], running: device.IsRunning(ctx, c, pkg)}
		if e, ok := sizes.Packages[pkg]; ok {
			pl.size = e.App + e.Data + e.Cache
		}
		for _, dir := range []string{"/sdcard/Android/data/", "/sdcard/Android/obb/", "/sdcard/Android/media/"} {
			p := dir + pkg
			exists, err := c.Shell(ctx, "[ -e "+adb.Quote(p)+" ] && echo yes")
			if err != nil || !strings.Contains(exists, "yes") {
				continue
			}
			b := duBytes(ctx, c, p)
			if strings.HasSuffix(dir, "media/") && db.IsDataProtected(pkg) {
				pl.kept = append(pl.kept, fmt.Sprintf("%s · %s (personal media)", shortPath(p), ui.Bytes(b)))
				continue
			}
			pl.leftovers = append(pl.leftovers, candidate{Path: p, Bytes: b})
		}
		for _, l := range db.Leftovers {
			if l.Package != pkg {
				continue
			}
			p := "/sdcard/" + l.Path
			exists, err := c.Shell(ctx, "[ -e "+adb.Quote(p)+" ] && echo yes")
			if err != nil || !strings.Contains(exists, "yes") {
				continue
			}
			b := duBytes(ctx, c, p)
			if db.IsDataProtected(pkg) {
				pl.kept = append(pl.kept, fmt.Sprintf("%s · %s (personal data)", shortPath(p), ui.Bytes(b)))
			} else {
				pl.leftovers = append(pl.leftovers, candidate{Path: p, Bytes: b})
			}
		}
		fmt.Printf("\n%s %s, %s", ui.Green.Render(ui.IconOK), ui.Bold.Render(pkg), ui.Bytes(pl.size))
		if pl.system {
			fmt.Print(ui.Gray.Render("  (preinstalled: removed for this user, restorable with `sapu restore`)"))
		}
		if pl.running {
			fmt.Print(ui.Yellow.Render("  running, will be stopped"))
		}
		fmt.Println()
		total += pl.size
		for _, lo := range pl.leftovers {
			fmt.Printf("  %s %s · %s\n", ui.Green.Render(ui.IconOK), shortPath(lo.Path), ui.Bytes(lo.Bytes))
			total += lo.Bytes
		}
		if len(pl.kept) > 0 {
			fmt.Println(ui.Gray.Render("  Personal data kept:"))
			for _, k := range pl.kept {
				fmt.Println("    " + ui.Gray.Render(k))
			}
		}
		plans = append(plans, pl)
	}
	if len(plans) == 0 {
		return nil
	}
	fmt.Println()
	if opt.DryRun {
		fmt.Printf("%s %d app(s), %s would be removed.\n", ui.Yellow.Render("Dry run:"), len(plans), ui.Bytes(total))
	} else if !opt.Yes && !ui.Confirm(fmt.Sprintf("Remove %d app(s) and free about %s?", len(plans), ui.Bytes(total)), false) {
		fmt.Println("Cancelled.")
		return nil
	}
	runner, err := safety.New(c, db, "uninstall", opt.DryRun)
	if err != nil {
		return err
	}
	defer runner.Close()
	var removed int
	var freed int64
	for _, pl := range plans {
		res := runner.Uninstall(ctx, pl.pkg, pl.system, pl.size)
		if res.Action == "UNINSTALLED" || res.Action == "WOULD-UNINSTALL" {
			removed++
			freed += pl.size
			// After uninstall the package is gone, so its folders are leftovers by definition.
			delete(installed, pl.pkg)
			runner.Installed = installed
			for _, lo := range pl.leftovers {
				r := runner.Trash(ctx, lo.Path, lo.Bytes)
				if r.Action == "TRASHED" || r.Action == "WOULD-TRASH" {
					freed += lo.Bytes
				} else {
					fmt.Printf("  %s %s · %s\n", ui.Gray.Render(ui.IconSkip), shortPath(lo.Path), ui.Gray.Render(r.Reason))
				}
			}
		} else {
			fmt.Printf("  %s %s · %s\n", ui.Gray.Render(ui.IconSkip), pl.pkg, ui.Gray.Render(res.Reason))
		}
	}
	fmt.Println()
	ui.Rule()
	if opt.DryRun {
		fmt.Printf("%s\nWould remove %d app(s), about %s\n", ui.Bold.Render("Preview complete"), removed, ui.Bytes(freed))
	} else {
		fmt.Printf("%s\nRemoved %d app(s), freed about %s\n", ui.Bold.Render("Uninstall complete"), removed, ui.Bytes(freed))
	}
	ui.Rule()
	return nil
}

// RunRestore implements `sapu restore <package>` for preinstalled apps removed with -k.
func RunRestore(ctx context.Context, c *adb.Client, db *rules.DB, pkgs []string, dryRun bool) error {
	if len(pkgs) == 0 {
		out, err := c.Shell(ctx, "pm list packages -u --user 0")
		if err != nil {
			return err
		}
		installed, _ := device.InstalledSet(ctx, c)
		var gone []string
		for _, p := range device.ParsePackageList(out, false) {
			if !installed[p.Name] {
				gone = append(gone, p.Name)
			}
		}
		sort.Strings(gone)
		if len(gone) == 0 {
			fmt.Println("No removed preinstalled apps to restore.")
			return nil
		}
		fmt.Println(ui.Bold.Render("Removed preinstalled apps (restorable):"))
		for _, g := range gone {
			fmt.Println("  " + g)
		}
		fmt.Println(ui.Gray.Render("sapu restore <package>"))
		return nil
	}
	runner, err := safety.New(c, db, "restore", dryRun)
	if err != nil {
		return err
	}
	defer runner.Close()
	for _, p := range pkgs {
		res := runner.Restore(ctx, p)
		if res.Action == "RESTORED" || res.Action == "WOULD-RESTORE" {
			fmt.Printf("  %s %s\n", ui.Green.Render(ui.IconOK), p)
		} else {
			fmt.Printf("  %s %s · %s\n", ui.Gray.Render(ui.IconSkip), p, res.Reason)
		}
	}
	return nil
}

// selectModel is the multi-select list shared by uninstall.
type selectModel struct {
	title     string
	rows      []appRow
	filtered  []int
	selected  map[string]bool
	cursor    int
	query     string
	filtering bool
	cancelled bool
	bloat     bool
	height    int
}

func (m *selectModel) Init() tea.Cmd { m.applyFilter(); return nil }

func (m *selectModel) applyFilter() {
	m.filtered = m.filtered[:0]
	q := strings.ToLower(m.query)
	for i, r := range m.rows {
		if q == "" || strings.Contains(strings.ToLower(r.Name), q) || strings.Contains(strings.ToLower(r.Label), q) || (r.Bloat != nil && strings.Contains(strings.ToLower(r.Bloat.Note), q)) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

func (m *selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "enter", "esc":
				m.filtering = false
			case "backspace":
				if len(m.query) > 0 {
					m.query = m.query[:len(m.query)-1]
					m.applyFilter()
				}
			default:
				if len(msg.String()) == 1 {
					m.query += msg.String()
					m.applyFilter()
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case " ":
			if m.cursor < len(m.filtered) {
				n := m.rows[m.filtered[m.cursor]].Name
				m.selected[n] = !m.selected[n]
			}
		case "a":
			all := true
			for _, i := range m.filtered {
				if !m.selected[m.rows[i].Name] {
					all = false
				}
			}
			for _, i := range m.filtered {
				m.selected[m.rows[i].Name] = !all
			}
		case "/":
			m.filtering = true
			m.query = ""
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *selectModel) View() string {
	var b strings.Builder
	now := time.Now()
	count := 0
	var size int64
	for _, r := range m.rows {
		if m.selected[r.Name] {
			count++
			size += r.TotalBytes()
		}
	}
	fmt.Fprintf(&b, "\n %s  %s\n", ui.Title.Render(m.title), ui.Gray.Render(fmt.Sprintf("%d/%d selected · %s", count, len(m.rows), ui.Bytes(size))))
	if m.filtering || m.query != "" {
		fmt.Fprintf(&b, " / %s\n", m.query)
	}
	b.WriteString("\n")
	rows := m.height - 7
	if rows < 5 {
		rows = 25
	}
	start := 0
	if m.cursor >= rows {
		start = m.cursor - rows + 1
	}
	for i := start; i < len(m.filtered) && i < start+rows; i++ {
		r := m.rows[m.filtered[i]]
		cur := "  "
		if i == m.cursor {
			cur = ui.Blue.Render(ui.IconArrow) + " "
		}
		mark := ui.Gray.Render(ui.IconUnsel)
		if m.selected[r.Name] {
			mark = ui.Green.Render(ui.IconSel)
		}
		// Fixed 44-column name cell; ANSI codes are added after padding so widths stay exact.
		name := ui.Pad(ui.Truncate(r.Name, 44), 44)
		if r.Label != "" {
			name = ui.Pad(ui.Truncate(r.Label, 20), 21) + ui.Gray.Render(ui.Pad(ui.Truncate(r.Name, 23), 23))
		}
		extra := ""
		if r.Bloat != nil && (r.Label == "" || r.Bloat.Risk == "medium") {
			extra = ui.Gray.Render(" " + r.Bloat.Note)
			if r.Bloat.Risk == "medium" {
				extra += ui.Yellow.Render(" medium")
			}
		}
		if !r.Enabled {
			extra += ui.Gray.Render(" disabled")
		}
		fmt.Fprintf(&b, " %s%s %s %9s | %-9s%s\n", cur, mark, name, ui.Bytes(r.TotalBytes()), device.Ago(r.LastUsed, now), extra)
	}
	b.WriteString("\n " + ui.Gray.Render("↑↓ move · Space select · a all · / search · Enter continue · q quit") + "\n")
	return b.String()
}
