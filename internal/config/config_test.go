package config

import "testing"

func TestParseExpiry(t *testing.T) {
	sec, err := ParseExpiry("90d")
	if err != nil || sec != 90*86400 {
		t.Fatalf("90d = %d %v", sec, err)
	}
	sec, err = ParseExpiry("7")
	if err != nil || sec != 7*86400 {
		t.Fatalf("7 = %d %v", sec, err)
	}
	sec, err = ParseExpiry("12h")
	if err != nil || sec != 12*3600 {
		t.Fatalf("12h = %d %v", sec, err)
	}
	sec, err = ParseExpiry("never")
	if err != nil || sec != 0 {
		t.Fatalf("never = %d %v", sec, err)
	}
	if _, err := ParseExpiry("nope"); err == nil {
		t.Fatal("expected an error")
	}
	if err := ValidateExpiry(0); err == nil {
		t.Fatal("expected unlimited expiry to be rejected")
	}
	if err := ValidateExpiry(90 * 86400); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExpiry(91 * 86400); err == nil {
		t.Fatal("expected more than 90 days to be rejected")
	}
	canon, err := CanonicalExpiry("48h")
	if err != nil || canon != "2d" {
		t.Fatalf("canonical = %s %v", canon, err)
	}
}

func TestPasswordChoice(t *testing.T) {
	mode, pw := ParsePassword("  Random ")
	if mode != "random" || pw != "" {
		t.Fatalf("%s %q", mode, pw)
	}
	mode, pw = ParsePassword("none")
	if mode != "none" || pw != "" {
		t.Fatalf("%s %q", mode, pw)
	}
	mode, pw = ParsePassword("s3cret")
	if mode != "fixed" || pw != "s3cret" {
		t.Fatalf("%s %q", mode, pw)
	}
}
