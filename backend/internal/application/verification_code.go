package application

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// generateVerificationCode produces a random 6-digit numeric string
// (leading zeros preserved via %06d) -- crypto/rand, not math/rand, since
// this is a short-lived credential even though it's low-stakes (15-minute
// expiry, single use).
func generateVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
