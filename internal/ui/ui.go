// Package ui holds the terminal look shared by every command: the same
// symbols, colors and byte formatting on every screen.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var (
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Blue   = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	Purple = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	Gray   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	Bold   = lipgloss.NewStyle().Bold(true)
	Title  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
)

const (
	IconOK      = "✓"
	IconSkip    = "◎"
	IconSection = "➤"
	IconSel     = "●"
	IconUnsel   = "○"
	IconArrow   = "➤"
)

// Bytes formats like Mole: 1.2GB, 248.5MB, 12KB.
func Bytes(n int64) string {
	f := float64(n)
	switch {
	case n >= 1<<40:
		return fmt.Sprintf("%.2fTB", f/(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.2fGB", f/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", f/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", f/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// Bar renders a block bar of the given width for a 0..100 percentage.
func Bar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// ColorPct colors by thresholds (green < warn <= yellow < danger <= red).
func ColorPct(s string, pct, warn, danger float64) string {
	switch {
	case pct >= danger:
		return Red.Render(s)
	case pct >= warn:
		return Yellow.Render(s)
	default:
		return Green.Render(s)
	}
}

// Section prints a section header.
func Section(name string) {
	fmt.Println()
	fmt.Println(Blue.Render(IconSection) + " " + Bold.Render(name))
}

// Row prints an item line under a section.
func Row(ok bool, text string) {
	icon := Green.Render(IconOK)
	if !ok {
		icon = Gray.Render(IconSkip)
	}
	fmt.Println("  " + icon + " " + text)
}

// Rule prints the 70-char separator Mole uses around summaries.
func Rule() { fmt.Println(strings.Repeat("=", 70)) }

// IsTTY reports whether stdin and stdout are a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// Confirm asks y/N; without a TTY it returns the non-interactive default.
func Confirm(prompt string, nonInteractive bool) bool {
	if !IsTTY() {
		return nonInteractive
	}
	fmt.Printf("%s [y/N] ", prompt)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// Truncate cuts a string to width with an ellipsis.
func Truncate(s string, width int) string {
	if width <= 1 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}

// Pad right-pads to width.
func Pad(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(r))
}
