package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
)

type PlaceStatus string

const (
	PlaceStatusActive  PlaceStatus = "active"
	PlaceStatusRemoved PlaceStatus = "removed"
)

var (
	ErrPlaceNameRequired   = errors.New("place: name is required")
	ErrPlaceInvalidCoords  = errors.New("place: coordinates out of range")
	ErrPlaceAlreadyRemoved = errors.New("place: already removed")
	ErrPlaceRemoved        = errors.New("place: cannot edit a removed place")
	ErrInvalidWikidataQID  = errors.New("place: wikidata QID must look like Q123")
)

// --- Value Objects ---
// Value Objects: structs without an identifier and persistent values after creation.
// Value objects are often found inside domains and used to describe certain aspects in that domain.

type PlaceName string

func NewPlaceName(s string) (PlaceName, error) {
	if s == "" {
		return "", ErrPlaceNameRequired
	}
	return PlaceName(s), nil
}

func (n PlaceName) String() string { return string(n) }

type Coordinates struct {
	// all values lowercase since they are immutable
	lat float64
	lon float64
}

func NewCoordinates(lat, lon float64) (Coordinates, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return Coordinates{}, ErrPlaceInvalidCoords
	}
	return Coordinates{lat: lat, lon: lon}, nil
}

func (c Coordinates) Lat() float64 { return c.lat }
func (c Coordinates) Lon() float64 { return c.lon }

type WikidataQID string

var wikidataQIDPattern = regexp.MustCompile(`^Q[0-9]+$`)

func NewWikidataQID(s string) (WikidataQID, error) {
	if s == "" {
		return "", nil // pas de QID = valide, champ optionnel
	}
	if !wikidataQIDPattern.MatchString(s) {
		return "", ErrInvalidWikidataQID
	}
	return WikidataQID(s), nil
}

func (q WikidataQID) String() string { return string(q) }

// --- Entity ---
//	An entity is a struct with a unique identifier to reference it, which has states that can change.

//  An Aggregate is a set of entities and value objects combined.
//  EX:  Customer is a aggregate that combines all entities needed to represent a customer
// type Customer struct {
// 	// person is the root entity of a customer
// 	// which means the person.ID is the main identifier for this aggregate
// 	person *entity.Person

// 	// a customer can hold many products
// 	products []*entity.Item

// 	// a customer can perform many transactions
// 	transactions []valueobject.Transaction
// }

type Place struct {
	id             string
	name           PlaceName
	category       string // reste une string brute, volontairement — voir note plus bas
	coords         Coordinates
	wikidataQID    WikidataQID
	source         string
	sourceRichness string
	status         PlaceStatus
	removedReason  string
}

// NewPlace ne retourne plus d'erreur : name/coords/wikidataQID sont déjà
// validés avant d'arriver ici (ce sont des Value Objects). Il n'y a plus
// rien à valider au niveau de l'entité elle-même.
func NewPlace(name PlaceName, category string, coords Coordinates, wikidataQID WikidataQID, source, sourceRichness string) *Place {
	return &Place{
		id:             newID(),
		name:           name,
		category:       category,
		coords:         coords,
		wikidataQID:    wikidataQID,
		source:         source,
		sourceRichness: sourceRichness,
		status:         PlaceStatusActive,
	}
}

// ReconstructPlace rebâtit un Place à partir de données déjà considérées
// valides (typiquement une ligne lue depuis Postgres) — contrairement à
// NewPlace, elle préserve l'ID donné plutôt que d'en générer un nouveau,
// et ne revalide rien : la donnée était valide au moment où elle a été
// sauvegardée.
func ReconstructPlace(id string, name PlaceName, category string, coords Coordinates, wikidataQID WikidataQID, source, sourceRichness string, status PlaceStatus, removedReason string) *Place {
	return &Place{
		id:             id,
		name:           name,
		category:       category,
		coords:         coords,
		wikidataQID:    wikidataQID,
		source:         source,
		sourceRichness: sourceRichness,
		status:         status,
		removedReason:  removedReason,
	}
}

func (p *Place) Edit(name PlaceName, category string, coords Coordinates, wikidataQID WikidataQID) error {
	if p.status == PlaceStatusRemoved {
		return ErrPlaceRemoved
	}
	p.name = name
	p.category = category
	p.coords = coords
	p.wikidataQID = wikidataQID
	return nil
}

func (p *Place) Remove(reason string) error {
	if p.status == PlaceStatusRemoved {
		return ErrPlaceAlreadyRemoved
	}
	p.status = PlaceStatusRemoved
	p.removedReason = reason
	return nil
}

// --- lecture, puisque les champs ne sont plus exportés ---

func (p *Place) ID() string               { return p.id }
func (p *Place) Name() PlaceName          { return p.name }
func (p *Place) Category() string         { return p.category }
func (p *Place) Coordinates() Coordinates { return p.coords }
func (p *Place) WikidataQID() WikidataQID { return p.wikidataQID }
func (p *Place) Source() string           { return p.source }
func (p *Place) SourceRichness() string   { return p.sourceRichness }
func (p *Place) Status() PlaceStatus      { return p.status }
func (p *Place) RemovedReason() string    { return p.removedReason }

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
