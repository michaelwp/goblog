package auth

import (
	"slices"
	"strings"
	"testing"
)

func TestPasswordProblems(t *testing.T) {
	cases := []struct {
		password string
		missing  []string // substrings, in order
	}{
		{"Str0ng!Passw0rd", nil},
		{"Ünïcödé-Pässwört-9", nil}, // non-ASCII letters count
		{"Sh0rt!", []string{"at least 12"}},
		{"lowercase-only-123", []string{"uppercase"}},
		{"UPPERCASE-ONLY-123", []string{"lowercase"}},
		{"No-Numbers-Here!", []string{"number"}},
		{"NoSpecialChars123", []string{"special"}},
		{"has spaces 123 ABC", []string{"special"}}, // a space isn't a special character
		{"", []string{"at least 12", "uppercase", "lowercase", "number", "special"}},
		{"Aa1!" + strings.Repeat("x", 70), []string{"at most 72 bytes"}},
	}
	for _, c := range cases {
		got := PasswordProblems(c.password)
		if len(got) != len(c.missing) {
			t.Errorf("%q: got %q, want %d problems", c.password, got, len(c.missing))
			continue
		}
		for i, want := range c.missing {
			if !strings.Contains(got[i], want) {
				t.Errorf("%q: problem %d = %q, want it to mention %q", c.password, i, got[i], want)
			}
		}
	}
}

func TestDescribePasswordProblems(t *testing.T) {
	got := DescribePasswordProblems([]string{"a number", "an uppercase letter", "a lowercase letter"})
	if got != "The password needs a number, an uppercase letter and a lowercase letter." {
		t.Errorf("got %q", got)
	}
	if DescribePasswordProblems(nil) != "" || !slices.Equal(PasswordProblems("Str0ng!Passw0rd"), nil) {
		t.Error("valid password reported problems")
	}
}
