// Package promptpay builds the EMVCo payload behind a Thai PromptPay QR code.
// The app turns the returned string into a QR image; no bank connection is involved.
package promptpay

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidTarget = errors.New("promptpay id must be a 10-digit phone number or a 13-digit national/tax id")

// Payload returns the QR text for paying amountBaht to target.
// target is a Thai mobile number (0812345678) or a 13-digit national/tax id;
// dashes and spaces are ignored. amountBaht <= 0 produces a reusable QR with no amount.
func Payload(target string, amountBaht float64) (string, error) {
	acct, err := account(target)
	if err != nil {
		return "", err
	}

	method := "11" // static: payer types the amount
	if amountBaht > 0 {
		method = "12" // dynamic: amount is fixed
	}

	merchant := field("00", "A000000677010111") + acct

	out := field("00", "01") +
		field("01", method) +
		field("29", merchant) +
		field("58", "TH") +
		field("53", "764")
	if amountBaht > 0 {
		out += field("54", fmt.Sprintf("%.2f", amountBaht))
	}
	out += "6304"
	return out + crc16(out), nil
}

func account(target string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '-' || r == ' ' {
			return -1
		}
		return '?'
	}, target)
	if strings.Contains(digits, "?") {
		return "", ErrInvalidTarget
	}
	switch len(digits) {
	case 10: // phone: 0812345678 -> 0066812345678
		return field("01", "0066"+digits[1:]), nil
	case 13: // national id / tax id
		return field("02", digits), nil
	}
	return "", ErrInvalidTarget
}

func field(tag, value string) string {
	return fmt.Sprintf("%s%02d%s", tag, len(value), value)
}

// crc16 is CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF), as EMVCo requires.
func crc16(s string) string {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for b := 0; b < 8; b++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return fmt.Sprintf("%04X", crc)
}
