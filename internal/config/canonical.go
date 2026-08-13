package config

import (
	"encoding/binary"
	"sort"
)

// CanonicalBytes returns the deterministic identity of one fully resolved
// configuration. It excludes source spelling, comments, and cache lifecycle
// policy while retaining every value that can affect formatting or analysis.
func (c Config) CanonicalBytes() []byte {
	encoded := []byte("glippy-configuration-v1")
	encoded = binary.AppendVarint(encoded, int64(c.Version))
	encoded = binary.AppendVarint(encoded, int64(c.Format.LineWidth))
	encoded = binary.AppendVarint(encoded, int64(c.Format.TabWidth))
	encoded = binary.AppendUvarint(encoded, uint64(len(c.Analysis.BuildTags)))
	for _, tag := range c.Analysis.BuildTags {
		encoded = appendCanonicalString(encoded, tag)
	}
	encoded = appendCanonicalString(encoded, c.Analysis.GOOS)
	encoded = appendCanonicalString(encoded, c.Analysis.GOARCH)
	if c.Analysis.CGOEnabled {
		encoded = append(encoded, 1)
	} else {
		encoded = append(encoded, 0)
	}
	encoded = binary.AppendUvarint(encoded, uint64(len(c.Lint.Presets)))
	for _, preset := range c.Lint.Presets {
		encoded = appendCanonicalString(encoded, string(preset))
	}
	if c.Lint.WarningsAsErrors {
		encoded = append(encoded, 1)
	} else {
		encoded = append(encoded, 0)
	}

	ruleIDs := make([]string, 0, len(c.Lint.Rules))
	for ruleID := range c.Lint.Rules {
		ruleIDs = append(ruleIDs, ruleID)
	}
	sort.Strings(ruleIDs)
	encoded = binary.AppendUvarint(encoded, uint64(len(ruleIDs)))
	for _, ruleID := range ruleIDs {
		encoded = appendCanonicalString(encoded, ruleID)
		encoded = appendCanonicalString(encoded, string(c.Lint.Rules[ruleID]))
	}

	optionRuleIDs := make([]string, 0, len(c.Lint.RuleOptions))
	for ruleID := range c.Lint.RuleOptions {
		optionRuleIDs = append(optionRuleIDs, ruleID)
	}
	sort.Strings(optionRuleIDs)
	encoded = binary.AppendUvarint(encoded, uint64(len(optionRuleIDs)))
	for _, ruleID := range optionRuleIDs {
		encoded = appendCanonicalString(encoded, ruleID)
		encoded = appendCanonicalBytes(encoded, c.Lint.RuleOptions[ruleID].CanonicalBytes())
	}

	if c.Lint.Suppressions.RequireReason {
		encoded = append(encoded, 1)
	} else {
		encoded = append(encoded, 0)
	}
	encoded = appendCanonicalString(encoded, c.Lint.Suppressions.ExpiryCutoff)
	encoded = appendCanonicalString(encoded, c.Lint.Baseline.Path)
	if c.Lint.Baseline.ReportStale {
		encoded = append(encoded, 1)
	} else {
		encoded = append(encoded, 0)
	}
	encoded = appendCanonicalString(encoded, c.Lint.Baseline.ExpiryCutoff)
	return encoded
}

func appendCanonicalString(encoded []byte, value string) []byte {
	return appendCanonicalBytes(encoded, []byte(value))
}

func appendCanonicalBytes(encoded, value []byte) []byte {
	encoded = binary.AppendUvarint(encoded, uint64(len(value)))
	return append(encoded, value...)
}
