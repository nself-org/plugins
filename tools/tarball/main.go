// Command tarball builds deterministic plugin tarballs. It ports
// scripts/build-tarballs.sh (build) and scripts/deterministic-tar.sh
// (det-tar): sorted members, epoch mtime, uid/gid 0, normalized modes.
//
//	go run ./tarball det-tar <out.tar.gz> [-C dir] <path...>
//	go run ./tarball build [-plugins-dir free] [-dist dist] [-source-only] [VERSION]
//
// The tar stream is byte-identical to the script's (GNU tar). The gzip layer
// is Go's deflate, not `gzip -n -9`, so .tar.gz bytes and their sha256 differ
// from the script's output; callers that record checksums must regenerate
// them when they switch to this tool.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// platforms lists the per-platform targets for plugins with a binaryName.
// It is a var so tests can build for one target only.
var platforms = []string{"darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64", "windows-amd64"}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "[tarball] ERROR:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: tarball build|det-tar ...")
	}
	switch args[0] {
	case "det-tar":
		return detTarCmd(args[1:])
	case "build":
		return buildCmd(args[1:], out)
	}
	return fmt.Errorf("unknown verb %q", args[0])
}

func detTarCmd(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: det-tar <output.tar.gz> [-C dir] <path...>")
	}
	out, rest, base := args[0], args[1:], "."
	if len(rest) >= 2 && rest[0] == "-C" {
		base, rest = rest[1], rest[2:]
	}
	if len(rest) == 0 {
		return errors.New("det-tar: no paths")
	}
	for _, p := range rest {
		if p == "-C" {
			return errors.New("det-tar: -C is only supported as the first argument after the output")
		}
	}
	return DetTar(out, base, rest)
}

// writeSum writes "<sha256>  <basename>\n" next to path and returns the hash.
// The file is streamed, never read whole into memory.
func writeSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	h := hex.EncodeToString(sum.Sum(nil))
	line := h + "  " + filepath.Base(path) + "\n"
	return h, os.WriteFile(path+".sha256", []byte(line), 0o644)
}

func buildCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	pluginsDir := fs.String("plugins-dir", envOr("PLUGINS_DIR", "free"), "plugin root")
	dist := fs.String("dist", envOr("DIST_DIR", "dist"), "output directory")
	sourceOnly := fs.Bool("source-only", os.Getenv("NSELF_SKIP_PLATFORM_TARBALLS") == "1", "skip platform tarballs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	version := fs.Arg(0)
	if version == "" {
		var reg struct{ Version string }
		b, err := os.ReadFile("registry.json")
		if err == nil {
			err = json.Unmarshal(b, &reg)
		}
		if err != nil || reg.Version == "" {
			return errors.New("cannot read version from registry.json; pass VERSION")
		}
		version = reg.Version
	}
	if strings.ContainsAny(version, `/\`) || strings.Contains(version, "..") {
		return fmt.Errorf("invalid version %q: it becomes part of file names", version)
	}
	if err := os.MkdirAll(*dist, 0o755); err != nil {
		return err
	}
	ents, err := os.ReadDir(*pluginsDir)
	if err != nil {
		return err
	}
	built, failed := 0, 0
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(*pluginsDir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "plugin.json")); err != nil {
			fmt.Fprintf(out, "[build-tarballs] WARN: Skipping %s (no plugin.json)\n", e.Name())
			continue
		}
		tgz := filepath.Join(*dist, e.Name()+"-"+version+".tar.gz")
		fmt.Fprintf(out, "[build-tarballs] Building %s ...\n", filepath.Base(tgz))
		// The member path is the plugins dir as given plus the name, as the
		// script passes "${plugin_dir%/}": "free/<name>/..." for the default.
		member := strings.TrimRight(filepath.ToSlash(*pluginsDir), "/") + "/" + e.Name()
		if err := DetTar(tgz, ".", []string{member}); err != nil {
			fmt.Fprintf(out, "[build-tarballs] ERROR: %s: %v\n", e.Name(), err)
			failed++
			continue
		}
		h, err := writeSum(tgz)
		if err != nil {
			fmt.Fprintf(out, "[build-tarballs] ERROR: %s: %v\n", e.Name(), err)
			failed++
			continue
		}
		fmt.Fprintf(out, "[build-tarballs]   sha256: %s\n", h)
		built++
		if !*sourceOnly {
			if err := buildPlatforms(dir, e.Name(), version, *dist, out); err != nil {
				fmt.Fprintf(out, "[build-tarballs] ERROR: %s platform tarballs: %v\n", e.Name(), err)
				failed++
			}
		}
	}
	fmt.Fprintf(out, "[build-tarballs] Built %d plugin(s) in %s/\n", built, *dist)
	if failed > 0 {
		return fmt.Errorf("completed with %d error(s)", failed)
	}
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// commands returns the cli commands a plugin builds, or nil if it has no
// binaryName (source tarball only).
func commands(dir string) ([]string, error) {
	var pj struct {
		BinaryName     string `json:"binaryName"`
		Implementation struct {
			BinaryName string `json:"binaryName"`
		} `json:"implementation"`
		CLICommands []struct {
			Name string `json:"name"`
		} `json:"cliCommands"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &pj); err != nil {
		return nil, err
	}
	bin := pj.BinaryName
	if bin == "" {
		bin = pj.Implementation.BinaryName
	}
	if bin == "" {
		return nil, nil
	}
	var cmds []string
	for _, c := range pj.CLICommands {
		cmds = append(cmds, c.Name)
	}
	if len(cmds) == 0 {
		cmds = []string{strings.TrimPrefix(bin, "nself-")}
	}
	return cmds, nil
}

// buildPlatforms cross-compiles each command per platform into a staged
// copy of the plugin and archives it as <name>-<version>-<platform>.tar.gz.
func buildPlatforms(dir, name, version, dist string, out io.Writer) error {
	cmds, err := commands(dir)
	if err != nil || cmds == nil {
		return err
	}
	distAbs, err := filepath.Abs(dist)
	if err != nil {
		return err
	}
	for _, p := range platforms {
		goos, goarch, ok := strings.Cut(p, "-")
		if !ok {
			return fmt.Errorf("bad platform %q", p)
		}
		stage := filepath.Join(distAbs, "stage-"+name+"-"+p)
		if err := os.RemoveAll(stage); err != nil {
			return err
		}
		// Go 1.25 CopyFS copies symlinks; FIFOs and devices still fail loudly.
		if err := os.CopyFS(filepath.Join(stage, name), os.DirFS(dir)); err != nil {
			os.RemoveAll(stage)
			return err
		}
		exe := ""
		if goos == "windows" {
			exe = ".exe"
		}
		for _, c := range cmds {
			pkg := "./cmd/" + c + "/"
			if fi, err := os.Stat(filepath.Join(dir, "cmd", c)); err != nil || !fi.IsDir() {
				pkg = "./cmd/"
			}
			cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w",
				"-o", filepath.Join(stage, name, "nself-"+c+exe), pkg)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
			cmd.Stdout, cmd.Stderr = out, out
			if err := cmd.Run(); err != nil {
				os.RemoveAll(stage)
				return fmt.Errorf("build nself-%s for %s: %w", c, p, err)
			}
		}
		ptar := filepath.Join(dist, name+"-"+version+"-"+p+".tar.gz")
		err := DetTar(ptar, stage, []string{name})
		os.RemoveAll(stage)
		if err != nil {
			return err
		}
		h, err := writeSum(ptar)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "[build-tarballs]   %s sha256: %s\n", p, h)
	}
	return nil
}
