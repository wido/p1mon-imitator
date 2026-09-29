// Package meter reads telegrams from a P1 port and detects the meter's serial
// settings automatically.
package meter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"go.bug.st/serial"

	"github.com/wido/p1mon-imitator/internal/dsmr"
)

// Mode is a serial line configuration.
type Mode struct {
	Name string
	serial.Mode
}

// Modes lists the line settings used by Dutch smart meters, in the order they
// are tried. DSMR 4 and 5 use 115200 8N1; DSMR 2.2 and 3 use 9600 7E1.
var Modes = []Mode{
	{"DSMR 4/5 (115200 8N1)", serial.Mode{BaudRate: 115200, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}},
	{"DSMR 2/3 (9600 7E1)", serial.Mode{BaudRate: 9600, DataBits: 7, Parity: serial.EvenParity, StopBits: serial.OneStopBit}},
}

// Handler receives every valid telegram together with its arrival time.
type Handler func(t dsmr.Telegram, received time.Time)

// Options tune the reader; the zero value uses sensible defaults.
type Options struct {
	// DetectTimeout is how long one line setting gets to deliver a valid
	// telegram before the next one is tried. Meters send at least every 10 s.
	DetectTimeout time.Duration
	// StallTimeout is how long the port may stay silent (or produce only
	// invalid data) before the connection is reset and detection restarts.
	StallTimeout time.Duration
	// ReconnectDelay is the pause after a failure before reopening the port.
	ReconnectDelay time.Duration
	// ReplayInterval is the pause between telegrams when Device is a regular file.
	ReplayInterval time.Duration
}

func (o *Options) defaults() {
	if o.DetectTimeout <= 0 {
		o.DetectTimeout = 25 * time.Second
	}
	if o.StallTimeout <= 0 {
		o.StallTimeout = 90 * time.Second
	}
	if o.ReconnectDelay <= 0 {
		o.ReconnectDelay = 5 * time.Second
	}
	if o.ReplayInterval <= 0 {
		o.ReplayInterval = time.Second
	}
}

// Run reads telegrams from device until ctx is cancelled, calling handle for
// each valid one. It never returns early on I/O errors: the port is reopened
// and re-detected after every failure. If device is a regular file it is
// replayed in a loop instead, which is useful for development without a meter.
func Run(ctx context.Context, device string, opts Options, log *slog.Logger, handle Handler) error {
	opts.defaults()

	if fi, err := os.Stat(device); err == nil && fi.Mode().IsRegular() {
		return replayFile(ctx, device, opts, log, handle)
	}

	preferred := 0 // index into Modes of the last setting that worked
	for {
		port, mode, first, err := detect(ctx, device, preferred, opts, log)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warn("meter: detection failed, retrying", "device", device, "err", err)
			if !sleep(ctx, opts.ReconnectDelay) {
				return ctx.Err()
			}
			continue
		}
		preferred = mode
		log.Info("meter: connected", "device", device, "mode", Modes[mode].Name,
			"meter", first.Identification, "dsmr_version", versionString(first.Version))
		handle(first, time.Now())

		err = readLoop(ctx, port, opts, log, handle)
		_ = port.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Warn("meter: connection lost, reconnecting", "err", err)
		if !sleep(ctx, opts.ReconnectDelay) {
			return ctx.Err()
		}
	}
}

// detect opens the device with each line setting in turn, starting at
// preferred, until one yields a CRC-valid telegram. It returns the open port,
// the index of the working mode, and that first telegram.
func detect(ctx context.Context, device string, preferred int, opts Options, log *slog.Logger) (serial.Port, int, dsmr.Telegram, error) {
	var lastErr error
	for i := range Modes {
		idx := (preferred + i) % len(Modes)
		m := Modes[idx]
		if ctx.Err() != nil {
			return nil, 0, dsmr.Telegram{}, ctx.Err()
		}
		log.Info("meter: probing", "device", device, "mode", m.Name)

		port, err := serial.Open(device, &m.Mode)
		if err != nil {
			return nil, 0, dsmr.Telegram{}, fmt.Errorf("open %s: %w", device, err)
		}
		if err := port.SetReadTimeout(500 * time.Millisecond); err != nil {
			_ = port.Close()
			return nil, 0, dsmr.Telegram{}, fmt.Errorf("set read timeout: %w", err)
		}
		_ = port.ResetInputBuffer()

		src := &boundedReader{r: port, ctx: ctx, deadline: time.Now().Add(opts.DetectTimeout)}
		rd := dsmr.NewReader(src)
		t, err := nextValid(rd, log)
		if err == nil {
			// Detection done; the same port keeps being used, now with only the
			// stall guard active.
			return port, idx, t, nil
		}
		_ = port.Close()
		lastErr = err
		log.Info("meter: no valid telegram", "mode", m.Name, "err", err)
	}
	return nil, 0, dsmr.Telegram{}, lastErr
}

// readLoop consumes telegrams from an already detected port until an error.
func readLoop(ctx context.Context, port serial.Port, opts Options, log *slog.Logger, handle Handler) error {
	src := &boundedReader{r: port, ctx: ctx, idle: opts.StallTimeout, last: time.Now()}
	rd := dsmr.NewReader(src)
	for {
		t, err := nextValid(rd, log)
		if err != nil {
			return err
		}
		handle(t, time.Now())
	}
}

// nextValid returns the next telegram that parses and passes its CRC. Invalid
// telegrams are logged and skipped; only reader errors are returned.
func nextValid(rd *dsmr.Reader, log *slog.Logger) (dsmr.Telegram, error) {
	for {
		raw, err := rd.Next()
		if err != nil {
			return dsmr.Telegram{}, err
		}
		t, err := dsmr.Parse(raw)
		if err != nil {
			log.Debug("meter: dropping telegram", "err", err)
			continue
		}
		return t, nil
	}
}

var (
	errDetectTimeout = errors.New("no valid telegram within detection window")
	errStalled       = errors.New("no data from meter")
)

// boundedReader adapts a serial port whose Read returns (0, nil) on timeout
// into a reader that fails when a hard deadline passes or when no bytes have
// arrived for idle. Either limit may be zero to disable it.
type boundedReader struct {
	r        io.Reader
	ctx      context.Context
	deadline time.Time
	idle     time.Duration
	last     time.Time
}

func (b *boundedReader) Read(p []byte) (int, error) {
	for {
		if err := b.ctx.Err(); err != nil {
			return 0, err
		}
		n, err := b.r.Read(p)
		if n > 0 {
			b.last = time.Now()
			return n, err
		}
		if err != nil {
			return 0, err
		}
		now := time.Now()
		if !b.deadline.IsZero() && now.After(b.deadline) {
			return 0, errDetectTimeout
		}
		if b.idle > 0 && now.Sub(b.last) > b.idle {
			return 0, errStalled
		}
	}
}

// replayFile parses all telegrams in a file once and then re-emits them in a
// loop, one per ReplayInterval, so the rest of the program behaves as if a
// meter were attached.
func replayFile(ctx context.Context, path string, opts Options, log *slog.Logger, handle Handler) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	rd := dsmr.NewReader(io.LimitReader(f, 1<<20))
	var telegrams []dsmr.Telegram
	for {
		raw, err := rd.Next()
		if err != nil {
			break
		}
		t, err := dsmr.Parse(raw)
		if err != nil {
			log.Warn("replay: skipping invalid telegram", "err", err)
			continue
		}
		telegrams = append(telegrams, t)
	}
	_ = f.Close()
	if len(telegrams) == 0 {
		return fmt.Errorf("replay: no valid telegrams in %s", path)
	}
	log.Info("meter: replaying file", "path", path, "telegrams", len(telegrams), "interval", opts.ReplayInterval)

	tick := time.NewTicker(opts.ReplayInterval)
	defer tick.Stop()
	for i := 0; ; i = (i + 1) % len(telegrams) {
		handle(telegrams[i], time.Now())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// versionString turns the raw 1-3:0.2.8 value into something readable.
func versionString(raw string) string {
	switch {
	case raw == "":
		return "2.x/3.x (no version object)"
	case len(raw) == 2:
		return raw[:1] + "." + raw[1:]
	default:
		return raw
	}
}
