package p1mon

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wido/p1mon-imitator/internal/dsmr"
)

func sample() dsmr.Telegram {
	return dsmr.Telegram{
		Tariff:                 2,
		EnergyDeliveredTariff1: 4988.071,
		EnergyDeliveredTariff2: 2770.133,
		EnergyReturnedTariff1:  1432.279,
		EnergyReturnedTariff2:  3971.604,
		PowerDelivered:         0.877,
		PowerDeliveredL1:       0.315,
		PowerDeliveredL3:       0.624,
		CurrentL1:              1.6,
		CurrentL2:              4.44,
		CurrentL3:              3.51,
		VoltageL1:              233.6,
		VoltageL3:              233,
	}
}

func TestRenderSmartMeterMatchesP1MonShape(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	received := time.Date(2021, 8, 16, 19, 23, 52, 0, loc)
	s, err := Render(sample(), received)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(s.SmartMeter, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	want := map[string]any{
		"CONSUMPTION_GAS_M3":   0.0,
		"CONSUMPTION_KWH_HIGH": 2770.133,
		"CONSUMPTION_KWH_LOW":  4988.071,
		"CONSUMPTION_W":        877.0,
		"PRODUCTION_KWH_HIGH":  3971.604,
		"PRODUCTION_KWH_LOW":   1432.279,
		"PRODUCTION_W":         0.0,
		"RECORD_IS_PROCESSED":  0.0,
		"TARIFCODE":            "P",
		"TIMESTAMP_UTC":        1629134632.0,
	}
	for k, v := range want {
		if r[k] != v {
			t.Errorf("%s = %v (%T), want %v", k, r[k], r[k], v)
		}
	}
	if _, ok := r["TIMESTAMP_lOCAL"]; !ok {
		t.Error("TIMESTAMP_lOCAL missing (note the lowercase l, as in the real API)")
	}
	if len(r) != len(want)+1 {
		t.Errorf("unexpected extra keys: %v", r)
	}
}

func TestRenderStatusIDsAndStrings(t *testing.T) {
	s, err := Render(sample(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Status   string `json:"STATUS"`
		StatusID int    `json:"STATUS_ID"`
	}
	if err := json.Unmarshal(s.Status, &rows); err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, r := range rows {
		got[r.StatusID] = r.Status
	}
	want := map[int]string{
		74: "0.315", 75: "0.0", 76: "0.624", 77: "0.0", 78: "0.0", 79: "0.0",
		100: "1.6", 101: "4.44", 102: "3.51", 103: "233.6", 104: "0.0", 105: "233.0",
	}
	for id, v := range want {
		if got[id] != v {
			t.Errorf("STATUS_ID %d = %q, want %q", id, got[id], v)
		}
	}
}

func TestRenderConfigurationHasAllIDsHomeAssistantReads(t *testing.T) {
	b, err := RenderConfiguration(Prices{ConsumptionLow: 0.20522, Gas: 0.64})
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID        int    `json:"CONFIGURATION_ID"`
		Parameter string `json:"PARAMETER"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, r := range rows {
		got[r.ID] = r.Parameter
	}
	// The Python client raises if any of these IDs is absent.
	for _, id := range []int{1, 2, 3, 4, 15} {
		if _, ok := got[id]; !ok {
			t.Errorf("CONFIGURATION_ID %d missing", id)
		}
	}
	if got[1] != "0.20522" || got[15] != "0.64" || got[2] != "0.0" {
		t.Errorf("unexpected parameters %v", got)
	}
}

func TestTariffLowIsD(t *testing.T) {
	tg := sample()
	tg.Tariff = 1
	s, _ := Render(tg, time.Now())
	var rows []map[string]any
	_ = json.Unmarshal(s.SmartMeter, &rows)
	if rows[0]["TARIFCODE"] != "D" {
		t.Fatalf("TARIFCODE = %v, want D", rows[0]["TARIFCODE"])
	}
}

func TestRenderThreePhaseWithProduction(t *testing.T) {
	tg := dsmr.Telegram{
		Tariff:        2,
		PowerReturned: 2.145,
		VoltageL1:     231.4, VoltageL2: 234.8, VoltageL3: 229.9,
		CurrentL1: 2, CurrentL2: 5, CurrentL3: 3,
		PowerDeliveredL1: 0.112, PowerDeliveredL3: 0.087,
		PowerReturnedL2: 1.23, PowerReturnedL3: 1.114,
	}
	s, err := Render(tg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Status   string `json:"STATUS"`
		StatusID int    `json:"STATUS_ID"`
	}
	if err := json.Unmarshal(s.Status, &rows); err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, r := range rows {
		got[r.StatusID] = r.Status
	}
	// Each STATUS_ID must carry its own phase's value; kW stays in kW.
	want := map[int]string{
		74: "0.112", 75: "0.0", 76: "0.087", // consumed L1..L3
		77: "0.0", 78: "1.23", 79: "1.114", // produced L1..L3
		100: "2.0", 101: "5.0", 102: "3.0", // amps L1..L3
		103: "231.4", 104: "234.8", 105: "229.9", // volts L1..L3
	}
	for id, v := range want {
		if got[id] != v {
			t.Errorf("STATUS_ID %d = %q, want %q", id, got[id], v)
		}
	}
	var sm []map[string]any
	_ = json.Unmarshal(s.SmartMeter, &sm)
	if sm[0]["PRODUCTION_W"] != 2145.0 || sm[0]["CONSUMPTION_W"] != 0.0 {
		t.Errorf("totals: %v", sm[0])
	}
}
