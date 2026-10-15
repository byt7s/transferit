// Package crypto implements the MEGA-style keys transfer.it uses for files and sessions.
package crypto

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

func Base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func Base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// DerivePassword hashes a transfer password with PBKDF2-SHA256.
func DerivePassword(xh, password string) string {
	decoded, _ := Base64URLDecode(xh)
	var xhBytes []byte
	if len(xh) > 12 {
		decoded2, _ := Base64URLDecode(string(decoded))
		if len(decoded2) >= 7 {
			xhBytes = decoded2[len(decoded2)-7 : len(decoded2)-1]
		}
	} else {
		if len(decoded) >= 6 {
			xhBytes = decoded[len(decoded)-6:]
		}
	}

	salt := []byte(strings.Repeat(string(xhBytes), 3))
	key := pbkdf2.Key([]byte(strings.TrimSpace(password)), salt, 100000, 32, sha256.New)
	return Base64URLEncode(key)
}

func BytesToA32(b []byte) []uint32 {
	if rem := len(b) % 4; rem != 0 {
		padded := make([]byte, len(b)+(4-rem))
		copy(padded, b)
		b = padded
	}
	result := make([]uint32, len(b)/4)
	for i := range result {
		result[i] = binary.BigEndian.Uint32(b[i*4 : (i+1)*4])
	}
	return result
}

func A32ToBytes(a []uint32) []byte {
	b := make([]byte, len(a)*4)
	for i, v := range a {
		binary.BigEndian.PutUint32(b[i*4:], v)
	}
	return b
}

func Base64ToA32(s string) ([]uint32, error) {
	b, err := Base64URLDecode(s)
	if err != nil {
		return nil, err
	}
	return BytesToA32(b), nil
}

func RandomA32(n int) ([]uint32, error) {
	buf := make([]byte, n*4)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.BigEndian.Uint32(buf[i*4:])
	}
	return out, nil
}

func AESEncryptBlock(key, block []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(block))
	for i := 0; i < len(block); i += aes.BlockSize {
		c.Encrypt(out[i:i+aes.BlockSize], block[i:i+aes.BlockSize])
	}
	return out, nil
}
