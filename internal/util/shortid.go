// Package util provides shared helpers used by cmd and internal packages.
package util

// ShortID returns the first 12 characters of a container ID.
func ShortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
