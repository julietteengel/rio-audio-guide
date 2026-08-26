package awspolly

import "fmt"

// languageCodes maps the internal language code ("fr"/"en"/"es"/"pt", the same
// as already carried by ttsJobMessage on the rabbitmq side) to Polly's
// LanguageCode. pt-BR not pt-PT: this guide covers Rio de Janeiro, not Portugal.
var languageCodes = map[string]string{
	"fr": "fr-FR",
	"en": "en-US",
	"es": "es-ES",
	"pt": "pt-BR",
}

func languageCode(language string) (string, error) {
	code, ok := languageCodes[language]
	if !ok {
		return "", fmt.Errorf("awspolly: unsupported language %q", language)
	}
	return code, nil
}
