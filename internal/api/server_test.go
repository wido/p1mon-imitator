package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wido/p1mon-imitator/internal/dsmr"
	"github.com/wido/p1mon-imitator/internal/p1mon"
)

func newTestServer(t *testing.T, withData bool) *httptest.Server {
	t.Helper()
	latest := &p1mon.Latest{}
	if withData {
		s, err := p1mon.Render(dsmr.Telegram{Tariff: 1, PowerDelivered: 0.5, VoltageL1: 230}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		latest.Set(s)
	}
	conf, err := p1mon.RenderConfiguration(p1mon.Prices{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(latest, conf, nil))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func TestEndpointsAsHomeAssistantCallsThem(t *testing.T) {
	srv := newTestServer(t, true)
	cases := []struct {
		path     string
		wantRows int
	}{
		{"/api/v1/smartmeter?json=object&limit=1", 1},
		{"/api/v1/status?json=object", 12},
		{"/api/v1/configuration?json=object", 5},
		{"/api/v2/watermeter/day?json=object&limit=1", 0},
	}
	for _, c := range cases {
		resp, body := get(t, srv.URL+c.path)
		if resp.StatusCode != 200 {
			t.Errorf("%s: status %d", c.path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: content-type %q", c.path, ct)
		}
		var rows []any
		if err := json.Unmarshal(body, &rows); err != nil {
			t.Errorf("%s: not a JSON array: %v", c.path, err)
		}
		if len(rows) != c.wantRows {
			t.Errorf("%s: %d rows, want %d", c.path, len(rows), c.wantRows)
		}
	}
}

func TestNoDataYet(t *testing.T) {
	srv := newTestServer(t, false)
	resp, _ := get(t, srv.URL+"/api/v1/smartmeter?json=object&limit=1")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", resp.StatusCode)
	}
	// Configuration works before the first telegram so the HA config flow succeeds.
	resp, _ = get(t, srv.URL+"/api/v1/configuration?json=object")
	if resp.StatusCode != 200 {
		t.Fatalf("configuration status %d, want 200", resp.StatusCode)
	}
}

func TestUnknownPathIs404(t *testing.T) {
	srv := newTestServer(t, true)
	resp, _ := get(t, srv.URL+"/api/v1/powergas/day")
	if resp.StatusCode != 404 {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}
