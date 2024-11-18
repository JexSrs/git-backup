package utils

import "strings"

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

func FindIndex[T any](ss []T, test func(T) bool) int {
	for i, s := range ss {
		if test(s) {
			return i
		}
	}
	return -1
}
