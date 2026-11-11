package device

import (
	"testing"
	"time"
)

func TestParseBattery(t *testing.T) {
	b := ParseBattery("Current Battery Service state:\n  AC powered: true\n  USB powered: false\n  status: 2\n  health: 2\n  present: true\n  level: 87\n  scale: 100\n  voltage: 4123\n  temperature: 312\n  technology: Li-ion\n")
	if b.Level != 87 || b.Status != "Charging" || b.Health != "Good" || b.TempC != 31.2 || !b.Plugged {
		t.Fatalf("%+v", b)
	}
}

func TestParseThermal(t *testing.T) {
	th := ParseThermal("Thermal Status: 2\nCached temperatures:\n\tTemperature{mValue=30.8, mType=3, mName=skin, mStatus=0}\n\tTemperature{mValue=41.0, mType=0, mName=cpu, mStatus=0}\n")
	if th.Status != 2 || th.StatusLabel != "Moderate" || th.MaxC != 41.0 || th.Sensors["skin"] != 30.8 {
		t.Fatalf("%+v", th)
	}
}

func TestParseDiskstats(t *testing.T) {
	out := "Latency: 12ms\nPhotos Size: 100\nVideos Size: 200\nSystem Size: 8000000000\nPackage Names: [\"com.a\",\"com.b\"]\nApp Sizes: [10,20]\nApp Data Sizes: [30,40]\nCache Sizes: [5,6]\n"
	s, err := ParseDiskstats(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.Packages["com.b"] != (SizeEntry{App: 20, Data: 40, Cache: 6}) || s.Photos != 100 || s.TotalCache() != 11 {
		t.Fatalf("%+v", s)
	}
	if _, err := ParseDiskstats("Latency: 12ms\n"); err == nil {
		t.Fatal("expected error without package data")
	}
}

func TestParseUsage(t *testing.T) {
	out := `      package=com.android.chrome totalTimeUsed="00:00" lastTimeUsed="2026-11-01 10:00:00" totalTimeVisible="00:00" appLaunchCount=3
      package=com.android.chrome totalTimeUsed="00:00" lastTimeUsed="2026-11-05 09:30:00" totalTimeVisible="00:00" appLaunchCount=7
      package=com.never.used totalTimeUsed="00:00" lastTimeUsed="1970-01-01 00:00:00" appLaunchCount=0
`
	u := ParseUsage(out)
	want, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-11-05 09:30:00", time.Local)
	if !u["com.android.chrome"].LastUsed.Equal(want) || u["com.android.chrome"].LaunchCount != 7 {
		t.Fatalf("%+v", u["com.android.chrome"])
	}
	if !u["com.never.used"].LastUsed.IsZero() {
		t.Fatal("epoch must read as never")
	}
}

func TestParseTop(t *testing.T) {
	out := "Tasks: 338 total\n  PID %CPU  %MEM[CMDLINE]\n  359  0.0   5.0 zygote64\n 4554  3.3   0.2 top -b -n 1 -o PID,%CPU,%MEM,CMDLINE\n  547 26.6  15.4 system_server\n"
	p := ParseTop(out)
	if len(p) != 2 || p[0].Name != "system_server" || p[0].CPU != 26.6 {
		t.Fatalf("%+v", p)
	}
}

func TestParseDF(t *testing.T) {
	s, err := ParseDF("Filesystem       1K-blocks    Used Available Use% Mounted on\n/dev/block/dm-40   6082144 1475692   4464240  25% /data/user/0\n")
	if err != nil || s.TotalBytes != 6082144*1024 || s.FreeBytes != 4464240*1024 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestParseProcStatAndPercent(t *testing.T) {
	a, err := ParseProcStat("cpu  100 0 100 800 0 0 0 0 0 0\ncpu0 1 1 1 1 1 1 1 1 0 0\ncpu1 1 1 1 1 1 1 1 1 0 0\n")
	if err != nil || a.Cores != 2 {
		t.Fatal(err)
	}
	b, _ := ParseProcStat("cpu  200 0 200 1200 0 0 0 0 0 0\n")
	if pct := CPUPercent(a, b); pct < 33.2 || pct > 33.4 {
		t.Fatalf("pct %v", pct)
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	cases := map[time.Time]string{
		{}:                            "never",
		now.Add(-3 * 24 * time.Hour):  "3d ago",
		now.Add(-70 * 24 * time.Hour): "2mo ago",
	}
	for in, want := range cases {
		if got := Ago(in, now); got != want {
			t.Errorf("Ago(%v) = %q want %q", in, got, want)
		}
	}
}

func TestParsePackageList(t *testing.T) {
	p := ParsePackageList("package:com.a  installer=com.android.vending uid:10101\npackage:com.b  installer=null uid:10102\n", false)
	if len(p) != 2 || p[0].Installer != "com.android.vending" || p[1].Installer != "" || p[1].UID != 10102 {
		t.Fatalf("%+v", p)
	}
}

func TestParseDeviceIdleWhitelist(t *testing.T) {
	out := "system-excidle,com.android.vending,10134\nsystem,com.google.android.gms,10128\nuser,com.whatsapp,10201\nuser,id.dana,10250\n"
	got := ParseDeviceIdleWhitelist(out)
	if len(got) != 2 || got[0] != "com.whatsapp" || got[1] != "id.dana" {
		t.Fatalf("got %v", got)
	}
}
