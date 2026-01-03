//go:build !gofuzz && cgo
// +build !gofuzz,cgo

package secp256k1

import (
	"unsafe"
)

/*
#include "libsecp256k1/include/secp256k1.h"
*/
import "C"

// PubkeyFromSeckey returns the uncompressed public key (65 bytes) from a 32-byte private key.
func PubkeyFromSeckey(seckey []byte) ([]byte, error) {
	if len(seckey) != 32 {
		return nil, ErrInvalidSeckey
	}
	var pubkey C.secp256k1_pubkey
	if C.secp256k1_ec_pubkey_create(context, &pubkey, (*C.uchar)(unsafe.Pointer(&seckey[0]))) != 1 {
		return nil, ErrInvalidSeckey
	}
	out := make([]byte, 65)
	outlen := C.size_t(len(out))
	if C.secp256k1_ec_pubkey_serialize(context, (*C.uchar)(unsafe.Pointer(&out[0])), &outlen, &pubkey, C.SECP256K1_EC_UNCOMPRESSED) != 1 {
		return nil, ErrInvalidSeckey
	}
	return out[:outlen], nil
}
