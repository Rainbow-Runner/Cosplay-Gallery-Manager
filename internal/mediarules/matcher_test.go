package mediarules

import "testing"

func TestMatcherSubjectsAndOperators(t *testing.T) {
	tests := []struct {
		name, relative, matched string
		spec                    Spec
	}{
		{"folder unicode exact", "Event/自拍/IMG_001.JPG", "自拍", Spec{Subject: SubjectParentFolder, Operator: OperatorExact, Pattern: "selfie\n自拍"}},
		{"parent path subtree exact", "bonus/screenshots/raw/a.jpg", "bonus/screenshots", Spec{Subject: SubjectParentPath, Operator: OperatorExact, Pattern: "bonus/screenshots"}},
		{"filename glob", "set/IMG_selfie.JPG", "IMG_selfie.JPG", Spec{Subject: SubjectFileName, Operator: OperatorGlob, Pattern: "*_selfie.*"}},
		{"stem regex", "set/Selfie-07.JPG", "Selfie-07", Spec{Subject: SubjectFileStem, Operator: OperatorRE2, Pattern: `^selfie-[0-9]+$`}},
		{"relative regex", "day/selfie/a.jpg", "day/selfie/a.jpg", Spec{Subject: SubjectRelativePath, Operator: OperatorRE2, Pattern: `^day/.+\.jpg$`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher, err := Compile(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			matched, value := matcher.Match(test.relative)
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
		err := Validate(Spec{Subject: SubjectFileName, Operator: test.operator, Pattern: test.pattern})
		code, _ := ValidationDetails(err)
		if code != test.code {
			t.Fatalf("Validate(%q) code=%q, want %q (err=%v)", test.pattern, code, test.code, err)
		}
	}
}
