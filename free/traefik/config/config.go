// Package config embeds the hand-written static Traefik configuration. traefik.yml stays the
// commented source of truth; the renderer (internal/staticcfg) derives the file Traefik reads.
package config

import _ "embed"

// Base is config/traefik.yml, verbatim.
//
//go:embed traefik.yml
var Base []byte
