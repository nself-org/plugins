package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type configStruct struct {
	ProjectName string
	Functions   struct {
		Port int
	}
}

func loadHealthConfig() (*configStruct, string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}

	env := lookup("ENV", dir, "dev")
	files := []string{
		".env",
		".env." + env,
		".env.secrets",
		".env.local",
	}

	merged := map[string]string{}
	for _, f := range files {
		vars, err := parseEnvFile(filepath.Join(dir, f))
		if err == nil {
			for k, v := range vars {
				merged[k] = v
			}
		}
	}

	get := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		if v, ok := merged[key]; ok && v != "" {
			return v
		}
		return def
	}

	port := 3008
	if p := get("FUNCTIONS_PORT", ""); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			port = n
		}
	}

	projectName := get("PROJECT_NAME", "myproject")

	cfg := &configStruct{
		ProjectName: projectName,
	}
	cfg.Functions.Port = port

	return cfg, dir, nil
}

func lookup(key, dir, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	vars, err := parseEnvFile(filepath.Join(dir, ".env"))
	if err == nil {
		if v, ok := vars[key]; ok && v != "" {
			return v
		}
	}
	return def
}

func parseEnvFile(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		if key != "" {
			out[key] = val
		}
	}
	return out, scanner.Err()
}
