package device

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/depapp/sapu/internal/adb"
)

// Package is one installed app as seen through pm, diskstats and usagestats.
type Package struct {
	Name        string    `json:"package"`
	System      bool      `json:"system"`
	Installer   string    `json:"installer,omitempty"`
	UID         int       `json:"uid,omitempty"`
	AppBytes    int64     `json:"app_bytes"`
	DataBytes   int64     `json:"data_bytes"`
	CacheBytes  int64     `json:"cache_bytes"`
	LastUsed    time.Time `json:"last_used,omitempty"`
	LaunchCount int       `json:"launch_count"`
	Enabled     bool      `json:"enabled"`
}

// TotalBytes is app + data + cache.
func (p Package) TotalBytes() int64 { return p.AppBytes + p.DataBytes + p.CacheBytes }

// ListPackages lists third-party (user) apps or system apps for user 0.
func ListPackages(ctx context.Context, c *adb.Client, system bool) ([]Package, error) {
	flag := "-3"
	if system {
		flag = "-s"
	}
	out, err := c.Shell(ctx, "pm list packages "+flag+" -U -i --user 0")
	if err != nil {
		// Older Android lacks -U/-i combinations.
		out, err = c.Shell(ctx, "pm list packages "+flag+" --user 0")
		if err != nil {
			return nil, err
		}
	}
	pkgs := ParsePackageList(out, system)
	dis, err := c.Shell(ctx, "pm list packages -d --user 0")
	if err == nil {
		disabled := map[string]bool{}
		for _, p := range ParsePackageList(dis, system) {
			disabled[p.Name] = true
		}
		for i := range pkgs {
			pkgs[i].Enabled = !disabled[pkgs[i].Name]
		}
	}
	return pkgs, nil
}

// ParsePackageList parses `pm list packages [-U -i]`.
func ParsePackageList(out string, system bool) []Package {
	var pkgs []Package
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		f := strings.Fields(strings.TrimPrefix(line, "package:"))
		if len(f) == 0 {
			continue
		}
		p := Package{Name: f[0], System: system, Enabled: true}
		for _, kv := range f[1:] {
			if v, ok := strings.CutPrefix(kv, "installer="); ok && v != "null" {
				p.Installer = v
			}
			if v, ok := strings.CutPrefix(kv, "uid:"); ok {
				p.UID, _ = strconv.Atoi(v)
			}
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// Sizes is the per-package breakdown from `dumpsys diskstats`, plus media totals.
type Sizes struct {
	Packages map[string]SizeEntry `json:"packages"`
	Photos   int64                `json:"photos"`
	Videos   int64                `json:"videos"`
	Audio    int64                `json:"audio"`
	Download int64                `json:"downloads"`
	System   int64                `json:"system"`
	Other    int64                `json:"other"`
	Fresh    bool                 `json:"fresh"`
}

// SizeEntry is app/data/cache for one package.
type SizeEntry struct {
	App   int64 `json:"app"`
	Data  int64 `json:"data"`
	Cache int64 `json:"cache"`
}

// TotalCache sums cache bytes across packages.
func (s Sizes) TotalCache() int64 {
	var n int64
	for _, e := range s.Packages {
		n += e.Cache
	}
	return n
}

// ParseDiskstats parses `dumpsys diskstats`.
func ParseDiskstats(out string) (Sizes, error) {
	s := Sizes{Packages: map[string]SizeEntry{}}
	var names []string
	var app, data, cache []int64
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Package Names":
			if err := json.Unmarshal([]byte(v), &names); err != nil {
				return s, fmt.Errorf("diskstats package names: %w", err)
			}
		case "App Sizes":
			_ = json.Unmarshal([]byte(v), &app)
		case "App Data Sizes":
			_ = json.Unmarshal([]byte(v), &data)
		case "Cache Sizes":
			_ = json.Unmarshal([]byte(v), &cache)
		case "Photos Size":
			s.Photos, _ = strconv.ParseInt(v, 10, 64)
		case "Videos Size":
			s.Videos, _ = strconv.ParseInt(v, 10, 64)
		case "Audio Size":
			s.Audio, _ = strconv.ParseInt(v, 10, 64)
		case "Downloads Size":
			s.Download, _ = strconv.ParseInt(v, 10, 64)
		case "System Size":
			s.System, _ = strconv.ParseInt(v, 10, 64)
		case "Other Size":
			s.Other, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	if len(names) == 0 {
		return s, fmt.Errorf("diskstats has no per-package data yet")
	}
	for i, n := range names {
		var e SizeEntry
		if i < len(app) {
			e.App = app[i]
		}
		if i < len(data) {
			e.Data = data[i]
		}
		if i < len(cache) {
			e.Cache = cache[i]
		}
		s.Packages[n] = e
	}
	s.Fresh = true
	return s, nil
}

var diskstatsJobRe = regexp.MustCompile(`android/com\.android\.server\.storage\.DiskStatsLoggingService#(\d+)`)

// ReadSizes reads diskstats. When refresh is true (or no data exists) it asks
// the system's own DiskStatsLoggingService job to recompute first, which is
// how Settings > Storage gets its numbers; shell may run that job.
func ReadSizes(ctx context.Context, c *adb.Client, refresh bool) (Sizes, error) {
	if !refresh {
		out, err := c.Shell(ctx, "dumpsys diskstats")
		if err == nil {
			if s, perr := ParseDiskstats(out); perr == nil {
				return s, nil
			}
		}
	}
	jobOut, err := c.ShellTimeout(ctx, 30*time.Second, "dumpsys jobscheduler | grep -m1 'DiskStatsLoggingService#'")
	if err == nil {
		if m := diskstatsJobRe.FindStringSubmatch(jobOut); m != nil {
			_, _ = c.Shell(ctx, "cmd jobscheduler run -f android "+m[1])
		}
	}
	deadline := time.Now().Add(45 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		out, err := c.Shell(ctx, "dumpsys diskstats")
		if err != nil {
			last = err
			continue
		}
		s, perr := ParseDiskstats(out)
		if perr == nil {
			return s, nil
		}
		last = perr
	}
	return Sizes{Packages: map[string]SizeEntry{}}, fmt.Errorf("per-app sizes unavailable: %v", last)
}

var usageRe = regexp.MustCompile(`package=(\S+)\s.*?lastTimeUsed="([^"]+)".*?appLaunchCount=(\d+)`)
var usageEpochRe = regexp.MustCompile(`package=(\S+)\s.*?lastTimeUsed=(\d{10,})`)

// Usage is last-used per package.
type Usage struct {
	LastUsed    time.Time
	LaunchCount int
}

// ReadUsage dumps usagestats and keeps the most recent lastTimeUsed per package.
func ReadUsage(ctx context.Context, c *adb.Client) (map[string]Usage, error) {
	out, err := c.ShellTimeout(ctx, 60*time.Second, "dumpsys usagestats")
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	return ParseUsage(out), nil
}

// ParseUsage parses the package stats rows of `dumpsys usagestats`.
func ParseUsage(out string) map[string]Usage {
	res := map[string]Usage{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "lastTimeUsed=") {
			continue
		}
		var name string
		var t time.Time
		var launches int
		if m := usageRe.FindStringSubmatch(line); m != nil {
			name = m[1]
			t, _ = time.ParseInLocation("2006-01-02 15:04:05", m[2], time.Local)
			launches, _ = strconv.Atoi(m[3])
		} else if m := usageEpochRe.FindStringSubmatch(line); m != nil {
			name = m[1]
			ms, _ := strconv.ParseInt(m[2], 10, 64)
			t = time.UnixMilli(ms)
		} else {
			continue
		}
		if t.Year() <= 1970 {
			t = time.Time{}
		}
		u := res[name]
		if t.After(u.LastUsed) {
			u.LastUsed = t
		}
		if launches > u.LaunchCount {
			u.LaunchCount = launches
		}
		res[name] = u
	}
	return res
}

// Enrich merges sizes and usage into packages, sorted by total size descending.
func Enrich(pkgs []Package, sizes Sizes, usage map[string]Usage) []Package {
	for i := range pkgs {
		if e, ok := sizes.Packages[pkgs[i].Name]; ok {
			pkgs[i].AppBytes, pkgs[i].DataBytes, pkgs[i].CacheBytes = e.App, e.Data, e.Cache
		}
		if u, ok := usage[pkgs[i].Name]; ok {
			pkgs[i].LastUsed, pkgs[i].LaunchCount = u.LastUsed, u.LaunchCount
		}
	}
	sort.SliceStable(pkgs, func(i, j int) bool { return pkgs[i].TotalBytes() > pkgs[j].TotalBytes() })
	return pkgs
}

// ParseDeviceIdleWhitelist returns packages the user exempted from battery
// optimization (rows "user,<pkg>,<uid>" of `dumpsys deviceidle whitelist`).
func ParseDeviceIdleWhitelist(out string) []string {
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimSpace(line), ",")
		if len(f) >= 2 && f[0] == "user" && f[1] != "" {
			pkgs = append(pkgs, f[1])
		}
	}
	return pkgs
}

// IsRunning reports whether any process of the package is alive.
func IsRunning(ctx context.Context, c *adb.Client, pkg string) bool {
	out, err := c.Shell(ctx, "pidof "+adb.Quote(pkg)+" 2>/dev/null")
	return err == nil && strings.TrimSpace(out) != ""
}

// InstalledSet returns every package name installed for user 0, including disabled ones.
func InstalledSet(ctx context.Context, c *adb.Client) (map[string]bool, error) {
	out, err := c.Shell(ctx, "pm list packages --user 0")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, p := range ParsePackageList(out, false) {
		set[p.Name] = true
	}
	return set, nil
}

// Ago renders a last-used time the way Mole does: "3d ago", "2mo ago", "never".
func Ago(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/24/7))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/24/365))
	}
}
