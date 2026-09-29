package dsmr

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCRC16KnownVector(t *testing.T) {
	// CRC-16/ARC check value from the reveng catalogue.
	if got := CRC16([]byte("123456789")); got != 0xBB3D {
		t.Fatalf("CRC16 = %04X, want BB3D", got)
	}
}

// WithCRC appends the '!' terminator plus a valid checksum to a telegram body.
func WithCRC(body string) []byte {
	b := []byte(body + "!")
	return append(b, []byte(fmt.Sprintf("%04X\r\n", CRC16(b)))...)
}

const dsmr5Body = "/KFM5KAIFA-METER\r\n\r\n" +
	"1-3:0.2.8(50)\r\n" +
	"0-0:1.0.0(210816192352S)\r\n" +
	"0-0:96.1.1(4530303235303030303030303030303030)\r\n" +
	"1-0:1.8.1(004988.071*kWh)\r\n" +
	"1-0:1.8.2(002770.133*kWh)\r\n" +
	"1-0:2.8.1(001432.279*kWh)\r\n" +
	"1-0:2.8.2(003971.604*kWh)\r\n" +
	"0-0:96.14.0(0002)\r\n" +
	"1-0:1.7.0(00.877*kW)\r\n" +
	"1-0:2.7.0(00.000*kW)\r\n" +
	"0-0:96.7.21(00004)\r\n" +
	"0-0:96.7.9(00002)\r\n" +
	"1-0:99.97.0(1)(0-0:96.7.19)(180110123456W)(0000001234*s)\r\n" +
	"1-0:32.32.0(00000)\r\n" +
	"1-0:32.36.0(00000)\r\n" +
	"0-0:96.13.0()\r\n" +
	"1-0:32.7.0(233.6*V)\r\n" +
	"1-0:52.7.0(232.1*V)\r\n" +
	"1-0:72.7.0(233.0*V)\r\n" +
	"1-0:31.7.0(001*A)\r\n" +
	"1-0:51.7.0(004*A)\r\n" +
	"1-0:71.7.0(003*A)\r\n" +
	"1-0:21.7.0(00.315*kW)\r\n" +
	"1-0:41.7.0(00.000*kW)\r\n" +
	"1-0:61.7.0(00.624*kW)\r\n" +
	"1-0:22.7.0(00.000*kW)\r\n" +
	"1-0:42.7.0(00.000*kW)\r\n" +
	"1-0:62.7.0(00.000*kW)\r\n" +
	"0-1:24.1.0(003)\r\n" +
	"0-1:96.1.0(4730303332353631323334353637383930)\r\n" +
	"0-1:24.2.1(210816190000S)(02273.447*m3)\r\n"

func TestParseDSMR5(t *testing.T) {
	tg, err := Parse(WithCRC(dsmr5Body))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Identification", tg.Identification, "KFM5KAIFA-METER"},
		{"Version", tg.Version, "50"},
		{"Tariff", tg.Tariff, 2},
		{"EnergyDeliveredTariff1", tg.EnergyDeliveredTariff1, 4988.071},
		{"EnergyDeliveredTariff2", tg.EnergyDeliveredTariff2, 2770.133},
		{"EnergyReturnedTariff1", tg.EnergyReturnedTariff1, 1432.279},
		{"EnergyReturnedTariff2", tg.EnergyReturnedTariff2, 3971.604},
		{"PowerDelivered", tg.PowerDelivered, 0.877},
		{"PowerReturned", tg.PowerReturned, 0.0},
		{"VoltageL1", tg.VoltageL1, 233.6},
		{"VoltageL2", tg.VoltageL2, 232.1},
		{"VoltageL3", tg.VoltageL3, 233.0},
		{"CurrentL1", tg.CurrentL1, 1.0},
		{"CurrentL2", tg.CurrentL2, 4.0},
		{"CurrentL3", tg.CurrentL3, 3.0},
		{"PowerDeliveredL1", tg.PowerDeliveredL1, 0.315},
		{"PowerDeliveredL3", tg.PowerDeliveredL3, 0.624},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	want := time.Date(2021, 8, 16, 19, 23, 52, 0, time.Local)
	if !tg.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", tg.Timestamp, want)
	}
}

func TestParseRejectsBadCRC(t *testing.T) {
	raw := WithCRC(dsmr5Body)
	raw[len(raw)-3] ^= 0x01 // corrupt last hex digit
	_, err := Parse(raw)
	if !errors.Is(err, ErrCRC) {
		t.Fatalf("err = %v, want ErrCRC", err)
	}
}

func TestParseDSMR3WithoutCRC(t *testing.T) {
	body := "/ISk5\\2MT382-1000\r\n\r\n" +
		"0-0:96.1.1(5A424556303035313233343536373839)\r\n" +
		"1-0:1.8.1(12345.678*kWh)\r\n" +
		"1-0:1.8.2(12345.678*kWh)\r\n" +
		"1-0:2.8.1(12345.678*kWh)\r\n" +
		"1-0:2.8.2(12345.678*kWh)\r\n" +
		"0-0:96.14.0(0001)\r\n" +
		"1-0:1.7.0(0001.19*kW)\r\n" +
		"1-0:2.7.0(0000.00*kW)\r\n" +
		"0-0:17.0.0(016*A)\r\n" +
		"0-0:96.3.10(1)\r\n" +
		"0-0:96.13.1()\r\n" +
		"0-0:96.13.0()\r\n" +
		"0-1:24.1.0(3)\r\n" +
		"0-1:96.1.0(3232323241424344313233343536373839)\r\n" +
		"0-1:24.3.0(090212160000)(00)(60)(1)(0-1:24.2.1)(m3)\r\n" +
		"(00000.000)\r\n" +
		"0-1:24.4.0(1)\r\n" +
		"!\r\n"
	tg, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if tg.Version != "" || tg.Tariff != 1 || tg.PowerDelivered != 1.19 || tg.EnergyDeliveredTariff1 != 12345.678 {
		t.Fatalf("unexpected telegram %+v", tg)
	}
}

func TestReaderResyncsAndYieldsEachTelegram(t *testing.T) {
	garbage := "\xff\xfe\x00junk before header\r\n1-0:1.7.0(00.100*kW)\r\n"
	one := WithCRC(dsmr5Body)
	two := WithCRC(strings.Replace(dsmr5Body, "00.877*kW", "01.000*kW", 1))
	var stream bytes.Buffer
	stream.WriteString(garbage)
	stream.Write(one)
	stream.WriteString("partial\r\n")
	stream.Write(two)

	r := NewReader(&stream)
	first, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, one) {
		t.Fatalf("first telegram mismatch:\n%q\n%q", first, one)
	}
	tg, err := Parse(first)
	if err != nil || tg.PowerDelivered != 0.877 {
		t.Fatalf("first parse: %v %v", err, tg.PowerDelivered)
	}
	second, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	tg, err = Parse(second)
	if err != nil || tg.PowerDelivered != 1.0 {
		t.Fatalf("second parse: %v %v", err, tg.PowerDelivered)
	}
	if _, err := r.Next(); err == nil {
		t.Fatal("expected EOF after last telegram")
	}
}

func TestReaderDropsOversizedTelegram(t *testing.T) {
	var stream bytes.Buffer
	stream.WriteString("/XXX5BIG\r\n")
	for i := 0; i < 200; i++ {
		stream.WriteString("0-0:96.13.0(0123456789012345678901234567890123456789)\r\n")
	}
	stream.WriteString("!0000\r\n")
	stream.Write(WithCRC(dsmr5Body))
	r := NewReader(&stream)
	raw, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err != nil {
		t.Fatalf("expected the valid telegram after the oversized one, got %v", err)
	}
}

// threePhaseBody is a DSMR 5 telegram from a 3-phase meter with solar panels:
// every per-phase field carries a distinct value so a swapped OBIS code
// cannot go unnoticed.
const threePhaseBody = "/Ene5\\T210-D ESMR5.0\r\n\r\n" +
	"1-3:0.2.8(50)\r\n" +
	"0-0:1.0.0(240612133000S)\r\n" +
	"0-0:96.1.1(4530303534303037353936373139323139)\r\n" +
	"1-0:1.8.1(010245.117*kWh)\r\n" +
	"1-0:1.8.2(009873.402*kWh)\r\n" +
	"1-0:2.8.1(003011.008*kWh)\r\n" +
	"1-0:2.8.2(007520.655*kWh)\r\n" +
	"0-0:96.14.0(0002)\r\n" +
	"1-0:1.7.0(00.000*kW)\r\n" +
	"1-0:2.7.0(02.145*kW)\r\n" +
	"0-0:96.7.21(00012)\r\n" +
	"0-0:96.7.9(00003)\r\n" +
	"1-0:99.97.0(0)(0-0:96.7.19)\r\n" +
	"1-0:32.32.0(00002)\r\n" +
	"1-0:52.32.0(00001)\r\n" +
	"1-0:72.32.0(00003)\r\n" +
	"1-0:32.36.0(00000)\r\n" +
	"1-0:52.36.0(00000)\r\n" +
	"1-0:72.36.0(00000)\r\n" +
	"0-0:96.13.0()\r\n" +
	"1-0:32.7.0(231.4*V)\r\n" +
	"1-0:52.7.0(234.8*V)\r\n" +
	"1-0:72.7.0(229.9*V)\r\n" +
	"1-0:31.7.0(002*A)\r\n" +
	"1-0:51.7.0(005*A)\r\n" +
	"1-0:71.7.0(003*A)\r\n" +
	"1-0:21.7.0(00.112*kW)\r\n" +
	"1-0:41.7.0(00.000*kW)\r\n" +
	"1-0:61.7.0(00.087*kW)\r\n" +
	"1-0:22.7.0(00.000*kW)\r\n" +
	"1-0:42.7.0(01.230*kW)\r\n" +
	"1-0:62.7.0(01.114*kW)\r\n"

// singlePhaseBody is a DSMR 5 telegram from a 1-phase meter: no L2/L3 objects at all.
const singlePhaseBody = "/XMX5LGBBFG1009325446\r\n\r\n" +
	"1-3:0.2.8(50)\r\n" +
	"0-0:1.0.0(240612133000S)\r\n" +
	"1-0:1.8.1(001234.567*kWh)\r\n" +
	"1-0:1.8.2(000987.654*kWh)\r\n" +
	"1-0:2.8.1(000000.000*kWh)\r\n" +
	"1-0:2.8.2(000000.000*kWh)\r\n" +
	"0-0:96.14.0(0001)\r\n" +
	"1-0:1.7.0(00.421*kW)\r\n" +
	"1-0:2.7.0(00.000*kW)\r\n" +
	"1-0:32.7.0(228.7*V)\r\n" +
	"1-0:31.7.0(002*A)\r\n" +
	"1-0:21.7.0(00.421*kW)\r\n" +
	"1-0:22.7.0(00.000*kW)\r\n"

func TestParseThreePhase(t *testing.T) {
	tg, err := Parse(WithCRC(threePhaseBody))
	if err != nil {
		t.Fatal(err)
	}
	want := Telegram{
		Identification:         "Ene5\\T210-D ESMR5.0",
		Version:                "50",
		Timestamp:              tg.Timestamp,
		Tariff:                 2,
		EnergyDeliveredTariff1: 10245.117,
		EnergyDeliveredTariff2: 9873.402,
		EnergyReturnedTariff1:  3011.008,
		EnergyReturnedTariff2:  7520.655,
		PowerDelivered:         0,
		PowerReturned:          2.145,
		VoltageL1:              231.4, VoltageL2: 234.8, VoltageL3: 229.9,
		CurrentL1: 2, CurrentL2: 5, CurrentL3: 3,
		PowerDeliveredL1: 0.112, PowerDeliveredL2: 0, PowerDeliveredL3: 0.087,
		PowerReturnedL1: 0, PowerReturnedL2: 1.23, PowerReturnedL3: 1.114,
	}
	if tg != want {
		t.Fatalf("telegram mismatch\n got %+v\nwant %+v", tg, want)
	}
}

func TestParseSinglePhaseLeavesL2L3Zero(t *testing.T) {
	tg, err := Parse(WithCRC(singlePhaseBody))
	if err != nil {
		t.Fatal(err)
	}
	if tg.VoltageL1 != 228.7 || tg.CurrentL1 != 2 || tg.PowerDeliveredL1 != 0.421 || tg.Tariff != 1 {
		t.Fatalf("L1 values wrong: %+v", tg)
	}
	for name, v := range map[string]float64{
		"VoltageL2": tg.VoltageL2, "VoltageL3": tg.VoltageL3,
		"CurrentL2": tg.CurrentL2, "CurrentL3": tg.CurrentL3,
		"PowerDeliveredL2": tg.PowerDeliveredL2, "PowerDeliveredL3": tg.PowerDeliveredL3,
		"PowerReturnedL2": tg.PowerReturnedL2, "PowerReturnedL3": tg.PowerReturnedL3,
	} {
		if v != 0 {
			t.Errorf("%s = %v, want 0 on a single-phase meter", name, v)
		}
	}
}
