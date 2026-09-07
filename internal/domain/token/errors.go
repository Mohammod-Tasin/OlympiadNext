package token

import "errors"

// ErrDuplicateTokenHash is returned by Repository.Create when the new
// row's token_hash collides with one already stored.
//
// Refresh tokens are deterministic JWTs — user id plus second-granularity
// issued-at/expiry, with no random component — so two rotations for the
// same user within the same wall-clock second mint a byte-identical
// token. Whichever insert lands second trips the unique index on
// token_hash. The refresh flow treats this as a lost rotation race (the
// presented token is already revoked and consumed), not as token reuse.
var ErrDuplicateTokenHash = errors.New("token: duplicate token hash")
