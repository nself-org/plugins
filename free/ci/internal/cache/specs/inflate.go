package specs

// Every spec scans gzip and ZIP payloads. Share an expanded-byte budget across
// all files in a directory, so compression cannot bypass the save-time cap.

import "github.com/nself-org/plugins/free/ci/internal/authz"

const inflateBudget = 8 << 30

type inflater struct {
	sc   authz.ValueScanner
	left int64
}

func newInflater(sc authz.ValueScanner) *inflater { return &inflater{sc: sc, left: inflateBudget} }
