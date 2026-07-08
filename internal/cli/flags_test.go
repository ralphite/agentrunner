package cli

import (
	"flag"
	"reflect"
	"testing"
)

// reorderFlags lets users put flags after positionals (the natural place), and
// must not disturb messages/paths that merely start with "-", nor swallow the
// value of a non-bool flag.
func TestReorderFlags(t *testing.T) {
	newFS := func() *flag.FlagSet {
		fs := flag.NewFlagSet("send", flag.ContinueOnError)
		fs.String("image", "", "value flag")
		fs.Bool("detach", false, "bool flag")
		return fs
	}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"flag after positional", []string{"sid", "msg", "--image", "x.png"},
			[]string{"--image", "x.png", "sid", "msg"}},
		{"bool flag after positional", []string{"sid", "msg", "--detach"},
			[]string{"--detach", "sid", "msg"}},
		{"already-first stays", []string{"--image", "x.png", "sid", "msg"},
			[]string{"--image", "x.png", "sid", "msg"}},
		{"equals form", []string{"sid", "msg", "--image=x.png"},
			[]string{"--image=x.png", "sid", "msg"}},
		{"unknown flag stays positional", []string{"sid", "--nope", "msg"},
			[]string{"sid", "--nope", "msg"}},
		{"message starting with dash stays", []string{"sid", "-not a flag"},
			[]string{"sid", "-not a flag"}},
		{"double dash ends scanning", []string{"--detach", "--", "--image", "literal"},
			[]string{"--detach", "--image", "literal"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := reorderFlags(newFS(), c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("reorderFlags(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
