package config_test

import (
	"reflect"
	"testing"

	"github.com/faustbrian/gox/internal/config"
)

func FuzzParseDeterministic(f *testing.F) {
	f.Add([]byte("version = 1\n"))
	f.Add([]byte("version = 1\n[format]\nline-width = 88\ntab-width = 4\n"))
	f.Add([]byte("version = 1\n[lint.rules]\nknown-rule = \"warn\"\n"))
	f.Add([]byte("version = 1\n[lint.suppressions]\nexpiry-cutoff = \"2026-08-11\"\n"))
	f.Add([]byte("version = 1\n[lint.suppressions]\nexpiry-cutoff = \"2026-02-30\"\n"))
	f.Add([]byte("version = 1\nunknown = true\n"))
	f.Add([]byte("version = [\n"))

	options := config.ParseOptions{KnownRules: []string{"known-rule"}}
	f.Fuzz(func(t *testing.T, input []byte) {
		first, firstErr := config.Parse("fuzz/.gox.toml", input, options)
		second, secondErr := config.Parse("fuzz/.gox.toml", input, options)
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("Parse() success changed between identical runs: %v, %v", firstErr, secondErr)
		}
		if firstErr != nil {
			if firstErr.Error() != secondErr.Error() {
				t.Fatalf("Parse() error changed between identical runs: %q, %q", firstErr, secondErr)
			}
			return
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("Parse() changed between identical runs: %#v, %#v", first, second)
		}
		if first.Version != config.Version || first.Format.LineWidth <= 0 || first.Format.TabWidth <= 0 {
			t.Fatalf("Parse() accepted invalid typed configuration: %#v", first)
		}
	})
}
