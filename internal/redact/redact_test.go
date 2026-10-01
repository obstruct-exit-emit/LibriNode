package redact

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// doFailingRequest issues a request that always fails at the transport level
// (nothing listens on this port) so http.Client.Do returns a real *url.Error
// carrying the exact URL that was requested — the same shape production
// code hits.
func doFailingRequest(t *testing.T, rawURL string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&http.Client{}).Do(req)
	if err == nil {
		t.Fatal("expected the request to fail (nothing should be listening on this port)")
	}
	return err
}

func TestURLErrorRedactsKnownParams(t *testing.T) {
	cases := []struct{ param string }{{"apikey"}, {"api_key"}, {"token"}, {"password"}}
	for _, c := range cases {
		t.Run(c.param, func(t *testing.T) {
			raw := "http://127.0.0.1:1/api?t=search&" + c.param + "=super-secret-value&q=dune"
			err := doFailingRequest(t, raw)
			out := URLError(err).Error()
			if strings.Contains(out, "super-secret-value") {
				t.Errorf("secret leaked through: %q", out)
			}
			if !strings.Contains(out, "REDACTED") {
				t.Errorf("expected REDACTED marker in %q", out)
			}
			// Everything else about the URL survives, so the message still
			// helps debugging.
			if !strings.Contains(out, "q=dune") {
				t.Errorf("unrelated query params should survive: %q", out)
			}
		})
	}
}

func TestURLErrorPassesThroughWithoutSecrets(t *testing.T) {
	raw := "http://127.0.0.1:1/api?t=search&q=dune"
	err := doFailingRequest(t, raw)
	out := URLError(err)
	if out.Error() != err.Error() {
		t.Errorf("error without sensitive params should pass through unchanged:\n got  %q\n want %q",
			out.Error(), err.Error())
	}
}

func TestValuesAndText(t *testing.T) {
	raw := "http://indexer.example/api?t=search&apikey=super-secret-value&q=dune"
	secrets := Values(raw)
	if len(secrets) != 1 || secrets[0] != "super-secret-value" {
		t.Fatalf("Values = %v, want [super-secret-value]", secrets)
	}

	body := `<error code="100" description="query was: q=dune&apikey=super-secret-value"/>`
	scrubbed := Text(body, secrets)
	if strings.Contains(scrubbed, "super-secret-value") {
		t.Errorf("secret survived Text(): %q", scrubbed)
	}
	if !strings.Contains(scrubbed, "REDACTED") {
		t.Errorf("expected REDACTED marker in %q", scrubbed)
	}

	// A URL with nothing sensitive yields no values, and Text is a no-op.
	if got := Values("http://indexer.example/api?t=search&q=dune"); len(got) != 0 {
		t.Errorf("Values with no secrets = %v, want empty", got)
	}
	if got := Text("plain text", nil); got != "plain text" {
		t.Errorf("Text with no secrets should pass through unchanged, got %q", got)
	}
}

// TestNestedURLParam covers SABnzbd's addurl, which carries the indexer
// download URL — its own apikey included — as the "name" parameter. Both the
// error URL and an echoed response body must have the nested key scrubbed.
func TestNestedURLParam(t *testing.T) {
	indexerURL := "https://indexer.example/dl?id=42&apikey=indexer-secret"
	raw := "http://sab.local:8080/api?mode=addurl&apikey=sab-secret&name=" + url.QueryEscape(indexerURL)

	err := doFailingRequest(t, raw)
	out := URLError(err).Error()
	for _, leak := range []string{"indexer-secret", "sab-secret"} {
		if strings.Contains(out, leak) {
			t.Errorf("secret %q leaked through URLError: %q", leak, out)
		}
	}

	secrets := Values(raw)
	body := `{"status": false, "error": "could not fetch ` + indexerURL + `"}`
	if scrubbed := Text(body, secrets); strings.Contains(scrubbed, "indexer-secret") {
		t.Errorf("nested indexer key survived Values+Text: %q", scrubbed)
	}
}

func TestURLErrorNonURLErrorPassesThrough(t *testing.T) {
	plain := errors.New("some other failure")
	if got := URLError(plain); got != plain {
		t.Errorf("non-*url.Error should pass through as the same value, got %v", got)
	}
	if URLError(nil) != nil {
		t.Error("URLError(nil) should return nil")
	}
}
