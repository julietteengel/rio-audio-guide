// cmd/import/main.go
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

	matchedPlaces, scripts := buildImportPlan(places, narrations)
	log.Printf("matched %d places with narrations, %d scripts to import", len(matchedPlaces), len(scripts))

	if *dryRun {
		return
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	placeRepo := postgres.NewPlaceRepository(pool)
	scriptRepo := postgres.NewScriptRepository(pool)

	placeIDs := make(map[string]string, len(matchedPlaces))
	for _, p := range matchedPlaces {
		existing, err := placeRepo.FindByName(ctx, p.Name)
		if err == nil {
			placeIDs[p.Name] = existing.ID()
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("check existing place %q: %v", p.Name, err)
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
		if err := placeRepo.Save(ctx, place); err != nil {
			log.Printf("save place %q: %v", p.Name, err)
			continue
		}
		placeIDs[p.Name] = place.ID()
	}

	imported := 0
	for _, s := range scripts {
		placeID, ok := placeIDs[s.PlaceName]
		if !ok {
			continue // le lieu correspondant a échoué plus haut — pas de script orphelin
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

		script := domain.NewScript(placeID, language, text, "")
		if err := scriptRepo.Save(ctx, script); err != nil {
			if isUniqueViolation(err) {
				log.Printf("script %q/%s already imported, skipping", s.PlaceName, s.Language)
				continue
			}
			log.Printf("save script %q/%s: %v", s.PlaceName, s.Language, err)
			continue
		}
		imported++
	}

	log.Printf("imported %d places, %d scripts", len(placeIDs), imported)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
