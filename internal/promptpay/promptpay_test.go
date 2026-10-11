package promptpay

import (
	"strings"
	"testing"
)

func TestCRC16KnownVector(t *testing.T) {
	// Standard check value for CRC-16/CCITT-FALSE.
	if got := crc16("123456789"); got != "29B1" {
		t.Fatalf("crc16 = %s, want 29B1", got)
	}
}

func TestPayloadPhoneWithAmount(t *testing.T) {
	p, err := Payload("081-234-5678", 120.5)
	if err != nil {
		t.Fatal(err)
	}
	want := "00020101021229370016A000000677010111011300668123456785802TH5303764" + "5406120.50" + "6304"
	if !strings.HasPrefix(p, "000201010212") {
		t.Fatalf("expected dynamic payload, got %s", p)
	}
	if !strings.HasPrefix(p, want[:len(want)]) {
		t.Fatalf("payload = %s\nwant prefix %s", p, want)
	}
	body, crc := p[:len(p)-4], p[len(p)-4:]
	if crc16(body) != crc {
		t.Fatalf("crc mismatch: %s", crc)
	}
}

func TestPayloadNoAmountIsStatic(t *testing.T) {
	p, err := Payload("0812345678", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, "000201010211") || strings.Contains(p, "5406") {
		t.Fatalf("unexpected static payload: %s", p)
	}
}

func TestPayloadNationalID(t *testing.T) {
	p, err := Payload("1234567890123", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, "02131234567890123") {
		t.Fatalf("national id tag missing: %s", p)
	}
}

func TestPayloadRejectsBadTarget(t *testing.T) {
	for _, bad := range []string{"", "12345", "08123abc78", "081234567890"} {
		if _, err := Payload(bad, 10); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
