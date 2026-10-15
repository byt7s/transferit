package upload

import (
	"bytes"
	"crypto/aes"
	"encoding/binary"
	"testing"

	"transferit/internal/crypto"
)

func TestPipeRequeueSkipsAckedChunks(t *testing.T) {
	state := newPipe([]piece{{pos: 0, n: 16}, {pos: 16, n: 16}, {pos: 32, n: 8}}, 3)
	first, ok := state.take()
	if !ok || first.pos != 0 {
		t.Fatalf("first = %+v %v", first, ok)
	}
	second, ok := state.take()
	if !ok || second.pos != 16 {
		t.Fatalf("second = %+v", second)
	}
	if got := state.ack(first.pos, first.n); got != 16 {
		t.Fatalf("ack = %d", got)
	}
	if got := state.ack(first.pos, first.n); got != 0 {
		t.Fatalf("duplicate ack = %d", got)
	}
	state.requeue([]piece{first, second})
	got, ok := state.take()
	if !ok || got.pos != 16 {
		t.Fatalf("requeued = %+v", got)
	}
	state.complete([]byte("token"))
	state.requeue([]piece{{pos: 99, n: 1}})
	if _, ok := state.take(); ok {
		t.Fatal("queue accepted work after completion")
	}
}

func TestEncryptAtMatchesSequential(t *testing.T) {
	ul := []uint32{0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x55555555, 0x66666666}
	plain := bytes.Repeat([]byte("hello transferit\n"), 8000)
	block, err := aes.NewCipher(crypto.A32ToBytes(ul[:4]))
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 8)
	binary.BigEndian.PutUint32(nonce[:4], ul[4])
	binary.BigEndian.PutUint32(nonce[4:], ul[5])

	var counter uint64
	sequential := make([]byte, len(plain))
	seqMAC := map[int64][]byte{}
	parts, _ := fileChunks(int64(len(plain)))
	off := 0
	for _, part := range parts {
		if part[0]%aes.BlockSize != 0 {
			t.Fatalf("chunk %d is not block aligned", part[0])
		}
		chunk, mac := crypto.EncryptChunk(block, nonce, &counter, plain[off:off+int(part[1])])
		copy(sequential[off:], chunk)
		seqMAC[part[0]] = mac
		off += int(part[1])
	}

	parallel := make([]byte, len(plain))
	for _, part := range parts {
		chunk, mac := encryptAt(block, nonce, part[0], plain[part[0]:part[0]+part[1]])
		copy(parallel[part[0]:], chunk)
		if !bytes.Equal(mac, seqMAC[part[0]]) {
			t.Fatalf("mac mismatch at %d", part[0])
		}
	}
	if !bytes.Equal(parallel, sequential) {
		t.Fatal("parallel encryption diverged")
	}
}
