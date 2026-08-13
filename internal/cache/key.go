// Package cache owns deterministic persistent-cache identity and storage.
package cache

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"slices"
	"sort"
	"strings"
)

// Digest is one SHA-256 identity used as a cache-key component.
type Digest [sha256.Size]byte

// DigestOf returns the SHA-256 digest of exact bytes.
func DigestOf(value []byte) Digest {
	return sha256.Sum256(value)
}

// Key is one complete cache identity.
type Key [sha256.Size]byte

// String returns the lowercase hexadecimal key.
func (k Key) String() string {
	return hex.EncodeToString(k[:])
}

// ComponentKind identifies one result-changing input class.
type ComponentKind string

const (
	ComponentSource ComponentKind = "source"
	ComponentModule ComponentKind = "module"
	ComponentWorkspace ComponentKind = "workspace"
	ComponentOverlay ComponentKind = "overlay"
	ComponentBuildSelection ComponentKind = "build-selection"
	ComponentEnvironment ComponentKind = "environment"
	ComponentDependencyExport ComponentKind = "dependency-export"
	ComponentFact ComponentKind = "fact"
)

// Component binds one named input to its exact digest.
type Component struct {
	Kind ComponentKind
	Identity string
	Digest Digest
}

// RuleInput binds one enabled rule and severity to its canonical options.
type RuleInput struct {
	ID string
	Severity string
	Options Digest
}

// KeyInput contains every explicit result-changing input selected by a cache
// consumer for one result namespace. It performs no ambient environment lookup.
type KeyInput struct {
	Namespace string
	ToolVersion string
	BuildGoVersion string
	SourceGoVersion string
	Configuration Digest
	Rules []RuleInput
	BuildTags []string
	GOOS string
	GOARCH string
	CGOEnabled bool
	FormatterMode string
	Components []Component
}

// BuildKey validates and canonically hashes one complete input set.
func BuildKey(input KeyInput) (Key, error) {
	if err := validateKeyInput(input); err != nil {
		return Key{}, err
	}
	tags := slices.Clone(input.BuildTags)
	sort.Strings(tags)
	tags = slices.Compact(tags)
	rules := slices.Clone(input.Rules)
	sort.Slice(
		rules,
		func(left, right int) bool {
			return rules[left].ID < rules[right].ID
		},
	)
	components := slices.Clone(input.Components)
	sort.Slice(
		components,
		func(left, right int) bool {
			if components[left].Kind != components[right].Kind {
				return components[left].Kind < components[right].Kind
			}
			return components[left].Identity < components[right].Identity
		},
	)

	digest := sha256.New()
	writeString(digest, "glippy-cache-key-v1")
	writeString(digest, input.Namespace)
	writeString(digest, input.ToolVersion)
	writeString(digest, input.BuildGoVersion)
	writeString(digest, input.SourceGoVersion)
	writeDigest(digest, input.Configuration)
	writeStrings(digest, tags)
	writeString(digest, input.GOOS)
	writeString(digest, input.GOARCH)
	if input.CGOEnabled {
		writeString(digest, "cgo-enabled")
	} else {
		writeString(digest, "cgo-disabled")
	}
	writeString(digest, input.FormatterMode)
	writeUint64(digest, uint64(len(rules)))
	for _, rule := range rules {
		writeString(digest, rule.ID)
		writeString(digest, rule.Severity)
		writeDigest(digest, rule.Options)
	}
	writeUint64(digest, uint64(len(components)))
	for _, component := range components {
		writeString(digest, string(component.Kind))
		writeString(digest, component.Identity)
		writeDigest(digest, component.Digest)
	}
	var key Key
	copy(key[:], digest.Sum(nil))
	return key, nil
}

func validateKeyInput(input KeyInput) error {
	values := []struct {
		name string
		value string
	}{
		{name: "namespace", value: input.Namespace},
		{name: "tool version", value: input.ToolVersion},
		{name: "build Go version", value: input.BuildGoVersion},
		{name: "source Go version", value: input.SourceGoVersion},
		{name: "GOOS", value: input.GOOS},
		{name: "GOARCH", value: input.GOARCH},
		{name: "formatter mode", value: input.FormatterMode},
	}
	for _, item := range values {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("cache key %s is required", item.name)
		}
	}
	if input.Configuration == (Digest{}) {
		return fmt.Errorf("cache key configuration digest is required")
	}
	if len(input.Components) == 0 {
		return fmt.Errorf("cache key requires at least one component")
	}
	for _, tag := range input.BuildTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("cache key build tags must not be empty")
		}
	}
	ruleIDs := make(map[string]struct{}, len(input.Rules))
	for _, rule := range input.Rules {
		if strings.TrimSpace(rule.ID) == "" {
			return fmt.Errorf("cache key rule ID is required")
		}
		if strings.TrimSpace(rule.Severity) == "" {
			return fmt.Errorf("cache key rule %q severity is required", rule.ID)
		}
		if rule.Options == (Digest{}) {
			return fmt.Errorf("cache key rule %q options digest is required", rule.ID)
		}
		if _, duplicate := ruleIDs[rule.ID]; duplicate {
			return fmt.Errorf("cache key rule %q is duplicated", rule.ID)
		}
		ruleIDs[rule.ID] = struct{}{}
	}
	type componentIdentity struct {
		kind ComponentKind
		identity string
	}
	components := make(map[componentIdentity]struct{}, len(input.Components))
	for _, component := range input.Components {
		if !validComponentKind(component.Kind) {
			return fmt.Errorf("cache key component kind %q is invalid", component.Kind)
		}
		if strings.TrimSpace(component.Identity) == "" {
			return fmt.Errorf(
				"cache key %s component identity is required",
				component.Kind,
			)
		}
		if component.Digest == (Digest{}) {
			return fmt.Errorf(
				"cache key %s component %q digest is required",
				component.Kind,
				component.Identity,
			)
		}
		identity := componentIdentity{kind: component.Kind, identity: component.Identity}
		if _, duplicate := components[identity]; duplicate {
			return fmt.Errorf(
				"cache key %s component %q is duplicated",
				component.Kind,
				component.Identity,
			)
		}
		components[identity] = struct{}{}
	}
	return nil
}

func validComponentKind(kind ComponentKind) bool {
	switch kind {
	case ComponentSource,
		ComponentModule,
		ComponentWorkspace,
		ComponentOverlay,
		ComponentBuildSelection,
		ComponentEnvironment,
		ComponentDependencyExport,
		ComponentFact:
		return true
	default:
		return false
	}
}

func writeStrings(digest hash.Hash, values []string) {
	writeUint64(digest, uint64(len(values)))
	for _, value := range values {
		writeString(digest, value)
	}
}

func writeString(digest hash.Hash, value string) {
	writeUint64(digest, uint64(len(value)))
	_, _ = digest.Write([]byte(value))
}

func writeDigest(output hash.Hash, digest Digest) {
	_, _ = output.Write(digest[:])
}

func writeUint64(digest hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = digest.Write(encoded[:])
}
