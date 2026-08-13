// Package suppressions parses and owns physical lint-suppression ranges.
package suppressions

import (
	"bytes"
	"cmp"
	"errors"
	"go/token"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

// Scope identifies the physical ownership policy of one suppression.
type Scope uint8

const (
	ScopeLine Scope = iota + 1
	ScopeNextLine
	ScopeRange
	ScopeFile
)

// ProblemKind classifies one suppression or parser-configuration defect.
type ProblemKind string

const (
	ProblemMalformed ProblemKind = "malformed"
	ProblemUnknownRule ProblemKind = "unknown-rule"
	ProblemMissingReason ProblemKind = "missing-reason"
	ProblemInvalidExpiry ProblemKind = "invalid-expiry"
	ProblemExpired ProblemKind = "expired"
	ProblemMisplacedFileScope ProblemKind = "misplaced-file-scope"
	ProblemUnmatchedRangeEnd ProblemKind = "unmatched-range-end"
	ProblemNestedRange ProblemKind = "nested-range"
	ProblemUnclosedRange ProblemKind = "unclosed-range"
	ProblemInvalidConfiguration ProblemKind = "invalid-configuration"
)

// Problem is one source-ordered suppression diagnostic.
type Problem struct {
	Kind ProblemKind
	Range source.Range
	Message string
}

// Directive is one validated exact-rule suppression.
type Directive struct {
	Scope Scope
	RuleID string
	Range source.Range
	Target source.Range
	Reason string
	ExpiresOn string
}

// ParseOptions supplies registry, reason, and deterministic expiry policy.
type ParseOptions struct {
	KnownRules []string
	RequireReason bool
	ExpiryCutoff string
}

// Index is an immutable source-ordered suppression set.
type Index struct {
	directives []Directive
	path string
	digest source.Digest
	size int
}

// SuppressedDiagnostic binds one removed diagnostic to its owning directive.
type SuppressedDiagnostic struct {
	Diagnostic rules.Diagnostic
	Directive Directive
}

// Application is one deterministic suppression pass over ordered diagnostics.
type Application struct {
	Diagnostics []rules.Diagnostic
	Suppressed []SuppressedDiagnostic
	Unused []Directive
}

type parsedDirective struct {
	scope Scope
	rangeEnd bool
	ruleID string
	reason string
	expiresOn string
	range_ source.Range
}

type physicalSource interface {
	Bytes() []byte
	Tokens() []source.Token
	Directives() []source.Directive
}

type ownershipFingerprint struct {
	directive int
	start int
	end int
}

// Directives returns an independent source-ordered suppression snapshot.
func (i Index) Directives() []Directive {
	return slices.Clone(i.directives)
}

// ValidateStable verifies that every structurally valid suppression continues
// to own the same normalized physical tokens after a source transformation.
// Rule registration and configurable reason or expiry policy deliberately do
// not participate because formatting must be stable without lint configuration.
func ValidateStable(before, after physicalSource) error {
	if before == nil || after == nil {
		return errors.New("suppression stability requires two source units")
	}
	beforeOwnership := suppressionOwnership(before)
	afterOwnership := suppressionOwnership(after)
	if !slices.EqualFunc(
		beforeOwnership,
		afterOwnership,
		func(left, right ownershipFingerprint) bool {
			return left == right
		},
	) {
		return errors.New("suppression ownership changed")
	}
	return nil
}

// Parse validates Gox suppression directives and assigns physical targets.
func Parse(file *source.File, options ParseOptions) (Index, []Problem) {
	known, configurationProblem := knownRuleSet(options.KnownRules)
	if configurationProblem != nil {
		return Index{}, []Problem{*configurationProblem}
	}
	if options.ExpiryCutoff != "" && !validDate(options.ExpiryCutoff) {
		return Index{}, []Problem{
			{
				Kind: ProblemInvalidConfiguration,
				Message: "suppression expiry cutoff must be a valid YYYY-MM-DD date",
			},
		}
	}
	if file == nil {
		return Index{}, []Problem{
			{
				Kind: ProblemInvalidConfiguration,
				Message: "suppression parsing requires a source file",
			},
		}
	}

	input := file.Bytes()
	packageOffset := len(input)
	for _, item := range file.Tokens() {
		if item.Kind == token.PACKAGE {
			packageOffset = item.Range.Start
			break
		}
	}

	directives := make([]Directive, 0)
	problems := make([]Problem, 0)
	openRanges := make(map[string]parsedDirective)
	for _, candidate := range file.Directives() {
		if candidate.Kind != source.DirectiveGoxSuppression {
			continue
		}
		parsed, problem := parseDirective(
			candidate,
			known,
			options.RequireReason,
			options.ExpiryCutoff,
		)
		if problem != nil {
			problems = append(problems, *problem)
			continue
		}

		if parsed.rangeEnd {
			opened, found := openRanges[parsed.ruleID]
			if !found {
				problems = append(
					problems,
					Problem{
						Kind: ProblemUnmatchedRangeEnd,
						Range: parsed.range_,
						Message: "suppression range end has no matching start for " +
							parsed.ruleID,
					},
				)
				continue
			}
			directives = append(
				directives,
				Directive{
					Scope: ScopeRange,
					RuleID: parsed.ruleID,
					Range: opened.range_,
					Target: source.Range{
						Start: opened.range_.End,
						End: parsed.range_.Start,
					},
					Reason: opened.reason,
					ExpiresOn: opened.expiresOn,
				},
			)
			delete(openRanges, parsed.ruleID)
			continue
		}

		if parsed.scope == ScopeRange {
			if _, found := openRanges[parsed.ruleID]; found {
				problems = append(
					problems,
					Problem{
						Kind: ProblemNestedRange,
						Range: parsed.range_,
						Message: "suppression range is already open for " +
							parsed.ruleID,
					},
				)
				continue
			}
			openRanges[parsed.ruleID] = parsed
			continue
		}

		target := source.Range{}
		switch parsed.scope {
		case ScopeLine:
			target = lineRange(input, parsed.range_.Start)
		case ScopeNextLine:
			target = nextLineRange(input, parsed.range_.End)
		case ScopeFile:
			if parsed.range_.Start >= packageOffset {
				problems = append(
					problems,
					Problem{
						Kind: ProblemMisplacedFileScope,
						Range: parsed.range_,
						Message: "file suppression must appear before the package clause",
					},
				)
				continue
			}
			target = source.Range{Start: 0, End: len(input)}
		}
		directives = append(
			directives,
			Directive{
				Scope: parsed.scope,
				RuleID: parsed.ruleID,
				Range: parsed.range_,
				Target: target,
				Reason: parsed.reason,
				ExpiresOn: parsed.expiresOn,
			},
		)
	}

	for _, opened := range openRanges {
		problems = append(
			problems,
			Problem{
				Kind: ProblemUnclosedRange,
				Range: opened.range_,
				Message: "suppression range is not closed for " + opened.ruleID,
			},
		)
	}
	slices.SortFunc(
		problems,
		func(left, right Problem) int {
			if order := cmp.Compare(left.Range.Start, right.Range.Start); order != 0 {
				return order
			}
			if order := cmp.Compare(left.Range.End, right.Range.End); order != 0 {
				return order
			}
			if order := cmp.Compare(left.Kind, right.Kind); order != 0 {
				return order
			}
			return cmp.Compare(left.Message, right.Message)
		},
	)
	slices.SortFunc(
		directives,
		func(left, right Directive) int {
			if order := cmp.Compare(left.Range.Start, right.Range.Start); order != 0 {
				return order
			}
			return cmp.Compare(left.RuleID, right.RuleID)
		},
	)
	return Index{
		directives: directives,
		path: file.Path(),
		digest: file.Digest(),
		size: len(input),
	}, problems
}

// Match returns the first source-ordered directive owning a diagnostic start.
func (i Index) Match(diagnostic rules.Diagnostic) (Directive, bool) {
	index, found := i.matchIndex(diagnostic)
	if !found {
		return Directive{}, false
	}
	return i.directives[index], true
}

// Apply removes matched diagnostics and reports used and unused directives.
func (i Index) Apply(diagnostics []rules.Diagnostic) Application {
	result := Application{
		Diagnostics: make([]rules.Diagnostic, 0, len(diagnostics)),
		Suppressed: make([]SuppressedDiagnostic, 0),
		Unused: make([]Directive, 0, len(i.directives)),
	}
	used := make([]bool, len(i.directives))
	for _, diagnostic := range diagnostics {
		index, found := i.matchIndex(diagnostic)
		if !found {
			result.Diagnostics = append(result.Diagnostics, diagnostic)
			continue
		}
		used[index] = true
		result.Suppressed = append(
			result.Suppressed,
			SuppressedDiagnostic{
				Diagnostic: diagnostic,
				Directive: i.directives[index],
			},
		)
	}
	for index, directive := range i.directives {
		if !used[index] {
			result.Unused = append(result.Unused, directive)
		}
	}
	return result
}

func (i Index) matchIndex(diagnostic rules.Diagnostic) (int, bool) {
	if diagnostic.Path != i.path || diagnostic.Digest != i.digest {
		return 0, false
	}
	if diagnostic.Range.Start < 0 ||
		diagnostic.Range.End < diagnostic.Range.Start ||
		diagnostic.Range.End > i.size {
		return 0, false
	}
	for index, directive := range i.directives {
		if directive.RuleID != diagnostic.RuleID {
			continue
		}
		if diagnostic.Range.Start >= directive.Target.Start &&
			diagnostic.Range.Start < directive.Target.End {
			return index, true
		}
	}
	return 0, false
}

func knownRuleSet(ruleIDs []string) (map[string]struct{}, *Problem) {
	known := make(map[string]struct{}, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		if strings.TrimSpace(ruleID) == "" {
			return nil, &Problem{
				Kind: ProblemInvalidConfiguration,
				Message: "known suppression rule IDs must not be empty",
			}
		}
		if _, duplicate := known[ruleID]; duplicate {
			return nil, &Problem{
				Kind: ProblemInvalidConfiguration,
				Message: "known suppression rule ID appears more than once: " +
					ruleID,
			}
		}
		known[ruleID] = struct{}{}
	}
	return known, nil
}

func parseDirective(
	candidate source.Directive,
	known map[string]struct{},
	requireReason bool,
	expiryCutoff string,
) (parsedDirective, *Problem) {
	parsed, reason, hasReason, problem := parseDirectiveShape(candidate)
	if problem != nil {
		return parsedDirective{}, problem
	}
	if _, found := known[parsed.ruleID]; !found {
		return parsedDirective{}, &Problem{
			Kind: ProblemUnknownRule,
			Range: candidate.Range,
			Message: "suppression names unknown rule " + parsed.ruleID,
		}
	}
	parsed, problem = validateDirectiveSyntax(candidate, parsed, reason, hasReason)
	if problem != nil {
		return parsedDirective{}, problem
	}
	if requireReason && !parsed.rangeEnd && parsed.reason == "" {
		return parsedDirective{}, &Problem{
			Kind: ProblemMissingReason,
			Range: candidate.Range,
			Message: "suppression requires a non-empty reason",
		}
	}
	if expiryCutoff != "" && parsed.expiresOn != "" && parsed.expiresOn <= expiryCutoff {
		return parsedDirective{}, &Problem{
			Kind: ProblemExpired,
			Range: candidate.Range,
			Message: "suppression expired on " + parsed.expiresOn,
		}
	}
	return parsed, nil
}

func parseDirectiveSyntax(candidate source.Directive) (parsedDirective, *Problem) {
	parsed, reason, hasReason, problem := parseDirectiveShape(candidate)
	if problem != nil {
		return parsedDirective{}, problem
	}
	return validateDirectiveSyntax(candidate, parsed, reason, hasReason)
}

func parseDirectiveShape(candidate source.Directive) (parsedDirective, string, bool, *Problem) {
	text := strings.TrimSpace(strings.TrimPrefix(candidate.Raw, "//gox:"))
	separator := strings.IndexFunc(text, unicode.IsSpace)
	if separator < 0 {
		return parsedDirective{}, "", false, malformed(
			candidate.Range,
			"suppression directive requires a rule ID",
		)
	}
	label, rest := text[:separator], text[separator:]
	rest = strings.TrimSpace(rest)
	parsed := parsedDirective{range_: candidate.Range}
	switch label {
	case "ignore":
		parsed.scope = ScopeNextLine
	case "ignore-line":
		parsed.scope = ScopeLine
	case "ignore-start":
		parsed.scope = ScopeRange
	case "ignore-end":
		parsed.scope = ScopeRange
		parsed.rangeEnd = true
	case "ignore-file":
		parsed.scope = ScopeFile
	default:
		return parsedDirective{}, "", false, malformed(
			candidate.Range,
			"unknown suppression directive " + label,
		)
	}

	ruleText, reason, hasReason := splitReason(rest)
	fields := strings.Fields(ruleText)
	if len(fields) != 1 {
		return parsedDirective{}, "", false, malformed(
			candidate.Range,
			"suppression directive requires exactly one rule ID",
		)
	}
	parsed.ruleID = fields[0]
	return parsed, reason, hasReason, nil
}

func validateDirectiveSyntax(
	candidate source.Directive,
	parsed parsedDirective,
	reason string,
	hasReason bool,
) (parsedDirective, *Problem) {
	if parsed.rangeEnd {
		if hasReason {
			return parsedDirective{}, malformed(
				candidate.Range,
				"suppression range end must not include a reason",
			)
		}
		return parsed, nil
	}
	parsed.reason = reason
	if hasReason {
		var expiryProblem *Problem
		parsed.reason, parsed.expiresOn, expiryProblem = parseExpiryMetadata(
			reason,
			candidate.Range,
		)
		if expiryProblem != nil {
			return parsedDirective{}, expiryProblem
		}
	}
	if hasReason && parsed.reason == "" {
		return parsedDirective{}, &Problem{
			Kind: ProblemMissingReason,
			Range: candidate.Range,
			Message: "suppression requires a non-empty reason",
		}
	}
	return parsed, nil
}

func suppressionOwnership(unit physicalSource) []ownershipFingerprint {
	input := unit.Bytes()
	tokens := unit.Tokens()
	tokenStarts := normalizedTokenStarts(tokens)
	packageOffset := len(input)
	for _, item := range tokens {
		if item.Kind == token.PACKAGE {
			packageOffset = item.Range.Start
			break
		}
	}

	result := make([]ownershipFingerprint, 0)
	openRanges := make(
		map[string]struct {
			directive int
			parsed parsedDirective
		},
	)
	directiveIndex := 0
	for _, candidate := range unit.Directives() {
		if candidate.Kind != source.DirectiveGoxSuppression {
			continue
		}
		current := directiveIndex
		directiveIndex++
		parsed, problem := parseDirectiveSyntax(candidate)
		if problem != nil {
			continue
		}
		if parsed.rangeEnd {
			opened, found := openRanges[parsed.ruleID]
			if !found {
				continue
			}
			result = append(
				result,
				ownershipFingerprintForRange(
					opened.directive,
					tokenStarts,
					source.Range{
						Start: opened.parsed.range_.End,
						End: parsed.range_.Start,
					},
				),
			)
			delete(openRanges, parsed.ruleID)
			continue
		}
		if parsed.scope == ScopeRange {
			if _, found := openRanges[parsed.ruleID]; found {
				continue
			}
			openRanges[parsed.ruleID] = struct {
				directive int
				parsed parsedDirective
			}{directive: current, parsed: parsed}
			continue
		}

		var target source.Range
		switch parsed.scope {
		case ScopeLine:
			target = lineRange(input, parsed.range_.Start)
		case ScopeNextLine:
			target = nextLineRange(input, parsed.range_.End)
		case ScopeFile:
			if parsed.range_.Start >= packageOffset {
				continue
			}
			target = source.Range{Start: 0, End: len(input)}
		default:
			continue
		}
		result = append(result, ownershipFingerprintForRange(current, tokenStarts, target))
	}
	slices.SortFunc(
		result,
		func(left, right ownershipFingerprint) int {
			return cmp.Compare(left.directive, right.directive)
		},
	)
	return result
}

func normalizedTokenStarts(tokens []source.Token) []int {
	result := make([]int, 0, len(tokens))
	for _, item := range tokens {
		switch item.Kind {
		case token.COMMENT, token.SEMICOLON, token.COMMA:
			continue
		}
		result = append(result, item.Range.Start)
	}
	return result
}

func ownershipFingerprintForRange(
	directive int,
	tokenStarts []int,
	target source.Range,
) ownershipFingerprint {
	start := sort.SearchInts(tokenStarts, target.Start)
	end := sort.SearchInts(tokenStarts, target.End)
	return ownershipFingerprint{directive: directive, start: start, end: end}
}

func parseExpiryMetadata(reason string, sourceRange source.Range) (string, string, *Problem) {
	separator := strings.IndexFunc(reason, unicode.IsSpace)
	field := reason
	remainder := ""
	if separator >= 0 {
		field = reason[:separator]
		remainder = strings.TrimSpace(reason[separator:])
	}
	if !strings.HasPrefix(field, "expires=") {
		return reason, "", nil
	}
	expiresOn := strings.TrimPrefix(field, "expires=")
	if !validDate(expiresOn) {
		return "", "", &Problem{
			Kind: ProblemInvalidExpiry,
			Range: sourceRange,
			Message: "suppression expiry must be a valid YYYY-MM-DD date",
		}
	}
	return remainder, expiresOn, nil
}

func validDate(value string) bool {
	parsed, err := time.Parse(time.DateOnly, value)
	return err == nil && parsed.Format(time.DateOnly) == value
}

func splitReason(rest string) (ruleID, reason string, found bool) {
	separator := strings.Index(rest, "--")
	if separator < 0 {
		return strings.TrimSpace(rest), "", false
	}
	return strings.TrimSpace(rest[:separator]), strings.TrimSpace(rest[separator + 2:]), true
}

func malformed(sourceRange source.Range, message string) *Problem {
	return &Problem{Kind: ProblemMalformed, Range: sourceRange, Message: message}
}

func lineRange(input []byte, offset int) source.Range {
	if offset < 0 {
		offset = 0
	}
	if offset > len(input) {
		offset = len(input)
	}
	start := bytes.LastIndexByte(input[:offset], '\n') + 1
	relativeEnd := bytes.IndexByte(input[offset:], '\n')
	end := len(input)
	if relativeEnd >= 0 {
		end = offset + relativeEnd
	}
	return source.Range{Start: start, End: end}
}

func nextLineRange(input []byte, offset int) source.Range {
	current := lineRange(input, offset)
	if current.End >= len(input) {
		return source.Range{Start: len(input), End: len(input)}
	}
	start := current.End + 1
	return lineRange(input, start)
}
