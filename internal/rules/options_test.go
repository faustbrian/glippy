package rules_test

import (
	"bytes"
	"testing"

	"github.com/faustbrian/gox/internal/rules"
)

func TestOptionSetCanonicalBytesAreOrderedAndValueSensitive(t *testing.T) {
	t.Parallel()

	first := rules.NewOptionSet(
		map[string]rules.OptionValue{
			"boolean": rules.BooleanOption(true),
			"integer": rules.IntegerOption(-12),
			"string": rules.StringOption("value"),
			"strings": rules.StringsOption([]string{"first", "second"}),
		},
	)
	second := rules.NewOptionSet(
		map[string]rules.OptionValue{
			"strings": rules.StringsOption([]string{"first", "second"}),
			"string": rules.StringOption("value"),
			"integer": rules.IntegerOption(-12),
			"boolean": rules.BooleanOption(true),
		},
	)
	if !bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("equivalent option maps have different canonical bytes")
	}
	changed := rules.NewOptionSet(
		map[string]rules.OptionValue{
			"boolean": rules.BooleanOption(false),
			"integer": rules.IntegerOption(-12),
			"string": rules.StringOption("value"),
			"strings": rules.StringsOption([]string{"first", "second"}),
		},
	)
	if bytes.Equal(first.CanonicalBytes(), changed.CanonicalBytes()) {
		t.Fatal("different option values have identical canonical bytes")
	}
}

func TestOptionSetOwnsStringLists(t *testing.T) {
	t.Parallel()

	input := []string{"first", "second"}
	values := map[string]rules.OptionValue{"strings": rules.StringsOption(input)}
	options := rules.NewOptionSet(values)
	input[0] = "mutated input"
	values["strings"] = rules.StringsOption([]string{"replaced"})

	first, found := options.Strings("strings")
	if !found || len(first) != 2 || first[0] != "first" {
		t.Fatalf("Strings() = %#v, %t", first, found)
	}
	first[0] = "mutated result"
	second, _ := options.Strings("strings")
	if second[0] != "first" {
		t.Fatalf("Strings() returned shared storage: %#v", second)
	}
}
