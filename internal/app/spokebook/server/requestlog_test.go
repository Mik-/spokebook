package server_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spokebook/spokebook/internal/app/spokebook/server"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite"
)

func TestRequestLog(t *testing.T) {
	st, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app, err := server.New(context.Background(), st, logger, server.Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()

	log.Reset()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/setup?token=SECRET", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/bikes/new", nil))
	got := log.String()
	for _, want := range []string{
		"msg=request method=GET path=/setup status=",
		"path=/bikes/new status=",
		"client=192.0.2.1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "SECRET") {
		t.Errorf("log contains the setup token:\n%s", got)
	}
}
