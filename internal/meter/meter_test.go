package meter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wido/p1mon-imitator/internal/dsmr"
)

// timeoutOnly imitates a serial port at the wrong baud rate that has gone
// quiet: every Read returns (0, nil).
type timeoutOnly struct{}

func (timeoutOnly) Read([]byte) (int, error) { return 0, nil }

func TestBoundedReaderDeadline(t *testing.T) {
	b := &boundedReader{r: timeoutOnly{}, ctx: context.Background(), deadline: time.Now().Add(20 * time.Millisecond)}
	_, err := b.Read(make([]byte, 16))
	if !errors.Is(err, errDetectTimeout) {
		t.Fatalf("err = %v, want errDetectTimeout", err)
	}
}

func TestBoundedReaderIdle(t *testing.T) {
	b := &boundedReader{r: timeoutOnly{}, ctx: context.Background(), idle: 20 * time.Millisecond, last: time.Now()}
	_, err := b.Read(make([]byte, 16))
	if !errors.Is(err, errStalled) {
		t.Fatalf("err = %v, want errStalled", err)
	}
}

func TestBoundedReaderContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &boundedReader{r: timeoutOnly{}, ctx: ctx}
	if _, err := b.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestReplayFile(t *testing.T) {
	body := "/TST5REPLAY\r\n\r\n1-3:0.2.8(50)\r\n1-0:1.7.0(00.250*kW)\r\n"
	raw := []byte(body + "!")
	crc := dsmr.CRC16(raw)
	raw = append(raw, []byte{hexDigit(crc >> 12), hexDigit(crc >> 8), hexDigit(crc >> 4), hexDigit(crc), '\r', '\n'}...)

	path := filepath.Join(t.TempDir(), "telegram.txt")
	if err := os.WriteFile(path, append(raw, raw...), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan dsmr.Telegram, 4)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, path, Options{ReplayInterval: 5 * time.Millisecond}, slog.New(slog.NewTextHandler(io.Discard, nil)),
			func(tg dsmr.Telegram, _ time.Time) { got <- tg })
	}()
	for i := 0; i < 3; i++ {
		select {
		case tg := <-got:
			if tg.PowerDelivered != 0.25 {
				t.Fatalf("PowerDelivered = %v", tg.PowerDelivered)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no telegram replayed")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
}

func hexDigit(v uint16) byte { return "0123456789ABCDEF"[v&0xF] }
