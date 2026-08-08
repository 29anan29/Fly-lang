package version

import "testing"

func TestIsDev(t *testing.T) {
	cases := []struct {
		v   string
		dev bool
	}{
		{"dev", true},
		{"", true},
		{"0.3.0-dev", true},
		{"0.3.0-dev.1", true},
		{"v0.2.0", false},
		{"0.2.0", false},
		{"v1.0.0-rc.1", false},
	}
	for _, c := range cases {
		old := Version
		Version = c.v
		if got := IsDev(); got != c.dev {
			t.Errorf("IsDev(%q) = %v, want %v", c.v, got, c.dev)
		}
		Version = old
	}
}

func TestString(t *testing.T) {
	oldV, oldC := Version, Commit
	defer func() { Version, Commit = oldV, oldC }()

	Version, Commit = "v0.2.0", "abcdef1234567"
	if s := String(); s != "v0.2.0 (release)" {
		t.Errorf("release String() = %q", s)
	}
	Version, Commit = "0.3.0-dev", "abcdef1234567"
	if s := String(); s != "0.3.0-dev (abcdef1)" {
		t.Errorf("dev String() = %q", s)
	}
	Version, Commit = "0.3.0-dev", ""
	if s := String(); s != "0.3.0-dev" {
		t.Errorf("dev no-commit String() = %q", s)
	}
}
