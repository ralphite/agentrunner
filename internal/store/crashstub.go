package store

// crashAfter is the seam for the counting crash predicate
// (`AGENTRUNNER_CRASH=after:<EventType>:<n>`), checked after a successful
// fsynced append. TODO(2.6): replace this stub with the internal/crash
// registry when the injection harness lands.
func crashAfter(string) {}
