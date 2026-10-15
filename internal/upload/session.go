package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"transferit/internal/api"
	"transferit/internal/crypto"
)

func (u *Uploader) createSession(ctx context.Context) error {
	master, err := crypto.RandomA32(4)
	if err != nil {
		return err
	}
	passwordKey, err := crypto.RandomA32(4)
	if err != nil {
		return err
	}
	ssc, err := crypto.RandomA32(4)
	if err != nil {
		return err
	}
	masterBytes := crypto.A32ToBytes(master)
	sscBytes := crypto.A32ToBytes(ssc)
	encMaster, err := crypto.AESEncryptBlock(crypto.A32ToBytes(passwordKey), masterBytes)
	if err != nil {
		return err
	}
	encSSC, err := crypto.AESEncryptBlock(masterBytes, sscBytes)
	if err != nil {
		return err
	}
	ts := append(append([]byte{}, sscBytes...), encSSC...)

	raw, err := u.call(ctx, api.G, false, []map[string]string{{
		"a":  "up",
		"k":  crypto.Base64URLEncode(encMaster),
		"ts": crypto.Base64URLEncode(ts),
	}})
	if err != nil {
		return err
	}
	handle, err := parseUserHandle(raw)
	if err != nil {
		return fmt.Errorf("creating upload account: %w", err)
	}
	u.log("ephemeral user %s", handle)

	raw, err = u.call(ctx, api.G, false, []map[string]string{{
		"a":    "us",
		"user": handle,
	}})
	if err != nil {
		return err
	}
	session, err := ensureOK(raw)
	if err != nil {
		return fmt.Errorf("starting session: %w", err)
	}
	var us struct {
		TSID string `json:"tsid"`
	}
	if err := json.Unmarshal(session, &us); err != nil || us.TSID == "" {
		return fmt.Errorf("session response missing tsid: %s", api.Truncate(string(session), 180))
	}
	if err := verifySession(masterBytes, us.TSID); err != nil {
		return err
	}
	u.sid = us.TSID

	if _, err := u.call(ctx, api.G, false, []map[string]string{{"a": "ug"}}); err != nil {
		return fmt.Errorf("loading account: %w", err)
	}
	return nil
}

func parseUserHandle(raw json.RawMessage) (string, error) {
	body, err := ensureOK(raw)
	if err != nil {
		return "", err
	}
	var found []string
	collectStrings(body, &found)
	collectStrings(raw, &found)
	for _, s := range found {
		if len(s) == 11 {
			return s, nil
		}
	}
	return "", fmt.Errorf("unexpected account response: %s", api.Truncate(string(raw), 180))
}

func verifySession(masterKey []byte, tsid string) error {
	raw, err := crypto.Base64URLDecode(tsid)
	if err != nil {
		return fmt.Errorf("decoding session: %w", err)
	}
	if len(raw) < 32 {
		return fmt.Errorf("session token is too short")
	}
	got, err := crypto.AESEncryptBlock(masterKey, raw[:16])
	if err != nil {
		return err
	}
	if !bytes.Equal(got, raw[len(raw)-16:]) {
		return fmt.Errorf("session challenge did not match")
	}
	return nil
}
