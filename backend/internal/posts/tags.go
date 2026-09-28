package posts

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MaxTags      = 10
	MaxTagLength = 30
)

// A tag is lowercase letters and digits (any script), with "-" between
// words and the "+#." found in names like c++, c#, node.js and .net.
var tagPattern = regexp.MustCompile(`^[\p{Ll}\p{Lo}\p{N}+#.]+(?:-[\p{Ll}\p{Lo}\p{N}+#.]+)*$`)

// NormalizeTag turns user input like " Node JS " into "node-js".
func NormalizeTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#")))
	s = strings.Join(strings.Fields(s), "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// ParseTags reads a comma-separated tag list, normalizing each tag and
// dropping duplicates and blanks. It reports the first problem, if any.
func ParseTags(input string) ([]string, string) {
	tags := []string{}
	for _, raw := range strings.Split(input, ",") {
		t := NormalizeTag(raw)
		if t == "" || contains(tags, t) {
			continue
		}
		if utf8.RuneCountInString(t) > MaxTagLength {
			return nil, fmt.Sprintf("Keep each tag under %d characters (%q is too long).", MaxTagLength, t)
		}
		if !tagPattern.MatchString(t) {
			return nil, fmt.Sprintf("Tags can use letters, numbers, - + # and . (check %q).", t)
		}
		tags = append(tags, t)
	}
	if len(tags) > MaxTags {
		return nil, fmt.Sprintf("Use at most %d tags.", MaxTags)
	}
	return tags, ""
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
