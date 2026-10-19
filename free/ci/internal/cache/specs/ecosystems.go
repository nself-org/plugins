package specs

// Purpose: the five ecosystem registrations (Epic CACHE D5): Go, pnpm and npm,
// cargo, uv and pip, and the buildx local cache for container jobs.
// Inputs:  detect facts. Outputs: registered Specs.
// Constraints: one Dir per package-manager directory; cargo caches only the
// registry and git dirs, never target/; python specs switch on a `stack`
// python fact or a uv/pip/poetry package_manager or lockfile fact (the
// detector has no python rule yet, so they stay dormant until it does).

import "github.com/nself-org/plugins/free/ci/internal/detect"

func init() {
	mustRegister(Spec{ID: "go", Version: "1", Ecosystem: "go",
		Dirs:      []Dir{{"mod", "GOMODCACHE"}, {"build", "GOCACHE"}},
		EnvAllow:  []string{"GOFLAGS", "GOTOOLCHAIN", "CGO_ENABLED", "GOOS", "GOARCH", "GOAMD64", "CC"},
		Lockfiles: []string{"go"}, Probes: []Probe{{"go", []string{"go", "version"}}},
		applies: func(f []detect.Fact) bool { return has(f, "stack", "go") }})
	mustRegister(Spec{ID: "pnpm", Version: "1", Ecosystem: "node",
		Dirs:      []Dir{{"store", "npm_config_store_dir"}},
		EnvAllow:  []string{"npm_config_registry", "pnpm_config_registry"},
		Lockfiles: []string{"pnpm"}, Probes: []Probe{{"node", []string{"node", "--version"}}, {"pnpm", []string{"pnpm", "--version"}}},
		applies: func(f []detect.Fact) bool { return has(f, "package_manager", "pnpm") }})
	mustRegister(Spec{ID: "npm", Version: "1", Ecosystem: "node",
		Dirs:      []Dir{{"cache", "npm_config_cache"}},
		EnvAllow:  []string{"npm_config_registry"},
		Lockfiles: []string{"npm"}, Probes: []Probe{{"node", []string{"node", "--version"}}, {"npm", []string{"npm", "--version"}}},
		applies: func(f []detect.Fact) bool { return has(f, "package_manager", "npm") }})
	mustRegister(Spec{ID: "cargo", Version: "1", Ecosystem: "rust",
		RootEnv:   "CARGO_HOME",
		Dirs:      []Dir{{"git", ""}, {"registry", ""}},
		EnvAllow:  []string{"RUSTFLAGS", "CARGO_*", "CC"},
		Lockfiles: []string{"cargo"}, Probes: []Probe{{"cargo", []string{"cargo", "--version"}}, {"rustc", []string{"rustc", "--version"}}},
		Seed:    SeedFile{HomeEnv: "CARGO_HOME", HomeDir: ".cargo", Names: []string{"config.toml", "config"}, Blockers: []string{"credentials.toml", "credentials"}},
		applies: func(f []detect.Fact) bool { return has(f, "stack", "rust") }})
	mustRegister(Spec{ID: "uv", Version: "1", Ecosystem: "python",
		Dirs:      []Dir{{"cache", "UV_CACHE_DIR"}},
		EnvAllow:  []string{"PIP_INDEX_URL", "UV_INDEX_URL"},
		Lockfiles: []string{"uv"}, Probes: []Probe{{"uv", []string{"uv", "--version"}}},
		applies: func(f []detect.Fact) bool { return pythonWith(f, "uv") }})
	mustRegister(Spec{ID: "pip", Version: "1", Ecosystem: "python",
		Dirs:      []Dir{{"cache", "PIP_CACHE_DIR"}},
		EnvAllow:  []string{"PIP_INDEX_URL", "UV_INDEX_URL"},
		Lockfiles: []string{"pip"}, Probes: []Probe{{"python", []string{"python3", "--version"}}, {"pip", []string{"pip", "--version"}}},
		applies: func(f []detect.Fact) bool { return pythonWith(f, "pip") }})
	mustRegister(Spec{ID: "buildx", Version: "1", Ecosystem: "container", ContainerOnly: true,
		Dirs:    []Dir{{"layers", "NSELF_CI_BUILDX_CACHE_DIR"}},
		applies: func(f []detect.Fact) bool { return has(f, "container", "dockerfile") }})
}

// pythonWith reports a python project that uses manager (uv, pip) or, for pip,
// one that names no manager at all.
func pythonWith(facts []detect.Fact, manager string) bool {
	if has(facts, "package_manager", manager) || has(facts, "lockfile", manager) {
		return true
	}
	return manager == "pip" && has(facts, "stack", "python") && !has(facts, "package_manager", "uv") && !has(facts, "lockfile", "uv")
}
