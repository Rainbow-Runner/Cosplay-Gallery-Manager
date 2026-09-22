package mediaclassification

import "testing"

func TestCompileAndMatchSubjectsAndOperators(t *testing.T) {
	tests := []struct {
		name, relative, matched string
		rule                    Rule
	}{
		{"folder unicode exact", "Event/自拍/IMG_001.JPG", "自拍", validRule(SubjectParentFolder, OperatorExact, "selfie\n自拍")},
		{"folder case fold", "Event/SELFIE/IMG.JPG", "SELFIE", validRule(SubjectParentFolder, OperatorExact, "selfie")},
		{"filename glob", "set/IMG_selfie.JPG", "IMG_selfie.JPG", validRule(SubjectFileName, OperatorGlob, "*_selfie.*")},
		{"stem regex", "set/Selfie-07.JPG", "Selfie-07", validRule(SubjectFileStem, OperatorRE2, `^selfie-[0-9]+$`)},
		{"relative regex", "day/selfie/a.jpg", "day/selfie/a.jpg", validRule(SubjectRelativePath, OperatorRE2, `^day/.+\.jpg$`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := Compile(test.rule)
			if err != nil {
				t.Fatal(err)
			}
			matched, value := compiled.Match(test.relative)
			if !matched || value != test.matched {
				t.Fatalf("Match()=(%v,%q), want (true,%q)", matched, value, test.matched)
			}
		})
	}
}

func TestValidateRejectsInvalidRE2AndGlob(t *testing.T) {
	for _, test := range []struct {
		operator Operator
		pattern  string
		code     string
	}{
		{OperatorRE2, `([`, "RULE_RE2_INVALID"},
		{OperatorRE2, "selfie\nphoto", "RULE_RE2_MULTILINE"},
		{OperatorGlob, `[abc`, "RULE_GLOB_INVALID"},
	} {
		rule := validRule(SubjectFileName, test.operator, test.pattern)
		err := Validate(rule)
		code, _ := ValidationDetails(err)
		if code != test.code {
			t.Fatalf("Validate(%q) code=%q, want %q (err=%v)", test.pattern, code, test.code, err)
		}
	}
}

func validRule(subject Subject, operator Operator, pattern string) Rule {
	return Rule{Name: "test", Enabled: true, Order: 100, Subject: subject, Operator: operator, Pattern: pattern, Category: CategorySelfie, Revision: 1}
}
