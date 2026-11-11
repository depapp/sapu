package commands

import "testing"

func TestLooksLikePackage(t *testing.T) {
	for _, ok := range []string{"com.whatsapp", "id.dana", "org.telegram.messenger"} {
		if !looksLikePackage(ok) {
			t.Errorf("%q should look like a package", ok)
		}
	}
	for _, no := range []string{"Download", ".thumbnails", "my photos", "a/b", "Telegram"} {
		if looksLikePackage(no) {
			t.Errorf("%q must not look like a package", no)
		}
	}
}

func TestClassifyRootHidesPersonalNames(t *testing.T) {
	top := []Entry{
		{Name: "Download", Bytes: 100},
		{Name: "Telegram", Bytes: 50},
		{Name: "com.oldgame", Bytes: 20},
		{Name: "Skripsi Budi", Bytes: 7},
		{Name: "Foto KTP", Bytes: 3},
	}
	got := classifyRoot(top, map[string]string{"Telegram": "org.telegram.messenger"})
	want := map[string]ProbeFolder{
		"Download":    {Path: "Download", Bytes: 100},
		"Telegram":    {Path: "Telegram", Bytes: 50, Owner: "org.telegram.messenger"},
		"com.oldgame": {Path: "com.oldgame", Bytes: 20, Owner: "com.oldgame"},
		"(other)":     {Path: "(other)", Bytes: 10},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, f := range got {
		if want[f.Path] != f {
			t.Errorf("unexpected %+v", f)
		}
	}
}
