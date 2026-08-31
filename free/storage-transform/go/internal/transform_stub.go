//go:build !cgo

// transform_stub.go provides no-op implementations of Transformer for builds
// where CGO is disabled (e.g. unit test runs on machines without libvips).
// The real implementation in transform.go requires CGO for govips.
package internal

import "errors"

// Transformer is the no-op stub used when CGO is disabled.
type Transformer struct{}

// NewTransformer returns a stub transformer.
func NewTransformer() *Transformer { return &Transformer{} }

// Shutdown is a no-op.
func (t *Transformer) Shutdown() {}

// Transform returns an error when CGO is not available.
func (t *Transformer) Transform(_ []byte, _ TransformParams) ([]byte, string, error) {
	return nil, "", errors.New("image transform unavailable: build with CGO_ENABLED=1 and libvips installed")
}
