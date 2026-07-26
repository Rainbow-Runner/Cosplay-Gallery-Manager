// Package discovery implements deterministic Gallery-root recognition. It has
// no heuristic scoring or evidence-provider extension point.
package discovery

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type RuleKind string

const (
	RuleKindMarker       RuleKind = "MARKER"
	RuleKindPathTemplate RuleKind = "PATH_TEMPLATE"
	RuleKindFixedDepth   RuleKind = "FIXED_DEPTH"
)

type Rule struct {
	ID              int64
	Name            string
	Kind            RuleKind
	Enabled         bool
	AutoCreateDraft bool
	Order           int
	Pattern         string
	FixedDepth      int
}

type Suggestion struct {
	Field string
	Value string
}

type Match struct {
	Root        string
	RuleID      int64
	Kind        RuleKind
	AutoCreate  bool
	Suggestions []Suggestion
}

var allowedCaptures = map[string]struct{}{
	"title":     {},
	"coser":     {},
	"work":      {},
	"character": {},
	"year":      {},
	"month":     {},
}

func NormalizeRelativeDirectory(value string) (string, error) {
	value = norm.NFC.String(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Trim(value, "/")
	if value == "" || value == "." || strings.HasPrefix(value, "../") || value == ".." {
		return "", errors.New("relative directory must remain within its media library")
	}
	cleaned := path.Clean(value)
	if cleaned != value || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("relative directory %q is not canonical", value)
	}
	return cleaned, nil
}

func ValidateRule(rule Rule) error {
	switch rule.Kind {
	case RuleKindMarker:
		if rule.Pattern != "" || rule.FixedDepth != 0 {
			return errors.New("MARKER rule cannot contain pattern or depth")
		}
	case RuleKindFixedDepth:
		if rule.FixedDepth < 1 || rule.FixedDepth > 64 {
			return errors.New("FIXED_DEPTH must be between 1 and 64")
		}
		if rule.Pattern != "" {
			return errors.New("FIXED_DEPTH rule cannot contain a pattern")
		}
	case RuleKindPathTemplate:
		if rule.Pattern == "" || len([]rune(rule.Pattern)) > 2000 {
			return errors.New("PATH_TEMPLATE pattern must contain 1 to 2000 characters")
		}
		compiled, err := regexp.Compile("^(?:" + rule.Pattern + ")$")
		if err != nil {
			return fmt.Errorf("invalid RE2 path template: %w", err)
		}
		named := 0
		for _, name := range compiled.SubexpNames() {
			if name == "" {
				continue
			}
			if _, allowed := allowedCaptures[name]; !allowed {
				return fmt.Errorf("unsupported PATH_TEMPLATE capture %q", name)
			}
			named++
		}
		if named == 0 {
			return errors.New("PATH_TEMPLATE requires at least one supported named capture")
		}
	default:
		return fmt.Errorf("unsupported discovery rule kind %q", rule.Kind)
	}
	return nil
}

// MatchDirectory applies enabled automatic rules in the confirmed priority:
// MARKER, ordered PATH_TEMPLATE, then ordered FIXED_DEPTH. Manifest and bound
// source priorities are handled by the persistence discovery coordinator.
func MatchDirectory(relativeDirectory string, hasMarker bool, rules []Rule) (*Match, error) {
	normalized, err := NormalizeRelativeDirectory(relativeDirectory)
	if err != nil {
		return nil, err
	}

	enabled := append([]Rule(nil), rules...)
	sort.SliceStable(enabled, func(i int, j int) bool {
		if rulePriority(enabled[i].Kind) != rulePriority(enabled[j].Kind) {
			return rulePriority(enabled[i].Kind) < rulePriority(enabled[j].Kind)
		}
		return enabled[i].Order < enabled[j].Order
	})

	for _, rule := range enabled {
		if !rule.Enabled {
			continue
		}
		if err := ValidateRule(rule); err != nil {
			return nil, fmt.Errorf("rule %d: %w", rule.ID, err)
		}
		switch rule.Kind {
		case RuleKindMarker:
			if hasMarker {
				return &Match{Root: normalized, RuleID: rule.ID, Kind: rule.Kind, AutoCreate: rule.AutoCreateDraft}, nil
			}
		case RuleKindPathTemplate:
			compiled := regexp.MustCompile("^(?:" + rule.Pattern + ")$")
			values := compiled.FindStringSubmatch(normalized)
			if values == nil {
				continue
			}
			result := Match{Root: normalized, RuleID: rule.ID, Kind: rule.Kind, AutoCreate: rule.AutoCreateDraft}
			for index, name := range compiled.SubexpNames() {
				if index == 0 || name == "" || values[index] == "" {
					continue
				}
				result.Suggestions = append(result.Suggestions, Suggestion{
					Field: name,
					Value: norm.NFC.String(strings.TrimSpace(values[index])),
				})
			}
			return &result, nil
		case RuleKindFixedDepth:
			segments := strings.Split(normalized, "/")
			if len(segments) < rule.FixedDepth {
				continue
			}
			return &Match{
				Root:       strings.Join(segments[:rule.FixedDepth], "/"),
				RuleID:     rule.ID,
				Kind:       rule.Kind,
				AutoCreate: rule.AutoCreateDraft,
			}, nil
		}
	}

	return nil, nil
}

func rulePriority(kind RuleKind) int {
	switch kind {
	case RuleKindMarker:
		return 0
	case RuleKindPathTemplate:
		return 1
	case RuleKindFixedDepth:
		return 2
	default:
		return 3
	}
}
