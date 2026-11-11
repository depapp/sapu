// Sapu: sweep your Android clean from the terminal, over adb, no root.
// main is a router only: it parses arguments, draws the menu and dispatches.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/commands"
	"github.com/depapp/sapu/internal/rules"
	"github.com/depapp/sapu/internal/ui"
)

// Version is set by the release build (-ldflags "-X main.Version=...").
var Version = "0.2.0"

const tagline = "Sweep your Android clean."

const usage = `Sapu ` + tagline + `

Usage: sapu [--serial SERIAL] <command> [options]

Commands:
  clean        Trim app caches, remove leftovers and disposable files
  uninstall    Remove apps and their leftovers (--bloat for preinstalled apps)
  analyze      Explore storage usage
  status       Live health dashboard
  optimize     Compile most used apps, run bg-dexopt, battery audit
  restore      Reinstall a preinstalled app removed with uninstall
  trash        List, restore or empty Sapu's trash on the phone
  history      What Sapu did, by run
  devices      List connected devices
  rules        Show or check the bundled rules database
  probe        Anonymized device report to paste into a rules issue
  update       Update sapu to the latest release (--check only looks)

Preview safely:
  sapu clean --dry-run
  sapu optimize --dry-run
  sapu uninstall --dry-run
  sapu uninstall --list --json
  sapu analyze --json
  sapu status --json

Options:
  --serial     Pick a device when several are connected (or $SAPU_SERIAL)
  --dry-run    Show what would change without changing it
  --json       Machine-readable output (automatic when piped)
  -h, --help   Show help
  -V, --version Show version

Files Sapu removes go to /sdcard/.sapu/trash/<run> (sapu trash restore). The
one permanent step is the app cache eviction in 'sapu clean', which is the same
"Clear cache" Android performs in Settings > Storage. Preinstalled apps are
removed for the current user only and come back with 'sapu restore'. No root.
`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, ui.Red.Render("✗")+" "+err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	var serial string
	var dryRun, jsonOut, yes, help, version bool
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--serial", "-s":
			if i+1 < len(args) {
				serial = args[i+1]
				i++
			}
		case "--dry-run", "-n":
			dryRun = true
		case "--json":
			jsonOut = true
		case "--yes", "-y":
			yes = true
		case "--debug":
			os.Setenv("SAPU_DEBUG", "1")
		case "-h", "--help", "help":
			help = true
		case "-V", "--version", "version":
			version = true
		default:
			if strings.HasPrefix(a, "--serial=") {
				serial = strings.TrimPrefix(a, "--serial=")
			} else {
				rest = append(rest, a)
			}
		}
	}
	if version {
		fmt.Printf("sapu %s\n", Version)
		return nil
	}
	if help {
		fmt.Print(usage)
		return nil
	}
	db, err := rules.Load()
	if err != nil {
		return err
	}
	cmd := ""
	if len(rest) > 0 {
		cmd, rest = rest[0], rest[1:]
	}
	switch cmd {
	case "history":
		limit := 20
		for i, a := range rest {
			if a == "--limit" && i+1 < len(rest) {
				fmt.Sscanf(rest[i+1], "%d", &limit)
			}
		}
		return commands.RunHistory(jsonOut || !ui.IsTTY(), limit)
	case "rules":
		if len(rest) > 0 && rest[0] == "check" {
			if p := db.Validate(); len(p) > 0 {
				return fmt.Errorf("%s", strings.Join(p, "\n"))
			}
			fmt.Println("rules ok")
			return nil
		}
		fmt.Printf("bloatware: %d packages · leftover folders: %d · cleanable targets: %d\n", len(db.Bloat), len(db.Leftovers), len(db.Cleanable))
		return nil
	case "update":
		check := false
		for _, a := range rest {
			if a == "--check" {
				check = true
			}
		}
		return commands.RunUpdate(ctx, Version, check)
	case "devices":
		bin, err := adb.FindBinary()
		if err != nil {
			return err
		}
		devs, err := adb.Devices(ctx, bin)
		if err != nil {
			return err
		}
		if len(devs) == 0 {
			return adb.ErrNoDevice
		}
		for _, d := range devs {
			fmt.Printf("%-24s %-12s %s\n", d.Serial, d.State, d.Model)
		}
		return nil
	}

	c, err := adb.Connect(ctx, serial)
	if err != nil {
		return err
	}
	switch cmd {
	case "":
		return menu(ctx, c, db)
	case "status":
		watch := false
		interval := time.Second
		for i, a := range rest {
			if a == "--watch" {
				watch = true
			}
			if a == "--interval" && i+1 < len(rest) {
				if d, err := time.ParseDuration(rest[i+1]); err == nil {
					interval = d
				}
			}
		}
		return commands.RunStatus(ctx, c, jsonOut, watch, interval)
	case "analyze", "analyse":
		target := ""
		if len(rest) > 0 {
			target = rest[0]
		}
		return commands.RunAnalyze(ctx, c, db, target, jsonOut, dryRun)
	case "clean":
		for _, a := range rest {
			if a == "--whitelist" {
				return fmt.Errorf("edit ~/.config/sapu/whitelist: one /sdcard path per line")
			}
		}
		return commands.RunClean(ctx, c, db, commands.CleanOptions{DryRun: dryRun, Yes: yes})
	case "uninstall":
		opt := commands.UninstallOptions{DryRun: dryRun, JSON: jsonOut, Yes: yes}
		for _, a := range rest {
			switch a {
			case "--list":
				opt.List = true
			case "--bloat", "--system":
				opt.Bloat = true
			default:
				if !strings.HasPrefix(a, "-") {
					opt.Packages = append(opt.Packages, a)
				}
			}
		}
		return commands.RunUninstall(ctx, c, db, opt)
	case "restore":
		return commands.RunRestore(ctx, c, db, rest, dryRun)
	case "trash":
		return commands.RunTrash(ctx, c, rest, dryRun)
	case "optimize":
		return commands.RunOptimize(ctx, c, db, commands.OptimizeOptions{DryRun: dryRun, Yes: yes})
	case "probe":
		return commands.RunProbe(ctx, c, db, Version, jsonOut)
	default:
		return fmt.Errorf("unknown command %q; see sapu --help", cmd)
	}
}

type menuModel struct {
	cursor int
	choice int
}

var menuItems = []struct{ name, desc string }{
	{"Clean", "Free up storage"},
	{"Uninstall", "Remove apps completely"},
	{"Debloat", "Remove preinstalled apps"},
	{"Analyze", "Explore storage usage"},
	{"Status", "Monitor phone health"},
	{"Optimize", "Faster app starts, battery audit"},
}

func (m menuModel) Init() tea.Cmd { return nil }
func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "ctrl+c", "esc":
			m.choice = -1
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		case "enter":
			m.choice = m.cursor + 1
			return m, tea.Quit
		case "1", "2", "3", "4", "5", "6":
			m.choice = int(k.String()[0] - '0')
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m menuModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  %s  %s\n\n", ui.Title.Render("Sapu"), ui.Gray.Render(tagline+" v"+Version))
	for i, it := range menuItems {
		cur := "   "
		name := it.name
		if i == m.cursor {
			cur = " " + ui.Blue.Render(ui.IconArrow) + " "
			name = ui.Bold.Render(name)
		}
		fmt.Fprintf(&b, "%s%d. %-12s %s\n", cur, i+1, name, ui.Gray.Render(it.desc))
	}
	b.WriteString("\n  " + ui.Gray.Render("↑↓  |  Enter  |  Q Quit") + "\n")
	return b.String()
}

func menu(ctx context.Context, c *adb.Client, db *rules.DB) error {
	if !ui.IsTTY() {
		fmt.Print(usage)
		return nil
	}
	m, err := tea.NewProgram(menuModel{}).Run()
	if err != nil {
		return err
	}
	switch m.(menuModel).choice {
	case 1:
		return commands.RunClean(ctx, c, db, commands.CleanOptions{})
	case 2:
		return commands.RunUninstall(ctx, c, db, commands.UninstallOptions{})
	case 3:
		return commands.RunUninstall(ctx, c, db, commands.UninstallOptions{Bloat: true})
	case 4:
		return commands.RunAnalyze(ctx, c, db, "", false, false)
	case 5:
		return commands.RunStatus(ctx, c, false, false, time.Second)
	case 6:
		return commands.RunOptimize(ctx, c, db, commands.OptimizeOptions{})
	}
	return nil
}

var _ = flag.Parse
