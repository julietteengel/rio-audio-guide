// cmd/import/join_test.go
package main

import "testing"

func TestBuildImportPlan(t *testing.T) {
	places := []placeRow{
		{Name: "Cristo Redentor", Category: "monument", Source: "wikidata", Lat: -22.9519, Lon: -43.2105, WikidataQID: "Q1963380"},
		{Name: "Lugar sans narration", Category: "monument", Source: "overture", Lat: -22.9, Lon: -43.2},
	}
	narrations := []narrationRow{
		{Name: "Cristo Redentor", FR: "Texte FR", EN: "Text EN", ES: "", PT: "Texto PT"},
		{Name: "Lieu inconnu", FR: "Orphelin"},
	}

	matchedPlaces, scripts := buildImportPlan(places, narrations)

	if len(matchedPlaces) != 1 || matchedPlaces[0].Name != "Cristo Redentor" {
		t.Fatalf("got matched places %+v, want only Cristo Redentor", matchedPlaces)
	}

	want := []scriptToImport{
		{PlaceName: "Cristo Redentor", Language: "fr", Text: "Texte FR"},
		{PlaceName: "Cristo Redentor", Language: "en", Text: "Text EN"},
		{PlaceName: "Cristo Redentor", Language: "pt", Text: "Texto PT"},
	}
	if len(scripts) != len(want) {
		t.Fatalf("got %d scripts, want %d: %+v", len(scripts), len(want), scripts)
	}
	for i, s := range scripts {
		if s != want[i] {
			t.Fatalf("script %d: got %+v, want %+v", i, s, want[i])
		}
	}
}
