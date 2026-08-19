package jwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"rioaudioguide/backend/internal/domain"
)

func TestIssuer_IssueThenVerify_RoundTrips(t *testing.T) {
	issuer, err := NewIssuer("test-secret")
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}

	token, err := issuer.Issue("user-123", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	userID, role, err := issuer.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if userID != "user-123" || role != domain.RoleAdmin {
		t.Fatalf("got (%q, %q), want (%q, %q)", userID, role, "user-123", domain.RoleAdmin)
	}
}

func TestIssuer_Verify_RejectsTokenSignedWithDifferentSecret(t *testing.T) {
	issuerA, _ := NewIssuer("secret-a")
	issuerB, _ := NewIssuer("secret-b")

	token, err := issuerA.Issue("user-123", domain.RoleUser)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	if _, _, err := issuerB.Verify(token); err == nil {
		t.Fatal("expected verification to fail for a token signed with a different secret")
	}
}

// TestIssuer_Verify_RejectsExpiredToken construit un token déjà expiré à la
// main (Issuer.Issue ne permet pas de choisir tokenTTL) -- c'est le seul
// moyen de tester l'expiration sans attendre 24h pour de vrai.
func TestIssuer_Verify_RejectsExpiredToken(t *testing.T) {
	issuer, _ := NewIssuer("test-secret")

	past := time.Now().Add(-1 * time.Hour)
	c := claims{
		Role: domain.RoleUser.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			IssuedAt:  jwt.NewNumericDate(past.Add(-1 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(past),
		},
	}
	expiredToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(issuer.secret)
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}

	if _, _, err := issuer.Verify(expiredToken); err == nil {
		t.Fatal("expected verification to fail for an expired token")
	}
}

func TestIssuer_Verify_RejectsGarbageToken(t *testing.T) {
	issuer, _ := NewIssuer("test-secret")

	if _, _, err := issuer.Verify("not-a-real-token"); err == nil {
		t.Fatal("expected verification to fail for a malformed token")
	}
}

func TestNewIssuer_RejectsEmptySecret(t *testing.T) {
	if _, err := NewIssuer(""); err != ErrEmptySecret {
		t.Fatalf("got error %v, want ErrEmptySecret", err)
	}
}
