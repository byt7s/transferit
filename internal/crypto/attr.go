package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"strings"
)

func DecryptAttr(attrB64 string, keyA32 []uint32) (map[string]interface{}, error) {
	attrBytes, err := Base64URLDecode(attrB64)
	if err != nil {
		return nil, err
	}

	k := []uint32{
		keyA32[0] ^ keyA32[4],
		keyA32[1] ^ keyA32[5],
		keyA32[2] ^ keyA32[6],
		keyA32[3] ^ keyA32[7],
	}
	aesKey := A32ToBytes(k)

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

// JSONString matches JSON.stringify for the attribute strings transfer.it encrypts.
func JSONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func FileAttrJSON(name string) string {
	return `{"n":` + JSONString(name) + `}`
}

func FolderAttrJSON(name string, mtime int64) string {
	return `{"t":` + fmt.Sprintf("%d", mtime) + `,"n":` + JSONString(name) + `}`
}

// EncryptAttr encrypts a MEGA attribute object with AES-CBC, zero IV, and zero padding.
// key is the 4-word attribute key.
func EncryptAttr(jsonBody string, key []uint32) (string, error) {
	if len(key) < 4 {
		return "", fmt.Errorf("attribute key must be 4 words")
	}
	plain := []byte("MEGA" + jsonBody)
	if rem := len(plain) % aes.BlockSize; rem != 0 {
		plain = append(plain, make([]byte, aes.BlockSize-rem)...)
	}
	block, err := aes.NewCipher(A32ToBytes(key[:4]))
	if err != nil {
		return "", err
	}
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(out, plain)
	return Base64URLEncode(out), nil
}
