package textsafe

import "testing"

func TestValidateMarkdownSubset(t *testing.T) {
	for _, valid := range []string{
		"## Biography\n\n**Bold** and [profile](https://example.test/user).",
		"> quote\n\n- item\n- item",
	} {
		if err := ValidateMarkdown(valid); err != nil {
			t.Fatalf("valid Markdown rejected: %v", err)
		}
	}
	for _, invalid := range []string{
		"# H1", "<script>alert(1)</script>", "![image](https://example.test/a.jpg)",
		"[bad](javascript:alert(1))", "[relative](/path)",
	} {
		if err := ValidateMarkdown(invalid); err == nil {
			t.Fatalf("invalid Markdown accepted: %q", invalid)
		}
	}
}
