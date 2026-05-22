package music

import (
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(str string) string {
	s := strings.ToLower(str)
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
