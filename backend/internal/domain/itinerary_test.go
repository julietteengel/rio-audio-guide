package domain

import (
	"testing"
	"time"
)

func TestNewPlaceStop(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		stop, err := NewPlaceStop("place-1", "Escadaria Selarón", 10, 8)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stop.Kind() != ItineraryStopKindPlace {
			t.Fatalf("got kind %q, want place", stop.Kind())
		}
		if stop.PlaceID() != "place-1" || stop.Label() != "Escadaria Selarón" {
			t.Fatalf("got placeID=%q label=%q", stop.PlaceID(), stop.Label())
		}
		if stop.TimeOnSiteMinutes() != 10 || stop.WalkToNextMinutes() != 8 {
			t.Fatalf("got timeOnSite=%d walkToNext=%d", stop.TimeOnSiteMinutes(), stop.WalkToNextMinutes())
		}
		if stop.ID() == "" {
			t.Fatal("expected a generated ID")
		}
	})
	t.Run("empty place id", func(t *testing.T) {
		if _, err := NewPlaceStop("", "label", 10, 8); err != ErrItineraryStopPlaceIDRequired {
			t.Fatalf("got %v, want ErrItineraryStopPlaceIDRequired", err)
		}
	})
	t.Run("empty label", func(t *testing.T) {
		if _, err := NewPlaceStop("place-1", "", 10, 8); err != ErrItineraryStopLabelRequired {
			t.Fatalf("got %v, want ErrItineraryStopLabelRequired", err)
		}
	})
	t.Run("non-positive time on site", func(t *testing.T) {
		if _, err := NewPlaceStop("place-1", "label", 0, 8); err != ErrItineraryStopInvalidDuration {
			t.Fatalf("got %v, want ErrItineraryStopInvalidDuration", err)
		}
	})
	t.Run("negative walk to next", func(t *testing.T) {
		if _, err := NewPlaceStop("place-1", "label", 10, -1); err != ErrItineraryStopInvalidWalk {
			t.Fatalf("got %v, want ErrItineraryStopInvalidWalk", err)
		}
	})
}

func TestNewSuggestionStop(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		stop, err := NewSuggestionStop("Pause déjeuner", 12)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stop.Kind() != ItineraryStopKindSuggestion {
			t.Fatalf("got kind %q, want suggestion", stop.Kind())
		}
		if stop.PlaceID() != "" {
			t.Fatalf("suggestion stop must not carry a place id, got %q", stop.PlaceID())
		}
		if stop.TimeOnSiteMinutes() != 0 {
			t.Fatalf("suggestion stop has no time-on-site, got %d", stop.TimeOnSiteMinutes())
		}
	})
	t.Run("empty label", func(t *testing.T) {
		if _, err := NewSuggestionStop("", 12); err != ErrItineraryStopLabelRequired {
			t.Fatalf("got %v, want ErrItineraryStopLabelRequired", err)
		}
	})
	t.Run("negative walk to next", func(t *testing.T) {
		if _, err := NewSuggestionStop("label", -1); err != ErrItineraryStopInvalidWalk {
			t.Fatalf("got %v, want ErrItineraryStopInvalidWalk", err)
		}
	})
}

func TestNewItinerary(t *testing.T) {
	title, _ := NewItineraryTitle("Art et rue à Santa Teresa")
	stop, _ := NewPlaceStop("place-1", "Escadaria Selarón", 10, 8)

	t.Run("valid", func(t *testing.T) {
		it, err := NewItinerary("user-1", title, []ItineraryStop{stop})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if it.ID() == "" {
			t.Fatal("expected a generated ID")
		}
		if it.UserID() != "user-1" || it.Title() != title {
			t.Fatalf("got userID=%q title=%q", it.UserID(), it.Title())
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		if _, err := NewItinerary("", title, []ItineraryStop{stop}); err != ErrItineraryUserIDRequired {
			t.Fatalf("got %v, want ErrItineraryUserIDRequired", err)
		}
	})
	t.Run("no stops", func(t *testing.T) {
		if _, err := NewItinerary("user-1", title, nil); err != ErrItineraryNoStops {
			t.Fatalf("got %v, want ErrItineraryNoStops", err)
		}
	})
	t.Run("more than one suggestion stop", func(t *testing.T) {
		lunch1, _ := NewSuggestionStop("Pause déjeuner", 5)
		lunch2, _ := NewSuggestionStop("Café", 5)
		if _, err := NewItinerary("user-1", title, []ItineraryStop{stop, lunch1, lunch2}); err != ErrItineraryTooManySuggestions {
			t.Fatalf("got %v, want ErrItineraryTooManySuggestions", err)
		}
	})
	t.Run("no real place stops", func(t *testing.T) {
		lunch, _ := NewSuggestionStop("Pause déjeuner", 5)
		if _, err := NewItinerary("user-1", title, []ItineraryStop{lunch}); err != ErrItineraryNoPlaceStops {
			t.Fatalf("got %v, want ErrItineraryNoPlaceStops", err)
		}
	})
}

func TestItinerary_TotalMinutesAndPlaceCount(t *testing.T) {
	title, _ := NewItineraryTitle("Art et rue à Santa Teresa")
	stop1, _ := NewPlaceStop("place-1", "Escadaria Selarón", 10, 8)
	lunch, _ := NewSuggestionStop("Pause déjeuner", 5)
	stop2, _ := NewPlaceStop("place-2", "Parque das Ruínas", 20, 0)

	it, err := NewItinerary("user-1", title, []ItineraryStop{stop1, lunch, stop2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 10+8 (stop1) + 0+5 (lunch, no time-on-site) + 20+0 (stop2) = 43
	if got := it.TotalMinutes(); got != 43 {
		t.Fatalf("got TotalMinutes()=%d, want 43", got)
	}
	// Only stop1/stop2 are real places -- the meal-break suggestion doesn't count.
	if got := it.PlaceCount(); got != 2 {
		t.Fatalf("got PlaceCount()=%d, want 2", got)
	}
}

func TestItinerary_MarkFeatured(t *testing.T) {
	title, _ := NewItineraryTitle("Roteiro do Rio Colonial")
	stop, _ := NewPlaceStop("place-1", "Paço Imperial", 20, 5)
	it, err := NewItinerary("admin-1", title, []ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	if it.IsFeatured() {
		t.Fatal("a freshly created itinerary must not start featured")
	}

	it.MarkFeatured()
	if !it.IsFeatured() {
		t.Fatal("expected IsFeatured() true after MarkFeatured()")
	}

	// Idempotent: calling it twice must not error or toggle back off.
	it.MarkFeatured()
	if !it.IsFeatured() {
		t.Fatal("expected IsFeatured() to remain true after a second MarkFeatured() call")
	}
}

func TestReconstructItinerary_PreservesIsFeatured(t *testing.T) {
	title, _ := NewItineraryTitle("Roteiro do Rio Colonial")
	stop := ReconstructPlaceStop("stop-1", "place-1", "Paço Imperial", 20, 5)

	featured := ReconstructItinerary("it-1", "admin-1", title, []ItineraryStop{stop}, time.Now(), true)
	if !featured.IsFeatured() {
		t.Fatal("expected IsFeatured() true when reconstructed with isFeatured=true")
	}

	notFeatured := ReconstructItinerary("it-2", "user-1", title, []ItineraryStop{stop}, time.Now(), false)
	if notFeatured.IsFeatured() {
		t.Fatal("expected IsFeatured() false when reconstructed with isFeatured=false")
	}
}
