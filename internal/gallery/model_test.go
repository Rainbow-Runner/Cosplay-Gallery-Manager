package gallery

import "testing"

func TestActivationBlockers(t *testing.T) {
	gallery := Gallery{}
	blockers := gallery.ActivationBlockers(ActivationFacts{})
	if len(blockers) != 5 {
		t.Fatalf("blocker count = %d, want 5: %#v", len(blockers), blockers)
	}

	gallery.Title = "Album"
	gallery.ContentRating = ContentRatingNonAdult
	blockers = gallery.ActivationBlockers(ActivationFacts{
		HasSource:            true,
		SourceAvailable:      true,
		DisplayableItemCount: 1,
		CreditCount:          1,
	})
	if len(blockers) != 0 {
		t.Fatalf("valid Album blockers = %#v", blockers)
	}
}

func TestCosplayRequiresCharacterForEveryCredit(t *testing.T) {
	gallery := Gallery{Title: "Cosplay", ContentRating: ContentRatingNonAdult}
	blockers := gallery.ActivationBlockers(ActivationFacts{
		HasSource:                    true,
		SourceAvailable:              true,
		DisplayableItemCount:         1,
		CreditCount:                  2,
		CastCount:                    1,
		CreditsWithoutCharacterCount: 1,
	})
	if len(blockers) != 1 || blockers[0].Code != "CAST_REQUIRED_FOR_EACH_CREDIT" {
		t.Fatalf("blockers = %#v", blockers)
	}
}

func TestDeriveBrowsable(t *testing.T) {
	facts := ActivationFacts{
		HasSource:            true,
		SourceAvailable:      true,
		DisplayableItemCount: 1,
	}
	if !DeriveBrowsable(StateActive, facts) {
		t.Fatal("valid active Gallery should be browsable")
	}
	facts.SourceAvailable = false
	if DeriveBrowsable(StateActive, facts) {
		t.Fatal("unavailable source must pause browsing")
	}
}
