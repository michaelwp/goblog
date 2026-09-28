package auth

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PasswordProblems lists the password rules p breaks, in a fixed order, as
// short phrases ("an uppercase letter"). An empty result means p is allowed.
// Keep in sync with passwordRules in frontend/src/admin/PasswordField.tsx.
func PasswordProblems(p string) []string {
	var upper, lower, digit, special bool
	for _, r := range p {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			special = true
		}
	}
	var missing []string
	if utf8.RuneCountInString(p) < MinPasswordLength {
		missing = append(missing, fmt.Sprintf("at least %d characters", MinPasswordLength))
	}
	if !upper {
		missing = append(missing, "an uppercase letter")
	}
	if !lower {
		missing = append(missing, "a lowercase letter")
	}
	if !digit {
		missing = append(missing, "a number")
	}
	if !special {
		missing = append(missing, "a special character such as ! @ # ?")
	}
	if len(p) > MaxPasswordBytes {
		missing = append(missing, fmt.Sprintf("at most %d bytes (about %d characters)", MaxPasswordBytes, MaxPasswordBytes))
	}
	return missing
}

// DescribePasswordProblems turns PasswordProblems into one sentence, e.g.
// "The password needs a number and a special character such as ! @ # ?."
func DescribePasswordProblems(missing []string) string {
	switch len(missing) {
	case 0:
		return ""
	case 1:
		return "The password needs " + missing[0] + "."
	default:
		return "The password needs " + strings.Join(missing[:len(missing)-1], ", ") + " and " + missing[len(missing)-1] + "."
	}
}
