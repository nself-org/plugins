package run

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/locality"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// CacheLocalitySource uses the restore hook's facts and keys without restoring,
// saving, touching cache rows or reading blobs. AttemptID is the scheduler key;
// plan callers pass their planned key and leave RunID empty (local owner facts).
func CacheLocalitySource(st *store.Store, row store.PipelineRow, attempts []AttemptInfo) locality.Source {
	return func(ctx context.Context) (locality.Inputs, error) {
		in := locality.Inputs{Ledger: st}
		if st == nil || topLevelDisabled(row.Project) || off(lookup(cacheOn.environ(), "NSELF_CI_CACHE_DISABLE")) {
			return in, nil
		}
		for _, a := range attempts {
			j := a.Job
			if j.Cache != nil && j.Cache.Disable || off(j.Env["NSELF_CI_CACHE_DISABLE"]) {
				continue
			}
			level, ok := cacheLevel(a)
			if !ok {
				if j.Isolation == "sandboxed-container" {
					level, ok = "sandboxed-container", true
				}
				if !ok {
					continue
				}
			}
			var scope, keys []string
			if j.Cache != nil {
				var err error
				scope, err = specs.CheckPaths(j.Cache.Paths)
				if err != nil {
					continue
				}
				keys = append(keys, j.Cache.Keys...)
			}
			dir, err := workdirOf(row.Project, j.Workdir)
			if err != nil {
				continue
			}
			detected, err := detect.DefaultRegistry().Detect(os.DirFS(dir), func(string) bool { return true })
			if err != nil {
				continue
			}
			detected = specs.Narrow(detected, scope)
			att := CacheAttestation{Protection: string(authz.Unprotected)}
			if a.RunID != "" && CacheAttest != nil {
				att, err = CacheAttest(ctx, a, row)
				if err != nil {
					continue
				}
			}
			env := flatEnv(mergeEnv(cacheOn.environ(), j.Env))
			priv, _ := specs.PrivateDeps(specs.PrivInput{Dirs: factDirs(row.Project, dir, detected), Home: cacheOn.home(), Environ: env})
			image := ""
			container := level == "container" || level == "sandboxed-container"
			if container {
				// Placement may only use an already pinned image. The execution image
				// hook can pull a toolchain, which is forbidden during a world build.
				if j.Container != nil {
					_, image, _ = strings.Cut(j.Container.Image, "@")
				}
				if !strings.HasPrefix(image, "sha256:") {
					image = ""
				}
			}
			in.Jobs = append(in.Jobs, locality.Job{Key: a.AttemptID, Facts: cacheFacts(a, row, att, priv), Detected: detected, Scope: scope,
				ExtraKeys: append(keys, "job="+a.JobName), Environ: env, Isolation: level, Container: container, Image: image, Probe: cacheOn.probe})
		}
		return in, nil
	}
}

func (rd *runDispatch) cacheLocalitySource() locality.Source {
	return func(ctx context.Context) (locality.Inputs, error) {
		rows, err := rd.sess.Store.PipelineJobs(ctx, rd.sess.RunID)
		if err != nil {
			return locality.Inputs{}, err
		}
		rd.sess.mu.Lock()
		jobs := make(map[string]model.Job, len(rd.sess.byID))
		for id, j := range rd.sess.byID {
			jobs[id] = j
		}
		rd.sess.mu.Unlock()
		var attempts []AttemptInfo
		for _, row := range rows {
			j, ok := jobs[row.ID]
			if !ok {
				continue
			}
			aa, err := rd.sess.Store.AttemptsByJob(ctx, row.ID)
			if err != nil {
				return locality.Inputs{}, err
			}
			for _, a := range aa {
				if a.State != "queued" && a.State != "leased" && a.State != "running" {
					continue
				}
				attempts = append(attempts, AttemptInfo{AttemptID: a.ID, JobName: row.Name, Job: j, RunID: rd.sess.RunID, Store: rd.sess.Store, Home: rd.sess.Home, Container: rd.sess.Container})
			}
		}
		sort.Slice(attempts, func(i, j int) bool { return attempts[i].AttemptID < attempts[j].AttemptID })
		return CacheLocalitySource(rd.sess.Store, rd.row, attempts)(ctx)
	}
}
