package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func tosConfig(t *testing.T, overrides map[string]string) json.RawMessage {
	t.Helper()
	c := map[string]string{
		"endpoint":   "https://tos-s3-cn-beijing.volces.com",
		"region":     "cn-beijing",
		"bucket":     "picfast-test",
		"access_key": "test-ak",
		"secret_key": "test-sk",
	}
	for k, v := range overrides {
		c[k] = v
	}
	cfg, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestTOSConfigValidation(t *testing.T) {
	if !IsKnownType("tos") {
		t.Fatal("TOS storage is not registered")
	}
	if err := ValidateConfig("tos", tosConfig(t, nil)); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for _, field := range []string{"endpoint", "region", "bucket", "access_key", "secret_key"} {
		t.Run("missing_"+field, func(t *testing.T) {
			cfg := tosConfig(t, map[string]string{field: ""})
			if err := ValidateConfig("tos", cfg); err == nil {
				t.Fatal("expected a validation error")
			}
			if _, err := New("tos", cfg); err == nil {
				t.Fatal("constructor accepted an invalid config")
			}
		})
	}
	for _, tt := range []struct {
		field string
		value string
	}{
		{"region", "auto"},
		{"bucket", "a"},
		{"bucket", "bucket.with.dots"},
		{"bucket", "Bucket"},
		{"bucket", "-bucket"},
		{"bucket", strings.Repeat("a", 64)},
		{"endpoint", "tos-s3-cn-beijing.volces.com"},
		{"endpoint", "http://127.0.0.1:9000"},
		{"endpoint", "https://tos-cn-beijing.volces.com"},
		{"endpoint", "https://picfast-test.tos-s3-cn-beijing.volces.com"},
		{"endpoint", "https://tos-s3-cn-beijing.volces.com/picfast-test"},
		{"endpoint", "https://user:pass@tos-s3-cn-beijing.volces.com"},
		{"endpoint", "https://tos-s3-cn-beijing.volces.com?bucket=picfast-test"},
	} {
		t.Run(tt.field+"_"+tt.value, func(t *testing.T) {
			if err := ValidateConfig("tos", tosConfig(t, map[string]string{tt.field: tt.value})); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
	if _, err := New("tos", json.RawMessage(`{`)); err == nil {
		t.Fatal("constructor accepted invalid JSON")
	}
}

func TestTOSPublicURL(t *testing.T) {
	for _, tt := range []struct {
		name      string
		overrides map[string]string
		want      string
	}{
		{"bucket", nil, "https://picfast-test.tos-s3-cn-beijing.volces.com/images/cat.png"},
		{"trailing_slash", map[string]string{"endpoint": "https://tos-s3-cn-beijing.volces.com/"}, "https://picfast-test.tos-s3-cn-beijing.volces.com/images/cat.png"},
		{"cdn", map[string]string{"url": "https://images.example.com/picfast/"}, "https://images.example.com/picfast/images/cat.png"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := New("tos", tosConfig(t, tt.overrides))
			if err != nil {
				t.Fatal(err)
			}
			if got := store.URL("/images/cat.png"); got != tt.want {
				t.Fatalf("URL = %q, want %q", got, tt.want)
			}
		})
	}
}

// Exercise the real SDK over HTTP, routing the provider hostname to a local
// server so both virtual hosting and legacy S3 path-style requests are checked.
func TestTOSAndS3ObjectOperations(t *testing.T) {
	for _, typ := range []string{"tos", "s3"} {
		t.Run(typ, func(t *testing.T) {
			payload := []byte("image data")
			wantHost := "tos-s3-cn-beijing.volces.com"
			prefix := "/picfast-test"
			if typ == "tos" {
				wantHost = "picfast-test." + wantHost
				prefix = ""
			}
			requests := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.Method
				if r.Host != wantHost {
					t.Errorf("Host = %q, want %q", r.Host, wantHost)
				}
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=test-ak/") || !strings.Contains(auth, "/cn-beijing/s3/aws4_request") || !strings.Contains(auth, ";host;") {
					t.Errorf("request is not signed with the expected SigV4 scope: %q", auth)
				}
				if typ == "tos" {
					for _, h := range []string{"X-Amz-Checksum-Crc32", "X-Amz-Sdk-Checksum-Algorithm", "X-Amz-Checksum-Mode", "X-Amz-Trailer"} {
						if r.Header.Get(h) != "" {
							t.Errorf("unexpected optional AWS checksum header %s", h)
						}
					}
				}
				wantPath := prefix + "/images/cat.png"
				if r.Method == http.MethodHead {
					wantPath = prefix
					if wantPath == "" {
						wantPath = "/"
					}
				}
				if r.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
				}
				switch r.Method {
				case http.MethodPut:
					body, err := io.ReadAll(r.Body)
					if err != nil || !bytes.Equal(body, payload) {
						t.Errorf("uploaded body = %q, err = %v", body, err)
					}
					if got := r.Header.Get("Content-Type"); got != "image/png" {
						t.Errorf("Content-Type = %q", got)
					}
				case http.MethodGet:
					_, _ = w.Write(payload)
				case http.MethodDelete:
					w.WriteHeader(http.StatusNoContent)
				case http.MethodHead:
					w.WriteHeader(http.StatusOK)
				default:
					http.Error(w, "unsupported method", http.StatusBadRequest)
				}
			}))
			t.Cleanup(server.Close)
			transport := &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
				},
			}
			t.Cleanup(transport.CloseIdleConnections)
			store, err := New(typ, tosConfig(t, map[string]string{"endpoint": "http://tos-s3-cn-beijing.volces.com"}))
			if err != nil {
				t.Fatal(err)
			}
			s := store.(*S3Storage)
			s.client = s3.New(s.client.Options(), func(o *s3.Options) {
				o.HTTPClient = &http.Client{Transport: transport}
			})
			ctx := t.Context()
			if err := store.Write(ctx, "images/cat.png", payload, "image/png"); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := store.Read(ctx, "images/cat.png")
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("Read = %q, err = %v", got, err)
			}
			if err := store.Delete(ctx, "images/cat.png"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if health := store.HealthCheck(ctx); !health.Healthy {
				t.Fatalf("HealthCheck: %s", health.Error)
			}
			for _, want := range []string{http.MethodPut, http.MethodGet, http.MethodDelete, http.MethodHead} {
				if got := <-requests; got != want {
					t.Fatalf("request method = %s, want %s", got, want)
				}
			}
		})
	}
}

type failedTOSRequest struct{}

func (failedTOSRequest) Do(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`<Error><Code>AccessDenied</Code></Error>`)),
		Request:    r,
	}, nil
}

func TestTOSReportsRequestFailures(t *testing.T) {
	store, err := NewTOSStorage(tosConfig(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	store.client = s3.New(store.client.Options(), func(o *s3.Options) {
		o.HTTPClient = failedTOSRequest{}
	})
	ctx := t.Context()
	if err := store.Write(ctx, "cat.png", []byte("cat"), "image/png"); err == nil {
		t.Fatal("Write hid the access denied error")
	}
	if _, err := store.Read(ctx, "cat.png"); err == nil {
		t.Fatal("Read hid the access denied error")
	}
	if err := store.Delete(ctx, "cat.png"); err == nil {
		t.Fatal("Delete hid the access denied error")
	}
	health := store.HealthCheck(ctx)
	if health.Healthy || !strings.Contains(health.Error, "403") {
		t.Fatalf("HealthCheck = %+v, want an access denied error", health)
	}
}
