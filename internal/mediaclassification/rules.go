// Package mediaclassification contains the database-independent media rule
// validator and matcher. It deliberately has no filesystem access: callers
// provide normalized, gallery-relative paths from trusted scan/database data.
package mediaclassification

import (
	"fmt"
	"strings"

	"github.com/stashapp/stash/internal/mediarules"
)

const (
	MaxNameBytes    = 300
	MaxPatternBytes = mediarules.MaxPatternBytes
	MaxPatterns     = mediarules.MaxPatterns
)

type Subject = mediarules.Subject

const (
	SubjectParentFolder = mediarules.SubjectParentFolder
	SubjectFileName     = mediarules.SubjectFileName
	SubjectFileStem     = mediarules.SubjectFileStem
	SubjectRelativePath = mediarules.SubjectRelativePath
)

type Operator = mediarules.Operator

const (
	OperatorExact = mediarules.OperatorExact
	OperatorGlob  = mediarules.OperatorGlob
	OperatorRE2   = mediarules.OperatorRE2
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

type ValidationError = mediarules.ValidationError

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
	return mediarules.Validate(mediarules.Spec{Subject: rule.Subject, Operator: rule.Operator, Pattern: rule.Pattern, CaseSensitive: rule.CaseSensitive})
}

type CompiledRule struct {
	rule    Rule
	matcher *mediarules.Matcher
}

func Compile(rule Rule) (*CompiledRule, error) {
	if err := Validate(rule); err != nil {
		return nil, err
	}
	matcher, err := mediarules.Compile(mediarules.Spec{Subject: rule.Subject, Operator: rule.Operator, Pattern: rule.Pattern, CaseSensitive: rule.CaseSensitive})
	if err != nil {
		return nil, err
	}
	return &CompiledRule{rule: rule, matcher: matcher}, nil
}

func (r *CompiledRule) Rule() Rule { return r.rule }

// Match returns the actual subject value that matched. Parent-folder rules
// evaluate each folder segment from the file's immediate parent outward.
func (r *CompiledRule) Match(relativePath string) (bool, string) {
	return r.matcher.Match(relativePath)
}

func invalid(code, message string) error {
	return &mediarules.ValidationError{Code: code, Message: message}
}

func ValidationDetails(err error) (string, string) {
	return mediarules.ValidationDetails(err)
}
