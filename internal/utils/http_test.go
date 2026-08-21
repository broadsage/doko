package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchCACert_Success(t *testing.T) {
	expectedData := []byte("mock-ca-certificate-payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expectedData)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	data, err := FetchCACert(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("FetchCACert failed: %v", err)
	}

	if string(data) != string(expectedData) {
		t.Errorf("expected %q, got %q", expectedData, data)
	}
}

func TestFetchCACert_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := FetchCACert(context.Background(), client, server.URL)
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestFetchCACert_InvalidRequest(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	_, err := FetchCACert(context.Background(), client, "http://invalid-url-12345.local")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
