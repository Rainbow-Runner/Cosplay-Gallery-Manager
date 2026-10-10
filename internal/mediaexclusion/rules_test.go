package mediaexclusion

import "testing"

func TestCompiledRuleFiltersMediaKindAndMatchesParentSubtree(t *testing.T) {
	rule := Rule{Name: "exclude screenshots", Enabled: true, Subject: SubjectParentPath, Operator: OperatorExact,
		Pattern: "bonus/screenshots", MediaKind: MediaKindStatic, Decision: DecisionExclude, Revision: 1}
	compiled, err := Compile(rule)
	if err != nil {
		t.Fatal(err)
	}
	if matched, value := compiled.Match("bonus/screenshots/raw/a.jpg", "STATIC_IMAGE"); !matched || value != "bonus/screenshots" {
		t.Fatalf("static match=(%v,%q)", matched, value)
	}
	if matched, _ := compiled.Match("bonus/screenshots/a.mp4", "VIDEO"); matched {
		t.Fatal("STATIC_IMAGE rule matched VIDEO")
	}
}

func TestValidateDecisionAndMediaKind(t *testing.T) {
	base := Rule{Name: "rule", Subject: SubjectFileName, Operator: OperatorExact, Pattern: "cover.jpg", MediaKind: MediaKindAll, Decision: DecisionExclude}
	for _, test := range []struct {
		name string
		rule Rule
		code string
	}{
		{"media", func() Rule { value := base; value.MediaKind = "AUDIO"; return value }(), "RULE_MEDIA_KIND_INVALID"},
		{"decision", func() Rule { value := base; value.Decision = "DELETE"; return value }(), "RULE_DECISION_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			code, _ := ValidationDetails(Validate(test.rule))
			if code != test.code {
				t.Fatalf("code=%q want=%q", code, test.code)
			}
		})
	}
}
