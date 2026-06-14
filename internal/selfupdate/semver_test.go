package selfupdate

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.7", "v0.1.8", true},
		{"v0.1.7", "v0.2.0", true},
		{"v0.1.7", "v1.0.0", true},
		{"0.1.7", "0.1.7", false},
		{"v0.1.8", "v0.1.7", false},
		{"v0.2.0", "v0.1.9", false},
		{"v1.0.0", "v0.9.9", false},
		{"dev", "v0.1.0", true},
		{"", "v0.1.0", true},
		{"v0.1.7", "garbage", false},
		{"v0.1.7", "", false},
		{"v0.1.7-rc1", "v0.1.7", false},
		{"v0.1.6", "v0.1.7-rc1", true},
		{"v1.2", "v1.2.1", true},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}
