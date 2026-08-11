package rules

import (
	"encoding/binary"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// OptionValue is one immutable typed rule configuration value.
type OptionValue struct {
	kind     OptionKind
	boolean  bool
	integer  int64
	text     string
	strings_ []string
}

// Kind returns the declared scalar kind of this value.
func (v OptionValue) Kind() OptionKind { return v.kind }

// String renders one deterministic human-readable option value.
func (v OptionValue) String() string {
	switch v.kind {
	case OptionBoolean:
		return strconv.FormatBool(v.boolean)
	case OptionInteger:
		return strconv.FormatInt(v.integer, 10)
	case OptionString:
		return strconv.Quote(v.text)
	case OptionStrings:
		quoted := make([]string, len(v.strings_))
		for index, item := range v.strings_ {
			quoted[index] = strconv.Quote(item)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	default:
		return "<invalid>"
	}
}

// BooleanOption constructs one boolean rule option value.
func BooleanOption(value bool) OptionValue {
	return OptionValue{kind: OptionBoolean, boolean: value}
}

// IntegerOption constructs one integer rule option value.
func IntegerOption(value int64) OptionValue {
	return OptionValue{kind: OptionInteger, integer: value}
}

// StringOption constructs one string rule option value.
func StringOption(value string) OptionValue {
	return OptionValue{kind: OptionString, text: value}
}

// StringsOption constructs one independently owned string-list rule option.
func StringsOption(value []string) OptionValue {
	return OptionValue{kind: OptionStrings, strings_: slices.Clone(value)}
}

// OptionSet is one immutable name-indexed rule configuration snapshot.
type OptionSet struct {
	values map[string]OptionValue
}

// NewOptionSet snapshots independently owned rule option values.
func NewOptionSet(values map[string]OptionValue) OptionSet {
	if len(values) == 0 {
		return OptionSet{}
	}
	result := OptionSet{values: make(map[string]OptionValue, len(values))}
	for name, value := range values {
		value.strings_ = slices.Clone(value.strings_)
		result.values[name] = value
	}
	return result
}

// Boolean returns one configured boolean value.
func (s OptionSet) Boolean(name string) (bool, bool) {
	value, found := s.values[name]
	return value.boolean, found && value.kind == OptionBoolean
}

// Integer returns one configured integer value.
func (s OptionSet) Integer(name string) (int64, bool) {
	value, found := s.values[name]
	return value.integer, found && value.kind == OptionInteger
}

// String returns one configured string value.
func (s OptionSet) String(name string) (string, bool) {
	value, found := s.values[name]
	return value.text, found && value.kind == OptionString
}

// Strings returns one independently owned configured string list.
func (s OptionSet) Strings(name string) ([]string, bool) {
	value, found := s.values[name]
	if !found || value.kind != OptionStrings {
		return nil, false
	}
	return slices.Clone(value.strings_), true
}

// CanonicalBytes returns one deterministic cache identity for this option set.
func (s OptionSet) CanonicalBytes() []byte {
	encoded := []byte("gox-rule-options-v1")
	names := make([]string, 0, len(s.values))
	for name := range s.values {
		names = append(names, name)
	}
	sort.Strings(names)
	encoded = binary.AppendUvarint(encoded, uint64(len(names)))
	for _, name := range names {
		value := s.values[name]
		encoded = appendOptionString(encoded, name)
		encoded = appendOptionString(encoded, string(value.kind))
		switch value.kind {
		case OptionBoolean:
			if value.boolean {
				encoded = append(encoded, 1)
			} else {
				encoded = append(encoded, 0)
			}
		case OptionInteger:
			encoded = binary.AppendVarint(encoded, value.integer)
		case OptionString:
			encoded = appendOptionString(encoded, value.text)
		case OptionStrings:
			encoded = binary.AppendUvarint(encoded, uint64(len(value.strings_)))
			for _, item := range value.strings_ {
				encoded = appendOptionString(encoded, item)
			}
		}
	}
	return encoded
}

func appendOptionString(encoded []byte, value string) []byte {
	encoded = binary.AppendUvarint(encoded, uint64(len(value)))
	return append(encoded, value...)
}

func cloneOptionValue(value OptionValue) OptionValue {
	value.strings_ = slices.Clone(value.strings_)
	return value
}

func cloneOptionMetadata(options []OptionMetadata) []OptionMetadata {
	result := make([]OptionMetadata, len(options))
	for index, option := range options {
		result[index] = option
		if option.Default != nil {
			value := cloneOptionValue(*option.Default)
			result[index].Default = &value
		}
	}
	return result
}
