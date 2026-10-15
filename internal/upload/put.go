package upload

import (
	"context"
	"crypto/aes"
	"encoding/binary"
	"fmt"
	"os"

	"transferit/internal/api"
	"transferit/internal/crypto"
)

func (u *Uploader) putFile(ctx context.Context, root, path, name string, onBytes func(int64)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a file", path)
	}
	size := info.Size()

	pools, err := u.uploadPools(ctx)
	if err != nil {
		return err
	}
	pool, err := choosePool(pools, size)
	if err != nil {
		return err
	}
	wsURL := "wss://" + pool.host + "/" + pool.uri
	u.log("upload %s (%d bytes) via %s", name, size, pool.host)

	ulKey, err := crypto.RandomA32(6)
	if err != nil {
		return err
	}
	key := crypto.A32ToBytes(ulKey[:4])
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, 8)
	binary.BigEndian.PutUint32(nonce[:4], ulKey[4])
	binary.BigEndian.PutUint32(nonce[4:], ulKey[5])

	token, macs, err := u.sendParallel(ctx, f, size, wsURL, key, nonce, onBytes)
	if err != nil {
		return err
	}
	handle, err := completionHandle(token)
	if err != nil {
		return err
	}

	fileKey := crypto.BuildFileKey(ulKey, crypto.CondenseMacs(block, macs))
	attrs, err := crypto.EncryptAttr(crypto.FileAttrJSON(name), ulKey[:4])
	if err != nil {
		return err
	}
	raw, err := u.call(ctx, api.BT7, true, []map[string]any{{
		"a": "xp",
		"v": 3,
		"t": root,
		"n": []map[string]any{{
			"t": 0,
			"h": handle,
			"a": attrs,
			"k": crypto.Base64URLEncode(crypto.A32ToBytes(fileKey)),
		}},
	}})
	if err != nil {
		return err
	}
	if _, err := ensureOK(raw); err != nil {
		return fmt.Errorf("saving %s: %w", name, err)
	}
	return nil
}

func naturalChunkLen(pos int64) int64 {
	var p, dp int64
	for dp < 1<<20 {
		dp += 1 << 17
		if p == pos {
			return dp
		}
		p += dp
	}
	return 1 << 20
}

func fileChunks(size int64) (parts [][2]int64, trailingEmpty bool) {
	var pos int64
	for pos < size {
		natural := naturalChunkLen(pos)
		n := natural
		if pos+n > size {
			n = size - pos
		}
		parts = append(parts, [2]int64{pos, n})
		pos += n
		if n < natural {
			return parts, false
		}
	}
	return parts, true
}
