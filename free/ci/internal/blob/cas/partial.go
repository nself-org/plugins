package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// Partial is a resumable, unpublished transfer owned by one writer.
type Partial struct {
	store                    *Store
	realm                    Realm
	digest, data, offsetPath string
	offset                   int64
}

var truncateFile = os.Truncate

// Partial opens or creates a transfer keyed by realm, digest and writer ID.
func (s *Store) Partial(realm Realm, digest, writerID string) (*Partial, error) {
	if err := realm.Validate(); err != nil {
		return nil, err
	}
	if err := validateDigest(digest); err != nil {
		return nil, err
	}
	if !writerPattern.MatchString(writerID) {
		return nil, ErrInvalid
	}
	dir := filepath.Join(s.blobs(), "tmp", string(realm))
	if err := s.secureDir(dir); err != nil {
		return nil, err
	}
	data := filepath.Join(dir, digest+"."+writerID+".partial")
	meta := data + ".offset"
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(data, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		if err := f.Close(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Lstat(data)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	p := &Partial{store: s, realm: realm, digest: digest, data: data, offsetPath: meta}
	if offsetInfo, statErr := os.Lstat(meta); statErr == nil && !offsetInfo.Mode().IsRegular() {
		return nil, ErrInvalid
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	b, err := os.ReadFile(meta)
	if errors.Is(err, os.ErrNotExist) {
		if info.Size() != 0 {
			return nil, ErrPartialConflict
		}
		if err = p.commitOffset(0); err != nil {
			return nil, err
		}
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	p.offset, err = strconv.ParseInt(string(b), 10, 64)
	if err != nil || p.offset < 0 || info.Size() < p.offset {
		return nil, ErrPartialConflict
	}
	if info.Size() > p.offset {
		if err = truncateFile(data, p.offset); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *Partial) Offset() int64 { return p.offset }

// Resume checks the sender's expected committed offset.
func (p *Partial) Resume(offset int64) error {
	if offset != p.offset {
		return ErrPartialConflict
	}
	return nil
}

// Write durably commits data before publishing its new offset.
func (p *Partial) Write(b []byte) (int, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	if err := p.store.checkDir(filepath.Dir(p.data)); err != nil {
		return 0, err
	}
	info, err := os.Lstat(p.data)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() || info.Size() != p.offset {
		return 0, ErrPartialConflict
	}
	f, err := os.OpenFile(p.data, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return 0, err
	}
	n, err := f.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return n, err
	}
	if err = p.commitOffset(p.offset + int64(n)); err != nil {
		return n, err
	}
	p.offset += int64(n)
	return n, nil
}

func (p *Partial) commitOffset(offset int64) error {
	name, err := randomName()
	if err != nil {
		return err
	}
	tmp := p.offsetPath + "." + name
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	_, err = f.WriteString(strconv.FormatInt(offset, 10))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, p.offsetPath); err != nil {
		return err
	}
	return syncDir(filepath.Dir(p.offsetPath))
}

// Finalize verifies all bytes and publishes only an exact digest match.
func (p *Partial) Finalize() error {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	if err := p.store.checkDir(filepath.Dir(p.data)); err != nil {
		return err
	}
	info, err := os.Lstat(p.data)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != p.offset {
		return ErrPartialConflict
	}
	f, err := os.Open(p.data)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != p.digest {
		if err := os.Remove(p.data); err != nil {
			return errors.Join(ErrPartialConflict, err)
		}
		if err := os.Remove(p.offsetPath); err != nil {
			return errors.Join(ErrPartialConflict, err)
		}
		return ErrPartialConflict
	}
	if err = p.store.locked(func() error { return p.store.publish(p.data, p.realm, p.digest) }); err != nil {
		return err
	}
	return os.Remove(p.offsetPath)
}
