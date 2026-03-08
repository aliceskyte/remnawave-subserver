package panel

import "strings"

const maxShortUUIDLength = 128

func ValidShortUUID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxShortUUIDLength {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
