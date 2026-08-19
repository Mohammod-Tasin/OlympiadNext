// Package hash provides a fast, deterministic digest for opaque bearer
// tokens (refresh tokens) that must be looked up by exact value in the
// database. bcrypt is deliberately not used here: it is for
// low-entropy user passwords, not high-entropy random tokens, and its
// per-call cost would make every refresh a deliberately slow query.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
)

func SHA256Hex(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
