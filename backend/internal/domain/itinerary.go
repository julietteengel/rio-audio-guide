package domain

import (
	"errors"
	"time"
)

var (
	ErrItineraryUserIDRequired      = errors.New("itinerary: user id is required")
	ErrItineraryTitleRequired       = errors.New("itinerary: title is required")
	ErrItineraryNoStops             = errors.New("itinerary: at least one stop is required")
	ErrItineraryStopPlaceIDRequired = errors.New("itinerary: a place stop requires a place id")
	ErrItineraryStopLabelRequired   = errors.New("itinerary: a stop requires a label")
	ErrItineraryStopInvalidDuration = errors.New("itinerary: a place stop's time on site must be positive")
	ErrItineraryStopInvalidWalk     = errors.New("itinerary: walking time to the next stop cannot be negative")
)

// --- Value Objects ---

type ItineraryTitle string

func NewItineraryTitle(s string) (ItineraryTitle, error) {
	if s == "" {
		return "", ErrItineraryTitleRequired
	}
	return ItineraryTitle(s), nil
}

func (t ItineraryTitle) String() string { return string(t) }

type ItineraryStopKind string

const (
	ItineraryStopKindPlace      ItineraryStopKind = "place"
	ItineraryStopKindSuggestion ItineraryStopKind = "suggestion"
)

// ItineraryStop is a value object with its own generated ID (not identity in
// the DDD entity sense -- it never changes after construction -- but it
// still needs a stable ID so the Postgres adapter can persist an ordered
// list of stops as rows and read them back as the same values, the same
// reason WordMark-like child records elsewhere in this project carry an ID).
type ItineraryStop struct {
	id                string
	kind              ItineraryStopKind
	placeID           string // set only when kind == ItineraryStopKindPlace
	label             string // place name (kind=place) or suggestion label (kind=suggestion), e.g. "Pause déjeuner"
	timeOnSiteMinutes int    // 0 for a suggestion stop -- it's not a real visitable place with a duration
	walkToNextMinutes int    // 0 for the itinerary's last stop
}

// NewPlaceStop is the only way to add a real, grounded place to an
// itinerary -- placeID must be a real domain.Place ID, enforced by the
// caller (internal/application), not by this constructor, which only knows
// this is a non-empty string.
func NewPlaceStop(placeID, label string, timeOnSiteMinutes, walkToNextMinutes int) (ItineraryStop, error) {
	if placeID == "" {
		return ItineraryStop{}, ErrItineraryStopPlaceIDRequired
	}
	if label == "" {
		return ItineraryStop{}, ErrItineraryStopLabelRequired
	}
	if timeOnSiteMinutes <= 0 {
		return ItineraryStop{}, ErrItineraryStopInvalidDuration
	}
	if walkToNextMinutes < 0 {
		return ItineraryStop{}, ErrItineraryStopInvalidWalk
	}
	return ItineraryStop{
		id: newID(), kind: ItineraryStopKindPlace, placeID: placeID, label: label,
		timeOnSiteMinutes: timeOnSiteMinutes, walkToNextMinutes: walkToNextMinutes,
	}, nil
}

// NewSuggestionStop is the ONLY sanctioned way to add ungrounded content to
// an itinerary (the meal-break-style slot) -- deliberately has no
// timeOnSiteMinutes parameter at all, not just a validated-to-zero one, so
// a caller can never accidentally give an unverified suggestion the same
// "real place with a visit duration" shape as a grounded stop.
func NewSuggestionStop(label string, walkToNextMinutes int) (ItineraryStop, error) {
	if label == "" {
		return ItineraryStop{}, ErrItineraryStopLabelRequired
	}
	if walkToNextMinutes < 0 {
		return ItineraryStop{}, ErrItineraryStopInvalidWalk
	}
	return ItineraryStop{id: newID(), kind: ItineraryStopKindSuggestion, label: label, walkToNextMinutes: walkToNextMinutes}, nil
}

// ReconstructPlaceStop/ReconstructSuggestionStop rebuild a stop from
// already-valid data (a Postgres row) -- preserve the given ID, don't
// revalidate, same convention as ReconstructScript/ReconstructUser.
func ReconstructPlaceStop(id, placeID, label string, timeOnSiteMinutes, walkToNextMinutes int) ItineraryStop {
	return ItineraryStop{id: id, kind: ItineraryStopKindPlace, placeID: placeID, label: label, timeOnSiteMinutes: timeOnSiteMinutes, walkToNextMinutes: walkToNextMinutes}
}

func ReconstructSuggestionStop(id, label string, walkToNextMinutes int) ItineraryStop {
	return ItineraryStop{id: id, kind: ItineraryStopKindSuggestion, label: label, walkToNextMinutes: walkToNextMinutes}
}

func (s ItineraryStop) ID() string              { return s.id }
func (s ItineraryStop) Kind() ItineraryStopKind { return s.kind }
func (s ItineraryStop) PlaceID() string         { return s.placeID }
func (s ItineraryStop) Label() string           { return s.label }
func (s ItineraryStop) TimeOnSiteMinutes() int  { return s.timeOnSiteMinutes }
func (s ItineraryStop) WalkToNextMinutes() int  { return s.walkToNextMinutes }

// --- Entity ---

type Itinerary struct {
	id        string
	userID    string
	title     ItineraryTitle
	stops     []ItineraryStop
	createdAt time.Time
}

// NewItinerary ne retourne une erreur que sur userID/stops -- title est déjà
// un Value Object validé (même convention que NewScript pour language/text).
func NewItinerary(userID string, title ItineraryTitle, stops []ItineraryStop) (*Itinerary, error) {
	if userID == "" {
		return nil, ErrItineraryUserIDRequired
	}
	if len(stops) == 0 {
		return nil, ErrItineraryNoStops
	}
	return &Itinerary{id: newID(), userID: userID, title: title, stops: stops, createdAt: time.Now()}, nil
}

// ReconstructItinerary rebâtit un Itinerary depuis des données déjà valides
// (des lignes Postgres) -- préserve l'ID et createdAt donnés, ne revalide rien.
func ReconstructItinerary(id, userID string, title ItineraryTitle, stops []ItineraryStop, createdAt time.Time) *Itinerary {
	return &Itinerary{id: id, userID: userID, title: title, stops: stops, createdAt: createdAt}
}

func (i *Itinerary) ID() string             { return i.id }
func (i *Itinerary) UserID() string         { return i.userID }
func (i *Itinerary) Title() ItineraryTitle  { return i.title }
func (i *Itinerary) Stops() []ItineraryStop { return i.stops }
func (i *Itinerary) CreatedAt() time.Time   { return i.createdAt }

// TotalMinutes sums every stop's time-on-site plus every walk segment --
// the "1h · 4 lieux" duration the app shows is derived, never stored
// separately, so it can never drift from the stops that make it up.
func (i *Itinerary) TotalMinutes() int {
	total := 0
	for _, s := range i.stops {
		total += s.TimeOnSiteMinutes() + s.WalkToNextMinutes()
	}
	return total
}

// PlaceCount counts only real place stops -- the meal-break suggestion slot
// is not a "lieu" in the sense the UI shows ("4 lieux").
func (i *Itinerary) PlaceCount() int {
	count := 0
	for _, s := range i.stops {
		if s.Kind() == ItineraryStopKindPlace {
			count++
		}
	}
	return count
}
