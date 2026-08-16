// cmd/import/join.go
package main

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

// buildImportPlan croise les lieux sourcés et les narrations traduites sur le
// nom du lieu. Une narration sans lieu correspondant est ignorée (orpheline —
// on ne crée jamais un Place à partir d'une narration seule). Un lieu sans
// narration n'est simplement pas importé : rien à publier pour lui pour
// l'instant.
func buildImportPlan(places []placeRow, narrations []narrationRow) ([]placeRow, []scriptToImport) {
	byName := make(map[string]placeRow, len(places))
	for _, p := range places {
		byName[p.Name] = p
	}

	var matchedPlaces []placeRow
	var scripts []scriptToImport
	for _, n := range narrations {
		place, ok := byName[n.Name]
		if !ok {
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
			scripts = append(scripts, scriptToImport{PlaceName: n.Name, Language: l.code, Text: l.text})
		}
	}
	return matchedPlaces, scripts
}
