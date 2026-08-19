// cmd/import/join.go
package main

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type placeRow struct {
	Name        string
	Category    string
	Source      string
	Lat, Lon    float64
	WikidataQID string
}

type narrationRow struct {
	Name           string
	FR, EN, ES, PT string
}

type scriptToImport struct {
	PlaceName string
	Language  string
	Text      string
}

// normalizeName rend la jointure lieu/narration tolérante aux écarts de saisie
// entre les deux CSV : casse, espaces, accents. Les deux fichiers viennent de
// pipelines Python différents et divergent réellement ("Museu do Amanhã" côté
// narrations vs "Museu do amanhã" côté lieux) — une égalité stricte perdait ces
// narrations en silence. Même esprit que normalize_name dans
// pipeline/sourcing/dedup.py, en plus simple : pas de retrait de préfixes
// génériques ici, on ne veut surtout pas créer de faux rapprochements.
func normalizeName(s string) string {
	lowered := strings.ToLower(strings.TrimSpace(s))
	decomposed := norm.NFKD.String(lowered)
	var b strings.Builder
	for _, r := range decomposed {
		// Mn = marques diacritiques combinantes, isolées par la décomposition NFKD.
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// buildImportPlan croise les lieux sourcés et les narrations traduites sur le
// nom du lieu normalisé. Une narration sans lieu correspondant est ignorée
// (orpheline — on ne crée jamais un Place à partir d'une narration seule) mais
// remontée dans la troisième valeur de retour, pour que l'appelant puisse la
// signaler au lieu de la perdre en silence. Un lieu sans narration n'est
// simplement pas importé : rien à publier pour lui pour l'instant.
//
// Les noms renvoyés restent les noms d'origine, non normalisés : Postgres doit
// stocker le vrai nom, accents et casse compris. scriptToImport.PlaceName porte
// le nom du LIEU (pas celui de la narration) — c'est la clé qui relie le script
// au Place créé côté appelant, et les deux peuvent différer par la casse.
func buildImportPlan(places []placeRow, narrations []narrationRow) ([]placeRow, []scriptToImport, []string) {
	byName := make(map[string]placeRow, len(places))
	for _, p := range places {
		byName[normalizeName(p.Name)] = p
	}

	var matchedPlaces []placeRow
	var scripts []scriptToImport
	var unmatched []string
	for _, n := range narrations {
		place, ok := byName[normalizeName(n.Name)]
		if !ok {
			unmatched = append(unmatched, n.Name)
			continue
		}
		matchedPlaces = append(matchedPlaces, place)

		languages := []struct{ code, text string }{
			{"fr", n.FR}, {"en", n.EN}, {"es", n.ES}, {"pt", n.PT},
		}
		for _, l := range languages {
			if l.text == "" {
				continue
			}
			scripts = append(scripts, scriptToImport{PlaceName: place.Name, Language: l.code, Text: l.text})
		}
	}
	return matchedPlaces, scripts, unmatched
}
