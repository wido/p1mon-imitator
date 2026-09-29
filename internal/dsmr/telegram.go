// Package dsmr parses P1 telegrams as defined by the Dutch Smart Meter
// Requirements (DSMR) 2.2 through 5.0.
package dsmr

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Telegram holds the electricity-related values of one P1 telegram. Values that
// a meter does not report stay zero; single-phase meters, for example, never
// fill L2 and L3.
type Telegram struct {
	// Identification is the header line without the leading '/', e.g. "KFM5KAIFA-METER".
	Identification string
	// Version is the raw value of OBIS 1-3:0.2.8 ("42" for DSMR 4.2, "50" for
	// DSMR 5.0). Empty for DSMR 2.x/3.x meters, which do not send it.
	Version string
	// Timestamp is OBIS 0-0:1.0.0 interpreted in the local time zone. Zero if absent.
	Timestamp time.Time

	// Tariff is OBIS 0-0:96.14.0: 1 = low (dal), 2 = high (piek).
	Tariff int

	// Cumulative energy in kWh.
	EnergyDeliveredTariff1 float64 // 1-0:1.8.1, consumed, low tariff
	EnergyDeliveredTariff2 float64 // 1-0:1.8.2, consumed, high tariff
	EnergyReturnedTariff1  float64 // 1-0:2.8.1, produced, low tariff
	EnergyReturnedTariff2  float64 // 1-0:2.8.2, produced, high tariff

	// Instantaneous totals in kW.
	PowerDelivered float64 // 1-0:1.7.0
	PowerReturned  float64 // 1-0:2.7.0

	// Per phase instantaneous values (DSMR 4+; voltage is DSMR 5 only).
	VoltageL1, VoltageL2, VoltageL3                      float64 // V
	CurrentL1, CurrentL2, CurrentL3                      float64 // A
	PowerDeliveredL1, PowerDeliveredL2, PowerDeliveredL3 float64 // kW
	PowerReturnedL1, PowerReturnedL2, PowerReturnedL3    float64 // kW
}

// ErrCRC is returned when a telegram carries a checksum that does not match.
var ErrCRC = errors.New("dsmr: crc mismatch")

// ErrMalformed is returned when the bytes do not look like a telegram at all.
var ErrMalformed = errors.New("dsmr: malformed telegram")

// Parse decodes one complete telegram: from the '/' header line up to and
// including the '!' terminator line. DSMR 4/5 telegrams carry a CRC after the
// '!' and it is verified; DSMR 2/3 telegrams have no CRC and are accepted as is.
func Parse(raw []byte) (Telegram, error) {
	var t Telegram

	start := bytes.IndexByte(raw, '/')
	if start < 0 {
		return t, ErrMalformed
	}
	end := bytes.LastIndexByte(raw, '!')
	if end < start {
		return t, ErrMalformed
	}

	// Verify the checksum when present. It covers '/' through '!' inclusive.
	crcText := strings.TrimSpace(string(raw[end+1:]))
	if len(crcText) == 4 {
		want, err := strconv.ParseUint(crcText, 16, 16)
		if err != nil {
			return t, fmt.Errorf("%w: bad crc %q", ErrMalformed, crcText)
		}
		if got := CRC16(raw[start : end+1]); got != uint16(want) {
			return t, fmt.Errorf("%w: got %04X want %04X", ErrCRC, got, want)
		}
	} else if len(crcText) != 0 {
		return t, fmt.Errorf("%w: unexpected trailer %q", ErrMalformed, crcText)
	}

	body := raw[start:end]
	first := true
	for len(body) > 0 {
		var line []byte
		if i := bytes.IndexByte(body, '\n'); i >= 0 {
			line, body = body[:i], body[i+1:]
		} else {
			line, body = body, nil
		}
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			continue
		}
		if first {
			first = false
			if line[0] != '/' {
				return t, ErrMalformed
			}
			t.Identification = string(line[1:])
			continue
		}
		if err := t.apply(line); err != nil {
			return t, err
		}
	}
	return t, nil
}

// apply decodes one data line, e.g. "1-0:1.8.1(000123.456*kWh)".
func (t *Telegram) apply(line []byte) error {
	open := bytes.IndexByte(line, '(')
	if open < 0 {
		// Continuation lines of the gas/event log or garbage; ignore.
		return nil
	}
	obis := string(line[:open])
	values := line[open:]

	// first returns the content of the first "(...)" group.
	first := func() string {
		if len(values) == 0 || values[0] != '(' {
			return ""
		}
		close := bytes.IndexByte(values, ')')
		if close < 0 {
			return ""
		}
		return string(values[1:close])
	}

	var dst *float64
	switch obis {
	case "1-3:0.2.8":
		t.Version = first()
		return nil
	case "0-0:1.0.0":
		t.Timestamp = parseTimestamp(first())
		return nil
	case "0-0:96.14.0":
		n, err := strconv.Atoi(first())
		if err != nil {
			return fmt.Errorf("%w: tariff %q", ErrMalformed, first())
		}
		t.Tariff = n
		return nil
	case "1-0:1.8.1":
		dst = &t.EnergyDeliveredTariff1
	case "1-0:1.8.2":
		dst = &t.EnergyDeliveredTariff2
	case "1-0:2.8.1":
		dst = &t.EnergyReturnedTariff1
	case "1-0:2.8.2":
		dst = &t.EnergyReturnedTariff2
	case "1-0:1.7.0":
		dst = &t.PowerDelivered
	case "1-0:2.7.0":
		dst = &t.PowerReturned
	case "1-0:32.7.0":
		dst = &t.VoltageL1
	case "1-0:52.7.0":
		dst = &t.VoltageL2
	case "1-0:72.7.0":
		dst = &t.VoltageL3
	case "1-0:31.7.0":
		dst = &t.CurrentL1
	case "1-0:51.7.0":
		dst = &t.CurrentL2
	case "1-0:71.7.0":
		dst = &t.CurrentL3
	case "1-0:21.7.0":
		dst = &t.PowerDeliveredL1
	case "1-0:41.7.0":
		dst = &t.PowerDeliveredL2
	case "1-0:61.7.0":
		dst = &t.PowerDeliveredL3
	case "1-0:22.7.0":
		dst = &t.PowerReturnedL1
	case "1-0:42.7.0":
		dst = &t.PowerReturnedL2
	case "1-0:62.7.0":
		dst = &t.PowerReturnedL3
	default:
		return nil // gas, water, failure logs, text messages: not needed
	}

	v, err := parseMeasurement(first())
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrMalformed, obis, err)
	}
	*dst = v
	return nil
}

// parseMeasurement turns "000123.456*kWh" into 123.456, dropping the unit.
func parseMeasurement(s string) (float64, error) {
	if i := strings.IndexByte(s, '*'); i >= 0 {
		s = s[:i]
	}
	return strconv.ParseFloat(s, 64)
}

// parseTimestamp decodes the DSMR YYMMDDhhmmssX format, where X is 'S' for
// summer time or 'W' for winter time. The value is interpreted in the local
// zone; an unparsable value yields the zero time.
func parseTimestamp(s string) time.Time {
	if len(s) < 12 {
		return time.Time{}
	}
	ts, err := time.ParseInLocation("060102150405", s[:12], time.Local)
	if err != nil {
		return time.Time{}
	}
	return ts
}
