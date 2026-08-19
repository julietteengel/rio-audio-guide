// cmd/import/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/domain"
)

func main() {
	placesPath := flag.String("places", "", "path to places_clean_vN.csv")
	narrationsPath := flag.String("narrations", "", "path to narrations_multi_full.csv")
	dryRun := flag.Bool("dry-run", false, "parse and report counts without writing to Postgres")
	flag.Parse()

	if *placesPath == "" || *narrationsPath == "" {
		log.Fatal("both -places and -narrations are required")
	}

	places, err := readPlacesCSV(*placesPath)
	if err != nil {
		log.Fatalf("read places csv: %v", err)
	}
	narrations, err := readNarrationsCSV(*narrationsPath)
	if err != nil {
		log.Fatalf("read narrations csv: %v", err)
	}

	matchedPlaces, scripts, unmatched := buildImportPlan(places, narrations)
	log.Printf("matched %d places with narrations, %d scripts to import", len(matchedPlaces), len(scripts))
	// Une narration orpheline est du contenu rédigé qui ne sera jamais publié —
	// il faut la voir passer, pas la découvrir via un compteur trop bas.
	if len(unmatched) > 0 {
		log.Printf("WARNING: %d narrations had no matching place: %v", len(unmatched), unmatched)
	}

	if *dryRun {
		return
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	// Tout le run passe dans une seule transaction : soit l'import complet
	// rentre, soit rien n'est écrit. Sans ça, un plantage à mi-parcours laisse
	// Postgres avec des lieux importés sans leurs scripts (ou l'inverse), un
	// état bâtard qu'il faut ensuite deviner pour relancer proprement.
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("begin transaction: %v", err)
	}

	placesCreated, imported, err := runImport(ctx, tx, matchedPlaces, scripts)
	if err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			log.Printf("rollback failed: %v", rbErr)
		}
		log.Fatalf("import failed, rolled back: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit import transaction: %v", err)
	}

	log.Printf("imported %d new places, %d scripts", placesCreated, imported)
}

// runImport does the actual writes for one import run, entirely inside the
// given transaction. Existence checks (which places/scripts already exist)
// are each a single batched round trip rather than one query per row, and
// the writes themselves are sent as one pipelined pgx.Batch per entity type
// instead of one round trip per row -- turning what used to be roughly
// len(matchedPlaces)+len(scripts) individual statements into four round
// trips total, regardless of import size.
func runImport(ctx context.Context, tx pgx.Tx, matchedPlaces []placeRow, scripts []scriptToImport) (placesCreated, imported int, err error) {
	placeRepo := postgres.NewPlaceRepository(tx)
	scriptRepo := postgres.NewScriptRepository(tx)

	names := make([]string, len(matchedPlaces))
	for i, p := range matchedPlaces {
		names[i] = p.Name
	}
	existingPlaceIDs, err := placeRepo.FindIDsByNames(ctx, names)
	if err != nil {
		return 0, 0, err
	}

	// placeIDs couvre tous les lieux de l'import (existants + nouveaux) : les
	// scripts en ont besoin pour savoir à quel Place se rattacher, que le lieu
	// vienne d'être créé ou existait déjà.
	placeIDs := make(map[string]string, len(matchedPlaces))
	var newPlaces []*domain.Place
	for _, p := range matchedPlaces {
		if id, ok := existingPlaceIDs[p.Name]; ok {
			placeIDs[p.Name] = id
			continue
		}

		name, err := domain.NewPlaceName(p.Name)
		if err != nil {
			log.Printf("skip place %q: %v", p.Name, err)
			continue
		}
		coords, err := domain.NewCoordinates(p.Lat, p.Lon)
		if err != nil {
			log.Printf("skip place %q: %v", p.Name, err)
			continue
		}
		qid, err := domain.NewWikidataQID(p.WikidataQID)
		if err != nil {
			log.Printf("skip place %q: %v", p.Name, err)
			continue
		}

		place := domain.NewPlace(name, p.Category, coords, qid, p.Source, "")
		placeIDs[p.Name] = place.ID()
		newPlaces = append(newPlaces, place)
	}

	if err := sendPlacesBatch(ctx, tx, placeRepo, newPlaces); err != nil {
		return 0, 0, err
	}

	allPlaceIDs := make([]string, 0, len(placeIDs))
	for _, id := range placeIDs {
		allPlaceIDs = append(allPlaceIDs, id)
	}
	existingLanguages, err := scriptRepo.FindExistingLanguages(ctx, allPlaceIDs)
	if err != nil {
		return 0, 0, err
	}

	var newScripts []*domain.Script
	skippedExisting := 0
	for _, s := range scripts {
		placeID, ok := placeIDs[s.PlaceName]
		if !ok {
			continue // le lieu correspondant a échoué plus haut — pas de script orphelin
		}
		if existingLanguages[placeID][s.Language] {
			skippedExisting++
			continue
		}

		language, err := domain.NewLanguage(s.Language)
		if err != nil {
			log.Printf("skip script %q/%s: %v", s.PlaceName, s.Language, err)
			continue
		}
		text, err := domain.NewScriptText(s.Text)
		if err != nil {
			log.Printf("skip script %q/%s: %v", s.PlaceName, s.Language, err)
			continue
		}

		newScripts = append(newScripts, domain.NewScript(placeID, language, text, ""))
	}
	if skippedExisting > 0 {
		log.Printf("%d scripts already imported, skipping", skippedExisting)
	}

	if err := sendScriptsBatch(ctx, tx, scriptRepo, newScripts); err != nil {
		return 0, 0, err
	}

	return len(newPlaces), len(newScripts), nil
}

func sendPlacesBatch(ctx context.Context, tx pgx.Tx, placeRepo *postgres.PlaceRepository, places []*domain.Place) error {
	if len(places) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, place := range places {
		placeRepo.QueueSave(batch, place)
	}
	results := tx.SendBatch(ctx, batch)
	for _, place := range places {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("save place %q: %w", place.Name(), err)
		}
	}
	return results.Close()
}

func sendScriptsBatch(ctx context.Context, tx pgx.Tx, scriptRepo *postgres.ScriptRepository, scripts []*domain.Script) error {
	if len(scripts) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, script := range scripts {
		scriptRepo.QueueSave(batch, script)
	}
	results := tx.SendBatch(ctx, batch)
	for _, script := range scripts {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("save script %q/%s: %w", script.PlaceID(), script.Language(), err)
		}
	}
	return results.Close()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
