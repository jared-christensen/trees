package pr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseBitbucketRemote(t *testing.T) {
	good := []string{
		"git@bitbucket.org:ws/r.git",
		"git@ssh.bitbucket.org:ws/r.git",
		"https://me@bitbucket.org/ws/r.git",
		"https://bitbucket.org/ws/r",
		"ssh://git@bitbucket.org/ws/r.git",
	}
	for _, u := range good {
		if got, ok := parseBitbucketRemote(u); !ok || got != "ws/r" {
			t.Errorf("parseBitbucketRemote(%q) = %q, %v; want ws/r, true", u, got, ok)
		}
	}
	for _, u := range []string{"git@github.com:o/r.git", "https://bitbucket.example.com/ws/r", ""} {
		if got, ok := parseBitbucketRemote(u); ok {
			t.Errorf("parseBitbucketRemote(%q) = %q, true; want false", u, got)
		}
	}
}

func TestBitbucketLookup(t *testing.T) {
	var gotQuery, gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		gotUser, _, _ = r.BasicAuth()
		if !strings.HasSuffix(r.URL.Path, "/repositories/ws/r/pullrequests") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if states := r.URL.Query()["state"]; len(states) != 4 {
			t.Errorf("states = %v, want all four", states)
		}
		w.Write([]byte(`{"values":[
		 {"id":1,"state":"DECLINED","updated_on":"2026-01-01T00:00:00Z","source":{"branch":{"name":"a"},"commit":{"hash":"aaa"}}},
		 {"id":3,"state":"OPEN","updated_on":"2026-03-01T00:00:00Z","source":{"branch":{"name":"a"},"commit":{"hash":"ccc"}}},
		 {"id":2,"state":"MERGED","updated_on":"2026-02-01T00:00:00Z","source":{"branch":{"name":"b\"q"},"commit":{"hash":"bbb"}}}
		]}`))
	}))
	defer srv.Close()
	b := newBitbucket("ws/r", "me@example.com", "tok")
	b.baseURL = srv.URL

	got := b.Lookup(context.Background(), []string{"a", `b"q`})
	if want := `source.branch.name="a" OR source.branch.name="b\"q"`; gotQuery != want {
		t.Errorf("q = %s, want %s", gotQuery, want)
	}
	if gotUser != "me@example.com" {
		t.Errorf("basic auth user = %q", gotUser)
	}
	if len(got["a"]) != 2 || got["a"][0].Number != 3 || got["a"][1].State != "CLOSED" {
		t.Errorf("a = %+v, want #3 OPEN then #1 CLOSED", got["a"])
	}
	if p := got[`b"q`]; len(p) != 1 || p[0].State != "MERGED" {
		t.Errorf(`b"q = %+v`, p)
	}
}

func TestBitbucketLookupFailsQuietly(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":{}}`, http.StatusUnauthorized)
		}))
		defer srv.Close()
		b := newBitbucket("ws/r", "u", "t")
		b.baseURL = srv.URL
		if got := b.Lookup(context.Background(), []string{"a"}); got == nil || len(got) != 0 {
			t.Errorf("want empty non-nil map, got %v", got)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}))
		defer srv.Close()
		b := newBitbucket("ws/r", "u", "t")
		b.baseURL = srv.URL
		b.timeout = 200 * time.Millisecond
		start := time.Now()
		if got := b.Lookup(context.Background(), []string{"a"}); len(got) != 0 {
			t.Errorf("want empty map, got %v", got)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Errorf("Lookup took %v, want prompt return", d)
		}
	})
	t.Run("no branches", func(t *testing.T) {
		b := newBitbucket("ws/r", "u", "t")
		b.baseURL = "http://127.0.0.1:1"
		if got := b.Lookup(context.Background(), nil); got == nil || len(got) != 0 {
			t.Errorf("want empty non-nil map, got %v", got)
		}
	})
}
