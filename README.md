# P1Mon imitator
This repository contains a single Go Binary which imitates the p1mon ( https://www.p1-monitor.nl/ ) API.

The single Go binary can be run on a Linux machine where the p1 port of a Dutch Smart Energy meter is connected via USB and present, often as /dev/ttyUSB0

This binary will read the incoming values from the p1mon, store them in memory and serve them via the API.

# AMD64 and ARCH64
The binary can be compiled as AMD64 and ARCH64, where for the last one it can run on a Raspberry Pi or similar device.

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o p1mon-imitator-amd64 ./cmd/p1mon-imitator
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o p1mon-imitator-arm64 ./cmd/p1mon-imitator
```

# Usage

```
p1mon-imitator [-device /dev/ttyUSB0] [-listen :8080] [-memory-limit-mib 32] [-verbose]
               [-price-consumption-low 0.25] [-price-consumption-high 0.30]
               [-price-production-low 0.10] [-price-production-high 0.10] [-price-gas 1.20]
```

- `-device` defaults to `/dev/ttyUSB0`. The meter type is detected automatically: DSMR 4/5 meters
  (115200 baud, 8N1, a telegram every 1 s or 10 s) and DSMR 2/3 meters (9600 baud, 7E1) both work.
  If the device disappears or goes silent the port is reopened and re-detected.
- `-listen` defaults to `:8080`, which listens on IPv4 and IPv6. Configure Home Assistant with the
  host and port 8080.
- The `-price-*` flags fill the tariff sensors Home Assistant reads from `/api/v1/configuration`.
- Nothing is ever written to disk. Only the latest telegram is kept in memory and the Go runtime
  is tuned for small devices (`-memory-limit-mib` sets a soft heap limit).

Pointing `-device` at a regular file replays the telegrams in it, which is handy for development
without a meter: `go run ./cmd/p1mon-imitator -device testdata/dsmr5-kaifa.txt`.

# Electricity only
The only supported functionality is electricity, water and gas are not supported at this moment.

# Home Assistant
The goal is that Home Assistant can read the API via this integrtion: https://www.home-assistant.io/integrations/p1_monitor/

The integration should not notice that its an imitator that pretents to be p1mon, but is not.

The endpoints served are the ones the integration polls every 5 seconds:

| Endpoint | Content |
|---|---|
| `/api/v1/smartmeter` | current consumption/production in W, meter readings in kWh, tariff |
| `/api/v1/status` | per-phase voltage, current and power |
| `/api/v1/configuration` | tariff prices from the `-price-*` flags |
| `/api/v2/watermeter/day` | always empty (no water meter) |
