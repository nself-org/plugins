package svc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type receipt struct {
	Hashes map[string]string `json:"hashes"`
	Active bool              `json:"active"`
}

func digest(b []byte) string         { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (c Config) receiptPath() string { return filepath.Join(c.dir(), ".nself-ci-watch.receipt.json") }

func readRegular(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing non-regular service file %s", path)
	}
	return os.ReadFile(path)
}

func (c Config) receipt() (receipt, error) {
	r := receipt{Hashes: map[string]string{}}
	b, err := readRegular(c.receiptPath())
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.Hashes == nil {
		return r, fmt.Errorf("invalid service receipt")
	}
	return r, nil
}

func atomicWrite(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ci-service-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (c Config) save(r receipt) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(c.receiptPath(), append(b, '\n'))
}

// Install refuses unowned or edited files, writes only changes, and reconciles
// the manager only after a change or an interrupted activation.
func (i Installer) Install(ctx context.Context) error {
	c := i.Config
	files, err := Files(c)
	if err != nil {
		return err
	}
	r, err := c.receipt()
	if err != nil {
		return err
	}
	var changed []string
	for _, n := range c.names() {
		b, e := readRegular(filepath.Join(c.dir(), n))
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if e == nil && (r.Hashes[n] == "" || digest(b) != r.Hashes[n]) {
			return fmt.Errorf("leaving user-edited or unowned unit alone: %s", n)
		}
		if e != nil || digest(b) != digest(files[n]) {
			changed = append(changed, n)
		}
	}
	if len(changed) == 0 && r.Active {
		fmt.Fprintln(i.writer(), "unchanged")
		return nil
	}
	manager, err := ManagerPath(c)
	if c.DryRun {
		manager = c.Manager
		if c.Manager == "launchd" {
			manager = "launchctl"
		} else {
			manager = "systemctl"
		}
		err = nil
	}
	if err != nil {
		return err
	}
	if !c.DryRun && i.Run == nil {
		return fmt.Errorf("no service manager runner")
	}
	if err = i.prepareSystem(ctx); err != nil {
		return err
	}
	for _, n := range changed {
		fmt.Fprintf(i.writer(), "write %s\n", filepath.Join(c.dir(), n))
		if c.DryRun {
			fmt.Fprint(i.writer(), string(files[n]))
		}
		if !c.DryRun {
			if err = os.MkdirAll(c.dir(), 0755); err != nil {
				return err
			}
			if err = atomicWrite(filepath.Join(c.dir(), n), files[n]); err != nil {
				return err
			}
			if err = os.Chmod(filepath.Join(c.dir(), n), 0644); err != nil {
				return err
			}
		}
		r.Hashes[n] = digest(files[n])
	}
	if !c.DryRun {
		if !c.System {
			err = os.MkdirAll(filepath.Join(c.CIHome, "logs", "watch"), 0700)
		}
		if err != nil {
			return err
		}
		r.Active = false
		if err = c.save(r); err != nil {
			return err
		}
	}
	// Retry all jobs after an interrupted activation, including unchanged files.
	if err = i.activate(ctx, manager, c.names()); err != nil {
		return err
	}
	if c.DryRun {
		return nil
	}
	r.Active = true
	return c.save(r)
}

// Uninstall removes only files whose contents still match the install receipt.
// Edited units remain loaded, remain on disk, and retain their ownership record.
func (i Installer) Uninstall(ctx context.Context) error {
	c := i.Config
	if err := c.validate(); err != nil {
		return err
	}
	r, err := c.receipt()
	if err != nil {
		return err
	}
	var remove []string
	for _, n := range c.names() {
		if r.Hashes[n] == "" {
			continue
		}
		b, e := readRegular(filepath.Join(c.dir(), n))
		if errors.Is(e, os.ErrNotExist) {
			delete(r.Hashes, n)
			continue
		}
		if e != nil || digest(b) != r.Hashes[n] {
			fmt.Fprintf(i.writer(), "leaving user-edited unit alone: %s\n", n)
			continue
		}
		remove = append(remove, n)
	}
	if len(remove) == 0 {
		return nil
	}
	manager, err := ManagerPath(c)
	if c.DryRun {
		manager = "systemctl"
		if c.Manager == "launchd" {
			manager = "launchctl"
		}
		err = nil
	}
	if err != nil {
		return err
	}
	if !c.DryRun && i.Run == nil {
		return fmt.Errorf("no service manager runner")
	}
	for _, n := range remove {
		if err = i.deactivate(ctx, manager, n); err != nil {
			return err
		}
		fmt.Fprintf(i.writer(), "remove %s\n", filepath.Join(c.dir(), n))
		if !c.DryRun {
			if err = os.Remove(filepath.Join(c.dir(), n)); err != nil {
				return err
			}
			delete(r.Hashes, n)
			if err = c.save(r); err != nil {
				return err
			}
		}
	}
	if c.Manager == "systemd" {
		args := []string{"daemon-reload"}
		if !c.System {
			args = append([]string{"--user"}, args...)
		}
		if err = i.command(ctx, manager, args...); err != nil {
			return err
		}
	}
	if !c.DryRun && len(r.Hashes) == 0 {
		return os.Remove(c.receiptPath())
	}
	return nil
}
