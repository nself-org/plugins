//go:build windows

package main

import "errors"

// trustedRootPath refuses: uninstall --system has no windows family, and there is no uid model to check.
func trustedRootPath(string) error { return errors.New("no root-ownership check on windows") }
