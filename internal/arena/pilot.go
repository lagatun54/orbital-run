package arena

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func PilotName(raw string) string {
	var name strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' || r == '_' {
			name.WriteRune(r)
			if utf8.RuneCountInString(name.String()) == 20 {
				break
			}
		}
	}
	if clean := strings.TrimSpace(name.String()); clean != "" {
		return clean
	}
	return "Пилот"
}
