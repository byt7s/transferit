package config

import (
	"crypto/rand"
	"strings"
)

func ParsePassword(v string) (mode, password string) {
	trimmed := strings.TrimSpace(v)
	switch strings.ToLower(trimmed) {
	case "", "none", "off", "no":
		return "none", ""
	case "random", "rand":
		return "random", ""
	default:
		return "fixed", trimmed
	}
}

func RandomPassword(n int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
