# p1mon-imitator

A small Go program that reads a Dutch smart meter over its P1 port and exposes
the readings through the same HTTP API as [P1 Monitor](https://www.p1-monitor.nl/).
This lets the [Home Assistant P1 Monitor integration](https://www.home-assistant.io/integrations/p1_monitor/)
use any Linux box with a P1 cable, no Raspberry Pi image or database needed.

Everything stays in memory. Nothing is written to disk.

## What it does

- Reads DSMR telegrams from the P1 cable (default `/dev/ttyUSB0`)
- Detects the meter type by itself: DSMR 4/5 (115200 8N1) and DSMR 2/3 (9600 7E1)
- Serves `/api/v1/smartmeter`, `/api/v1/status`, `/api/v1/configuration` and
  `/api/v2/watermeter/day` the way Home Assistant expects them
- Reconnects when the USB cable is unplugged or the meter goes quiet

Only electricity is supported. Gas and water are not.

## Build

Requires Go 1.25 or newer.

```sh
make build        # binary for this machine
make arm64        # p1mon-imitator-linux-arm64, for a Raspberry Pi or other 64-bit ARM board
make all          # amd64 and arm64
```

Binaries are static (no libc) and stripped, so they can be copied to any Linux
machine of the same architecture.

## Run

```sh
./p1mon-imitator
```

That reads `/dev/ttyUSB0` and listens on port 8080 (IPv4 and IPv6). Options:

| Flag | Default | Meaning |
|---|---|---|
| `-device` | `/dev/ttyUSB0` | Serial device of the P1 port |
| `-listen` | `:8080` | HTTP listen address |
| `-price-consumption-low` / `-high` | `0` | Electricity price per kWh, low and high tariff |
| `-price-production-low` / `-high` | `0` | Feed-in price per kWh |
| `-price-gas` | `0` | Gas price per m³ (only reported, gas is not read) |
| `-memory-limit-mib` | `32` | Soft heap limit for the Go runtime |
| `-verbose` | off | Log every telegram and request |
| `-version` | | Print the version and exit |

The user running it needs read access to the serial device, usually by being in
the `dialout` group.

Then add the P1 Monitor integration in Home Assistant with the host of this
machine and port 8080.

To try it without a meter, point `-device` at a file with telegrams:

```sh
go run ./cmd/p1mon-imitator -device testdata/dsmr5-kaifa.txt
```

## Credits

This project exists thanks to [P1 Monitor](https://www.p1-monitor.nl/) by
ztatz, whose API this imitates, and to the
[python-p1monitor](https://github.com/frenck/python-p1monitor) client by
Franck Nijhof that Home Assistant uses to talk to it. The telegram format is
described in the [DSMR P1 companion standard](https://www.netbeheernederland.nl/dossiers/slimme-meter-15/documenten)
from Netbeheer Nederland.

## License

[MIT](LICENSE)
