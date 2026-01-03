//go:build !cgo || gofuzz
// +build !cgo gofuzz

package secp256k1

// PubkeyFromSeckey returns the uncompressed public key (65 bytes) from a 32-byte private key.
func PubkeyFromSeckey(seckey []byte) ([]byte, error) {
	if len(seckey) != 32 {
		return nil, ErrInvalidSeckey
	}
	x, y := S256().ScalarBaseMult(seckey)
	if x == nil || y == nil {
		return nil, ErrInvalidSeckey
	}
	return S256().Marshal(x, y), nil
}
