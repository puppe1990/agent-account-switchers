package verify

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerify_accepts2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("auth %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if err := c.Verify("sk-test"); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_rejected401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Verify("sk-bad")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "a chave ativa foi recusada pela API OpenCode Go (HTTP 401). Troque de conta ou gere outra chave."
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestVerify_otherHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := (&Client{BaseURL: srv.URL, HTTPClient: srv.Client()}).Verify("sk-x")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "a API OpenCode Go respondeu HTTP 500 ao verificar a chave ativa."
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestVerify_network(t *testing.T) {
	c := &Client{
		BaseURL:    "http://127.0.0.1:1",
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	}
	err := c.Verify("sk-x")
	if err == nil {
		t.Fatal("expected error")
	}
	prefix := "não consegui falar com a API OpenCode Go em "
	if len(err.Error()) < len(prefix) || err.Error()[:len(prefix)] != prefix {
		t.Fatalf("got %q", err.Error())
	}
}
