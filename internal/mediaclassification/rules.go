// Package mediaclassification contains the database-independent media rule
// validator and matcher. It deliberately has no filesystem access: callers
// provide normalized, gallery-relative paths from trusted scan/database data.
package mediaclassification

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
	MaxNameBytes    = 300
	MaxPatternBytes = 4000
	MaxPatterns     = 100
)

type Subject string

const (
	SubjectParentFolder Subject = "PARENT_FOLDER"
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

type Category string

const (
	CategoryPhoto  Category = "PHOTO"
	CategorySelfie Category = "SELFIE"
)

type Rule struct {
	ID            int64
	LibraryID     *int64
	Name          string
	Enabled       bool
	Order         int
	Subject       Subject
	Operator      Operator
	Pattern       string
	CaseSensitive bool
	Category      Category
	Revision      int
	SystemDefault bool
}

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func Validate(rule Rule) error {
	if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > MaxNameBytes {
		return invalid("RULE_NAME_INVALID", fmt.Sprintf("Rule name must contain 1-%d bytes", MaxNameBytes))
	}
	switch rule.Subject {
	case SubjectParentFolder, SubjectFileName, SubjectFileStem, SubjectRelativePath:
	default:
		return invalid("RULE_SUBJECT_INVALID", "Match subject is not supported")
	}
	switch rule.Category {
	case CategoryPhoto, CategorySelfie:
	default:
		return invalid("RULE_CATEGORY_INVALID", "Result category must be PHOTO or SELFIE")
	}
	pattern := norm.NFC.String(strings.TrimSpace(rule.Pattern))
	if pattern == "" || len(pattern) > MaxPatternBytes {
		return invalid("RULE_PATTERN_INVALID", fmt.Sprintf("Pattern must contain 1-%d bytes", MaxPatternBytes))
	}
	switch rule.Operator {
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
		_, err := regexp.Compile(regexPattern(pattern, rule.CaseSensitive))
		if err != nil {
			return invalid("RULE_RE2_INVALID", fmt.Sprintf("Invalid RE2 expression: %v", err))
		}
		return nil
	default:
		return invalid("RULE_OPERATOR_INVALID", "Match operator is not supported")
	}
}

type CompiledRule struct {
	rule     Rule
	patterns []string
	re       *regexp.Regexp
}

func Compile(rule Rule) (*CompiledRule, error) {
	if err := Validate(rule); err != nil {
		return nil, err
	}
	compiled := &CompiledRule{rule: rule}
	if rule.Operator == OperatorRE2 {
		compiled.re = regexp.MustCompile(regexPattern(norm.NFC.String(strings.TrimSpace(rule.Pattern)), rule.CaseSensitive))
	} else {
		compiled.patterns, _ = patternLines(norm.NFC.String(strings.TrimSpace(rule.Pattern)))
		if !rule.CaseSensitive {
			for i := range compiled.patterns {
				compiled.patterns[i] = fold(compiled.patterns[i])
			}
		}
	}
	return compiled, nil
}

func (r *CompiledRule) Rule() Rule { return r.rule }

// Match returns the actual subject value that matched. Parent-folder rules
// evaluate each folder segment from the file's immediate parent outward.
func (r *CompiledRule) Match(relativePath string) (bool, string) {
	for _, candidate := range subjectValues(r.rule.Subject, relativePath) {
		value := candidate
		if !r.rule.CaseSensitive && r.rule.Operator != OperatorRE2 {
			value = fold(value)
		}
		switch r.rule.Operator {
		case OperatorExact:
			for _, pattern := range r.patterns {
				if value == pattern {
					return true, candidate
				}
			}
		case OperatorGlob:
			for _, pattern := range r.patterns {
				if ok, _ := path.Match(pattern, value); ok {
					return true, candidate
				}
			}
		case OperatorRE2:
			if r.re.MatchString(candidate) {
				return true, candidate
			}
		}
	}
	return false, ""
}

func subjectValues(subject Subject, relativePath string) []string {
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
		directory := path.Dir(relative)
		if directory == "." || directory == "/" {
			return nil
		}
		parts := strings.Split(strings.Trim(directory, "/"), "/")
		values := make([]string, 0, len(parts))
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" && parts[i] != "." && parts[i] != ".." {
				values = append(values, parts[i])
			}
		}
		return values
	default:
		return nil
	}
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
