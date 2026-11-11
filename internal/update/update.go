// Package update replaces the running sapu binary with the latest GitHub
// release, after checking its SHA256 against the release's SHA256SUMS.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	Repo       = "depapp/sapu"
	apiLatest  = "https://api.github.com/repos/" + Repo + "/releases/latest"
	webLatest  = "https://github.com/" + Repo + "/releases/latest"
	downloadFm = "https://github.com/" + Repo + "/releases/download/%s/%s"
)

// Release is the subset of the GitHub release payload sapu needs.
type Release struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

// Latest returns the newest published release. It first follows the
// redirect of the releases/latest page, which has no rate limit, and only
// falls back to the GitHub API (60 anonymous requests per hour per IP).
func Latest(ctx context.Context) (Release, error) {
	if rel, err := latestFromRedirect(ctx); err == nil {
		return rel, nil
	}
	return latestFromAPI(ctx)
}

func latestFromRedirect(ctx context.Context) (Release, error) {
	var rel Release
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, webLatest, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("User-Agent", "sapu-update")
	c := client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return rel, err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 3 {
		return rel, fmt.Errorf("github: %s", resp.Status)
	}
	tag := tagFromLocation(resp.Header.Get("Location"))
	if tag == "" {
		return rel, errors.New("no release published yet")
	}
	return Release{Tag: tag, URL: "https://github.com/" + Repo + "/releases/tag/" + tag}, nil
}

// tagFromLocation extracts "v0.1.0" from ".../releases/tag/v0.1.0"; it is
// empty for the bare releases page GitHub serves when nothing is published.
func tagFromLocation(loc string) string {
	const marker = "/releases/tag/"
	i := strings.LastIndex(loc, marker)
	if i < 0 {
		return ""
	}
	tag := loc[i+len(marker):]
	if j := strings.IndexAny(tag, "?#/"); j >= 0 {
		tag = tag[:j]
	}
	return tag
}

func latestFromAPI(ctx context.Context) (Release, error) {
	var rel Release
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiLatest, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sapu-update")
	resp, err := client().Do(req)
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return rel, errors.New("no release published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("github api: %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return rel, err
	}
	if rel.Tag == "" {
		return rel, errors.New("release has no tag")
	}
	return rel, nil
}

// AssetName is the release file for this OS and CPU, matching release.yml.
func AssetName() string {
	name := "sapu-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Newer reports whether tag (vX.Y.Z) is newer than the running version.
// Development builds ("-dev") are never considered up to date.
func Newer(tag, current string) bool {
	if strings.HasSuffix(current, "-dev") {
		return true
	}
	a, b := parse(tag), parse(current)
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parse(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-")
	for i, p := range strings.SplitN(v, ".", 3) {
		fmt.Sscanf(p, "%d", &out[i])
	}
	return out
}

// Install downloads the release asset for this machine, verifies it against
// SHA256SUMS and atomically replaces the running executable. It returns the
// path that was replaced.
func Install(ctx context.Context, rel Release) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	asset := AssetName()
	sums, err := fetch(ctx, fmt.Sprintf(downloadFm, rel.Tag, "SHA256SUMS"), 1<<20)
	if err != nil {
		return "", fmt.Errorf("SHA256SUMS: %w", err)
	}
	want := sumFor(string(sums), asset)
	if want == "" {
		return "", fmt.Errorf("release %s has no %s", rel.Tag, asset)
	}
	body, err := fetch(ctx, fmt.Sprintf(downloadFm, rel.Tag, asset), 200<<20)
	if err != nil {
		return "", fmt.Errorf("%s: %w", asset, err)
	}
	got := sha256.Sum256(body)
	if hex.EncodeToString(got[:]) != want {
		return "", fmt.Errorf("%s: checksum mismatch, refusing to install", asset)
	}
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".sapu-update-*")
	if err != nil {
		return "", fmt.Errorf("cannot write next to %s (try sudo or reinstall): %w", exe, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if runtime.GOOS == "windows" {
		// A running .exe cannot be overwritten but can be renamed.
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmpName)
			return "", err
		}
	}
	if err := os.Rename(tmpName, exe); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return exe, nil
}

func sumFor(sums, asset string) string {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return strings.ToLower(f[0])
		}
	}
	return ""
}

func fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sapu-update")
	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func client() *http.Client { return &http.Client{Timeout: 5 * time.Minute} }
