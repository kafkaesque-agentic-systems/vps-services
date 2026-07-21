package utils

// RemoveIndex : remove specified index from a slice
func RemoveIndex(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
