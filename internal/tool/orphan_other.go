//go:build !linux && !darwin

package tool

// No scan backend here: the sweep acts only on positive evidence, so an
// unsupported platform sweeps nothing rather than guessing.
func listOrphanSessionGroups() []int { return nil }
