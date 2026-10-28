package git

import "testing"

func TestCheckVersionParses(t *testing.T) {
	maj, min, pat, err := parseVersion("git version 2.39.3 (Apple Git-146)")
	if err != nil || maj != 2 || min != 39 || pat != 3 {
		t.Fatalf("got %d.%d.%d err=%v", maj, min, pat, err)
	}
	cases := []struct {
		in   string
		want bool
	}{
		{"git version 2.37.9", false},
		{"git version 2.38.0", true},
		{"git version 2.54.0 (Apple Git-157)", true},
		{"git version 1.99.0", false},
		{"git version 3.0.0", true},
		{"git version 2.38", true},
	}
	for _, c := range cases {
		maj, min, pat, err := parseVersion(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got := atLeast(maj, min, pat, 2, 38, 0); got != c.want {
			t.Errorf("%q: atLeast = %v, want %v", c.in, got, c.want)
		}
	}
	if _, _, _, err := parseVersion("garbage"); err == nil {
		t.Error("expected error for garbage")
	}
}
