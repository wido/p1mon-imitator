# Running as a systemd service

The unit in `systemd/p1mon-imitator.service` runs the daemon as root, reads
`/dev/ttyUSB0` and listens on port 8080. Running as root is not recommended,
but it keeps the setup to a few commands.

## Install

Build for the target machine (see the README), then:

```sh
sudo install -m 0755 p1mon-imitator /usr/local/bin/p1mon-imitator
sudo install -m 0644 systemd/p1mon-imitator.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now p1mon-imitator
```

Or, from a checkout on the target machine, `sudo make install` does the same.

Check that it found the meter:

```sh
systemctl status p1mon-imitator
journalctl -u p1mon-imitator -f
curl http://localhost:8080/api/v1/smartmeter?json=object
```

The log shows which serial mode was detected and the meter identification.

## Changing flags

Create `/etc/default/p1mon-imitator` with the flags you want, then restart:

```sh
P1MON_ARGS="-device /dev/ttyUSB1 -price-consumption-low 0.25 -price-consumption-high 0.30"
```

```sh
sudo systemctl restart p1mon-imitator
```

## Notes

- The service restarts itself 5 seconds after any crash. Losing the USB device
  does not crash it; the daemon reconnects on its own.
- The filesystem is mounted read-only for the process (`ProtectSystem=strict`).
  The daemon never writes to disk, so nothing needs to be whitelisted.
- `MemoryMax=64M` is generous; the process normally uses around 10 MB.

## Uninstall

```sh
sudo systemctl disable --now p1mon-imitator
sudo rm /etc/systemd/system/p1mon-imitator.service /usr/local/bin/p1mon-imitator
sudo systemctl daemon-reload
```
