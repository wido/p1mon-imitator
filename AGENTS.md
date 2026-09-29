# AGENTS.md

Guidance for coding agents and contributors working in this repository.

## Project

A single Go binary that imitates the P1 Monitor (p1mon, https://www.p1-monitor.nl/) HTTP API. It reads
telegrams from the P1 port of a Dutch smart energy meter (a USB serial device, default `/dev/ttyUSB0`),
keeps only the latest one in memory, and serves it over the p1mon-compatible API on `:8080` (IPv4+IPv6).

The consumer is the Home Assistant `p1_monitor` integration
(https://www.home-assistant.io/integrations/p1_monitor/) via the `p1monitor` Python package. The
integration must not be able to tell it is talking to an imitator, so API paths, JSON key names (note
the real API's `TIMESTAMP_lOCAL` typo), value types and ID numbers must match real p1mon exactly.

## Commands

```sh
make build                           # static, stripped binary for this machine (CGO_ENABLED=0)
make all                             # p1mon-imitator-linux-amd64 and -arm64
make test                            # go test ./...
make vet                             # go vet + gofmt check
go test ./internal/dsmr -run TestParseDSMR5   # a single test
go run ./cmd/p1mon-imitator -device testdata/dsmr5-kaifa.txt -listen :18080 -verbose     # run without a meter
```

The Makefile sets `CGO_ENABLED=0`, `-trimpath` and `-ldflags '-s -w'`, and injects the git version
into `main.version` (shown by `-version`). Keep builds static: check with `file` that the binary
says "statically linked".

Passing a regular file as `-device` replays its telegrams in a loop (1/s). `testdata/dsmr5-kaifa.txt`
holds CRC-valid DSMR 5 telegrams and `testdata/dsmr5-3phase-solar.txt` a 3-phase meter feeding back solar power. To regenerate such a file, build the body and append `!` plus the
uppercase hex of `dsmr.CRC16` over everything from `/` through `!` (see `WithCRC` in `dsmr_test.go`).

End-to-end check with the real client: `pip install --target ./pylib p1monitor`, then call
`settings()`, `smartmeter()`, `phases()` and `watermeter()` against the running binary; the last one
must raise `P1MonitorNoDataError`.

## Architecture

Data flows one way: `meter` → `dsmr` → `p1mon` → `api`.

- `internal/dsmr` — protocol layer. `Reader` frames telegrams (`/` header to `!` line, resyncs on
  garbage, 4 KiB cap). `Parse` verifies the CRC-16/ARC (absent on DSMR 2/3, which is accepted) and
  fills a flat `Telegram` struct by switching on OBIS codes. Gas/water OBIS codes are deliberately ignored.
- `internal/meter` — `Run` owns the serial port. It auto-detects the meter by trying 115200 8N1
  (DSMR 4/5) then 9600 7E1 (DSMR 2/3) until a CRC-valid telegram arrives, remembers the working
  mode, and reconnects after stalls or USB errors. `boundedReader` converts the serial library's
  `(0, nil)` timeout reads into deadline/idle errors. A regular-file `-device` takes the replay path.
- `internal/p1mon` — the p1mon data model. `Render` encodes a telegram into the `smartmeter` and
  `status` JSON documents once, on arrival; `Latest` is an `atomic.Pointer` to that snapshot so HTTP
  handlers do zero work and zero allocation. `RenderConfiguration` builds the static tariff document.
- `internal/api` — `net/http` routes only. Query parameters (`json=object`, `limit`, `round`) are
  ignored; responses always look like `json=object&limit=1`.
- `cmd/p1mon-imitator` — flags, wiring, signals, and GC tuning (`SetGCPercent(25)`,
  `SetMemoryLimit`, periodic `FreeOSMemory`).

## Mappings that are easy to get wrong

- DSMR tariff 1 = low/dal → `TARIFCODE "D"`, `*_KWH_LOW` comes from OBIS `x.8.1`.
  Tariff 2 = high/piek → `"P"`, `*_KWH_HIGH` from `x.8.2`.
- `/api/v1/status` rows are looked up by `STATUS_ID`: 74–76 kW consumed L1–L3, 77–79 kW produced,
  100–102 amps, 103–105 volts. `STATUS` is a string with at least one decimal; kW stays in kW
  (the client multiplies by 1000).
- `/api/v1/configuration` must contain `CONFIGURATION_ID` 1, 2, 3, 4 and 15 (gas), else the Python
  client raises. `PARAMETER` is a string.
- `/api/v2/watermeter/day` returns `[]` so Home Assistant marks the water meter absent.
- Before the first telegram, `smartmeter` and `status` return 503; `configuration` always works so
  the Home Assistant config flow (which calls `settings()`) succeeds.

## Constraints

- Electricity only. Gas and water are out of scope.
- Never write to the filesystem at runtime; logs go to stderr.
- Pure Go only (no cgo) so cross-compiling for arm64 keeps working. The single dependency is
  `go.bug.st/serial`.
- Keep allocation per telegram and per request minimal; this runs on low-memory devices.
