package safety

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// History appends one TSV row per action to ~/.config/sapu/history.log.
// Control bytes are escaped so a file name cannot forge a row.
type History struct {
	f    *os.File
	Path string
}

// OpenHistory opens the log for appending; SAPU_NO_HISTORY=1 disables it.
func OpenHistory() (*History, error) {
	if os.Getenv("SAPU_NO_HISTORY") == "1" {
		return &History{}, nil
	}
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	p := dir + "/history.log"
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "history log unavailable: %v\n", err)
		return &History{}, nil
	}
	return &History{f: f, Path: p}, nil
}

func (h *History) write(fields ...string) {
	if h == nil || h.f == nil {
		return
	}
	for i := range fields {
		fields[i] = escape(fields[i])
	}
	_, _ = fmt.Fprintln(h.f, strings.Join(fields, "\t"))
}

// StartSession writes the run header.
func (h *History) StartSession(runID, command, serial string) {
	h.write(time.Now().Format(time.RFC3339), runID, "SESSION-START", command, "", serial)
}

// EndSession writes the run footer.
func (h *History) EndSession(runID string) {
	h.write(time.Now().Format(time.RFC3339), runID, "SESSION-END", "", "", "")
}

// Record writes one action row.
func (h *History) Record(runID, action, target string, bytes int64, reason string) {
	h.write(time.Now().Format(time.RFC3339), runID, action, target, strconv.FormatInt(bytes, 10), reason)
}

// Close closes the file.
func (h *History) Close() {
	if h != nil && h.f != nil {
		_ = h.f.Close()
	}
}

func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Row is one parsed history line.
type Row struct {
	Time   time.Time `json:"time"`
	RunID  string    `json:"run_id"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	Bytes  int64     `json:"bytes"`
	Reason string    `json:"reason,omitempty"`
}

// ReadHistory parses the whole log.
func ReadHistory() ([]Row, string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, "", err
	}
	p := dir + "/history.log"
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, p, nil
		}
		return nil, p, err
	}
	defer f.Close()
	var rows []Row
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 4 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, f[0])
		row := Row{Time: t, RunID: f[1], Action: f[2], Target: f[3]}
		if len(f) > 4 {
			row.Bytes, _ = strconv.ParseInt(f[4], 10, 64)
		}
		if len(f) > 5 {
			row.Reason = f[5]
		}
		rows = append(rows, row)
	}
	return rows, p, sc.Err()
}
