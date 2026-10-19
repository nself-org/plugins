//go:build !unix

package publish

import "context"

// lockRun does nothing: the release signer needs a unix host, so no publication
// reaches this point elsewhere.
func lockRun(context.Context, string, string) (func(), error) { return func() {}, nil }
