package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
)

// EncryptChunk applies MEGA's AES-CTR plus chunk CBC-MAC.
// counter is the 64-bit block index at the start of plain and is advanced past this chunk.
func EncryptChunk(block cipher.Block, nonce []byte, counter *uint64, plain []byte) (ciphertext, mac []byte) {
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

// CondenseMacs folds per-chunk MACs into the 16-byte file MAC.
func CondenseMacs(block cipher.Block, macs [][]byte) []byte {
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

// BuildFileKey produces the 8-word key stored with a transfer.it file node.
func BuildFileKey(ulKey []uint32, mac []byte) []uint32 {
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
