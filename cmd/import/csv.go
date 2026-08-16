// cmd/import/csv.go
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
)

func columnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[col] = i
	}
	return idx
}

func readPlacesCSV(path string) ([]placeRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := columnIndex(header)

	var rows []placeRow
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		lat, err := strconv.ParseFloat(record[idx["lat"]], 64)
		if err != nil {
			return nil, fmt.Errorf("parse lat for %q: %w", record[idx["name"]], err)
		}
		lon, err := strconv.ParseFloat(record[idx["lon"]], 64)
		if err != nil {
			return nil, fmt.Errorf("parse lon for %q: %w", record[idx["name"]], err)
		}
		rows = append(rows, placeRow{
			Name:        record[idx["name"]],
			Category:    record[idx["category"]],
			Source:      record[idx["source"]],
			Lat:         lat,
			Lon:         lon,
			WikidataQID: record[idx["wikidata_qid"]],
		})
	}
	return rows, nil
}

func readNarrationsCSV(path string) ([]narrationRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := columnIndex(header)

	var rows []narrationRow
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, narrationRow{
			Name: record[idx["name"]],
			FR:   record[idx["narration_fr"]],
			EN:   record[idx["narration_en"]],
			ES:   record[idx["narration_es"]],
			PT:   record[idx["narration_pt"]],
		})
	}
	return rows, nil
}
