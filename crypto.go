package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
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

func randomA32(n int) ([]uint32, error) {
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

func aesEncryptBlock(key, block []byte) ([]byte, error) {
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

// megaJSONString matches JSON.stringify for the attribute strings transfer.it encrypts.
func megaJSONString(s string) string {
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

func fileAttrJSON(name string) string {
	return `{"n":` + megaJSONString(name) + `}`
}

func folderAttrJSON(name string, mtime int64) string {
	return `{"t":` + fmt.Sprintf("%d", mtime) + `,"n":` + megaJSONString(name) + `}`
}

// encryptAttr encrypts a MEGA attribute object with AES-CBC, zero IV, and zero padding.
// key is the 4-word attribute key.
func encryptAttr(jsonBody string, key []uint32) (string, error) {
	if len(key) < 4 {
		return "", fmt.Errorf("attribute key must be 4 words")
	}
	plain := []byte("MEGA" + jsonBody)
	if rem := len(plain) % aes.BlockSize; rem != 0 {
		plain = append(plain, make([]byte, aes.BlockSize-rem)...)
	}
	block, err := aes.NewCipher(a32ToBytes(key[:4]))
	if err != nil {
		return "", err
	}
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(out, plain)
	return base64URLEncode(out), nil
}

// encryptChunk applies MEGA's AES-CTR plus chunk CBC-MAC.
// counter is the 64-bit block index at the start of plain and is advanced past this chunk.
func encryptChunk(block cipher.Block, nonce []byte, counter *uint64, plain []byte) (ciphertext, mac []byte) {
	ciphertext = make([]byte, len(plain))
	mac = make([]byte, aes.BlockSize)
	copy(mac[:8], nonce)
	copy(mac[8:], nonce)
	ctrBlock := make([]byte, aes.BlockSize)
	stream := make([]byte, aes.BlockSize)
	padded := make([]byte, aes.BlockSize)
	xored := make([]byte, aes.BlockSize)

	for off := 0; off < len(plain); {
		n := copy(padded, plain[off:])
		for i := n; i < aes.BlockSize; i++ {
			padded[i] = 0
		}

		copy(ctrBlock[:8], nonce)
		binary.BigEndian.PutUint64(ctrBlock[8:], *counter)
		block.Encrypt(stream, ctrBlock)
		if n > 0 {
			for i := 0; i < n; i++ {
				ciphertext[off+i] = plain[off+i] ^ stream[i]
			}
		}

		for i := 0; i < aes.BlockSize; i++ {
			xored[i] = mac[i] ^ padded[i]
		}
		block.Encrypt(mac, xored)
		*counter++
		off += n
		if n == 0 {
			break
		}
	}
	return ciphertext, mac
}

// condenseMacs folds per-chunk MACs into the 16-byte file MAC.
func condenseMacs(block cipher.Block, macs [][]byte) []byte {
	mac := make([]byte, aes.BlockSize)
	xored := make([]byte, aes.BlockSize)
	for _, chunk := range macs {
		for i := 0; i < aes.BlockSize; i++ {
			xored[i] = mac[i] ^ chunk[i]
		}
		block.Encrypt(mac, xored)
	}
	return mac
}

// buildFileKey produces the 8-word key stored with a transfer.it file node.
func buildFileKey(ulKey []uint32, mac []byte) []uint32 {
	m0 := binary.BigEndian.Uint32(mac[0:4])
	m1 := binary.BigEndian.Uint32(mac[4:8])
	m2 := binary.BigEndian.Uint32(mac[8:12])
	m3 := binary.BigEndian.Uint32(mac[12:16])
	meta0 := m0 ^ m1
	meta1 := m2 ^ m3
	return []uint32{
		ulKey[0] ^ ulKey[4],
		ulKey[1] ^ ulKey[5],
		ulKey[2] ^ meta0,
		ulKey[3] ^ meta1,
		ulKey[4],
		ulKey[5],
		meta0,
		meta1,
	}
}
