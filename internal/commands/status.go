package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/device"
	"github.com/depapp/sapu/internal/ui"
)

// Snapshot is one status sample; it is also the --json output.
type Snapshot struct {
	Device      device.Info      `json:"device"`
	HealthScore int              `json:"health_score"`
	Issues      []string         `json:"issues"`
	CPUPercent  float64          `json:"cpu_percent"`
	Cores       int              `json:"cores"`
	Memory      device.Memory    `json:"memory"`
	Storage     device.Storage   `json:"storage"`
	Battery     device.Battery   `json:"battery"`
	Thermal     device.Thermal   `json:"thermal"`
	NetDownBps  float64          `json:"net_down_bps"`
	NetUpBps    float64          `json:"net_up_bps"`
	Uptime      string           `json:"uptime"`
	Processes   []device.Process `json:"top_processes"`
	CollectedAt time.Time        `json:"collected_at"`
	Errors      []string         `json:"errors,omitempty"`
}

type collector struct {
	c       *adb.Client
	info    device.Info
	lastCPU device.CPUSample
	lastNet device.NetSample
	lastAt  time.Time
	history []float64
}

func (col *collector) collect(ctx context.Context) Snapshot {
	s := Snapshot{Device: col.info, CollectedAt: time.Now()}
	// One round trip for the cheap /proc files.
	out, err := col.c.Shell(ctx, "cat /proc/stat; echo __SAPU__; cat /proc/net/dev; echo __SAPU__; cat /proc/uptime; echo __SAPU__; cat /proc/meminfo")
	if err != nil {
		s.Errors = append(s.Errors, err.Error())
	} else {
		parts := strings.Split(out, "__SAPU__")
		for len(parts) < 4 {
			parts = append(parts, "")
		}
		now := time.Now()
		if cpu, err := device.ParseProcStat(parts[0]); err == nil {
			s.Cores = cpu.Cores
			if col.lastCPU.Total > 0 {
				s.CPUPercent = device.CPUPercent(col.lastCPU, cpu)
			}
			col.lastCPU = cpu
		}
		net := device.ParseNetDev(parts[1])
		if !col.lastAt.IsZero() {
			dt := now.Sub(col.lastAt).Seconds()
			if dt > 0 && net.RxBytes >= col.lastNet.RxBytes {
				s.NetDownBps = float64(net.RxBytes-col.lastNet.RxBytes) / dt
				s.NetUpBps = float64(net.TxBytes-col.lastNet.TxBytes) / dt
			}
		}
		col.lastNet, col.lastAt = net, now
		s.Uptime = fmtDuration(device.ParseUptime(parts[2]))
		if m, ok := device.ParseProcMeminfo(parts[3]); ok {
			s.Memory = m
		}
	}
	if st, err := device.ReadStorage(ctx, col.c); err == nil {
		s.Storage = st
	} else {
		s.Errors = append(s.Errors, err.Error())
	}
	if b, err := device.ReadBattery(ctx, col.c); err == nil {
		s.Battery = b
	} else {
		s.Errors = append(s.Errors, err.Error())
	}
	if t, err := device.ReadThermal(ctx, col.c); err == nil {
		s.Thermal = t
	}
	if p, err := device.ReadTop(ctx, col.c, 6); err == nil {
		s.Processes = p
	}
	s.HealthScore, s.Issues = healthScore(s)
	col.history = append(col.history, s.NetDownBps)
	if len(col.history) > 20 {
		col.history = col.history[1:]
	}
	return s
}

// healthScore mirrors Mole's weights: CPU 30, memory 25, storage 20, thermal 15, battery 10.
func healthScore(s Snapshot) (int, []string) {
	score := 100.0
	var issues []string
	if s.CPUPercent > 50 {
		score -= 30 * (s.CPUPercent - 50) / 50
		if s.CPUPercent > 85 {
			issues = append(issues, "High CPU")
		}
	}
	if mp := s.Memory.UsedPercent(); mp > 70 {
		score -= 25 * (mp - 70) / 30
		if mp > 88 {
			issues = append(issues, "High memory")
		}
	}
	if sp := s.Storage.UsedPercent(); sp > 80 {
		score -= 20 * (sp - 80) / 20
		if sp > 90 && s.Storage.FreeBytes < 3<<30 {
			issues = append(issues, "Storage almost full")
		}
	}
	switch {
	case s.Thermal.Status >= 3:
		score -= 15
		issues = append(issues, "Thermal throttling")
	case s.Thermal.Status >= 1:
		score -= 5
	}
	if s.Battery.Health != "Good" && s.Battery.Health != "Unknown" {
		score -= 10
		issues = append(issues, "Battery "+strings.ToLower(s.Battery.Health))
	} else if s.Battery.TempC >= 42 {
		score -= 5
		issues = append(issues, "Battery hot")
	}
	if score < 0 {
		score = 0
	}
	return int(score + 0.5), issues
}

func fmtDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// RunStatus implements `sapu status`.
func RunStatus(ctx context.Context, c *adb.Client, jsonOut bool, watch bool, interval time.Duration) error {
	info, err := device.ReadInfo(ctx, c)
	if err != nil {
		return err
	}
	col := &collector{c: c, info: info}
	if jsonOut || watch || !ui.IsTTY() {
		col.collect(ctx)
		time.Sleep(700 * time.Millisecond)
		enc := json.NewEncoder(os.Stdout)
		for {
			s := col.collect(ctx)
			if err := enc.Encode(s); err != nil {
				return err
			}
			if !watch {
				return nil
			}
			time.Sleep(interval)
		}
	}
	m := statusModel{col: col, ctx: ctx}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type statusTick struct{}
type statusSample Snapshot

type statusModel struct {
	col    *collector
	ctx    context.Context
	snap   Snapshot
	width  int
	height int
	ready  bool
}

func (m statusModel) Init() tea.Cmd {
	return tea.Batch(m.sample(), tea.Tick(time.Second, func(time.Time) tea.Msg { return statusTick{} }))
}

func (m statusModel) sample() tea.Cmd {
	col, ctx := m.col, m.ctx
	return func() tea.Msg { return statusSample(col.collect(ctx)) }
}

func (m statusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case statusSample:
		m.snap = Snapshot(msg)
		m.ready = true
	case statusTick:
		return m, tea.Batch(m.sample(), tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return statusTick{} }))
	}
	return m, nil
}

func (m statusModel) View() string {
	if !m.ready {
		return "\n  Reading the phone…\n"
	}
	s := m.snap
	var b strings.Builder
	score := fmt.Sprintf("Health %s %d", scoreDot(s.HealthScore), s.HealthScore)
	fmt.Fprintf(&b, "\n %s  %s  %s\n", ui.Title.Render("Sapu Status"), score, ui.Gray.Render(fmt.Sprintf("%s %s · Android %s · %s", s.Device.Manufacturer, s.Device.Model, s.Device.Android, s.Device.ABI)))
	if len(s.Issues) > 0 {
		fmt.Fprintf(&b, " %s\n", ui.Yellow.Render(strings.Join(s.Issues, " · ")))
	}
	b.WriteString("\n")
	left := []string{
		ui.Bold.Render("⚙ CPU"),
		fmt.Sprintf("Usage   %s %s", ui.Bar(s.CPUPercent, 18), ui.ColorPct(fmt.Sprintf("%5.1f%%", s.CPUPercent), s.CPUPercent, 60, 85)),
		fmt.Sprintf("Cores   %d · uptime %s", s.Cores, s.Uptime),
		fmt.Sprintf("Thermal %s", thermalLine(s.Thermal)),
		"",
		ui.Bold.Render("▤ Storage"),
		fmt.Sprintf("Used    %s %s", ui.Bar(s.Storage.UsedPercent(), 18), ui.ColorPct(fmt.Sprintf("%5.1f%%", s.Storage.UsedPercent()), s.Storage.UsedPercent(), 80, 90)),
		fmt.Sprintf("Free    %s of %s", ui.Bytes(s.Storage.FreeBytes), ui.Bytes(s.Storage.TotalBytes)),
	}
	right := []string{
		ui.Bold.Render("▦ Memory"),
		fmt.Sprintf("Used    %s %s", ui.Bar(s.Memory.UsedPercent(), 18), ui.ColorPct(fmt.Sprintf("%5.1f%%", s.Memory.UsedPercent()), s.Memory.UsedPercent(), 75, 88)),
		fmt.Sprintf("Total   %s · avail %s", ui.Bytes(s.Memory.TotalBytes), ui.Bytes(s.Memory.FreeBytes)),
		"",
		"",
		ui.Bold.Render("⚡ Battery"),
		fmt.Sprintf("Level   %s %3d%% %s", ui.Bar(float64(s.Battery.Level), 18), s.Battery.Level, ui.Gray.Render(s.Battery.Status)),
		fmt.Sprintf("Health  %s · %.1f°C · %s", s.Battery.Health, s.Battery.TempC, s.Battery.Technology),
	}
	colW := 44
	for i := 0; i < len(left) || i < len(right); i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		fmt.Fprintf(&b, " %s %s\n", padANSI(l, colW), r)
	}
	b.WriteString("\n " + ui.Bold.Render("⇅ Network") + "\n")
	fmt.Fprintf(&b, " Down    %s\n Up      %s\n", rate(s.NetDownBps), rate(s.NetUpBps))
	b.WriteString("\n " + ui.Bold.Render("▶ Processes") + "\n")
	for _, p := range s.Processes {
		fmt.Fprintf(&b, " %-28s %s %5.1f%%  %s\n", ui.Truncate(p.Name, 28), ui.Bar(minf(p.CPU, 100), 10), p.CPU, ui.Gray.Render(fmt.Sprintf("mem %.1f%%", p.Mem)))
	}
	if len(s.Errors) > 0 {
		b.WriteString("\n " + ui.Yellow.Render(strings.Join(s.Errors, "; ")) + "\n")
	}
	b.WriteString("\n " + ui.Gray.Render("q quit") + "\n")
	return b.String()
}

func thermalLine(t device.Thermal) string {
	if t.MaxC > 0 {
		return fmt.Sprintf("%s · %.1f°C", t.StatusLabel, t.MaxC)
	}
	return t.StatusLabel
}

func scoreDot(score int) string {
	switch {
	case score >= 85:
		return ui.Green.Render("●")
	case score >= 65:
		return ui.Yellow.Render("●")
	default:
		return ui.Red.Render("●")
	}
}

func rate(bps float64) string {
	return ui.Bytes(int64(bps)) + "/s"
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// padANSI pads by visible width (lipgloss.Width ignores escape codes).
func padANSI(s string, width int) string {
	w := visibleWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func visibleWidth(s string) int {
	in := false
	n := 0
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case !in:
			n++
		}
	}
	return n
}
