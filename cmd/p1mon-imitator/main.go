// Command p1mon-imitator reads a Dutch smart meter's P1 port and serves the
// readings through a P1 Monitor compatible HTTP API for Home Assistant.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/wido/p1mon-imitator/internal/api"
	"github.com/wido/p1mon-imitator/internal/dsmr"
	"github.com/wido/p1mon-imitator/internal/meter"
	"github.com/wido/p1mon-imitator/internal/p1mon"
)

var version = "dev"

func main() {
	var (
		showVersion = flag.Bool("version", false, "print version and exit")
		device      = flag.String("device", "/dev/ttyUSB0", "serial device of the P1 port (a regular file is replayed in a loop)")
		listen      = flag.String("listen", ":8080", "HTTP listen address; \":8080\" binds IPv4 and IPv6")
		memLimit    = flag.Int("memory-limit-mib", 32, "soft heap limit in MiB passed to the Go runtime (0 disables)")
		verbose     = flag.Bool("verbose", false, "log every telegram and HTTP request")
		prices      p1mon.Prices
	)
	flag.Float64Var(&prices.ConsumptionLow, "price-consumption-low", 0, "electricity price, low tariff, euro/kWh")
	flag.Float64Var(&prices.ConsumptionHigh, "price-consumption-high", 0, "electricity price, high tariff, euro/kWh")
	flag.Float64Var(&prices.ProductionLow, "price-production-low", 0, "feed-in price, low tariff, euro/kWh")
	flag.Float64Var(&prices.ProductionHigh, "price-production-high", 0, "feed-in price, high tariff, euro/kWh")
	flag.Float64Var(&prices.Gas, "price-gas", 0, "gas price, euro/m3 (reported only; gas is not read)")
	flag.Parse()
	if *showVersion {
		fmt.Println("p1mon-imitator", version)
		return
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	tuneRuntime(*memLimit)

	if err := run(context.Background(), *device, *listen, prices, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, device, listen string, prices p1mon.Prices, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configuration, err := p1mon.RenderConfiguration(prices)
	if err != nil {
		return err
	}
	latest := &p1mon.Latest{}

	var reqLog *slog.Logger
	if log.Enabled(ctx, slog.LevelDebug) {
		reqLog = log
	}
	srv := api.NewServer(listen, api.Handler(latest, configuration, reqLog))

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", listen, err)
	}
	log.Info("http: listening", "addr", ln.Addr().String())

	errc := make(chan error, 2)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("http: %w", err)
		}
	}()
	go func() {
		errc <- meter.Run(ctx, device, meter.Options{}, log, func(t dsmr.Telegram, received time.Time) {
			s, err := p1mon.Render(t, received)
			if err != nil {
				log.Error("render telegram", "err", err)
				return
			}
			latest.Set(s)
			log.Debug("telegram", "consumption_w", int(t.PowerDelivered*1000), "production_w", int(t.PowerReturned*1000),
				"tariff", t.Tariff, "kwh_low", t.EnergyDeliveredTariff1, "kwh_high", t.EnergyDeliveredTariff2)
		})
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errc:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("component failed", "err", err)
		}
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// tuneRuntime configures the garbage collector for a small, long-running
// process on a memory-constrained device: collect eagerly, honour a soft heap
// limit, and hand freed pages back to the OS periodically.
func tuneRuntime(limitMiB int) {
	debug.SetGCPercent(25)
	if limitMiB > 0 {
		debug.SetMemoryLimit(int64(limitMiB) << 20)
	}
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			debug.FreeOSMemory()
		}
	}()
}
