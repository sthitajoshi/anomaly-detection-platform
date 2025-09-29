package preprocessing

import (
	"regexp"
	"strings"
)

func PreprocessLogText(input string) string {
	reTime := regexp.MustCompile(`\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\]`)
	cleaned := reTime.ReplaceAllString(input, "")

	reIP := regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	cleaned = reIP.ReplaceAllString(cleaned, "[REDACTED_IP]")

	reSpace := regexp.MustCompile(`\s+`)
	cleaned = reSpace.ReplaceAllString(cleaned, " ")

	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.ToLower(cleaned)

	return cleaned
}
