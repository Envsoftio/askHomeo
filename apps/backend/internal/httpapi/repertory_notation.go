package httpapi

import "strings"

func validSourceStyle(style string) bool {
	switch style {
	case "unknown", "ordinary", "italic", "bold", "bold_italic", "other":
		return true
	}
	return false
}

func conventionSupported(scheme string, locations []structuredLocationInput) bool {
	if strings.TrimSpace(scheme) == "" {
		return false
	}
	for _, loc := range locations {
		if strings.Contains(loc.ExactText, scheme) {
			return true
		}
	}
	return false
}
