package domain

import (
	"errors"
	"time"
)

type ScriptStatus string

const (
	ScriptStatusDraft     ScriptStatus = "draft"
	ScriptStatusReviewed  ScriptStatus = "reviewed"
	ScriptStatusPublished ScriptStatus = "published"
)

var (
	ErrScriptTextRequired    = errors.New("script: text is required")
	ErrScriptInvalidLanguage = errors.New("script: unsupported language")
	ErrScriptNotDraft        = errors.New("script: must be draft to be reviewed")
	ErrScriptNotReviewed     = errors.New("script: must be reviewed to be published")
)

// --- Value Objects ---

type Language string

const (
	LanguageEN Language = "en"
	LanguageFR Language = "fr"
	LanguagePT Language = "pt"
	LanguageES Language = "es"
)

func NewLanguage(s string) (Language, error) {
	l := Language(s)
	switch l {
	case LanguageEN, LanguageFR, LanguagePT, LanguageES:
		return l, nil
	}
	return "", ErrScriptInvalidLanguage
}

func (l Language) String() string { return string(l) }

type ScriptText string

func NewScriptText(s string) (ScriptText, error) {
	if s == "" {
		return "", ErrScriptTextRequired
	}
	return ScriptText(s), nil
}

func (t ScriptText) String() string { return string(t) }

// --- Entity ---

type Script struct {
	id          string
	placeID     string
	language    Language
	text        ScriptText
	sourceText  string
	status      ScriptStatus
	reviewerID  string
	reviewedAt  time.Time
	publishedAt time.Time
}

// NewScript ne retourne plus d'erreur : language et text sont déjà validés
// avant d'arriver ici (Value Objects) — même conséquence que NewPlace.
func NewScript(placeID string, language Language, text ScriptText, sourceText string) *Script {
	return &Script{
		id:         newID(),
		placeID:    placeID,
		language:   language,
		text:       text,
		sourceText: sourceText,
		status:     ScriptStatusDraft,
	}
}

// ReconstructScript rebâtit un Script depuis des données déjà valides
// (une ligne Postgres) — préserve l'ID et le statut donnés, ne revalide rien.
func ReconstructScript(id, placeID string, language Language, text ScriptText, sourceText string, status ScriptStatus, reviewerID string, reviewedAt, publishedAt time.Time) *Script {
	return &Script{
		id:          id,
		placeID:     placeID,
		language:    language,
		text:        text,
		sourceText:  sourceText,
		status:      status,
		reviewerID:  reviewerID,
		reviewedAt:  reviewedAt,
		publishedAt: publishedAt,
	}
}

// MarkReviewed prend l'ID d'un User (pas un nom en texte libre comme avant)
// -- qui a réellement reviewé un script doit être une vraie référence
// traçable vers un compte, pas une chaîne qu'on pourrait mal orthographier
// ou laisser vide sans erreur.
func (s *Script) MarkReviewed(reviewerID string) error {
	if s.status != ScriptStatusDraft {
		return ErrScriptNotDraft
	}
	s.status = ScriptStatusReviewed
	s.reviewerID = reviewerID
	s.reviewedAt = time.Now()
	return nil
}

func (s *Script) Publish() error {
	if s.status != ScriptStatusReviewed {
		return ErrScriptNotReviewed
	}
	s.status = ScriptStatusPublished
	s.publishedAt = time.Now()
	return nil
}

// --- lecture ---

func (s *Script) ID() string             { return s.id }
func (s *Script) PlaceID() string        { return s.placeID }
func (s *Script) Language() Language     { return s.language }
func (s *Script) Text() ScriptText       { return s.text }
func (s *Script) SourceText() string     { return s.sourceText }
func (s *Script) Status() ScriptStatus   { return s.status }
func (s *Script) ReviewerID() string     { return s.reviewerID }
func (s *Script) ReviewedAt() time.Time  { return s.reviewedAt }
func (s *Script) PublishedAt() time.Time { return s.publishedAt }
