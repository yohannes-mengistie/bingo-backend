package utils

import (
	"regexp"
	"strings"
)

// NormalizePhoneNumber normalizes a phone number to a standard format
// Removes all non-digit characters and ensures consistent formatting
func NormalizePhoneNumber(phone string) string {
	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	normalized := re.ReplaceAllString(phone, "")
	
	// Remove leading zeros if present
	normalized = strings.TrimLeft(normalized, "0")
	
	return normalized
}

