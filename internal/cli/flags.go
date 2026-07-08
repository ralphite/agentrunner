package cli

import (
	"flag"
	"strings"
)

// reorderFlags moves flags (and, for non-bool flags, their values) ahead of
// the positional args so a user can write flags in the natural place — after
// the subject — e.g. `agentrunner send <sid> "msg" --image x.png`. Go's flag
// package stops at the first positional, turning a trailing flag into a bare
// `usage:` error (blackbox R2-C-3/R2-E-7). Only DEFINED flags are moved, so a
// message or path that happens to start with "-" stays positional, and a
// genuinely mistyped flag still reaches flag.Parse to get its "not defined"
// error. A literal "--" ends flag scanning (the rest are positional).
func reorderFlags(fs *flag.FlagSet, args []string) []string {
	defined := func(a string) *flag.Flag {
		name := strings.TrimLeft(a, "-")
		name, _, _ = strings.Cut(name, "=")
		if name == "" {
			return nil
		}
		return fs.Lookup(name)
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		var f *flag.Flag
		if len(a) > 1 && a[0] == '-' {
			f = defined(a)
		}
		if f == nil {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		// A non-bool flag written as "-x value" (no '=') consumes the next arg.
		if !strings.Contains(a, "=") && i+1 < len(args) {
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !bf.IsBoolFlag() {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	return append(flags, positional...)
}
