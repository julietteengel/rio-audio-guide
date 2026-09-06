package awspolly

import "testing"

func TestLanguageCode(t *testing.T) {
	tests := []struct {
		language string
		want     string
		wantErr  bool
	}{
		{language: "fr", want: "fr-FR"},
		{language: "en", want: "en-US"},
		{language: "es", want: "es-ES"},
		{language: "pt", want: "pt-BR"}, // Rio/Brazil, not pt-PT
		{language: "de", wantErr: true},
		{language: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			got, err := languageCode(tt.language)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("languageCode(%q): expected an error, got %q", tt.language, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("languageCode(%q): unexpected error: %v", tt.language, err)
			}
			if got != tt.want {
				t.Fatalf("languageCode(%q) = %q, want %q", tt.language, got, tt.want)
			}
		})
	}
}
