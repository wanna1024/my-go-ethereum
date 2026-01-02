package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	startupPrivateKeyBatchSize = 4096
	privateKeyBytes            = 32
	startupKeygenTimeout       = 30 * time.Second
	keygenWorkerMultiplier     = 2
	keygenBatchKeysPerWorker   = 4096
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

func generateRandomPrivateKeysBatch(ctx context.Context, count int) ([]string, error) {
	workers := runtime.NumCPU() * keygenWorkerMultiplier
	return generateRandomPrivateKeysBatchParallel(ctx, count, workers)
}

func generateRandomPrivateKeysBatchParallel(ctx context.Context, count, workers int) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}

	if workers < 1 {
		workers = 1
	}
	if workers > count {
		workers = count
	}

	keys := make([]string, count)
	var filled uint64
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
				if atomic.LoadUint64(&filled) >= uint64(count) {
					return
				}
				select {
				case <-ctx.Done():
					return
				default:
				}

				localRNG.fill(buf)
				for offset := 0; offset < len(buf); offset += privateKeyBytes {
					if atomic.LoadUint64(&filled) >= uint64(count) {
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
					pos := atomic.AddUint64(&filled, 1) - 1
					if pos >= uint64(count) {
						return
					}
					keys[pos] = hex.EncodeToString(privBytes)
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return keys[:int(filled)], err
		}
	}
	if ctx.Err() != nil {
		return keys[:int(filled)], ctx.Err()
	}
	return keys[:int(filled)], nil
}
