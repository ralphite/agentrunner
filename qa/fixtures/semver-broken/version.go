// Package semver compares Semantic Versioning 2.0.0 version strings.
package semver

// Compare returns -1 if a is a lower precedence than b, 0 if they have equal
// precedence, and +1 if a is higher. Inputs are well-formed semver 2.0.0
// strings (major.minor.patch, with an optional -prerelease and +build).
//
// TODO(coder): implement per the semver 2.0.0 precedence rules. The
// pre-release ordering is the subtle part — do not guess it.
func Compare(a, b string) int {
	panic("Compare: not implemented")
}
