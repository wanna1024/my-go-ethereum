package secp256k1

import "errors"

// ErrInvalidSeckey is returned when a private key is invalid.
var ErrInvalidSeckey = errors.New("invalid private key")
