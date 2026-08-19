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

	matchedPlaces, scripts, unmatched := buildImportPlan(places, narrations)

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

	// La narration orpheline doit être remontée, pas jetée en silence.
	if len(unmatched) != 1 || unmatched[0] != "Lieu inconnu" {
		t.Fatalf("got unmatched %+v, want [Lieu inconnu]", unmatched)
	}
}

// Le cas réel qui perdait ~10% des narrations : les deux CSV divergent sur la
// casse et les accents du même lieu.
func TestBuildImportPlan_MatchesDespiteCaseAndAccentDifferences(t *testing.T) {
	places := []placeRow{
		{Name: "Museu do amanhã", Category: "museum", Source: "overture", Lat: -22.8936, Lon: -43.1795},
		{Name: "  Pão de Açúcar ", Category: "monument", Source: "wikidata", Lat: -22.9486, Lon: -43.1566},
	}
	narrations := []narrationRow{
		{Name: "Museu do Amanhã", FR: "Texte FR"},
		{Name: "Pao de Acucar", PT: "Texto PT"},
	}

	matchedPlaces, scripts, unmatched := buildImportPlan(places, narrations)

	if len(unmatched) != 0 {
		t.Fatalf("got unmatched %+v, want none — la normalisation doit rattraper casse/accents/espaces", unmatched)
	}
	if len(matchedPlaces) != 2 {
		t.Fatalf("got %d matched places, want 2: %+v", len(matchedPlaces), matchedPlaces)
	}

	// Le nom conservé est celui du LIEU, non normalisé : c'est lui que Postgres
	// stocke et qui sert de clé de jointure côté main.go.
	want := []scriptToImport{
		{PlaceName: "Museu do amanhã", Language: "fr", Text: "Texte FR"},
		{PlaceName: "  Pão de Açúcar ", Language: "pt", Text: "Texto PT"},
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

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Museu do Amanhã", "museu do amanha"},
		{"  Pão de Açúcar ", "pao de acucar"},
		{"Cristo   Redentor", "cristo redentor"},
		{"THEATRO MUNICIPAL", "theatro municipal"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeName(tt.in); got != tt.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
