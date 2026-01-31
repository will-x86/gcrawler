// Stolen and changed from https://github.com/subosito/gozaru
package storage

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	//characterFilter   = `[\x00-\x1F\/\\:\*\?\"<>\|]`
	characterFilter   = `[\x00-\x1F\/\\:\*\?\"<>\|]`
	unicodeWhitespace = `[[:space:]]+`
)

var (
	fallbackFilename = "file"
	/*windowsReservedNames = [...]string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5",
		"COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5",
		"LPT6", "LPT7", "LPT8", "LPT9",
	}*/
)

/*
	func Sanitize(s string) string {
		return sanitize(s, 0, "")
	}

	func SanitizeFallback(s string, fallback string) string {
		return sanitize(s, 0, fallback)
	}

	func SanitizePad(s string, n int) string {
		return sanitize(s, n, "")
	}

	func SanitizePadFallback(s string, n int, fallback string) string {
		return sanitize(s, n, fallback)
	}
*/
func sanitize(s string, n int, fallback string) string {
	if fallback == "" {
		fallback = fallbackFilename
	}

	sc := clean(s, fallback)
	nc := len(sc)

	if n > nc {
		return sc
	}

	if nc > 255 {
		nc = 255
	}

	if n != 0 {
		nc -= n
	}

	return sc[0:nc]
}

func replace(s string, pattern string, replacement string) string {
	rx := regexp.MustCompile(pattern)
	return strings.TrimSpace(rx.ReplaceAllString(s, replacement))
}

func clean(s string, fallback string) string {
	sc := replace(s, unicodeWhitespace, " ")
	sc = replace(sc, characterFilter, "_")
	sc = replace(sc, unicodeWhitespace, " ")
	return filter(sc, fallback)
}
func filter(s string, fallback string) string {
	//s = filterWindowsReservedNames(s, fallback) // Sorry windows
	s = filterBlank(s, fallback)
	s = filterDot(s, fallback)

	return s
}

/*func filterWindowsReservedNames(s string, fallback string) string {
	us := strings.ToUpper(s)

	for i := range windowsReservedNames {
		v := windowsReservedNames[i]

		if v == us {
			return fallback
		}
	}

	return s
}*/

func filterBlank(s string, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}

func filterDot(s string, fallback string) string {
	if strings.HasPrefix(s, ".") {
		return fmt.Sprintf("%s%s", fallback, s)
	}

	return s
}
