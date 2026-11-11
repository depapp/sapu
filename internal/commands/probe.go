package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/device"
	"github.com/depapp/sapu/internal/rules"
	"github.com/depapp/sapu/internal/ui"
)

// ProbePackage is one preinstalled package in a probe report.
type ProbePackage struct {
	Package  string `json:"package"`
	Enabled  bool   `json:"enabled"`
	Bytes    int64  `json:"bytes"`
	LastUsed string `json:"last_used"`
	Status   string `json:"status"` // protected, bloat or unknown
}

// ProbeFolder is a shared-storage folder in a probe report.
type ProbeFolder struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Owner string `json:"owner,omitempty"`
}

// ProbeResult is the --json output of `sapu probe`. It carries nothing that
// identifies a person or a device: no serial, no account, no user app names,
// no file names, and only well-known or package-named folders.
type ProbeResult struct {
	Sapu         string         `json:"sapu"`
	Manufacturer string         `json:"manufacturer"`
	Brand        string         `json:"brand"`
	Model        string         `json:"model"`
	Android      string         `json:"android"`
	SDK          int            `json:"sdk"`
	ABI          string         `json:"abi"`
	Storage      device.Storage `json:"storage"`
	UserApps     int            `json:"user_apps"`
	Protected    int            `json:"protected_preinstalled"`
	Preinstalled []ProbePackage `json:"preinstalled"`
	Leftovers    []ProbeFolder  `json:"leftovers"`
	Root         []ProbeFolder  `json:"root_folders"`
	CollectedAt  time.Time      `json:"collected_at"`
}

// Standard Android folders whose names are safe to report; anything else at
// the root of shared storage is shown as "(other)" unless it is a known
// leftover folder or a package name.
var standardRootFolders = map[string]bool{
	"Alarms": true, "Android": true, "Audiobooks": true, "DCIM": true, "Documents": true, "Download": true,
	"Movies": true, "Music": true, "Notifications": true, "Pictures": true, "Podcasts": true, "Recordings": true,
	"Ringtones": true, "MIUI": true, "Huawei": true, "Samsung": true, "LOST.DIR": true, "backups": true,
	"bluetooth": true, "Fonts": true, "Screenshots": true, ".thumbnails": true, ".sapu": true,
}

// RunProbe implements `sapu probe`.
func RunProbe(ctx context.Context, c *adb.Client, db *rules.DB, version string, jsonOut bool) error {
	info, err := device.ReadInfo(ctx, c)
	if err != nil {
		return err
	}
	res := ProbeResult{Sapu: version, Manufacturer: info.Manufacturer, Brand: info.Brand, Model: info.Model, Android: info.Android, SDK: info.SDK, ABI: info.ABI, CollectedAt: time.Now()}
	res.Storage, _ = device.ReadStorage(ctx, c)

	users, err := device.ListPackages(ctx, c, false)
	if err != nil {
		return err
	}
	res.UserApps = len(users)
	systems, err := device.ListPackages(ctx, c, true)
	if err != nil {
		return err
	}
	sizes, _ := device.ReadSizes(ctx, c, false)
	usage, _ := device.ReadUsage(ctx, c)
	systems = device.Enrich(systems, sizes, usage)
	now := time.Now()
	installed := map[string]bool{}
	for _, p := range users {
		installed[p.Name] = true
	}
	for _, p := range systems {
		installed[p.Name] = true
		if strings.Contains(p.Name, "auto_generated_rro") {
			continue // build-time resource overlays, not apps
		}
		pp := ProbePackage{Package: p.Name, Enabled: p.Enabled, Bytes: p.TotalBytes(), LastUsed: device.Ago(p.LastUsed, now), Status: "unknown"}
		switch {
		case db.IsProtected(p.Name):
			pp.Status = "protected"
			res.Protected++
		case func() bool { _, ok := db.BloatFor(p.Name); return ok }():
			pp.Status = "bloat"
		}
		res.Preinstalled = append(res.Preinstalled, pp)
	}
	sort.Slice(res.Preinstalled, func(i, j int) bool { return res.Preinstalled[i].Package < res.Preinstalled[j].Package })

	leftoverNames := map[string]string{}
	for _, l := range db.Leftovers {
		top, _, _ := strings.Cut(l.Path, "/")
		leftoverNames[top] = l.Package
	}
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
			res.Leftovers = append(res.Leftovers, ProbeFolder{Path: strings.TrimPrefix(p, "/sdcard/"), Bytes: duBytes(ctx, c, p), Owner: name})
		}
	}
	if top, err := ListDir(ctx, c, "/sdcard"); err == nil {
		res.Root = classifyRoot(top, leftoverNames)
	}
	if res.Preinstalled == nil {
		res.Preinstalled = []ProbePackage{}
	}
	if res.Leftovers == nil {
		res.Leftovers = []ProbeFolder{}
	}
	if res.Root == nil {
		res.Root = []ProbeFolder{}
	}
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	printProbe(res)
	return nil
}

func printProbe(r ProbeResult) {
	var unknown, bloat []ProbePackage
	for _, p := range r.Preinstalled {
		switch p.Status {
		case "unknown":
			unknown = append(unknown, p)
		case "bloat":
			bloat = append(bloat, p)
		}
	}
	fmt.Println("## Sapu probe")
	fmt.Printf("- Sapu %s · %s %s (%s) · Android %s (SDK %d) · %s\n", r.Sapu, r.Manufacturer, r.Model, r.Brand, r.Android, r.SDK, r.ABI)
	fmt.Printf("- Storage %s used of %s · %d preinstalled apps (%d protected, %d in the bloatware catalog, %d unknown) · %d user apps (names not included)\n",
		ui.Bytes(r.Storage.UsedBytes), ui.Bytes(r.Storage.TotalBytes), len(r.Preinstalled), r.Protected, len(bloat), len(unknown), r.UserApps)
	fmt.Println()
	fmt.Printf("### Preinstalled apps not in the rules database (%d)\n", len(unknown))
	fmt.Println("Candidates for protected_packages.yaml or bloatware.yaml.")
	fmt.Println()
	fmt.Println("| package | enabled | size | last used |")
	fmt.Println("|---|---|---|---|")
	for _, p := range unknown {
		fmt.Printf("| %s | %s | %s | %s |\n", p.Package, yesNo(p.Enabled), ui.Bytes(p.Bytes), p.LastUsed)
	}
	fmt.Println()
	fmt.Printf("### Bloatware catalog matches installed here (%d)\n", len(bloat))
	for _, p := range bloat {
		state := "enabled"
		if !p.Enabled {
			state = "disabled"
		}
		fmt.Printf("- %s · %s · %s · %s\n", p.Package, state, ui.Bytes(p.Bytes), p.LastUsed)
	}
	fmt.Println()
	fmt.Printf("### Leftover folders of apps that are not installed (%d)\n", len(r.Leftovers))
	for _, f := range r.Leftovers {
		fmt.Printf("- %s · %s\n", f.Path, ui.Bytes(f.Bytes))
	}
	fmt.Println()
	fmt.Println("### Shared storage, top level")
	for _, f := range r.Root {
		if f.Owner != "" && f.Owner != f.Path {
			fmt.Printf("- %s · %s · owned by %s\n", f.Path, ui.Bytes(f.Bytes), f.Owner)
		} else {
			fmt.Printf("- %s · %s\n", f.Path, ui.Bytes(f.Bytes))
		}
	}
	fmt.Println()
	fmt.Println("Paste this into a new issue: https://github.com/depapp/sapu/issues/new")
	fmt.Println("It contains no serial, account, user app names or file names.")
}

// classifyRoot keeps only folder names that are safe to share: Android's
// standard directories, known leftover roots and package-named folders.
// Everything else (personal folder names) is summed into "(other)".
func classifyRoot(top []Entry, leftoverNames map[string]string) []ProbeFolder {
	var out []ProbeFolder
	var other int64
	for _, e := range top {
		switch {
		case standardRootFolders[e.Name]:
			out = append(out, ProbeFolder{Path: e.Name, Bytes: e.Bytes})
		case leftoverNames[e.Name] != "":
			out = append(out, ProbeFolder{Path: e.Name, Bytes: e.Bytes, Owner: leftoverNames[e.Name]})
		case looksLikePackage(e.Name):
			out = append(out, ProbeFolder{Path: e.Name, Bytes: e.Bytes, Owner: e.Name})
		default:
			other += e.Bytes
		}
	}
	if other > 0 {
		out = append(out, ProbeFolder{Path: "(other)", Bytes: other})
	}
	return out
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
