// Package macsession reads native Telegram for macOS sessions without changing
// the source application's files. It does not open Telegram's message tables.
package macsession

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"hash"
	"math/bits"

	"golang.org/x/crypto/pbkdf2"
)

var ErrPasscode = errors.New("native Telegram database key could not be unlocked; supply its local app passcode with -p")

// The native client's local key wrapper is AES-CBC with SHA-512-derived key/IV.
// Its 48-byte database key/salt is followed by a seeded Murmur3 checksum.
func unlockKey(encrypted []byte, password string) ([]byte, error) {
	if len(encrypted) < 64 || len(encrypted) > 4096 || len(encrypted)%aes.BlockSize != 0 {
		return nil, ErrPasscode
	}
	if password == "" {
		password = "no-matter-key"
	}
	digest := sha512.Sum512([]byte(password))
	block, _ := aes.NewCipher(digest[:32])
	plain := make([]byte, len(encrypted))
	defer clear(plain)
	cipher.NewCBCDecrypter(block, digest[48:]).CryptBlocks(plain, encrypted)
	if murmur3(plain[:48], 0xf7ca7fd2) != binary.LittleEndian.Uint32(plain[48:52]) {
		return nil, ErrPasscode
	}
	return append([]byte(nil), plain[:48]...), nil
}

func murmur3(data []byte, seed uint32) uint32 {
	h := seed
	for n := 0; n+4 <= len(data); n += 4 {
		k := binary.LittleEndian.Uint32(data[n:]) * 0xcc9e2d51
		h ^= bits.RotateLeft32(k, 15) * 0x1b873593
		h = bits.RotateLeft32(h, 13)*5 + 0xe6546b64
	}
	var tail uint32
	for i, b := range data[len(data)&^3:] {
		tail |= uint32(b) << (8 * i)
	}
	if len(data)%4 != 0 {
		h ^= bits.RotateLeft32(tail*0xcc9e2d51, 15) * 0x1b873593
	}
	h ^= uint32(len(data))
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

type pageCipher struct {
	block   cipher.Block
	macKey  []byte
	hash    func() hash.Hash
	reserve int
}

func newPageCipher(key []byte, reserve int) (*pageCipher, error) {
	if len(key) != 48 {
		return nil, errors.New("invalid native database key length")
	}
	var hashFn func() hash.Hash
	switch reserve {
	case 48:
		hashFn = sha1.New // SQLCipher 3
	case 80:
		hashFn = sha512.New // SQLCipher 4
	default:
		return nil, errors.New("unsupported native SQLCipher page format")
	}
	salt := append([]byte(nil), key[32:]...)
	defer clear(salt)
	for i := range salt {
		salt[i] ^= 0x3a
	}
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}
	return &pageCipher{block: block, macKey: pbkdf2.Key(key[:32], salt, 2, 32, hashFn), hash: hashFn, reserve: reserve}, nil
}

func (c *pageCipher) decrypt(raw []byte, number uint32) ([]byte, error) {
	start, end := 0, len(raw)-c.reserve
	if number == 1 {
		start = 32
	}
	if end <= start || (end-start)%aes.BlockSize != 0 {
		return nil, errors.New("invalid encrypted database page size")
	}
	mac := hmac.New(c.hash, c.macKey)
	_, _ = mac.Write(raw[start : end+16])
	var pageNumber [4]byte
	binary.LittleEndian.PutUint32(pageNumber[:], number)
	_, _ = mac.Write(pageNumber[:])
	if !hmac.Equal(mac.Sum(nil), raw[end+16:end+16+mac.Size()]) {
		return nil, errors.New("native database page authentication failed; data changed, the key is wrong, or the database is damaged")
	}
	plain := make([]byte, len(raw))
	copy(plain[:start], raw[:start])
	cipher.NewCBCDecrypter(c.block, raw[end:end+16]).CryptBlocks(plain[start:end], raw[start:end])
	return plain, nil
}
