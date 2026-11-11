package commands

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/device"
	"github.com/depapp/sapu/internal/rules"
	"github.com/depapp/sapu/internal/safety"
	"github.com/depapp/sapu/internal/ui"
)

// OptimizeOptions are the flags of `sapu optimize`.
type OptimizeOptions struct {
	DryRun bool
	Yes    bool
}

const (
	compileTop   = 5
	unusedDays   = 90
	unusedMinUse = 24 * time.Hour * unusedDays
)

// RunOptimize implements `sapu optimize`: a short, explainable list, like
// Mole's. It never removes anything; the only changes are compilation.
func RunOptimize(ctx context.Context, c *adb.Client, db *rules.DB, opt OptimizeOptions) error {
	info, err := device.ReadInfo(ctx, c)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println(ui.Title.Render("Optimize Your Android"))
	fmt.Println()
	fmt.Printf("%s %s %s · Android %s\n", ui.Gray.Render("⚙"), info.Manufacturer, info.Model, info.Android)
	if opt.DryRun {
		fmt.Println(ui.Yellow.Render("Dry run: nothing is changed."))
	}

	apps, err := device.ListPackages(ctx, c, false)
	if err != nil {
		return err
	}
	sysApps, err := device.ListPackages(ctx, c, true)
	if err != nil {
		return err
	}
	usage, _ := device.ReadUsage(ctx, c)
	noSizes := device.Sizes{Packages: map[string]device.SizeEntry{}}
	apps = device.Enrich(apps, noSizes, usage)
	// Compilation helps any app the user opens, preinstalled ones included.
	all := device.Enrich(append(append([]device.Package(nil), apps...), sysApps...), noSizes, usage)
	now := time.Now()

	// Section 1: battery. Report only; the user decides in Settings or with sapu uninstall.
	ui.Section("Battery")
	exempt := []string{}
	if out, err := c.Shell(ctx, "dumpsys deviceidle whitelist"); err == nil {
		exempt = device.ParseDeviceIdleWhitelist(out)
	}
	if len(exempt) == 0 {
		ui.Row(true, "no app is exempt from battery optimization")
	}
	for _, p := range exempt {
		ui.Row(false, fmt.Sprintf("%s %s", db.Label(p), ui.Gray.Render("exempt from battery optimization (Settings > Apps > Battery)")))
	}
	var unused []device.Package
	for _, a := range apps {
		if !a.LastUsed.IsZero() && now.Sub(a.LastUsed) > unusedMinUse {
			unused = append(unused, a)
		}
	}
	sort.Slice(unused, func(i, j int) bool { return unused[i].LastUsed.Before(unused[j].LastUsed) })
	for i, a := range unused {
		if i == 10 {
			ui.Row(false, fmt.Sprintf("and %d more unused apps · sapu uninstall --list", len(unused)-10))
			break
		}
		ui.Row(false, fmt.Sprintf("%s %s", db.Label(a.Name), ui.Gray.Render("not opened for "+device.Ago(a.LastUsed, now)+" · sapu uninstall "+a.Name)))
	}

	// Section 2: compile the apps the user opens most, so they start faster.
	ui.Section("Compile most used apps (speed-profile)")
	byLaunch := all
	sort.SliceStable(byLaunch, func(i, j int) bool { return byLaunch[i].LaunchCount > byLaunch[j].LaunchCount })
	var toCompile []device.Package
	for _, a := range byLaunch {
		if a.LaunchCount == 0 || len(toCompile) == compileTop {
			break
		}
		toCompile = append(toCompile, a)
	}
	if len(toCompile) == 0 {
		ui.Row(false, "no launch statistics yet; Android compiles apps on its own after a few days of use")
	}
	for _, a := range toCompile {
		ui.Row(true, fmt.Sprintf("%s %s", db.Label(a.Name), ui.Gray.Render(fmt.Sprintf("%d launches", a.LaunchCount))))
	}

	ui.Section("Background dex optimization")
	ui.Row(true, "run Android's bg-dexopt job now instead of waiting for an idle, charging night")
	fmt.Println()
	if opt.DryRun {
		fmt.Printf("%s %d app(s) would be compiled, then bg-dexopt.\n", ui.Yellow.Render("Dry run:"), len(toCompile))
		return nil
	}
	if !opt.Yes && !ui.Confirm(fmt.Sprintf("Compile %d app(s) and run bg-dexopt? Takes a few minutes, removes nothing.", len(toCompile)), false) {
		fmt.Println("Cancelled.")
		return nil
	}
	runner, err := safety.New(c, db, "optimize", opt.DryRun)
	if err != nil {
		return err
	}
	defer runner.Close()
	compiled := 0
	for _, a := range toCompile {
		res := runner.Compile(ctx, a.Name)
		if res.Action == "COMPILED" {
			compiled++
			ui.Row(true, db.Label(a.Name)+" "+ui.Gray.Render("compiled"))
		} else {
			ui.Row(false, db.Label(a.Name)+" "+ui.Gray.Render("skipped: "+res.Reason))
		}
	}
	fmt.Println(ui.Gray.Render("Running bg-dexopt, this can take several minutes..."))
	res := runner.Dexopt(ctx)
	fmt.Println()
	ui.Rule()
	if res.Action == "DEXOPTED" {
		fmt.Printf("%s\nCompiled %d app(s), background dexopt done\n", ui.Bold.Render("Optimize complete"), compiled)
	} else {
		fmt.Printf("%s\nCompiled %d app(s), bg-dexopt skipped: %s\n", ui.Bold.Render("Optimize finished"), compiled, res.Reason)
	}
	ui.Rule()
	return nil
}
