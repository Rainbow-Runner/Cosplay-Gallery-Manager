// Package mediaexclusion defines automatic GalleryItem exclusion policy on
// top of the shared, filesystem-independent media path matcher.
package mediaexclusion

import (
	"fmt"
	"strings"

	"github.com/stashapp/stash/internal/mediarules"
)

const MaxNameBytes = 300

type Subject = mediarules.Subject

const (
	SubjectParentFolder = mediarules.SubjectParentFolder
	SubjectParentPath   = mediarules.SubjectParentPath
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

type MediaKind string

const (
	MediaKindAll      MediaKind = "ALL"
	MediaKindStatic   MediaKind = "STATIC_IMAGE"
	MediaKindAnimated MediaKind = "ANIMATED_IMAGE"
	MediaKindVideo    MediaKind = "VIDEO"
)

type Decision string

const (
	DecisionExclude Decision = "EXCLUDE"
	DecisionInclude Decision = "INCLUDE"
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
	MediaKind     MediaKind
	Decision      Decision
	Revision      int
	SystemDefault bool
}

type ValidationError = mediarules.ValidationError

func Validate(rule Rule) error {
	if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > MaxNameBytes {
		return invalid("RULE_NAME_INVALID", fmt.Sprintf("Rule name must contain 1-%d bytes", MaxNameBytes))
	}
	switch rule.MediaKind {
	case MediaKindAll, MediaKindStatic, MediaKindAnimated, MediaKindVideo:
	default:
		return invalid("RULE_MEDIA_KIND_INVALID", "Media kind must be ALL, STATIC_IMAGE, ANIMATED_IMAGE or VIDEO")
	}
	switch rule.Decision {
	case DecisionExclude, DecisionInclude:
	default:
		return invalid("RULE_DECISION_INVALID", "Decision must be EXCLUDE or INCLUDE")
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

func (r *CompiledRule) Match(relativePath string, mediaKind string) (bool, string) {
	if r.rule.MediaKind != MediaKindAll && string(r.rule.MediaKind) != mediaKind {
		return false, ""
	}
	return r.matcher.Match(relativePath)
}

func ValidationDetails(err error) (string, string) { return mediarules.ValidationDetails(err) }

func invalid(code, message string) error {
	return &mediarules.ValidationError{Code: code, Message: message}
}
