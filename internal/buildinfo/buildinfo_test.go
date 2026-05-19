package buildinfo

import "testing"

func TestCurrentReflectsPackageVariables(t *testing.T) {
	saved := [3]string{Version, Commit, Date}
	t.Cleanup(func() {
		Version, Commit, Date = saved[0], saved[1], saved[2]
	})
	Version, Commit, Date = "v1.2.3", "abc123", "2026-05-19"
	info := Current()
	if info.Version != "v1.2.3" || info.Commit != "abc123" || info.Date != "2026-05-19" {
		t.Fatalf("info = %#v", info)
	}
}

func TestCurrentDefaultsToDevWhenUnset(t *testing.T) {
	saved := [3]string{Version, Commit, Date}
	t.Cleanup(func() {
		Version, Commit, Date = saved[0], saved[1], saved[2]
	})
	Version, Commit, Date = "dev", "unknown", "unknown"
	info := Current()
	if info.Version != "dev" || info.Commit != "unknown" || info.Date != "unknown" {
		t.Fatalf("info = %#v", info)
	}
}
