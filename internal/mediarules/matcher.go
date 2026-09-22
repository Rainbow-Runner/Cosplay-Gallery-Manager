// Package mediarules provides a database-independent matcher for normalized,
// Gallery-relative media paths. It deliberately has no filesystem access and
// contains no classification or exclusion business result.
package mediarules

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxPatternBytes = 4000
	MaxPatterns     = 100
)

type Subject string

const (
	SubjectParentFolder Subject = "PARENT_FOLDER"
	SubjectParentPath   Subject = "PARENT_PATH"
	SubjectFileName     Subject = "FILE_NAME"
	SubjectFileStem     Subject = "FILE_STEM"
	SubjectRelativePath Subject = "RELATIVE_PATH"
)

type Operator string

const (
	OperatorExact Operator = "EXACT"
	OperatorGlob  Operator = "GLOB"
	OperatorRE2   Operator = "RE2"
)

type Spec struct {
	Subject       Subject
	Operator      Operator
	Pattern       string
	CaseSensitive bool
}

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func Validate(spec Spec) error {
	switch spec.Subject {
	case SubjectParentFolder, SubjectParentPath, SubjectFileName, SubjectFileStem, SubjectRelativePath:
	default:
		return invalid("RULE_SUBJECT_INVALID", "Match subject is not supported")
	}
	pattern := norm.NFC.String(strings.TrimSpace(spec.Pattern))
	if pattern == "" || len(pattern) > MaxPatternBytes {
		return invalid("RULE_PATTERN_INVALID", fmt.Sprintf("Pattern must contain 1-%d bytes", MaxPatternBytes))
	}
	switch spec.Operator {
	case OperatorExact:
		_, err := patternLines(pattern)
		return err
	case OperatorGlob:
		lines, err := patternLines(pattern)
		if err != nil {
			return err
		}
		for _, line := range lines {
			if _, err := path.Match(line, "validation-value"); err != nil {
				return invalid("RULE_GLOB_INVALID", fmt.Sprintf("Invalid glob pattern %q: %v", line, err))
			}
		}
		return nil
	case OperatorRE2:
		if strings.Contains(pattern, "\n") || strings.Contains(pattern, "\r") {
			return invalid("RULE_RE2_MULTILINE", "RE2 rules accept one expression; use grouping for alternatives")
		}
		if _, err := regexp.Compile(regexPattern(pattern, spec.CaseSensitive)); err != nil {
			return invalid("RULE_RE2_INVALID", fmt.Sprintf("Invalid RE2 expression: %v", err))
		}
		return nil
	default:
		return invalid("RULE_OPERATOR_INVALID", "Match operator is not supported")
	}
}

type Matcher struct {
	spec     Spec
	patterns []string
	re       *regexp.Regexp
}

func Compile(spec Spec) (*Matcher, error) {
	if err := Validate(spec); err != nil {
		return nil, err
	}
	result := &Matcher{spec: spec}
	if spec.Operator == OperatorRE2 {
		result.re = regexp.MustCompile(regexPattern(norm.NFC.String(strings.TrimSpace(spec.Pattern)), spec.CaseSensitive))
	} else {
		result.patterns, _ = patternLines(norm.NFC.String(strings.TrimSpace(spec.Pattern)))
		if !spec.CaseSensitive {
			for index := range result.patterns {
				result.patterns[index] = fold(result.patterns[index])
			}
		}
	}
	return result, nil
}

// Match returns the actual normalized subject value that matched.
func (m *Matcher) Match(relativePath string) (bool, string) {
	for _, candidate := range SubjectValues(m.spec.Subject, relativePath) {
		value := candidate
		if !m.spec.CaseSensitive && m.spec.Operator != OperatorRE2 {
			value = fold(value)
		}
		switch m.spec.Operator {
		case OperatorExact:
			for _, pattern := range m.patterns {
				if value == pattern {
					return true, candidate
				}
			}
		case OperatorGlob:
			for _, pattern := range m.patterns {
				if matched, _ := path.Match(pattern, value); matched {
					return true, candidate
				}
			}
		case OperatorRE2:
			if m.re.MatchString(candidate) {
				return true, candidate
			}
		}
	}
	return false, ""
}

// SubjectValues operates only on a normalized logical path. Parent-folder
// values are individual segments; parent-path values are complete ancestor
// prefixes from the immediate parent outwards so Exact can describe a subtree.
func SubjectValues(subject Subject, relativePath string) []string {
	relative := strings.TrimPrefix(path.Clean(strings.ReplaceAll(norm.NFC.String(relativePath), "\\", "/")), "./")
	file := path.Base(relative)
	switch subject {
	case SubjectFileName:
		return []string{file}
	case SubjectFileStem:
		if extension := path.Ext(file); extension != "" {
			file = strings.TrimSuffix(file, extension)
		}
		return []string{file}
	case SubjectRelativePath:
		return []string{relative}
	case SubjectParentFolder:
		directory := cleanParent(relative)
		if directory == "" {
			return nil
		}
		parts := strings.Split(directory, "/")
		values := make([]string, 0, len(parts))
		for index := len(parts) - 1; index >= 0; index-- {
			if parts[index] != "" && parts[index] != "." && parts[index] != ".." {
				values = append(values, parts[index])
			}
		}
		return values
	case SubjectParentPath:
		directory := cleanParent(relative)
		if directory == "" {
			return nil
		}
		parts := strings.Split(directory, "/")
		values := make([]string, 0, len(parts))
		for end := len(parts); end > 0; end-- {
			values = append(values, strings.Join(parts[:end], "/"))
		}
		return values
	default:
		return nil
	}
}

func cleanParent(relative string) string {
	directory := strings.Trim(path.Dir(relative), "/")
	if directory == "" || directory == "." || directory == ".." {
		return ""
	}
	return directory
}

func patternLines(pattern string) ([]string, error) {
	raw := strings.Split(strings.ReplaceAll(pattern, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value != "" {
			lines = append(lines, value)
		}
	}
	if len(lines) == 0 || len(lines) > MaxPatterns {
		return nil, invalid("RULE_PATTERN_COUNT_INVALID", fmt.Sprintf("Pattern list must contain 1-%d non-empty lines", MaxPatterns))
	}
	return lines, nil
}

func regexPattern(pattern string, caseSensitive bool) string {
	if caseSensitive {
		return pattern
	}
	return "(?i:" + pattern + ")"
}

func fold(value string) string { return cases.Fold().String(norm.NFC.String(value)) }

func invalid(code, message string) error { return &ValidationError{Code: code, Message: message} }

func ValidationDetails(err error) (string, string) {
	var invalidRule *ValidationError
	if errors.As(err, &invalidRule) {
		return invalidRule.Code, invalidRule.Message
	}
	if err == nil {
		return "", ""
	}
	return "RULE_INVALID", err.Error()
}
