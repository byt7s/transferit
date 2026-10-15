package config

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseExpiry(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "0", "never", "none", "off", "unlimited":
		return 0, nil
	}
	mult := 86400
	num := s
	if c := s[len(s)-1]; c == 'd' || c == 'h' || c == 'm' || c == 's' {
		num = s[:len(s)-1]
		switch c {
		case 'd':
			mult = 86400
		case 'h':
			mult = 3600
		case 'm':
			mult = 60
		default:
			mult = 1
		}
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid expiry %q (examples: 1d, 7d, 30d, 90d, 12h, 0)", s)
	}
	seconds := n * mult
	if n > 0 && seconds/n != mult {
		return 0, fmt.Errorf("expiry too large")
	}
	return seconds, nil
}

func CanonicalExpiry(s string) (string, error) {
	sec, err := ParseExpiry(s)
	if err != nil {
		return "", err
	}
	if sec == 0 {
		return "0", nil
	}
	if sec%86400 == 0 {
		return fmt.Sprintf("%dd", sec/86400), nil
	}
	if sec%3600 == 0 {
		return fmt.Sprintf("%dh", sec/3600), nil
	}
	if sec%60 == 0 {
		return fmt.Sprintf("%dm", sec/60), nil
	}
	return fmt.Sprintf("%ds", sec), nil
}

func ValidateExpiry(seconds int) error {
	const max = 90 * 86400
	if seconds < 1 || seconds > max {
		return fmt.Errorf("expiry must be from 1 second up to 90 days; unlimited, 180d and 365d are refused for these uploads")
	}
	return nil
}

func ExpiryPhrase(seconds int) string {
	if seconds <= 0 {
		return "never"
	}
	if seconds%86400 == 0 {
		d := seconds / 86400
		if d == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", d)
	}
	if seconds%3600 == 0 {
		h := seconds / 3600
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	if seconds%60 == 0 {
		m := seconds / 60
		if m == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", m)
	}
	if seconds == 1 {
		return "1 second"
	}
	return fmt.Sprintf("%d seconds", seconds)
}
