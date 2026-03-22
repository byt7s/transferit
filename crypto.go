package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// derivePassword creates the password hash using PBKDF2-SHA256
// xh: transfer handle
// password: plain text password
func derivePassword(xh, password string) string {
	// Decode xh and get last 6 bytes
	decoded, _ := base64URLDecode(xh)
	var xhBytes []byte
	if len(xh) > 12 {
		// Double decode for longer handles
		decoded2, _ := base64URLDecode(string(decoded))
		if len(decoded2) >= 7 {
			xhBytes = decoded2[len(decoded2)-7 : len(decoded2)-1]
		}
	} else {
		if len(decoded) >= 6 {
			xhBytes = decoded[len(decoded)-6:]
		}
	}

	// Create salt by repeating xhBytes 3 times (18 bytes)
	salt := []byte(strings.Repeat(string(xhBytes), 3))

	// PBKDF2-SHA256 with 100000 iterations, 32 bytes output
	key := pbkdf2.Key([]byte(strings.TrimSpace(password)), salt, 100000, 32, sha256.New)

	return base64URLEncode(key)
}

func bytesToA32(b []byte) []uint32 {
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

func a32ToBytes(a []uint32) []byte {
	b := make([]byte, len(a)*4)
	for i, v := range a {
		binary.BigEndian.PutUint32(b[i*4:], v)
	}
	return b
}

func base64ToA32(s string) ([]uint32, error) {
	b, err := base64URLDecode(s)
	if err != nil {
		return nil, err
	}
	return bytesToA32(b), nil
}

func decryptAttr(attrB64 string, keyA32 []uint32) (map[string]interface{}, error) {
	attrBytes, err := base64URLDecode(attrB64)
	if err != nil {
		return nil, err
	}

	k := []uint32{
		keyA32[0] ^ keyA32[4],
		keyA32[1] ^ keyA32[5],
		keyA32[2] ^ keyA32[6],
		keyA32[3] ^ keyA32[7],
	}
	aesKey := a32ToBytes(k)

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}

	if rem := len(attrBytes) % aes.BlockSize; rem != 0 {
		padded := make([]byte, len(attrBytes)+(aes.BlockSize-rem))
		copy(padded, attrBytes)
		attrBytes = padded
	}

	iv := make([]byte, 16)
	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(attrBytes))
	mode.CryptBlocks(plaintext, attrBytes)

	plaintext = bytes.TrimRight(plaintext, "\x00")
	text := string(plaintext)
	if !strings.HasPrefix(text, `MEGA{"`) {
		return nil, fmt.Errorf("invalid MEGA canary")
	}

	jsonStr := text[4:]
	depth := 0
	end := 0
	for i, ch := range jsonStr {
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end > 0 {
			break
		}
	}
	if end == 0 {
		return nil, fmt.Errorf("could not find JSON end")
	}

	var attrs map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr[:end]), &attrs); err != nil {
		return nil, err
	}
	return attrs, nil
}
