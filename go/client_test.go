package guardrail

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastTransport(t *testing.T, url string) *HTTPTransport {
	t.Helper()
	tr, err := NewHTTPTransport(HTTPOptions{APIKey: "test-key", BaseURL: url + "/", BackoffInitial: time.Millisecond, BackoffMax: 5 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestHTTPTransportSendsTheRequestAndUnpacksTheReply(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("path=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"model": "jev-1", "answers": {"hazard": {"type": "choice", "choice": "none", "confidence": 0.9, "probabilities": {"none": 0.9}}}, "usage": {"input_tokens": 120}}`))
	}))
	defer server.Close()

	reply, err := fastTransport(t, server.URL).SystemOne(ctx, InputState("hi", nil), Questions{}, CallOptions{Model: "jev-x"})
	if err != nil {
		t.Fatal(err)
	}
	if got["model"] != "jev-x" || reply.Model != "jev-1" || reply.Usage.InputTokens != 120 || reply.Answers["hazard"]["choice"] != "none" {
		t.Fatalf("sent %v, got %+v", got, reply)
	}
}

func TestHTTPTransportRetriesTransientFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"answers": {}}`))
	}))
	defer server.Close()

	if _, err := fastTransport(t, server.URL).SystemOne(ctx, State{}, Questions{}, CallOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestHTTPTransportDoesNotRetryAClientError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "bad key"}`))
	}))
	defer server.Close()

	_, err := fastTransport(t, server.URL).SystemOne(ctx, State{}, Questions{}, CallOptions{})
	if err == nil || !strings.Contains(err.Error(), "401") || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestHTTPTransportGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := fastTransport(t, server.URL).SystemOne(ctx, State{}, Questions{}, CallOptions{})
	if err == nil || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestNoKeyIsAnError(t *testing.T) {
	t.Setenv("JEV_API_KEY", "  ")
	if _, err := NewHTTPTransport(HTTPOptions{}); err == nil {
		t.Fatal("expected an error")
	}
}
