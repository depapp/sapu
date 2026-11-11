// Package safety is the single funnel through which Sapu changes a phone.
// Everything destructive calls Trash, Remove, Uninstall or Disable here, so
// dry-run, path validation, the on-device trash and the history log stay
// consistent. Raw `rm` or `pm uninstall` elsewhere fails `go test ./...`
// (see internal/safety/audit_test.go).
package safety

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/depapp/sapu/internal/adb"
	"github.com/depapp/sapu/internal/rules"
)

// Roots the funnel accepts; the first is canonical.
var storageRoots = []string{"/sdcard", "/storage/emulated/0", "/storage/self/primary", "/mnt/sdcard"}

// Bare roots that are never deleted even when their children are.
var bareRoots = []string{
	"", "Android", "Android/data", "Android/obb", "Android/media", "DCIM", "DCIM/Camera", "Pictures",
	"Movies", "Music", "Download", "Documents", "Alarms", "Audiobooks", "Notifications", "Podcasts",
	"Recordings", "Ringtones", ".sapu", ".sapu/trash",
}

// TrashRoot is where trashed items live on the phone; one folder per run.
const TrashRoot = "/sdcard/.sapu/trash"

// Runner executes validated actions on one device.
type Runner struct {
	Client  *adb.Client
	Rules   *rules.DB
	DryRun  bool
	RunID   string
	Command string
	History *History
	// Installed is consulted so a leftover sweep never touches an installed app's folder.
	Installed map[string]bool
	// Whitelist holds normalized /sdcard paths the user asked Sapu never to touch.
	Whitelist []string
}

// New builds a runner with a fresh run id and an open history log.
func New(c *adb.Client, db *rules.DB, command string, dryRun bool) (*Runner, error) {
	h, err := OpenHistory()
	if err != nil {
		return nil, err
	}
	r := &Runner{Client: c, Rules: db, DryRun: dryRun, RunID: newRunID(), Command: command, History: h, Whitelist: LoadWhitelist()}
	if dryRun {
		command += "-dry-run"
	}
	h.StartSession(r.RunID, command, c.Serial)
	return r, nil
}

// Close ends the history session.
func (r *Runner) Close() {
	if r.History != nil {
		r.History.EndSession(r.RunID)
		r.History.Close()
	}
}

// Normalize rewrites every accepted storage root to /sdcard and cleans the path.
func Normalize(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !strings.HasPrefix(p, "/") {
		return "", errors.New("path must be absolute")
	}
	for _, r := range storageRoots {
		if p == r || strings.HasPrefix(p, r+"/") {
			return path.Clean("/sdcard" + strings.TrimPrefix(p, r)), nil
		}
	}
	return "", fmt.Errorf("path is outside shared storage: %s", p)
}

// Validate is the deny gate every deletion passes. It never grants: a path
// that fails here is refused whatever the caller believes about it.
func Validate(p string) (string, error) {
	for _, ch := range p {
		if ch < 0x20 || ch == 0x7f {
			return "", errors.New("path contains control characters")
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", errors.New("path contains '..'")
		}
	}
	n, err := Normalize(p)
	if err != nil {
		return "", err
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(n, "/sdcard"), "/")
	for _, b := range bareRoots {
		if rel == b {
			return "", fmt.Errorf("refusing to delete a storage root: %s", n)
		}
	}
	if rel == ".sapu" || strings.HasPrefix(rel, ".sapu/") {
		return "", errors.New("refusing to touch Sapu's own trash through the funnel")
	}
	return n, nil
}

// LeftoverOwner returns the package that owns an Android/{data,obb,media}/<pkg> path.
func LeftoverOwner(p string) string {
	rel := strings.TrimPrefix(p, "/sdcard/")
	for _, pre := range []string{"Android/data/", "Android/obb/", "Android/media/"} {
		if rest, ok := strings.CutPrefix(rel, pre); ok {
			pkg, _, _ := strings.Cut(rest, "/")
			return pkg
		}
	}
	return ""
}

// Result is what one funnel call did.
type Result struct {
	Path    string
	Action  string // TRASHED REMOVED UNINSTALLED DISABLED SKIPPED WOULD-TRASH ...
	Bytes   int64
	Reason  string
	TrashTo string
}

// CheckTrash runs the validation Trash applies without touching the phone, so
// a plan can be previewed with the same refusals the real run will produce.
// It returns the normalized path and an empty reason when the path may be trashed.
func (r *Runner) CheckTrash(p string) (string, string) {
	n, err := Validate(p)
	if err != nil {
		return p, err.Error()
	}
	if Whitelisted(r.Whitelist, n) {
		return n, "whitelisted"
	}
	if owner := LeftoverOwner(n); owner != "" {
		if r.Installed != nil && r.Installed[owner] && !strings.Contains(strings.TrimPrefix(n, "/sdcard/Android/"), "/cache") && !strings.Contains(n, "/files/log") && !strings.Contains(n, "/files/crash") && !strings.Contains(n, "/files/tombstones") && !strings.HasSuffix(n, ".log") {
			return n, "app is installed, not a leftover"
		}
		if r.Rules != nil && r.Rules.IsDataProtected(owner) && strings.HasPrefix(n, "/sdcard/Android/media/") {
			return n, "personal media of " + owner
		}
	}
	return n, ""
}

// Trash moves a validated path into the run's trash folder on the phone.
// sizeBytes is the caller's measurement, recorded for history and the summary.
func (r *Runner) Trash(ctx context.Context, p string, sizeBytes int64) Result {
	n, reason := r.CheckTrash(p)
	if reason != "" {
		return r.skip(n, sizeBytes, reason)
	}
	dest := TrashRoot + "/" + r.RunID + "/" + encodeOrigin(n)
	if r.DryRun {
		return r.record(Result{Path: n, Action: "WOULD-TRASH", Bytes: sizeBytes, TrashTo: dest})
	}
	q := adb.Quote
	// Two origins can encode to the same flat name (Download/a__b and
	// Download/a/b), so the shell picks the first free name and reports it.
	cmd := fmt.Sprintf("mkdir -p %s && [ -e %s ] && d=%s && i=1 && while [ -e \"$d\" ]; do d=%s.$i; i=$((i+1)); done && mv %s \"$d\" && printf 'moved\\t%%s\\n' \"$d\"",
		q(path.Dir(dest)), q(n), q(dest), q(dest), q(n))
	out, err := r.Client.Shell(ctx, cmd)
	moved := movedDest(out)
	if err != nil || moved == "" {
		reason := "move failed"
		if err != nil {
			reason = err.Error()
		}
		return r.skip(n, sizeBytes, reason)
	}
	dest = moved
	_, _ = r.Client.Shell(ctx, fmt.Sprintf("printf '%%s\\t%%s\\n' %s %s >> %s", q(dest), q(n), q(TrashRoot+"/"+r.RunID+"/manifest.tsv")))
	return r.record(Result{Path: n, Action: "TRASHED", Bytes: sizeBytes, TrashTo: dest})
}

// Uninstall removes a package for user 0. keepApk uses -k so a system app can be restored.
func (r *Runner) Uninstall(ctx context.Context, pkg string, keepApk bool, sizeBytes int64) Result {
	if !validPackage(pkg) {
		return r.skip(pkg, sizeBytes, "invalid package name")
	}
	if r.Rules != nil && r.Rules.IsProtected(pkg) {
		return r.skip(pkg, sizeBytes, "protected system package")
	}
	if r.DryRun {
		return r.record(Result{Path: pkg, Action: "WOULD-UNINSTALL", Bytes: sizeBytes})
	}
	flag := ""
	if keepApk {
		flag = "-k "
	}
	out, err := r.Client.ShellTimeout(ctx, 60*time.Second, "pm uninstall "+flag+"--user 0 "+adb.Quote(pkg))
	if err != nil || !strings.Contains(out, "Success") {
		msg := strings.TrimSpace(out)
		if err != nil {
			msg = err.Error()
		}
		return r.skip(pkg, sizeBytes, msg)
	}
	return r.record(Result{Path: pkg, Action: "UNINSTALLED", Bytes: sizeBytes})
}

// Restore reinstalls a system app removed with -k.
func (r *Runner) Restore(ctx context.Context, pkg string) Result {
	if !validPackage(pkg) {
		return r.skip(pkg, 0, "invalid package name")
	}
	if r.DryRun {
		return r.record(Result{Path: pkg, Action: "WOULD-RESTORE"})
	}
	out, err := r.Client.ShellTimeout(ctx, 60*time.Second, "cmd package install-existing "+adb.Quote(pkg))
	if err != nil || !strings.Contains(out, "installed") {
		msg := strings.TrimSpace(out)
		if err != nil {
			msg = err.Error()
		}
		return r.skip(pkg, 0, msg)
	}
	return r.record(Result{Path: pkg, Action: "RESTORED"})
}

// TrimCaches asks the package manager to free cache until desired bytes are free.
// This is the system's own cache eviction: it clears every app's cache directory.
func (r *Runner) TrimCaches(ctx context.Context) Result {
	if r.DryRun {
		return r.record(Result{Path: "pm trim-caches", Action: "WOULD-TRIM"})
	}
	_, err := r.Client.ShellTimeout(ctx, 120*time.Second, "pm trim-caches 999G")
	if err != nil {
		return r.skip("pm trim-caches", 0, err.Error())
	}
	return r.record(Result{Path: "pm trim-caches", Action: "TRIMMED"})
}

// Dexopt runs Android's background dex optimization job now instead of
// waiting for an idle, charging night. Nothing is removed.
func (r *Runner) Dexopt(ctx context.Context) Result {
	if r.DryRun {
		return r.record(Result{Path: "bg-dexopt-job", Action: "WOULD-DEXOPT"})
	}
	_, err := r.Client.ShellTimeout(ctx, 15*time.Minute, "cmd package bg-dexopt-job")
	if err != nil {
		return r.skip("bg-dexopt-job", 0, err.Error())
	}
	return r.record(Result{Path: "bg-dexopt-job", Action: "DEXOPTED"})
}

// Compile ahead-of-time compiles one package with its usage profile.
func (r *Runner) Compile(ctx context.Context, pkg string) Result {
	if !validPackage(pkg) {
		return r.skip(pkg, 0, "invalid package name")
	}
	if r.DryRun {
		return r.record(Result{Path: pkg, Action: "WOULD-COMPILE"})
	}
	out, err := r.Client.ShellTimeout(ctx, 5*time.Minute, "cmd package compile -m speed-profile "+adb.Quote(pkg))
	if err != nil || !strings.Contains(out, "Success") {
		reason := strings.TrimSpace(out)
		if err != nil {
			reason = err.Error()
		}
		return r.skip(pkg, 0, reason)
	}
	return r.record(Result{Path: pkg, Action: "COMPILED"})
}

func (r *Runner) skip(p string, size int64, reason string) Result {
	return r.record(Result{Path: p, Action: "SKIPPED", Bytes: size, Reason: reason})
}

func (r *Runner) record(res Result) Result {
	if r.History != nil {
		r.History.Record(r.RunID, res.Action, res.Path, res.Bytes, res.Reason)
	}
	return res
}

func validPackage(pkg string) bool {
	if len(pkg) < 3 || len(pkg) > 255 {
		return false
	}
	for _, ch := range pkg {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_') {
			return false
		}
	}
	return true
}

// movedDest extracts the destination the trash move reported, or "".
func movedDest(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if d, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "moved\t"); ok && strings.HasPrefix(d, TrashRoot+"/") {
			return d
		}
	}
	return ""
}

// encodeOrigin turns /sdcard/Download/x.apk into Download__x.apk so a trash
// folder stays flat and a name cannot climb out of it.
func encodeOrigin(n string) string {
	rel := strings.TrimPrefix(n, "/sdcard/")
	return strings.ReplaceAll(rel, "/", "__")
}

// DecodeOrigin is the inverse used by restore when the manifest is missing.
func DecodeOrigin(name string) string {
	return "/sdcard/" + strings.ReplaceAll(name, "__", "/")
}

// LoadWhitelist reads ~/.config/sapu/whitelist: one /sdcard path or prefix
// per line, # comments allowed. Every command that trashes files honours it.
func LoadWhitelist() []string {
	dir, err := ConfigDir()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(dir + "/whitelist")
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if n, err := Normalize(line); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// Whitelisted reports whether p equals or lies under a whitelist entry.
func Whitelisted(list []string, p string) bool {
	for _, w := range list {
		if p == w || strings.HasPrefix(p, w+"/") {
			return true
		}
	}
	return false
}

// ConfigDir is ~/.config/sapu.
func ConfigDir() (string, error) {
	if v := os.Getenv("SAPU_CONFIG_DIR"); v != "" {
		return v, os.MkdirAll(v, 0o755)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := home + "/.config/sapu"
	return dir, os.MkdirAll(dir, 0o755)
}

// newRunID is sortable by time and unique within a second.
func newRunID() string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}
