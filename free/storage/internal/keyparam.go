package internal

import (
	"net/http"
	"net/url"
	"strings"
)

// objectKeyParam returns the object key of /storage/buckets/{bucket}/objects/{key}, percent-decoded exactly once.
// chi's URL parameter is sometimes already decoded (when net/url has no RawPath) and sometimes raw, so
// "a%2Fb" and "a%252Fb" would both arrive as "a%2Fb" and alias each other. The escaped path is always the same
// shape, so the key is cut from it and decoded here: "a%2Fb" is "a/b", "a%252Fb" is "a%2Fb". ok is false when the
// path has no key segment or the escape is malformed.
func objectKeyParam(r *http.Request) (key string, ok bool) {
	parts := strings.SplitN(r.URL.EscapedPath(), "/", 6) // "", storage, buckets, {bucket}, objects, {key}
	if len(parts) != 6 || parts[4] != "objects" {
		return "", false
	}
	key, err := url.PathUnescape(parts[5])
	if err != nil {
		return "", false
	}
	return key, true
}
