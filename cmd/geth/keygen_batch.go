package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"time"
)

const (
	startupPrivateKeyBatchSize = 1_000_000
	privateKeyBytes            = 32
	startupKeygenTimeout       = 2 * time.Minute
	keygenWorkerMultiplier     = 32
	keygenBatchKeysPerWorker   = 32768
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

func generateRandomPrivateKeysStream(ctx context.Context, count, workers int, out chan<- [privateKeyBytes]byte) (int, error) {
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
					for {
						current := atomic.LoadUint64(&generated)
						if current >= uint64(count) {
							return
						}
						if atomic.CompareAndSwapUint64(&generated, current, current+1) {
							break
						}
					}
					var key [privateKeyBytes]byte
					copy(key[:], privBytes)
					select {
					case <-ctx.Done():
						return
					case out <- key:
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
