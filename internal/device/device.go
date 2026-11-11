// Package device turns the text that Android tools print into typed values.
// Every parser is pure so it can be tested against recorded fixtures.
package device

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/depapp/sapu/internal/adb"
)

// Info is the static identity of the phone.
type Info struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Brand        string `json:"brand"`
	Android      string `json:"android"`
	SDK          int    `json:"sdk"`
	ABI          string `json:"abi"`
	Serial       string `json:"serial"`
}

// ReadInfo reads the identity properties.
func ReadInfo(ctx context.Context, c *adb.Client) (Info, error) {
	out, err := c.Shell(ctx, "getprop ro.product.manufacturer; getprop ro.product.model; getprop ro.product.brand; getprop ro.build.version.release; getprop ro.build.version.sdk; getprop ro.product.cpu.abi")
	if err != nil {
		return Info{}, err
	}
	l := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for len(l) < 6 {
		l = append(l, "")
	}
	sdk, _ := strconv.Atoi(strings.TrimSpace(l[4]))
	return Info{
		Manufacturer: strings.TrimSpace(l[0]), Model: strings.TrimSpace(l[1]), Brand: strings.TrimSpace(l[2]),
		Android: strings.TrimSpace(l[3]), SDK: sdk, ABI: strings.TrimSpace(l[5]), Serial: c.Serial,
	}, nil
}

// Storage is the /data volume (internal storage shares it with /sdcard).
type Storage struct {
	TotalBytes int64 `json:"total"`
	UsedBytes  int64 `json:"used"`
	FreeBytes  int64 `json:"free"`
}

// UsedPercent is used/total.
func (s Storage) UsedPercent() float64 {
	if s.TotalBytes == 0 {
		return 0
	}
	return float64(s.UsedBytes) * 100 / float64(s.TotalBytes)
}

// ReadStorage runs df on /data.
func ReadStorage(ctx context.Context, c *adb.Client) (Storage, error) {
	out, err := c.Shell(ctx, "df -k /data")
	if err != nil {
		return Storage{}, err
	}
	return ParseDF(out)
}

// ParseDF parses `df -k` output and returns the first data row.
func ParseDF(out string) (Storage, error) {
	for _, line := range strings.Split(out, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		total, e1 := strconv.ParseInt(f[1], 10, 64)
		used, e2 := strconv.ParseInt(f[2], 10, 64)
		avail, e3 := strconv.ParseInt(f[3], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		return Storage{TotalBytes: total * 1024, UsedBytes: used * 1024, FreeBytes: avail * 1024}, nil
	}
	return Storage{}, fmt.Errorf("df: no data row")
}

// Battery is `dumpsys battery`.
type Battery struct {
	Level      int     `json:"level"`
	Status     string  `json:"status"`
	Health     string  `json:"health"`
	TempC      float64 `json:"temp_c"`
	Voltage    int     `json:"voltage_mv"`
	Technology string  `json:"technology"`
	Plugged    bool    `json:"plugged"`
	CycleCount int     `json:"cycle_count,omitempty"`
	Present    bool    `json:"present"`
	rawStatus  int
	rawHealth  int
}

var batteryStatus = map[int]string{1: "Unknown", 2: "Charging", 3: "Discharging", 4: "Not charging", 5: "Full"}
var batteryHealth = map[int]string{1: "Unknown", 2: "Good", 3: "Overheat", 4: "Dead", 5: "Over voltage", 6: "Failure", 7: "Cold"}

// ReadBattery dumps the battery service.
func ReadBattery(ctx context.Context, c *adb.Client) (Battery, error) {
	out, err := c.Shell(ctx, "dumpsys battery")
	if err != nil {
		return Battery{}, err
	}
	return ParseBattery(out), nil
}

// ParseBattery parses `dumpsys battery`.
func ParseBattery(out string) Battery {
	b := Battery{Status: "Unknown", Health: "Unknown"}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "level":
			b.Level, _ = strconv.Atoi(v)
		case "status":
			b.rawStatus, _ = strconv.Atoi(v)
			if s, ok := batteryStatus[b.rawStatus]; ok {
				b.Status = s
			}
		case "health":
			b.rawHealth, _ = strconv.Atoi(v)
			if s, ok := batteryHealth[b.rawHealth]; ok {
				b.Health = s
			}
		case "temperature":
			t, _ := strconv.Atoi(v)
			b.TempC = float64(t) / 10
		case "voltage":
			b.Voltage, _ = strconv.Atoi(v)
		case "technology":
			b.Technology = v
		case "present":
			b.Present = v == "true"
		case "Charge counter":
		case "AC powered", "USB powered", "Wireless powered", "Dock powered":
			if v == "true" {
				b.Plugged = true
			}
		case "Cycle count", "cycle count":
			b.CycleCount, _ = strconv.Atoi(v)
		}
	}
	return b
}

// Thermal is `dumpsys thermalservice`.
type Thermal struct {
	Status      int                `json:"status"`
	StatusLabel string             `json:"status_label"`
	Sensors     map[string]float64 `json:"sensors,omitempty"`
	MaxC        float64            `json:"max_c"`
}

var thermalLabels = []string{"None", "Light", "Moderate", "Severe", "Critical", "Emergency", "Shutdown"}
var tempRe = regexp.MustCompile(`Temperature\{mValue=([-0-9.]+), mType=(\d+), mName=([^,}]+)`)

// ReadThermal dumps the thermal service.
func ReadThermal(ctx context.Context, c *adb.Client) (Thermal, error) {
	out, err := c.Shell(ctx, "dumpsys thermalservice")
	if err != nil {
		return Thermal{}, err
	}
	return ParseThermal(out), nil
}

// ParseThermal parses `dumpsys thermalservice`.
func ParseThermal(out string) Thermal {
	t := Thermal{Sensors: map[string]float64{}, StatusLabel: "Unknown"}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Thermal Status:") {
			t.Status, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Thermal Status:")))
			if t.Status >= 0 && t.Status < len(thermalLabels) {
				t.StatusLabel = thermalLabels[t.Status]
			}
		}
	}
	// Only the "Current temperatures" block is live; cached ones can be stale, so take whatever exists.
	for _, m := range tempRe.FindAllStringSubmatch(out, -1) {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil || v <= 0 || v > 150 {
			continue
		}
		name := strings.TrimSpace(m[3])
		if old, ok := t.Sensors[name]; !ok || v > old {
			t.Sensors[name] = v
		}
		if v > t.MaxC {
			t.MaxC = v
		}
	}
	return t
}

// Memory is the tail of `dumpsys meminfo`.
type Memory struct {
	TotalBytes int64  `json:"total"`
	UsedBytes  int64  `json:"used"`
	FreeBytes  int64  `json:"free"`
	ZRAMBytes  int64  `json:"zram_used"`
	Status     string `json:"status"`
}

// UsedPercent is used/total.
func (m Memory) UsedPercent() float64 {
	if m.TotalBytes == 0 {
		return 0
	}
	return float64(m.UsedBytes) * 100 / float64(m.TotalBytes)
}

var memRe = regexp.MustCompile(`^\s*(Total|Free|Used) RAM:\s*([\d,]+)K(?:\s*\(status (\w+)\))?`)
var zramRe = regexp.MustCompile(`^\s*ZRAM:\s*([\d,]+)K physical used`)

// ReadMemory reads /proc/meminfo (cheap) and falls back to dumpsys meminfo summary.
func ReadMemory(ctx context.Context, c *adb.Client) (Memory, error) {
	out, err := c.Shell(ctx, "cat /proc/meminfo")
	if err == nil {
		if m, ok := ParseProcMeminfo(out); ok {
			return m, nil
		}
	}
	out, err = c.ShellTimeout(ctx, 30*time.Second, "dumpsys meminfo | tail -20")
	if err != nil {
		return Memory{}, err
	}
	return ParseMeminfo(out), nil
}

// ParseProcMeminfo parses /proc/meminfo.
func ParseProcMeminfo(out string) (Memory, bool) {
	var m Memory
	var avail int64
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		n, _ := strconv.ParseInt(f[0], 10, 64)
		switch k {
		case "MemTotal":
			m.TotalBytes = n * 1024
		case "MemAvailable":
			avail = n * 1024
		case "MemFree":
			if avail == 0 {
				avail = n * 1024
			}
		}
	}
	if m.TotalBytes == 0 {
		return m, false
	}
	m.FreeBytes = avail
	m.UsedBytes = m.TotalBytes - avail
	m.Status = "normal"
	return m, true
}

// ParseMeminfo parses the summary at the end of `dumpsys meminfo`.
func ParseMeminfo(out string) Memory {
	var m Memory
	for _, line := range strings.Split(out, "\n") {
		if mm := memRe.FindStringSubmatch(line); mm != nil {
			n, _ := strconv.ParseInt(strings.ReplaceAll(mm[2], ",", ""), 10, 64)
			switch mm[1] {
			case "Total":
				m.TotalBytes = n * 1024
				m.Status = mm[3]
			case "Free":
				m.FreeBytes = n * 1024
			case "Used":
				m.UsedBytes = n * 1024
			}
		}
		if mm := zramRe.FindStringSubmatch(line); mm != nil {
			n, _ := strconv.ParseInt(strings.ReplaceAll(mm[1], ",", ""), 10, 64)
			m.ZRAMBytes = n * 1024
		}
	}
	return m
}

// CPUSample is the aggregate line of /proc/stat.
type CPUSample struct {
	Idle  uint64
	Total uint64
	Cores int
}

// ParseProcStat parses /proc/stat.
func ParseProcStat(out string) (CPUSample, error) {
	var s CPUSample
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			s.Cores++
			continue
		}
		for i, v := range f[1:] {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return s, fmt.Errorf("/proc/stat: %w", err)
			}
			s.Total += n
			if i == 3 || i == 4 { // idle + iowait
				s.Idle += n
			}
		}
	}
	if s.Total == 0 {
		return s, fmt.Errorf("/proc/stat: no cpu line")
	}
	return s, nil
}

// CPUPercent is usage between two samples.
func CPUPercent(a, b CPUSample) float64 {
	dt := float64(b.Total - a.Total)
	if dt <= 0 {
		return 0
	}
	return (1 - float64(b.Idle-a.Idle)/dt) * 100
}

// Process is one row of top.
type Process struct {
	PID  int     `json:"pid"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Name string  `json:"name"`
}

// ReadTop returns the busiest processes.
func ReadTop(ctx context.Context, c *adb.Client, n int) ([]Process, error) {
	out, err := c.Shell(ctx, "top -b -n 1 -o PID,%CPU,%MEM,CMDLINE")
	if err != nil {
		return nil, err
	}
	procs := ParseTop(out)
	if len(procs) > n {
		procs = procs[:n]
	}
	return procs, nil
}

// ParseTop parses `top -b -n 1 -o PID,%CPU,%MEM,CMDLINE`, dropping top itself.
func ParseTop(out string) []Process {
	var procs []Process
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		cpu, err1 := strconv.ParseFloat(f[1], 64)
		mem, err2 := strconv.ParseFloat(f[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		name := f[3]
		if strings.HasPrefix(name, "top") && strings.Contains(line, "-b -n") {
			continue
		}
		procs = append(procs, Process{PID: pid, CPU: cpu, Mem: mem, Name: name})
	}
	// top sorts by CPU already; keep order but make sure.
	for i := 1; i < len(procs); i++ {
		for j := i; j > 0 && procs[j].CPU > procs[j-1].CPU; j-- {
			procs[j], procs[j-1] = procs[j-1], procs[j]
		}
	}
	return procs
}

// NetSample is the sum of non-loopback interfaces in /proc/net/dev.
type NetSample struct {
	RxBytes uint64
	TxBytes uint64
}

// ParseNetDev parses /proc/net/dev.
func ParseNetDev(out string) NetSample {
	var s NetSample
	for _, line := range strings.Split(out, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || strings.HasPrefix(name, "dummy") || strings.HasPrefix(name, "ifb") {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(f[0], 10, 64)
		tx, _ := strconv.ParseUint(f[8], 10, 64)
		s.RxBytes += rx
		s.TxBytes += tx
	}
	return s
}

// ParseUptime parses /proc/uptime into a duration.
func ParseUptime(out string) time.Duration {
	f := strings.Fields(out)
	if len(f) == 0 {
		return 0
	}
	sec, _ := strconv.ParseFloat(f[0], 64)
	return time.Duration(sec * float64(time.Second))
}
