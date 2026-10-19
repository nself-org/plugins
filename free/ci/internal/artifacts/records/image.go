package records

// Purpose: read the index digest and the per-platform manifest digests out of
// an OCI image layout tarball (`docker buildx build --output type=oci,dest=…`)
// without extracting it or contacting a registry (Epic CACHE D6).
// Inputs:  the tarball bytes as a reader.
// Outputs: ImageInfo{IndexDigest, Manifests[{Platform, Digest}]}.
// Constraints: streaming, bounded: index.json and each nested index at most
// 1 MiB, at most 64 Ki entries, at most 32 MiB of small blobs held at once,
// index nesting at most 3 deep. Small blobs are verified against the digest
// in their name before use. Nothing is written to disk; the fabric never
// pushes an image.

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
)

const (
	imageMaxEntries = 1 << 16
	imageMaxSmall   = 1 << 20
	imageMaxHeld    = 32 << 20
	imageMaxDepth   = 3
)

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Platform  *struct {
		OS, Architecture, Variant string
	} `json:"platform"`
}

type ociIndex struct {
	Manifests []ociDescriptor `json:"manifests"`
}

func isIndexType(mt string) bool {
	return mt == "application/vnd.oci.image.index.v1+json" || mt == "application/vnd.docker.distribution.manifest.list.v2+json"
}

// ReadImageLayout parses an OCI layout tarball.
func ReadImageLayout(r io.Reader) (*ImageInfo, error) {
	tr := tar.NewReader(r)
	var index []byte
	small := map[string][]byte{}
	held := 0
	for n := 0; ; n++ {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, failf(ReasonImage, "not a readable tar: %v", err)
		}
		if n >= imageMaxEntries {
			return nil, failf(ReasonImage, "more than %d entries", imageMaxEntries)
		}
		name := strings.TrimPrefix(strings.TrimPrefix(h.Name, "./"), "/")
		if h.Typeflag != tar.TypeReg || h.Size > imageMaxSmall {
			continue
		}
		isIndex := name == "index.json"
		blobName, isBlob := strings.CutPrefix(name, "blobs/sha256/")
		if !isIndex && !isBlob {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, imageMaxSmall+1))
		if err != nil || len(b) > imageMaxSmall {
			return nil, failf(ReasonImage, "%s is unreadable or too large", name)
		}
		if isIndex {
			index = b
			continue
		}
		if held += len(b); held > imageMaxHeld {
			return nil, failf(ReasonImage, "small blobs exceed %d B", imageMaxHeld)
		}
		small[blobName] = b
	}
	if index == nil {
		return nil, failf(ReasonImage, "no index.json: not an OCI image layout")
	}
	sum := sha256.Sum256(index)
	info := &ImageInfo{IndexDigest: "sha256:" + hex.EncodeToString(sum[:])}
	if err := collectManifests(index, small, 1, info); err != nil {
		return nil, err
	}
	if len(info.Manifests) == 0 {
		return nil, failf(ReasonImage, "index.json lists no manifest")
	}
	sort.Slice(info.Manifests, func(i, j int) bool {
		a, b := info.Manifests[i], info.Manifests[j]
		return a.Platform < b.Platform || a.Platform == b.Platform && a.Digest < b.Digest
	})
	return info, nil
}

func collectManifests(index []byte, small map[string][]byte, depth int, out *ImageInfo) error {
	var ix ociIndex
	if err := json.NewDecoder(bytes.NewReader(index)).Decode(&ix); err != nil {
		return failf(ReasonImage, "index is not JSON: %v", err)
	}
	for _, d := range ix.Manifests {
		want, ok := strings.CutPrefix(d.Digest, "sha256:")
		if !ok || len(want) != 64 {
			return failf(ReasonImage, "descriptor digest %q is not sha256:<hex>", d.Digest)
		}
		if isIndexType(d.MediaType) {
			if depth >= imageMaxDepth {
				return failf(ReasonImage, "index nesting deeper than %d", imageMaxDepth)
			}
			b, ok := small[want]
			if !ok {
				return failf(ReasonImage, "nested index %s is not in the layout", d.Digest)
			}
			if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != want {
				return failf(ReasonImage, "nested index %s does not match its digest", d.Digest)
			}
			if err := collectManifests(b, small, depth+1, out); err != nil {
				return err
			}
			continue
		}
		platform := "unknown"
		if p := d.Platform; p != nil && p.OS != "" && p.Architecture != "" {
			platform = p.OS + "/" + p.Architecture
			if p.Variant != "" {
				platform += "/" + p.Variant
			}
		}
		out.Manifests = append(out.Manifests, ImageManifest{Platform: platform, Digest: d.Digest})
	}
	return nil
}
