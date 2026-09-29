// Package p1mon renders DSMR telegrams into the JSON documents the P1 Monitor
// HTTP API returns, and keeps the most recent one in memory.
package p1mon

import (
	"encoding/json"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/wido/p1mon-imitator/internal/dsmr"
)

// smartMeterRecord mirrors one row of P1 Monitor's /api/v1/smartmeter with
// json=object. Key names and casing (including TIMESTAMP_lOCAL) are the real
// API's and must not be changed.
type smartMeterRecord struct {
	ConsumptionGasM3   float64 `json:"CONSUMPTION_GAS_M3"`
	ConsumptionKWhHigh float64 `json:"CONSUMPTION_KWH_HIGH"`
	ConsumptionKWhLow  float64 `json:"CONSUMPTION_KWH_LOW"`
	ConsumptionW       int     `json:"CONSUMPTION_W"`
	ProductionKWhHigh  float64 `json:"PRODUCTION_KWH_HIGH"`
	ProductionKWhLow   float64 `json:"PRODUCTION_KWH_LOW"`
	ProductionW        int     `json:"PRODUCTION_W"`
	RecordIsProcessed  int     `json:"RECORD_IS_PROCESSED"`
	TarifCode          string  `json:"TARIFCODE"`
	TimestampUTC       int64   `json:"TIMESTAMP_UTC"`
	TimestampLocal     string  `json:"TIMESTAMP_lOCAL"`
}

// statusRecord mirrors one row of /api/v1/status. STATUS is a string in the
// real API even though it holds a number.
type statusRecord struct {
	Label    string `json:"LABEL"`
	Security int    `json:"SECURITY"`
	Status   string `json:"STATUS"`
	StatusID int    `json:"STATUS_ID"`
}

// configurationRecord mirrors one row of /api/v1/configuration.
type configurationRecord struct {
	ConfigurationID int    `json:"CONFIGURATION_ID"`
	Label           string `json:"LABEL"`
	Parameter       string `json:"PARAMETER"`
}

// Prices are the tariff settings exposed through /api/v1/configuration, in
// euro per kWh (or per m3 for gas).
type Prices struct {
	ConsumptionLow  float64
	ConsumptionHigh float64
	ProductionLow   float64
	ProductionHigh  float64
	Gas             float64
}

// Snapshot is one rendered telegram. Both documents are encoded once, when the
// telegram arrives, so serving a request allocates nothing.
type Snapshot struct {
	Telegram   dsmr.Telegram
	Received   time.Time
	SmartMeter []byte // JSON array with one smartMeterRecord
	Status     []byte // JSON array of statusRecord
}

// Latest publishes the most recent Snapshot to HTTP handlers. The zero value is
// ready to use and reports no data until the first Set.
type Latest struct {
	p atomic.Pointer[Snapshot]
}

// Set replaces the current snapshot.
func (l *Latest) Set(s *Snapshot) { l.p.Store(s) }

// Get returns the current snapshot or nil when no telegram has been received.
func (l *Latest) Get() *Snapshot { return l.p.Load() }

// Render converts a telegram into a Snapshot. received is the wall-clock time
// used for the timestamps; the telegram's own clock is ignored because many
// meters drift and P1 Monitor itself stamps rows on arrival.
func Render(t dsmr.Telegram, received time.Time) (*Snapshot, error) {
	tarif := "D" // dal / low
	if t.Tariff == 2 {
		tarif = "P" // piek / high
	}
	sm := [1]smartMeterRecord{{
		ConsumptionGasM3:   0,
		ConsumptionKWhHigh: t.EnergyDeliveredTariff2,
		ConsumptionKWhLow:  t.EnergyDeliveredTariff1,
		ConsumptionW:       kWToW(t.PowerDelivered),
		ProductionKWhHigh:  t.EnergyReturnedTariff2,
		ProductionKWhLow:   t.EnergyReturnedTariff1,
		ProductionW:        kWToW(t.PowerReturned),
		RecordIsProcessed:  0,
		TarifCode:          tarif,
		TimestampUTC:       received.Unix(),
		TimestampLocal:     received.Local().Format("2006-01-02 15:04:05"),
	}}
	smJSON, err := json.Marshal(sm)
	if err != nil {
		return nil, err
	}

	// STATUS_IDs and labels are those of the real P1 Monitor database; the
	// Home Assistant client looks rows up by STATUS_ID.
	st := [12]statusRecord{
		{"Huidige KW verbruik L1 (21.7.0)", 0, num(t.PowerDeliveredL1), 74},
		{"Huidige KW verbruik L2 (41.7.0)", 0, num(t.PowerDeliveredL2), 75},
		{"Huidige KW verbruik L3 (61.7.0)", 0, num(t.PowerDeliveredL3), 76},
		{"Huidige KW levering L1 (22.7.0)", 0, num(t.PowerReturnedL1), 77},
		{"Huidige KW levering L2 (42.7.0)", 0, num(t.PowerReturnedL2), 78},
		{"Huidige KW levering L3 (62.7.0)", 0, num(t.PowerReturnedL3), 79},
		{"Huidige Amperage L1 (31.7.0)", 0, num(t.CurrentL1), 100},
		{"Huidige Amperage L2 (51.7.0)", 0, num(t.CurrentL2), 101},
		{"Huidige Amperage L3 (71.7.0)", 0, num(t.CurrentL3), 102},
		{"Huidige Voltage L1 (32.7.0)", 0, num(t.VoltageL1), 103},
		{"Huidige Voltage L2 (52.7.0)", 0, num(t.VoltageL2), 104},
		{"Huidige Voltage L3 (72.7.0)", 0, num(t.VoltageL3), 105},
	}
	stJSON, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}

	return &Snapshot{Telegram: t, Received: received, SmartMeter: smJSON, Status: stJSON}, nil
}

// RenderConfiguration encodes the /api/v1/configuration document. It is static
// for the lifetime of the process.
func RenderConfiguration(p Prices) ([]byte, error) {
	rows := [5]configurationRecord{
		{1, "Verbruik tarief elektriciteit dal/nacht in euro.", num(p.ConsumptionLow)},
		{2, "Verbruik tarief elektriciteit piek/dag in euro.", num(p.ConsumptionHigh)},
		{3, "Geleverd tarief elektriciteit dal/nacht in euro.", num(p.ProductionLow)},
		{4, "Geleverd tarief elektriciteit piek/dag in euro.", num(p.ProductionHigh)},
		{15, "Verbruik tarief gas in euro.", num(p.Gas)},
	}
	return json.Marshal(rows)
}

// kWToW converts kilowatts to whole watts.
func kWToW(kw float64) int {
	return int(kw*1000 + 0.5)
}

// num formats a float the way P1 Monitor does for string-typed columns: the
// shortest exact representation with at least one decimal, e.g. "0.0", "233.6".
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	for _, c := range s {
		if c == '.' {
			return s
		}
	}
	return s + ".0"
}
