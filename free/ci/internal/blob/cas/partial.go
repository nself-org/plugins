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

var truncateFile = (*os.File).Truncate

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
	f, err := s.openStoreFile(data, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
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
	b, err := s.readStoreFile(meta)
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
		if s.afterPartialStat != nil {
			s.afterPartialStat()
		}
		f, openErr := s.openStoreFile(data, os.O_WRONLY, 0)
		if openErr != nil {
			return nil, openErr
		}
		current, statErr := f.Stat()
		if statErr != nil || !os.SameFile(info, current) || current.Size() != info.Size() {
			_ = f.Close()
			return nil, ErrPartialConflict
		}
		if err = truncateFile(f, p.offset); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err = f.Close(); err != nil {
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
	if p.store.afterPartialStat != nil {
		p.store.afterPartialStat()
	}
	f, err := p.store.openStoreFile(p.data, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return 0, err
	}
	current, err := f.Stat()
	if err != nil || !os.SameFile(info, current) || current.Size() != p.offset {
		_ = f.Close()
		return 0, ErrPartialConflict
	}
	n, err := writePartialFile(f, b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = syncPartialFile(f)
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
	f, err := p.store.openStoreFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = p.store.removeStore(tmp) }()
	_, err = writeOffsetFile(f, strconv.FormatInt(offset, 10))
	if err == nil {
		err = syncOffsetFile(f)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if p.store.beforeOffsetRename != nil {
		if err := p.store.beforeOffsetRename(); err != nil {
			return err
		}
	}
	if err = p.store.renameStore(tmp, p.offsetPath); err != nil {
		return err
	}
	return p.store.syncStoreDir(filepath.Dir(p.offsetPath))
}

var writePartialFile = (*os.File).Write
var syncPartialFile = (*os.File).Sync
var writeOffsetFile = (*os.File).WriteString
var syncOffsetFile = (*os.File).Sync

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
	f, err := p.store.openStoreFile(p.data, os.O_RDONLY, 0)
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
		if err := p.store.removeStore(p.data); err != nil {
			return errors.Join(ErrPartialConflict, err)
		}
		if err := p.store.removeStore(p.offsetPath); err != nil {
			return errors.Join(ErrPartialConflict, err)
		}
		return ErrPartialConflict
	}
	err = p.store.locked(func() error { return p.store.publish(p.data, p.realm, p.digest) })
	if err != nil {
		return err
	}
	return p.store.removeStore(p.offsetPath)
}
