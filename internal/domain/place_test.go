// internal/domain/place_test.go
package domain

import (
	"errors"
	"testing"
)

func TestNewPlaceName(t *testing.T) {
	if _, err := NewPlaceName(""); !errors.Is(err, ErrPlaceNameRequired) {
		t.Fatalf("got error %v, want ErrPlaceNameRequired", err)
	}
	name, err := NewPlaceName("Cristo Redentor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "Cristo Redentor" {
		t.Fatalf("got %q, want %q", name.String(), "Cristo Redentor")
	}
}

func TestNewCoordinates(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon float64
		wantErr  error
	}{
		{name: "valid", lat: -22.9519, lon: -43.2105, wantErr: nil},
		{name: "invalid latitude", lat: 200, lon: -43.21, wantErr: ErrPlaceInvalidCoords},
		{name: "invalid longitude", lat: -22.95, lon: 200, wantErr: ErrPlaceInvalidCoords},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCoordinates(tt.lat, tt.lon)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.Lat() != tt.lat || c.Lon() != tt.lon {
				t.Fatalf("got (%v,%v), want (%v,%v)", c.Lat(), c.Lon(), tt.lat, tt.lon)
			}
		})
	}
}

func TestNewWikidataQID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "empty is valid (no QID)", input: "", wantErr: nil},
		{name: "valid QID", input: "Q1963380", wantErr: nil},
		{name: "malformed", input: "not-a-qid", wantErr: ErrInvalidWikidataQID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := NewWikidataQID(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.String() != tt.input {
				t.Fatalf("got %q, want %q", q.String(), tt.input)
			}
		})
	}
}

func validPlaceFixture(t *testing.T) *Place {
	t.Helper()
	name, err := NewPlaceName("Cristo Redentor")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	coords, err := NewCoordinates(-22.9519, -43.2105)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	qid, err := NewWikidataQID("Q1963380")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	return NewPlace(name, "monument", coords, qid, "wikidata", "rich")
}

func TestNewPlace(t *testing.T) {
	p := validPlaceFixture(t)
	if p.ID() == "" {
		t.Fatal("expected a generated ID, got empty string")
	}
	if p.Status() != PlaceStatusActive {
		t.Fatalf("got status %v, want active", p.Status())
	}
}

func TestPlace_Edit(t *testing.T) {
	p := validPlaceFixture(t)

	newName, _ := NewPlaceName("New Name")
	newCoords, _ := NewCoordinates(-22.90, -43.20)
	newQID, _ := NewWikidataQID("Q123")

	if err := p.Edit(newName, "museum", newCoords, newQID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != newName || p.Category() != "museum" {
		t.Fatalf("edit did not apply: name=%v category=%v", p.Name(), p.Category())
	}
}

func TestPlace_Edit_RejectsWhenRemoved(t *testing.T) {
	p := validPlaceFixture(t)
	_ = p.Remove("test")

	newName, _ := NewPlaceName("New Name")
	newCoords, _ := NewCoordinates(-22.90, -43.20)

	if err := p.Edit(newName, "museum", newCoords, ""); !errors.Is(err, ErrPlaceRemoved) {
		t.Fatalf("got error %v, want ErrPlaceRemoved", err)
	}
}

func TestPlace_Remove(t *testing.T) {
	p := validPlaceFixture(t)

	if err := p.Remove("hors périmètre municipal"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status() != PlaceStatusRemoved {
		t.Fatalf("got status %v, want removed", p.Status())
	}
	if err := p.Remove("again"); !errors.Is(err, ErrPlaceAlreadyRemoved) {
		t.Fatalf("got error %v, want ErrPlaceAlreadyRemoved", err)
	}
}

func TestReconstructPlace(t *testing.T) {
	name, _ := NewPlaceName("Escadaria Selarón")
	coords, _ := NewCoordinates(-22.9147, -43.1806)

	p := ReconstructPlace("existing-id-123", name, "monument", coords, "", "overture", "correct", PlaceStatusActive, "")
	if p.ID() != "existing-id-123" {
		t.Fatalf("got ID %q, want %q", p.ID(), "existing-id-123")
	}
}
