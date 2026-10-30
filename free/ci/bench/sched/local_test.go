package sched

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func fixture(b *testing.B, packages int) string {
	b.Helper()
	root := b.TempDir()
	for i := 0; i < packages; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%03d", i))
		if err := os.Mkdir(dir, 0700); err != nil {
			b.Fatal(err)
		}
		for name, data := range map[string]string{
			"go.mod":  "module example.test/fixture\n\ngo 1.26\n",
			"main.go": "package fixture\nfunc Value() int { return 1 }\n",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
				b.Fatal(err)
			}
		}
	}
	return root
}

// BenchmarkDetect measures real filesystem discovery on generated fixture repos.
func BenchmarkDetect(b *testing.B) {
	for _, size := range []struct {
		name     string
		packages int
	}{{"tiny", 1}, {"medium", 32}} {
		b.Run(size.name, func(b *testing.B) {
			root := fixture(b, size.packages)
			registry := detect.DefaultRegistry()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				facts, err := registry.Detect(os.DirFS(root), func(string) bool { return true })
				if err != nil || len(facts) == 0 {
					b.Fatalf("detect facts=%d: %v", len(facts), err)
				}
			}
		})
	}
}

// BenchmarkPlan measures planning with detection outside the timed interval.
func BenchmarkPlan(b *testing.B) {
	for _, size := range []struct {
		name     string
		packages int
	}{{"tiny", 1}, {"medium", 32}} {
		b.Run(size.name, func(b *testing.B) {
			root := fixture(b, size.packages)
			facts, err := detect.DefaultRegistry().Detect(os.DirFS(root), func(string) bool { return true })
			if err != nil {
				b.Fatal(err)
			}
			in := plan.Input{Facts: facts}
			for i := 0; i < size.packages; i++ {
				in.Checks = append(in.Checks, plan.Check{
					ID: fmt.Sprintf("go.test.%d", i), Kind: "test", Tool: "go",
					Workdir: fmt.Sprintf("pkg%03d", i), Required: true,
					Argv: []string{"go", "test", "./..."},
				})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p, err := plan.Build(in)
				if err != nil || len(p.Jobs) != size.packages {
					b.Fatalf("plan: %v", err)
				}
			}
		})
	}
}

// BenchmarkStoreOpen measures fresh migrations separately from existing stores.
func BenchmarkStoreOpen(b *testing.B) {
	for _, fresh := range []bool{false, true} {
		b.Run(fmt.Sprintf("fresh=%t", fresh), func(b *testing.B) {
			root := b.TempDir()
			open := func(path string) {
				s, err := store.Open(path, store.Options{})
				if err != nil {
					b.Fatal(err)
				}
				if err := s.Close(); err != nil {
					b.Fatal(err)
				}
			}
			path := filepath.Join(root, "state.db")
			if !fresh {
				open(path)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if fresh {
					root = b.TempDir()
					path = filepath.Join(root, "state.db")
				}
				b.StartTimer()
				open(path)
			}
		})
	}
}
