package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	startupPrivateKeyBatchSize = 1_000_000
	privateKeyBytes            = 32
	startupKeygenTimeout       = 30 * time.Second
	keygenWorkerMultiplier     = 8
	keygenBatchKeysPerWorker   = 8192
)

type keygenRNG struct {
	stream  cipher.Stream
	zeroBuf []byte
}

func newKeygenRNG() (*keygenRNG, error) {
	key := make([]byte, 32)
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &keygenRNG{
		stream:  cipher.NewCTR(block, iv),
		zeroBuf: make([]byte, 64),
	}, nil
}

func (k *keygenRNG) fill(dst []byte) {
	filled := 0
	for filled < len(dst) {
		chunk := len(dst) - filled
		if chunk > len(k.zeroBuf) {
			chunk = len(k.zeroBuf)
		}
		k.stream.XORKeyStream(dst[filled:filled+chunk], k.zeroBuf[:chunk])
		filled += chunk
	}
}

func generateRandomPrivateKeysStream(ctx context.Context, count, workers int, out chan<- string) (int, error) {
	if count <= 0 {
		return 0, nil
	}

	if workers < 1 {
		workers = 1
	}
	if workers > count {
		workers = count
	}

	var generated uint64
	errCh := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localRNG, err := newKeygenRNG()
			if err != nil {
				errCh <- err
				return
			}
			buf := make([]byte, keygenBatchKeysPerWorker*privateKeyBytes)

			for {
				if atomic.LoadUint64(&generated) >= uint64(count) {
					return
				}
				select {
				case <-ctx.Done():
					return
				default:
				}

				localRNG.fill(buf)
				for offset := 0; offset < len(buf); offset += privateKeyBytes {
					if atomic.LoadUint64(&generated) >= uint64(count) {
						return
					}
					select {
					case <-ctx.Done():
						return
					default:
					}

					privBytes := buf[offset : offset+privateKeyBytes]
					if _, err := crypto.ToECDSA(privBytes); err != nil {
						continue
					}
					for {
						current := atomic.LoadUint64(&generated)
						if current >= uint64(count) {
							return
						}
						if atomic.CompareAndSwapUint64(&generated, current, current+1) {
							break
						}
					}
					keyHex := hex.EncodeToString(privBytes)
					select {
					case <-ctx.Done():
						return
					case out <- keyHex:
					}
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return int(atomic.LoadUint64(&generated)), err
		}
	}
	if ctx.Err() != nil {
		return int(atomic.LoadUint64(&generated)), ctx.Err()
	}
	return int(atomic.LoadUint64(&generated)), nil
}
