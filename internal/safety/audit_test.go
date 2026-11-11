package safety

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Destructive device commands may only appear inside this package, or on a
// line that carries a `// SAFE: <reason>` annotation. This is Mole's Layer 2
// (scripts/audit_destructive_sinks.py) for a Go codebase.
var destructive = regexp.MustCompile(`"[^"]*\b(rm -r|rm -f|rm -rf|pm uninstall|pm clear|pm disable|pm trim-caches|find [^"]*-delete)\b`)

func TestNoRawDestructiveSinksOutsideFunnel(t *testing.T) {
	root := filepath.Join("..", "..")
	var violations []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/internal/safety/") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		n := 0
		for sc.Scan() {
			n++
			line := sc.Text()
			if destructive.MatchString(line) && strings.Contains(line, "Shell") && !strings.Contains(line, "// SAFE:") {
				violations = append(violations, path+":"+itoa(n)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("raw destructive command outside internal/safety without // SAFE: annotation:\n%s", strings.Join(violations, "\n"))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestValidateRejectsDangerousPaths(t *testing.T) {
	bad := []string{
		"", "relative/path", "/sdcard", "/sdcard/", "/storage/emulated/0", "/sdcard/DCIM", "/sdcard/Download",
		"/sdcard/Android", "/sdcard/Android/data", "/sdcard/../data", "/sdcard/Download/../../system",
		"/data/data/com.example", "/system/app", "/sdcard/.sapu/trash", "/sdcard/Pictures\n", "/sdcard/x\x00y",
	}
	for _, p := range bad {
		if n, err := Validate(p); err == nil {
			t.Errorf("Validate(%q) accepted as %q", p, n)
		}
	}
	good := map[string]string{
		"/sdcard/Download/old.apk":                     "/sdcard/Download/old.apk",
		"/storage/emulated/0/Android/data/com.x/cache": "/sdcard/Android/data/com.x/cache",
		"/sdcard/DCIM/.thumbnails":                     "/sdcard/DCIM/.thumbnails",
		"/sdcard/Android/media/com.gone.app":           "/sdcard/Android/media/com.gone.app",
		"/sdcard/Pictures/name..with.dots":             "/sdcard/Pictures/name..with.dots",
	}
	for in, want := range good {
		got, err := Validate(in)
		if err != nil || got != want {
			t.Errorf("Validate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestLeftoverOwner(t *testing.T) {
	if LeftoverOwner("/sdcard/Android/data/com.a.b/cache") != "com.a.b" {
		t.Fatal("owner")
	}
	if LeftoverOwner("/sdcard/Download/x") != "" {
		t.Fatal("no owner")
	}
}

func TestEncodeOriginRoundTrip(t *testing.T) {
	p := "/sdcard/Android/data/com.a/cache"
	if DecodeOrigin(encodeOrigin(p)) != p {
		t.Fatal("round trip")
	}
}

func TestMovedDest(t *testing.T) {
	if got := movedDest("moved\t" + TrashRoot + "/r/Download__a.apk.1\n"); got != TrashRoot+"/r/Download__a.apk.1" {
		t.Fatalf("movedDest = %q", got)
	}
	if movedDest("mv: permission denied\n") != "" || movedDest("moved\t/sdcard/Download/x") != "" {
		t.Fatal("movedDest accepted a non-trash destination")
	}
}

func TestWhitelisted(t *testing.T) {
	list := []string{"/sdcard/Download/keep"}
	if !Whitelisted(list, "/sdcard/Download/keep") || !Whitelisted(list, "/sdcard/Download/keep/a/b") {
		t.Fatal("entry and children must be whitelisted")
	}
	if Whitelisted(list, "/sdcard/Download/keeper") || Whitelisted(nil, "/sdcard/Download/keep") {
		t.Fatal("prefix match must stop at a path boundary")
	}
}
