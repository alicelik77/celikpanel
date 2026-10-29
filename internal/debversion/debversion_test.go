package debversion

import "testing"

func TestCompareFollowsDpkg(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"5.40.1-6", "5.40.1-6+deb13u1", -1},
		{"10.46-1~deb13u1", "10.46-1~deb13u3", -1},
		{"10.46-1~deb13u1", "10.46-1", -1},
		{"1.0~rc1-1", "1.0-1", -1},
		{"1:1.0-1", "2.0-1", 1},
		{"4.9.17-0+deb13u1", "4.9.18-0+deb13u1", -1},
		{"4.9.18-0+deb13u1", "4.9.18-0+deb13u1", 0},
		{"1.0a-1", "1.0-1", 1},
		{"1.0-1", "1.0-1.1", -1},
		{"2.3-1ubuntu1", "2.3-1ubuntu1.1", -1},
		{"1.0+b1", "1.0", 1},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%q,%q)=%d want %d", c.b, c.a, got, -c.want)
		}
	}
	for value, want := range map[string]bool{"4.9.18-0+deb13u1": true, "1:2.3-1": true, "": false, "4.9;rm": false, "a1.0": false, ":1.0": false, "1:": false, "1.0-": false} {
		if Valid(value) != want {
			t.Errorf("Valid(%q) != %v", value, want)
		}
	}
}
