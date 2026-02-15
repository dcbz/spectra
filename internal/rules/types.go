package rules

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Severity represents the importance level a rule assigns to a match.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityNormal   Severity = "normal"
)

var orderedSeverities = []Severity{
	SeverityCritical,
	SeverityHigh,
	SeverityMedium,
	SeverityLow,
	SeverityNormal,
}

// Rule captures a compiled regular expression with metadata for styling.
type Rule struct {
	Name        string
	Pattern     string
	regex       *regexp.Regexp
	Severity    Severity
	Color       string
	Tags        []string
	Description string
	order       int
}

// Match contains the context returned when a rule triggers.
type Match struct {
	Rule           Rule
	Captures       map[string]string
	HighlightSpans [][2]int
}

// RuleSet provides matching behavior for a set of compiled rules.
type RuleSet struct {
	Rules []Rule
}

// Compile validates all rules and prepares regexes.
func Compile(defs []RuleDefinition) (RuleSet, error) {
	compiled := make([]Rule, 0, len(defs))
	for _, def := range defs {
		if def.Pattern == "" {
			return RuleSet{}, fmt.Errorf("rule %q missing pattern", def.Name)
		}
		re, err := regexp.Compile(def.Pattern)
		if err != nil {
			return RuleSet{}, fmt.Errorf("compile %q: %w", def.Name, err)
		}
		severity := normalizeSeverity(def.Severity)
		compiled = append(compiled, Rule{
			Name:        def.Name,
			Pattern:     def.Pattern,
			regex:       re,
			Severity:    severity,
			Color:       def.Color,
			Tags:        append([]string{}, def.Tags...),
			Description: def.Description,
			order:       len(compiled),
		})
	}
	return RuleSet{Rules: compiled}, nil
}

// Match evaluates the line against the rule set returning the first match ordered by severity then declaration order.
func (rs RuleSet) Match(line string) (Match, bool) {
	if len(rs.Rules) == 0 {
		return Match{}, false
	}

	for _, rule := range rs.sortedRules() {
		locs := rule.regex.FindAllStringIndex(line, -1)
		if len(locs) == 0 {
			continue
		}
		captures := captureMap(rule.regex, line)
		return Match{Rule: rule, Captures: captures, HighlightSpans: toPairs(locs)}, true
	}

	return Match{}, false
}

// FilterByTags returns a new ruleset containing only rules that match any tag in the provided selection.
func (rs RuleSet) FilterByTags(tags []string) RuleSet {
	if len(tags) == 0 {
		return rs
	}
	selected := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		selected[strings.ToLower(tag)] = struct{}{}
	}
	if len(selected) == 0 {
		return rs
	}
	filtered := make([]Rule, 0, len(rs.Rules))
	for _, rule := range rs.Rules {
		for _, tag := range rule.Tags {
			if _, ok := selected[strings.ToLower(tag)]; ok {
				filtered = append(filtered, rule)
				break
			}
		}
	}
	return RuleSet{Rules: filtered}
}

func (rs RuleSet) sortedRules() []Rule {
	copyRules := make([]Rule, len(rs.Rules))
	copy(copyRules, rs.Rules)
	sort.SliceStable(copyRules, func(i, j int) bool {
		iScore := severityScore(copyRules[i].Severity)
		jScore := severityScore(copyRules[j].Severity)
		if iScore == jScore {
			return copyRules[i].order < copyRules[j].order
		}
		return iScore < jScore
	})
	return copyRules
}

// SeverityRank exposes the ordering used for comparisons (lower is more urgent).
func SeverityRank(s Severity) int {
	for idx, val := range orderedSeverities {
		if val == s {
			return idx
		}
	}
	return len(orderedSeverities)
}

func severityScore(s Severity) int {
	return SeverityRank(s)
}

func normalizeSeverity(s Severity) Severity {
	switch strings.ToLower(string(s)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium", "", "med":
		return SeverityMedium
	case "low":
		return SeverityLow
	case "normal":
		return SeverityNormal
	default:
		return SeverityMedium
	}
}

// ParseSeverity converts user input into a Severity constant.
func ParseSeverity(value string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return SeverityCritical, nil
	case "high":
		return SeverityHigh, nil
	case "medium", "med":
		return SeverityMedium, nil
	case "low":
		return SeverityLow, nil
	case "normal":
		return SeverityNormal, nil
	default:
		return "", fmt.Errorf("unknown severity %q", value)
	}
}

// MeetsThreshold reports whether value is at least as urgent as min.
func MeetsThreshold(value, min Severity) bool {
	return SeverityRank(value) <= SeverityRank(min)
}

func captureMap(re *regexp.Regexp, line string) map[string]string {
	names := re.SubexpNames()
	matches := re.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil
	}
	captures := make(map[string]string)
	for i, name := range names {
		if i == 0 || name == "" {
			continue
		}
		captures[name] = matches[i]
	}
	return captures
}

func toPairs(spans [][]int) [][2]int {
	out := make([][2]int, 0, len(spans))
	for _, span := range spans {
		if len(span) != 2 {
			continue
		}
		out = append(out, [2]int{span[0], span[1]})
	}
	return out
}

// RuleDefinition mirrors the YAML representation for easier parsing.
type RuleDefinition struct {
	Name        string   `yaml:"name"`
	Pattern     string   `yaml:"pattern"`
	Severity    Severity `yaml:"severity"`
	Color       string   `yaml:"color"`
	Tags        []string `yaml:"tags"`
	Description string   `yaml:"description"`
}

type ruleFile struct {
	Rules []RuleDefinition `yaml:"rules"`
}
