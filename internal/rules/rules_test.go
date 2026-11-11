package rules

import "testing"

func TestLoadAndValidate(t *testing.T) {
	db, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if problems := db.Validate(); len(problems) > 0 {
		t.Fatalf("rules problems: %v", problems)
	}
	if !db.IsProtected("com.android.settings") || !db.IsProtected("com.google.android.gms") {
		t.Fatal("core packages must be protected")
	}
	if db.IsProtected("com.google.android.youtube") {
		t.Fatal("YouTube must stay removable")
	}
	if !db.IsDataProtected("com.whatsapp") || !db.IsDataProtected("id.co.bri.brimo") {
		t.Fatal("chat and bank apps must be data-protected")
	}
	if _, ok := db.BloatFor("com.miui.msa.global"); !ok {
		t.Fatal("bloat index missing")
	}
}
