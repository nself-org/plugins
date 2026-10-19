package backup

// Purpose: the coordinator backup archive (P7-NODE-20, Epic D5/D21): a plain tar of manifest.json,
//          manifest.mac, state.db, pki/*.sealed and, for a CI home, files/known_hosts and files/config.yaml.
// Inputs:  a KEK (never written), the entries, the manifest fields {schema, incarnation, created_at}.
// Outputs: Write emits the tar; Read parses it and authenticates it: HMAC-SHA256 under
//          HKDF-SHA256(KEK, info "nself-ci-backup-v1") over the fields and the SHA-256 of ALL entry bytes,
//          so one flipped byte anywhere in the content, or another KEK, is E668 reason mac.
// Constraints: the archive is not encrypted (the DB holds no secret values, keys are sealed in it); the
//          KEK and any unsealed key are never entries; the MAC compare is constant time; Read touches no
//          file; only the entry names listed above are accepted (no path from the archive reaches the disk
//          unchecked); the archive is read in memory up to MaxArchive.

import (
	"archive/tar"
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Schema is the manifest version this package writes and reads.
const Schema = 1

// MaxArchive caps the bytes Read accepts.
const MaxArchive = 1 << 30

const (
	hkdfInfo     = "nself-ci-backup-v1"
	nameManifest = "manifest.json"
	nameMAC      = "manifest.mac"
	nameState    = "state.db"
	dirPKI       = "pki"
	dirFiles     = "files"
)

// Manifest is manifest.json. SHA256 is the digest of the content entries (see payloadDigest).
type Manifest struct {
	Schema      int         `json:"schema"`
	Incarnation int64       `json:"incarnation"`
	CreatedAt   string      `json:"created_at"`
	SHA256      string      `json:"sha256"`
	Files       []FileEntry `json:"files"`
}

// FileEntry lists one content entry.
type FileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Entry is one content file of the archive.
type Entry struct {
	Name string
	Data []byte
}

func macKey(kek []byte) ([]byte, error) { return hkdf.Key(sha256.New, kek, nil, hkdfInfo, sha256.Size) }

// payloadDigest is SHA-256 over name NUL size NUL sha256(content) LF of each entry in name order.
func payloadDigest(es []Entry) string {
	sorted := append([]Entry(nil), es...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha256.New()
	for _, e := range sorted {
		sum := sha256.Sum256(e.Data)
		h.Write([]byte(e.Name + "\x00" + strconv.Itoa(len(e.Data)) + "\x00" + hex.EncodeToString(sum[:]) + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func macOf(kek []byte, schema int, incarnation int64, createdAt, digest string) ([]byte, error) {
	key, err := macKey(kek)
	if err != nil {
		return nil, err
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(hkdfInfo + "\nschema=" + strconv.Itoa(schema) + "\nincarnation=" + strconv.FormatInt(incarnation, 10) + "\ncreated_at=" + createdAt + "\nsha256=" + digest + "\n"))
	return m.Sum(nil), nil
}

// safeName reports whether an entry name is one this format writes.
func safeName(n string) bool {
	if n == nameState || n == nameManifest || n == nameMAC {
		return true
	}
	dir, base := path.Split(n)
	if base == "" || base != path.Base(n) || strings.ContainsAny(base, `\`+"\x00") || base == "." || base == ".." {
		return false
	}
	switch dir {
	case dirPKI + "/":
		return strings.HasSuffix(base, ".sealed") && base != ".sealed"
	case dirFiles + "/":
		return base == "known_hosts" || base == "config.yaml"
	}
	return false
}

// Write emits the archive. The manifest's SHA256 and Files are computed here.
func Write(w io.Writer, kek []byte, m Manifest, es []Entry) error {
	m.Schema = Schema
	m.SHA256 = payloadDigest(es)
	m.Files = nil
	sorted := append([]Entry(nil), es...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, e := range sorted {
		if !safeName(e.Name) || e.Name == nameManifest || e.Name == nameMAC {
			return fail(ReasonFormat, "entry %q is not a backup entry", e.Name)
		}
		m.Files = append(m.Files, FileEntry{Name: e.Name, Size: int64(len(e.Data))})
	}
	mac, err := macOf(kek, m.Schema, m.Incarnation, m.CreatedAt, m.SHA256)
	if err != nil {
		return fail(ReasonMissingKEK, "the key-encryption key cannot derive a backup key")
	}
	doc, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	tw := tar.NewWriter(w)
	all := append([]Entry{{nameManifest, doc}, {nameMAC, []byte(hex.EncodeToString(mac) + "\n")}}, sorted...)
	for _, e := range all {
		if err = tw.WriteHeader(&tar.Header{Name: e.Name, Mode: 0o600, Size: int64(len(e.Data)), Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			return err
		}
		if _, err = tw.Write(e.Data); err != nil {
			return err
		}
	}
	return tw.Close()
}

// Read parses and authenticates an archive. It writes nothing. Reasons: format (unexpected entry),
// checksum (the archive is damaged or its manifest disagrees with its content), version, mac.
func Read(r io.Reader, kek []byte) (Manifest, []Entry, error) {
	var m Manifest
	raw, err := io.ReadAll(io.LimitReader(r, MaxArchive+1))
	if err != nil {
		return m, nil, fail(ReasonIO, "the backup cannot be read: %v", err)
	}
	if len(raw) > MaxArchive {
		return m, nil, fail(ReasonTooLarge, "the backup is larger than %d bytes", MaxArchive)
	}
	byName, es, err := readTar(raw)
	if err != nil {
		return m, nil, err
	}
	doc, ok1 := byName[nameManifest]
	macHex, ok2 := byName[nameMAC]
	if !ok1 || !ok2 || len(es) == 0 {
		return m, nil, fail(ReasonChecksum, "the backup has no manifest or no content")
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&m); err != nil {
		return Manifest{}, nil, fail(ReasonChecksum, "the manifest is damaged")
	}
	if m.Schema != Schema {
		return Manifest{}, nil, fail(ReasonVersion, "backup schema %d is not supported (this build reads %d)", m.Schema, Schema)
	}
	want, err := macOf(kek, m.Schema, m.Incarnation, m.CreatedAt, payloadDigest(es))
	if err != nil {
		return Manifest{}, nil, fail(ReasonMissingKEK, "the key-encryption key cannot derive a backup key")
	}
	got, derr := hex.DecodeString(strings.TrimSpace(string(macHex)))
	if derr != nil || !hmac.Equal(got, want) {
		return Manifest{}, nil, fail(ReasonMAC, "the backup does not authenticate under this KEK (another KEK, or the backup was changed)")
	}
	if m.SHA256 != payloadDigest(es) {
		return Manifest{}, nil, fail(ReasonChecksum, "the manifest checksum differs from the content")
	}
	return m, es, nil
}

// readTar returns every entry by name and the content entries (everything but manifest.json and manifest.mac).
func readTar(raw []byte) (map[string][]byte, []Entry, error) {
	byName := map[string][]byte{}
	var es []Entry
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return byName, es, nil
		}
		if err != nil {
			return nil, nil, fail(ReasonChecksum, "the backup is damaged (not a readable archive)")
		}
		if h.Typeflag != tar.TypeReg || !safeName(h.Name) {
			return nil, nil, fail(ReasonFormat, "entry %q is not a backup entry", h.Name)
		}
		if _, dup := byName[h.Name]; dup {
			return nil, nil, fail(ReasonFormat, "entry %q appears twice", h.Name)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, nil, fail(ReasonChecksum, "the backup is damaged (entry %q is cut short)", h.Name)
		}
		byName[h.Name] = data
		if h.Name != nameManifest && h.Name != nameMAC {
			es = append(es, Entry{Name: h.Name, Data: data})
		}
	}
}
