// cmd/import/csv_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireColumns_MissingColumn(t *testing.T) {
	idx := columnIndex([]string{"name", "category", "source", "lat", "lon"}) // no wikidata_qid

	err := requireColumns(idx, "name", "category", "source", "lat", "lon", "wikidata_qid")
	if err == nil {
		t.Fatal("expected an error for missing column \"wikidata_qid\", got nil")
	}
	if !strings.Contains(err.Error(), "wikidata_qid") {
		t.Fatalf("error message %q does not name the missing column", err.Error())
	}
}

func TestRequireColumns_AllPresent(t *testing.T) {
	idx := columnIndex([]string{"name", "category", "source", "lat", "lon", "wikidata_qid"})

	if err := requireColumns(idx, "name", "category", "source", "lat", "lon", "wikidata_qid"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReadPlacesCSV_MissingColumnErrorsInsteadOfMisreading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "places.csv")
	// "wikidata_qid" column is missing from the header entirely.
	content := "name,category,source,lat,lon\nCristo Redentor,monument,wikidata,-22.9519,-43.2105\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := readPlacesCSV(path)
	if err == nil {
		t.Fatal("expected an error for a CSV missing the \"wikidata_qid\" column, got nil (silent wrong-column read)")
	}
	if !strings.Contains(err.Error(), "wikidata_qid") {
		t.Fatalf("error message %q does not name the missing column", err.Error())
	}
}

func TestReadNarrationsCSV_MissingColumnErrorsInsteadOfMisreading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "narrations.csv")
	// "narration_pt" column is missing from the header entirely.
	content := "name,narration_fr,narration_en,narration_es\nCristo Redentor,Texte FR,Text EN,Texto ES\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := readNarrationsCSV(path)
	if err == nil {
		t.Fatal("expected an error for a CSV missing the \"narration_pt\" column, got nil (silent wrong-column read)")
	}
	if !strings.Contains(err.Error(), "narration_pt") {
		t.Fatalf("error message %q does not name the missing column", err.Error())
	}
}
