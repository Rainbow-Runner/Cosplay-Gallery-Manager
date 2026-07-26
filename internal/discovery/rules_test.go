package discovery

import "testing"

func TestPathTemplateProducesOnlyNamedSuggestions(t *testing.T) {
	rules := []Rule{{
		ID: 1, Kind: RuleKindPathTemplate, Enabled: true, Order: 10,
		Pattern: `(?P<work>[^/]+)/(?P<character>[^/]+)/(?P<coser>[^/]+)`,
	}}
	match, err := MatchDirectory("作品/角色/Coser", false, rules)
	if err != nil {
		t.Fatal(err)
	}
	if match == nil || match.Root != "作品/角色/Coser" || len(match.Suggestions) != 3 {
		t.Fatalf("match = %#v", match)
	}
}

func TestPathTemplateHasPriorityOverFixedDepth(t *testing.T) {
	rules := []Rule{
		{ID: 2, Kind: RuleKindFixedDepth, Enabled: true, FixedDepth: 1, Order: 1},
		{ID: 1, Kind: RuleKindPathTemplate, Enabled: true, Pattern: `(?P<title>.+/.+)`, Order: 99},
	}
	match, err := MatchDirectory("series/set", false, rules)
	if err != nil {
		t.Fatal(err)
	}
	if match == nil || match.RuleID != 1 || match.Root != "series/set" {
		t.Fatalf("match = %#v", match)
	}
}

func TestDirectChildIsFixedDepthOneWithoutSuggestions(t *testing.T) {
	match, err := MatchDirectory("set/nested/media", false, []Rule{{
		ID: 3, Kind: RuleKindFixedDepth, Enabled: true, FixedDepth: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if match == nil || match.Root != "set" || len(match.Suggestions) != 0 {
		t.Fatalf("match = %#v", match)
	}
}

func TestMarkerHasHighestAutomaticPriority(t *testing.T) {
	rules := []Rule{
		{ID: 2, Kind: RuleKindPathTemplate, Enabled: true, Pattern: `(?P<title>.+)`},
		{ID: 1, Kind: RuleKindMarker, Enabled: true},
	}
	match, err := MatchDirectory("marked", true, rules)
	if err != nil {
		t.Fatal(err)
	}
	if match == nil || match.Kind != RuleKindMarker {
		t.Fatalf("match = %#v", match)
	}
}

func TestAllAutomaticRulesCanRemainDisabled(t *testing.T) {
	match, err := MatchDirectory("unowned/set", true, []Rule{
		{ID: 1, Kind: RuleKindMarker},
		{ID: 2, Kind: RuleKindFixedDepth, FixedDepth: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if match != nil {
		t.Fatalf("disabled rules produced match %#v", match)
	}
}

func TestRuleValidationRejectsUnknownCapture(t *testing.T) {
	err := ValidateRule(Rule{Kind: RuleKindPathTemplate, Pattern: `(?P<studio>.+)`})
	if err == nil {
		t.Fatal("unsupported studio capture unexpectedly accepted")
	}
}

func TestRelativeDirectoryNormalization(t *testing.T) {
	got, err := NormalizeRelativeDirectory("set\\子目录")
	if err != nil {
		t.Fatal(err)
	}
	if got != "set/子目录" {
		t.Fatalf("normalized = %q", got)
	}
	if _, err := NormalizeRelativeDirectory("../outside"); err == nil {
		t.Fatal("path traversal unexpectedly accepted")
	}
}
