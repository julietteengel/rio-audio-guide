package application

import (
	"context"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// allLanguages mirrors domain.NewLanguage's supported set -- kept here
// rather than in the domain package, since "which languages this product
// covers" is an application-level fact, not a rule the Language Value
// Object itself needs to know to validate one language string.
var allLanguages = []domain.Language{domain.LanguageFR, domain.LanguageEN, domain.LanguageES, domain.LanguagePT}

// MissingLanguages reports which of the 4 languages a place has no
// *published* script for -- a script that exists but is still draft/reviewed
// (audio requested, not confirmed ready) counts as missing, same as one that
// was never written at all. Matches how GetPlaceAudio already treats
// reviewed-but-unpublished as "not actually available" (see
// TestGetPlaceAudio_ScriptNotPublished).
func MissingLanguages(ctx context.Context, scriptRepo ports.ScriptRepository, placeID string) ([]domain.Language, error) {
	scripts, err := scriptRepo.FindByPlaceID(ctx, placeID)
	if err != nil {
		return nil, err
	}

	published := make(map[domain.Language]bool, len(scripts))
	for _, s := range scripts {
		if s.Status() == domain.ScriptStatusPublished {
			published[s.Language()] = true
		}
	}

	var missing []domain.Language
	for _, lang := range allLanguages {
		if !published[lang] {
			missing = append(missing, lang)
		}
	}
	return missing, nil
}
