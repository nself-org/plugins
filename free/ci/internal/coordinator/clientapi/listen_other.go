//go:build !unix

package clientapi

import (
	"errors"
	"net"
)

// ListenUnix is unavailable here; a local serve is then reached over the user API address only.
func ListenUnix(string) (net.Listener, error) {
	return nil, errors.New("unix sockets are not supported on this platform")
}
