package upload

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"strings"

	"transferit/internal/crypto"
)

type uploadMsg struct {
	fileno uint32
	pos    uint64
	typ    int8
	token  []byte
}

func parseUploadMsg(data []byte) (uploadMsg, error) {
	if len(data) < 17 {
		return uploadMsg{}, fmt.Errorf("short upload reply (%d bytes)", len(data))
	}
	body := data[:len(data)-4]
	want := binary.LittleEndian.Uint32(data[len(data)-4:])
	if crc32.ChecksumIEEE(body) != want {
		return uploadMsg{}, fmt.Errorf("upload reply checksum mismatch")
	}
	msg := uploadMsg{
		fileno: binary.LittleEndian.Uint32(body[0:4]),
		pos:    binary.LittleEndian.Uint64(body[4:12]),
		typ:    int8(body[12]),
	}
	if msg.typ == 4 {
		if len(body) < 14 {
			return uploadMsg{}, fmt.Errorf("short completion reply")
		}
		n := int(body[13])
		if len(body) < 14+n {
			return uploadMsg{}, fmt.Errorf("completion token truncated")
		}
		msg.token = append([]byte(nil), body[14:14+n]...)
	}
	return msg, nil
}

func completionHandle(token []byte) (string, error) {
	if len(token) == 36 {
		if token[35] == 1 {
			return crypto.Base64URLEncode(token), nil
		}
		return strings.TrimRight(string(token), "\x00"), nil
	}
	if len(token) > 0 && isASCIITicket(token) {
		return strings.TrimRight(string(token), "\x00"), nil
	}
	return "", fmt.Errorf("unexpected upload ticket (%d bytes)", len(token))
}

func isASCIITicket(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

func chunkHeader(fileno uint32, pos int64, chunk []byte) []byte {
	header := make([]byte, 20)
	binary.LittleEndian.PutUint32(header[0:4], fileno)
	binary.LittleEndian.PutUint64(header[4:12], uint64(pos))
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(chunk)))
	sum := crc32.Update(0, crc32.IEEETable, header[:16])
	sum = crc32.Update(sum, crc32.IEEETable, chunk)
	binary.LittleEndian.PutUint32(header[16:20], sum)
	return header
}
