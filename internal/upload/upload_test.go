package upload

import (
	"bytes"
	"crypto/aes"
	"encoding/binary"
	"testing"

	"transferit/internal/crypto"
)

func TestFileChunks(t *testing.T) {
	for _, size := range []int64{0, 1, 100, 131072, 131073, 200000, 393216, 5 << 20} {
		parts, trailing := fileChunks(size)
		var sum int64
		var pos int64
		for _, part := range parts {
			if part[0] != pos {
				t.Fatalf("size %d: chunk at %d, want %d", size, part[0], pos)
			}
			if part[1] <= 0 {
				t.Fatalf("size %d: empty data chunk", size)
			}
			sum += part[1]
			pos += part[1]
		}
		if sum != size {
			t.Fatalf("size %d: chunks sum to %d", size, sum)
		}
		exact := size == 0 || size == 131072 || size == 393216
		if trailing != exact {
			t.Fatalf("size %d: trailing empty = %v", size, trailing)
		}
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	ul := []uint32{0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x55555555, 0x66666666}
	plain := bytes.Repeat([]byte("hello transferit\n"), 8000)
	ciphertext, fileKey := encryptTestFile(t, ul, plain)

	block, err := aes.NewCipher(crypto.A32ToBytes(ul[:4]))
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 8)
	binary.BigEndian.PutUint32(nonce[:4], ul[4])
	binary.BigEndian.PutUint32(nonce[4:], ul[5])
	var counter uint64
	got := make([]byte, len(ciphertext))
	off := 0
	parts, _ := fileChunks(int64(len(ciphertext)))
	for _, part := range parts {
		chunk, _ := crypto.EncryptChunk(block, nonce, &counter, ciphertext[off:off+int(part[1])])
		copy(got[off:], chunk)
		off += int(part[1])
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("ciphertext did not decrypt back to the original")
	}

	for i := 0; i < 4; i++ {
		if fileKey[i]^fileKey[i+4] != ul[i] {
			t.Fatalf("file key word %d does not recover the AES key", i)
		}
	}
	attr, err := crypto.EncryptAttr(crypto.FileAttrJSON("arquivo-ção.txt"), ul[:4])
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := crypto.DecryptAttr(attr, fileKey)
	if err != nil {
		t.Fatal(err)
	}
	if decoded["n"] != "arquivo-ção.txt" {
		t.Fatalf("name = %#v", decoded["n"])
	}
}

func encryptTestFile(t *testing.T, ul []uint32, plain []byte) ([]byte, []uint32) {
	t.Helper()
	block, err := aes.NewCipher(crypto.A32ToBytes(ul[:4]))
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 8)
	binary.BigEndian.PutUint32(nonce[:4], ul[4])
	binary.BigEndian.PutUint32(nonce[4:], ul[5])
	var counter uint64
	var macs [][]byte
	out := make([]byte, len(plain))
	off := 0
	parts, _ := fileChunks(int64(len(plain)))
	for _, part := range parts {
		chunk, mac := crypto.EncryptChunk(block, nonce, &counter, plain[off:off+int(part[1])])
		copy(out[off:], chunk)
		macs = append(macs, mac)
		off += int(part[1])
	}
	return out, crypto.BuildFileKey(ul, crypto.CondenseMacs(block, macs))
}

func TestCompletionHandle(t *testing.T) {
	token := bytes.Repeat([]byte{0x11}, 36)
	token[35] = 1
	got, err := completionHandle(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != crypto.Base64URLEncode(token) {
		t.Fatalf("handle = %s", got)
	}
	ascii := []byte("Fq6dbkuoEt9hFPCtjOR7")
	got, err = completionHandle(ascii)
	if err != nil || got != string(ascii) {
		t.Fatalf("ascii handle = %q %v", got, err)
	}
}
