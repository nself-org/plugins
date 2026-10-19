package bad

import _ "unsafe"

//go:linkname hidden example.com/planted/internal/store.(*Store).AdmitPlacement
func hidden()
