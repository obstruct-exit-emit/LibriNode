// Package redact strips credential-shaped query parameters out of HTTP
// errors before they're logged or shown to a user. Several protocols
// LibriNode speaks put the secret directly in the request URL's query
// string (Newznab/Torznab's apikey, SABnzbd's apikey, ComicVine's api_key) —
// a failed request there comes back as a *url.Error whose default Error()
// text is "<Op> \"<URL-with-secret>\": <cause>", so logging or displaying it
// as-is leaks the credential into the log file's log viewer, health
// banners, and search-error messages a user might paste into a bug report.
package redact

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// sensitiveParams are the query-string keys every client in this codebase
// uses to carry a secret. Matched case-sensitively against how each client
// actually sets them (see indexer.apiURL, sabnzbd's params, comicvine.get).
var sensitiveParams = []string{"apikey", "api_key", "token", "password", "key"}

// URLError redacts any sensitive query parameters from a *url.Error's
// embedded URL, returning a new error with the same cause but a safe
// message. Errors that aren't a *url.Error (or whose URL carries none of the
// known sensitive params) pass through unchanged, so this is safe to wrap
// around every request unconditionally.
func URLError(err error) error {
	if err == nil {
		return nil
	}
	var uerr *url.Error
	if !errors.As(err, &uerr) {
		return err
	}
	u, perr := url.Parse(uerr.URL)
	if perr != nil {
		// Can't parse it to redact safely — drop the URL entirely rather
		// than risk leaking it verbatim.
		return fmt.Errorf("%s <url redacted, unparseable>: %w", uerr.Op, uerr.Err)
	}
	q := u.Query()
	if !redactQuery(q, 0) {
		return err
	}
	u.RawQuery = q.Encode()
	return fmt.Errorf("%s %q: %w", uerr.Op, u.String(), uerr.Err)
}

// redactQuery replaces sensitive parameter values in q with "REDACTED", in
// place, and recurses one level into any parameter value that is itself a URL
// (SABnzbd's addurl passes the indexer download URL — apikey and all — as its
// "name" parameter). Returns whether anything was redacted.
func redactQuery(q url.Values, depth int) bool {
	redacted := false
	for _, key := range sensitiveParams {
		if q.Get(key) != "" {
			q.Set(key, "REDACTED")
			redacted = true
		}
	}
	if depth < 2 {
		for key, vs := range q {
			for i, v := range vs {
				if !strings.Contains(v, "://") {
					continue
				}
				nu, err := url.Parse(v)
				if err != nil || nu.RawQuery == "" {
					continue
				}
				nq := nu.Query()
				if redactQuery(nq, depth+1) {
					nu.RawQuery = nq.Encode()
					q[key][i] = nu.String()
					redacted = true
				}
			}
		}
	}
	return redacted
}

// Values returns the values of any sensitive query parameters present in a
// raw request URL — the same ones URLError strips from the URL itself. Use
// it to also scrub those exact values out of a response body or other free
// text before it's included in an error, in case a server echoes the
// request back (some error pages restate the query string verbatim).
func Values(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	return queryValues(u.Query(), 0)
}

// queryValues collects sensitive parameter values from q, recursing one level
// into any URL-valued parameter (see redactQuery) so a nested indexer apikey is
// scrubbed from an echoed response body too.
func queryValues(q url.Values, depth int) []string {
	var out []string
	for _, key := range sensitiveParams {
		if v := q.Get(key); v != "" {
			out = append(out, v)
		}
	}
	if depth < 2 {
		for _, vs := range q {
			for _, v := range vs {
				if !strings.Contains(v, "://") {
					continue
				}
				if nu, err := url.Parse(v); err == nil && nu.RawQuery != "" {
					out = append(out, queryValues(nu.Query(), depth+1)...)
				}
			}
		}
	}
	return out
}

// Text replaces every occurrence of each given secret value in s with
// "REDACTED". Empty values are ignored (an empty string would otherwise
// "match" everywhere).
func Text(s string, secrets []string) string {
	for _, v := range secrets {
		if v == "" {
			continue
		}
		s = strings.ReplaceAll(s, v, "REDACTED")
	}
	return s
}
