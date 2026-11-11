package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/device"
	"github.com/depapp/sapu/internal/rules"
	"github.com/depapp/sapu/internal/safety"
	"github.com/depapp/sapu/internal/ui"
)

// Entry is one row of the analyzer.
type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Bytes     int64  `json:"size"`
	IsDir     bool   `json:"is_dir"`
	Cleanable bool   `json:"cleanable,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Count     int    `json:"count,omitempty"`
}

// AnalyzeResult is the --json output.
type AnalyzeResult struct {
	Path      string         `json:"path"`
	Overview  bool           `json:"overview"`
	Entries   []Entry        `json:"entries"`
	Total     int64          `json:"total_size"`
	Storage   device.Storage `json:"storage"`
	Timestamp time.Time      `json:"collected_at"`
}

// ListDir sizes every child of a directory with one du call.
func ListDir(ctx context.Context, c *adb.Client, dir string) ([]Entry, error) {
	q := adb.Quote
	// du for sizes, find for type; hidden entries included.
	out, err := c.ShellTimeout(ctx, 120*time.Second, "cd "+q(dir)+" 2>/dev/null || { echo __SAPU_NODIR__; exit 0; }; du -sk -- * .[!.]* 2>/dev/null; echo __SAPU__; find . -mindepth 1 -maxdepth 1 -type d 2>/dev/null")
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	if strings.Contains(out, "__SAPU_NODIR__") {
		return nil, fmt.Errorf("not a readable directory: %s", dir)
	}
	sizes, dirs, _ := strings.Cut(out, "__SAPU__")
	isDir := map[string]bool{}
	for _, d := range strings.Split(dirs, "\n") {
		d = strings.TrimPrefix(strings.TrimSpace(d), "./")
		if d != "" {
			isDir[d] = true
		}
	}
	var entries []Entry
	for _, line := range strings.Split(sizes, "\n") {
		kb, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(kb, 10, 64)
		if err != nil || name == "*" || name == ".[!.]*" {
			continue
		}
		p := dir + "/" + name
		if dir == "/" {
			p = "/" + name
		}
		entries = append(entries, Entry{Name: name, Path: p, Bytes: n * 1024, IsDir: isDir[name]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Bytes > entries[j].Bytes })
	return entries, nil
}

// Overview builds Mole-style insight rows for the phone.
func Overview(ctx context.Context, c *adb.Client, db *rules.DB) ([]Entry, error) {
	var rows []Entry
	sizes, err := device.ReadSizes(ctx, c, false)
	if err == nil {
		var app, data, cache int64
		for _, e := range sizes.Packages {
			app += e.App
			data += e.Data
			cache += e.Cache
		}
		rows = append(rows,
			Entry{Name: "Apps (code)", Path: "apps://code", Bytes: app, Count: len(sizes.Packages), Hint: "sapu uninstall"},
			Entry{Name: "App data", Path: "apps://data", Bytes: data, Hint: "sapu uninstall"},
			Entry{Name: "App caches", Path: "apps://cache", Bytes: cache, Cleanable: true, Hint: "sapu clean"},
		)
		// sizes.System is the system partition, not part of /data, so it is not
		// shown next to rows measured against the data volume.
	}
	top, err := ListDir(ctx, c, "/sdcard")
	if err != nil {
		return rows, err
	}
	named := map[string]string{"DCIM": "Camera & photos", "Pictures": "Pictures", "Movies": "Movies", "Music": "Music", "Download": "Downloads", "Documents": "Documents", "Android": "Android/data, obb & media", "WhatsApp": "WhatsApp (legacy)", "Telegram": "Telegram", "Recordings": "Recordings", "Podcasts": "Podcasts", "Audiobooks": "Audiobooks"}
	for _, e := range top {
		if e.Name == ".sapu" {
			e.Name = "Sapu trash"
			e.Hint = "sapu trash"
			rows = append(rows, e)
			continue
		}
		if n, ok := named[e.Name]; ok {
			e.Name = n
		} else if !e.IsDir || e.Bytes < 1<<20 {
			continue
		}
		rows = append(rows, e)
	}
	// Insight rows that only show when present.
	for _, p := range []struct{ name, path, hint string }{
		{"WhatsApp media", "/sdcard/Android/media/com.whatsapp/WhatsApp/Media", ""},
		{"WhatsApp statuses", "/sdcard/Android/media/com.whatsapp/WhatsApp/Media/.Statuses", "auto-downloaded statuses"},
		{"Telegram media", "/sdcard/Android/media/org.telegram.messenger/Telegram", ""},
		{"Screenshots", "/sdcard/Pictures/Screenshots", ""},
		{"Screenshots", "/sdcard/DCIM/Screenshots", ""},
		{"Screen recordings", "/sdcard/Movies/ScreenRecords", ""},
		{"Screen recordings", "/sdcard/DCIM/Screen recordings", ""},
		{"Thumbnail caches", "/sdcard/DCIM/.thumbnails", "sapu clean"},
		{"TikTok", "/sdcard/Android/data/com.zhiliaoapp.musically/cache", "sapu clean"},
		{"Instagram cache", "/sdcard/Android/data/com.instagram.android/cache", "sapu clean"},
	} {
		exists, err := c.Shell(ctx, "[ -d "+adb.Quote(p.path)+" ] && echo yes")
		if err != nil || !strings.Contains(exists, "yes") {
			continue
		}
		b := duBytes(ctx, c, p.path)
		if b == 0 {
			continue
		}
		rows = append(rows, Entry{Name: p.name, Path: p.path, Bytes: b, IsDir: true, Cleanable: p.hint == "sapu clean", Hint: p.hint})
	}
	old := oldDownloads(ctx, c, 90)
	if old.Bytes > 0 {
		rows = append(rows, old)
	}
	apks := findAPKs(ctx, c)
	if apks.Bytes > 0 {
		rows = append(rows, apks)
	}
	return rows, nil
}

func oldDownloads(ctx context.Context, c *adb.Client, days int) Entry {
	out, err := c.ShellTimeout(ctx, 60*time.Second, fmt.Sprintf("find /sdcard/Download -type f -mtime +%d -exec du -k {} + 2>/dev/null | awk '{s+=$1; n++} END {print s+0, n+0}'", days))
	if err != nil {
		return Entry{}
	}
	f := strings.Fields(out)
	if len(f) < 2 {
		return Entry{}
	}
	kb, _ := strconv.ParseInt(f[0], 10, 64)
	n, _ := strconv.Atoi(f[1])
	return Entry{Name: fmt.Sprintf("Old Downloads (%dd+)", days), Path: "/sdcard/Download", Bytes: kb * 1024, IsDir: true, Count: n, Hint: fmt.Sprintf(">%dd", days)}
}

func findAPKs(ctx context.Context, c *adb.Client) Entry {
	out, err := c.ShellTimeout(ctx, 60*time.Second, "find /sdcard -type f \\( -name '*.apk' -o -name '*.xapk' -o -name '*.apks' \\) -not -path '*/.sapu/*' -exec du -k {} + 2>/dev/null | awk '{s+=$1; n++} END {print s+0, n+0}'")
	if err != nil {
		return Entry{}
	}
	f := strings.Fields(out)
	if len(f) < 2 {
		return Entry{}
	}
	kb, _ := strconv.ParseInt(f[0], 10, 64)
	n, _ := strconv.Atoi(f[1])
	return Entry{Name: "Installer APKs", Path: "/sdcard/Download", Bytes: kb * 1024, IsDir: true, Count: n, Cleanable: true, Hint: "sapu clean"}
}

// RunAnalyze implements `sapu analyze [path]`.
func RunAnalyze(ctx context.Context, c *adb.Client, db *rules.DB, target string, jsonOut bool, dryRun bool) error {
	st, _ := device.ReadStorage(ctx, c)
	overview := target == ""
	if jsonOut || !ui.IsTTY() {
		var entries []Entry
		var err error
		if overview {
			entries, err = Overview(ctx, c, db)
		} else {
			n, nerr := safety.Normalize(target)
			if nerr != nil {
				return nerr
			}
			entries, err = ListDir(ctx, c, n)
			target = n
		}
		if err != nil && len(entries) == 0 {
			return err
		}
		var total int64
		for _, e := range entries {
			if !strings.Contains(e.Path, "://") {
				total += e.Bytes
			}
		}
		res := AnalyzeResult{Path: target, Overview: overview, Entries: entries, Total: total, Storage: st, Timestamp: time.Now()}
		if overview {
			res.Path = "/sdcard"
		}
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	m := &analyzeModel{ctx: ctx, c: c, db: db, storage: st, dryRun: dryRun, selected: map[string]bool{}}
	if overview {
		m.path = ""
	} else {
		n, err := safety.Normalize(target)
		if err != nil {
			return err
		}
		m.path = n
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type analyzeLoaded struct {
	path    string
	entries []Entry
	err     error
}
type analyzeDeleted struct {
	results []safety.Result
}

type analyzeModel struct {
	ctx      context.Context
	c        *adb.Client
	db       *rules.DB
	storage  device.Storage
	dryRun   bool
	path     string // "" = overview
	entries  []Entry
	cursor   int
	loading  bool
	err      error
	stack    []string
	selected map[string]bool
	confirm  bool
	status   string
	width    int
	height   int
}

func (m *analyzeModel) Init() tea.Cmd { return m.load() }

func (m *analyzeModel) load() tea.Cmd {
	m.loading = true
	ctx, c, db, p := m.ctx, m.c, m.db, m.path
	return func() tea.Msg {
		if p == "" {
			e, err := Overview(ctx, c, db)
			return analyzeLoaded{path: p, entries: e, err: err}
		}
		e, err := ListDir(ctx, c, p)
		return analyzeLoaded{path: p, entries: e, err: err}
	}
}

func (m *analyzeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case analyzeLoaded:
		m.loading = false
		if msg.path != m.path {
			return m, nil
		}
		m.entries, m.err = msg.entries, msg.err
		if m.cursor >= len(m.entries) {
			m.cursor = 0
		}
	case analyzeDeleted:
		var n int
		var b int64
		for _, r := range msg.results {
			if r.Action == "TRASHED" || r.Action == "WOULD-TRASH" {
				n++
				b += r.Bytes
			}
		}
		verb := "Moved"
		if m.dryRun {
			verb = "Would move"
		}
		m.status = fmt.Sprintf("%s %d items (%s) to trash · sapu trash restore", verb, n, ui.Bytes(b))
		m.selected = map[string]bool{}
		return m, m.load()
	case tea.KeyMsg:
		if m.confirm {
			switch msg.String() {
			case "y", "Y":
				m.confirm = false
				return m, m.trashSelected()
			default:
				m.confirm = false
				m.status = "Cancelled"
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "g":
			m.cursor = 0
		case "G":
			m.cursor = len(m.entries) - 1
		case "enter", "right", "l":
			if m.cursor < len(m.entries) {
				e := m.entries[m.cursor]
				if e.IsDir && !strings.Contains(e.Path, "://") {
					m.stack = append(m.stack, m.path)
					m.path = e.Path
					m.cursor = 0
					m.selected = map[string]bool{}
					return m, m.load()
				}
			}
		case "left", "h", "b", "esc", "backspace":
			if len(m.stack) > 0 {
				m.path = m.stack[len(m.stack)-1]
				m.stack = m.stack[:len(m.stack)-1]
				m.cursor = 0
				m.selected = map[string]bool{}
				return m, m.load()
			}
		case "r":
			return m, m.load()
		case " ":
			if m.cursor < len(m.entries) {
				e := m.entries[m.cursor]
				if _, err := safety.Validate(e.Path); err == nil {
					m.selected[e.Path] = !m.selected[e.Path]
				} else {
					m.status = err.Error()
				}
			}
		case "d", "delete":
			if len(m.selected) == 0 && m.cursor < len(m.entries) {
				e := m.entries[m.cursor]
				if _, err := safety.Validate(e.Path); err != nil {
					m.status = err.Error()
					return m, nil
				}
				m.selected[e.Path] = true
			}
			if len(m.selected) > 0 {
				m.confirm = true
			}
		}
	}
	return m, nil
}

func (m *analyzeModel) trashSelected() tea.Cmd {
	paths := make([]string, 0, len(m.selected))
	sizes := map[string]int64{}
	for _, e := range m.entries {
		if m.selected[e.Path] {
			paths = append(paths, e.Path)
			sizes[e.Path] = e.Bytes
		}
	}
	ctx, c, db, dry := m.ctx, m.c, m.db, m.dryRun
	return func() tea.Msg {
		runner, err := safety.New(c, db, "analyze", dry)
		if err != nil {
			return analyzeDeleted{}
		}
		installed, _ := device.InstalledSet(ctx, c)
		runner.Installed = installed
		defer runner.Close()
		var results []safety.Result
		for _, p := range paths {
			results = append(results, runner.Trash(ctx, p, sizes[p]))
		}
		return analyzeDeleted{results: results}
	}
}

func (m *analyzeModel) View() string {
	var b strings.Builder
	title := "Analyze Storage"
	fmt.Fprintf(&b, "\n %s  %s\n", ui.Title.Render(title), ui.Gray.Render(fmt.Sprintf("%s free of %s", ui.Bytes(m.storage.FreeBytes), ui.Bytes(m.storage.TotalBytes))))
	loc := "Overview"
	if m.path != "" {
		loc = shortPath(m.path)
	}
	fmt.Fprintf(&b, " %s\n\n", ui.Bold.Render(loc))
	if m.loading && len(m.entries) == 0 {
		b.WriteString("  Scanning…\n")
	}
	if m.err != nil && len(m.entries) == 0 {
		b.WriteString("  " + ui.Red.Render(m.err.Error()) + "\n")
	}
	// In the overview, rows overlap (apps, media, folders) and the honest
	// denominator is the whole disk; inside a folder it is the folder's contents.
	var total int64
	if m.path == "" {
		total = m.storage.TotalBytes
	} else {
		for _, e := range m.entries {
			total += e.Bytes
		}
	}
	rows := m.height - 9
	if rows < 5 {
		rows = 30
	}
	start := 0
	if m.cursor >= rows {
		start = m.cursor - rows + 1
	}
	for i := start; i < len(m.entries) && i < start+rows; i++ {
		e := m.entries[i]
		pct := 0.0
		if total > 0 {
			pct = float64(e.Bytes) * 100 / float64(total)
		}
		cursor := "  "
		if i == m.cursor {
			cursor = ui.Blue.Render(ui.IconArrow) + " "
		}
		sel := " "
		if m.selected[e.Path] {
			sel = ui.Green.Render(ui.IconSel)
		}
		name := e.Name
		if e.IsDir && !strings.Contains(e.Path, "://") {
			name += "/"
		}
		extra := ""
		if e.Count > 0 {
			extra += ui.Gray.Render(fmt.Sprintf(" %d items", e.Count))
		}
		if e.Hint != "" {
			extra += " " + ui.Purple.Render(e.Hint)
		}
		fmt.Fprintf(&b, " %s%s %s %5.1f%%  %-34s %9s%s\n", cursor, sel, ui.Bar(pct, 20), pct, ui.Truncate(name, 34), ui.Bytes(e.Bytes), extra)
	}
	b.WriteString("\n")
	if m.confirm {
		n := len(m.selected)
		verb := "Move"
		if m.dryRun {
			verb = "Preview moving"
		}
		fmt.Fprintf(&b, " %s\n", ui.Yellow.Render(fmt.Sprintf("%s %d selected item(s) to Sapu trash on the phone? [y/N]", verb, n)))
	} else if m.status != "" {
		fmt.Fprintf(&b, " %s\n", ui.Gray.Render(m.status))
	}
	b.WriteString(" " + ui.Gray.Render("↑↓ move · Enter open · ← back · Space select · d trash · r refresh · q quit") + "\n")
	return b.String()
}
