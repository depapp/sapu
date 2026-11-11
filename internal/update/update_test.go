package update

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		tag, cur string
		want     bool
	}{
		{"v0.2.0", "0.1.0", true},
		{"v0.1.0", "0.1.0", false},
		{"v0.1.0", "0.1.1", false},
		{"v1.0.0", "0.9.9", true},
		{"v0.1.0", "0.1.0-dev", true},
	}
	for _, c := range cases {
		if got := Newer(c.tag, c.cur); got != c.want {
			t.Errorf("Newer(%q,%q)=%v want %v", c.tag, c.cur, got, c.want)
		}
	}
}

func TestSumFor(t *testing.T) {
	sums := "abc  sapu-linux-amd64\nDEF *sapu-darwin-arm64\n"
	if sumFor(sums, "sapu-linux-amd64") != "abc" || sumFor(sums, "sapu-darwin-arm64") != "def" {
		t.Fatal("sum lookup failed")
	}
	if sumFor(sums, "sapu-windows-amd64.exe") != "" {
		t.Fatal("missing asset must return empty")
	}
}

func TestTagFromLocation(t *testing.T) {
	cases := map[string]string{
		"https://github.com/depapp/sapu/releases/tag/v0.1.0":     "v0.1.0",
		"https://github.com/depapp/sapu/releases/tag/v0.2.0?x=1": "v0.2.0",
		"https://github.com/depapp/sapu/releases":                "",
		"": "",
	}
	for loc, want := range cases {
		if got := tagFromLocation(loc); got != want {
			t.Errorf("tagFromLocation(%q) = %q, want %q", loc, got, want)
		}
	}
}
