package utils

import (
	"strings"
)

func Pointer[T any](s T) *T {
	return &s
}

func Reverse[T any](a []T) []T {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
	return a
}

func ContainsIgnoreCase(slice []string, value string) bool {
	vl := strings.ToLower(value)

	for _, v := range slice {
		if strings.ToLower(v) == vl {
			return true
		}
	}
	return false
}

func ExtractNameFromGitURL(url string) string {
	// Remove trailing slash if present
	url = strings.TrimSuffix(url, "/")

	// Find the last slash
	lastSlash := strings.LastIndex(url, "/")
	if lastSlash == -1 {
		return url // No slash found, return the whole string
	}

	// Extract everything after the last slash
	name := url[lastSlash+1:]

	// Remove .git suffix if present
	name = strings.TrimSuffix(name, ".git")

	return name
}

func CheckPrefix(id string, prefix string) bool {
	return strings.HasPrefix(id, prefix+"-") || id == prefix
}
