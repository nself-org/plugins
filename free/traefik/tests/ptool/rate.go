package main

import (
	"fmt"
	"time"
)

// burstRun is what one provider did with a rapid burst at a limited location.
type burstRun struct {
	first    int // status of the first request
	admitted int // requests answered with anything but 429
	limited  int // requests answered 429
	max      int // most the limiter may admit (burst allowance plus refill during the run)
}

// judgeBurst compares the burst size, not only that a 429 appears: both
// providers must admit at least the configured burst (any status that is not
// 429 counts as admitted, whatever the upstream answers), no more than the
// allowed maximum, and must then limit. It returns "" on success, else why not.
func judgeBurst(burst int, nginx, traefik burstRun) string {
	detail := func() string {
		return fmt.Sprintf("nginx first=%d admitted=%d (max %d) 429=%d; traefik first=%d admitted=%d (max %d) 429=%d; burst=%d",
			nginx.first, nginx.admitted, nginx.max, nginx.limited, traefik.first, traefik.admitted, traefik.max, traefik.limited, burst)
	}
	for _, p := range []struct {
		name string
		r    burstRun
	}{{"nginx", nginx}, {"traefik", traefik}} {
		switch {
		case p.r.limited == 0:
			return p.name + " never answered 429: " + detail()
		case burst > 0 && p.r.admitted == 0:
			return p.name + " admitted nothing before limiting: " + detail()
		case p.r.admitted < burst:
			return p.name + " admitted fewer than the burst: " + detail()
		case p.r.admitted > p.r.max:
			return p.name + " admitted more than the burst allows: " + detail()
		}
	}
	return ""
}

// refillWait is how long a bucket needs to hold burst+1 tokens again (capped at one minute).
func refillWait(burst int, perSec float64) time.Duration {
	if perSec <= 0 {
		return 0
	}
	d := time.Duration(float64(burst+1)/perSec*float64(time.Second)) + time.Second
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

func rates(r *runner, m routes, get func(route, string) req) {
	done := map[string]bool{}
	for _, rt := range m.Routes {
		type lim struct {
			path  string
			burst int
		}
		var ls []lim
		if rt.RateLimit != nil && hasRoot(rt) {
			ls = append(ls, lim{"/", rt.RateLimit.Burst})
		}
		for _, l := range rt.Locations {
			if l.RateLimit != nil && l.Upstream != nil {
				ls = append(ls, lim{sample(l.Path, l.Match), l.RateLimit.Burst})
			}
		}
		for _, l := range ls {
			key := fmt.Sprint(l.path, l.burst)
			if done[key] {
				continue
			}
			done[key] = true
			perSec := zoneRate(m, zoneOf(rt, l.path))
			var spent [2]time.Duration
			count := func(t target) (first int, ok, limited int) {
				start := time.Now()
				defer func() {
					if t.name == r.n.name {
						spent[0] = time.Since(start)
					} else {
						spent[1] = time.Since(start)
					}
				}()
				for i := 0; i < l.burst+8; i++ {
					res := t.do(get(rt, l.path))
					if i == 0 {
						first = res.status
					}
					if res.status == 429 {
						limited++
					} else {
						ok++ // admitted: any answer that is not the limiter's 429
					}
				}
				return
			}
			// earlier matrix requests drained this bucket: let both fill again so the burst is comparable
			time.Sleep(refillWait(l.burst, perSec))
			fa, oa, la := count(r.n)
			fb, ob, lb := count(r.t)
			label := fmt.Sprintf("%s rate %s burst %d", rt.ID, l.path, l.burst)
			a := burstRun{first: fa, admitted: oa, limited: la, max: allowed(l.burst, perSec, spent[0])}
			b := burstRun{first: fb, admitted: ob, limited: lb, max: allowed(l.burst, perSec, spent[1])}
			if msg := judgeBurst(l.burst, a, b); msg != "" {
				r.fail(label, msg)
			} else {
				r.ok(fmt.Sprintf("%s (admitted nginx=%d traefik=%d, then 429)", label, oa, ob))
			}
			time.Sleep(2 * time.Second)
		}
	}
	if len(done) == 0 {
		r.fail("rate limits", "no limited location in the fixture")
	}
}

// zoneOf returns the zone name limiting path on rt (location zone, else the server zone for "/").
func zoneOf(rt route, path string) string {
	for _, l := range rt.Locations {
		if l.RateLimit != nil && sample(l.Path, l.Match) == path {
			return l.RateLimit.Zone
		}
	}
	if rt.RateLimit != nil {
		return rt.RateLimit.Zone
	}
	return ""
}

// zoneRate is the zone's requests per second ("5r/s", "100r/m").
func zoneRate(m routes, zone string) float64 {
	for _, z := range m.Zones {
		if z.Name == zone && z.Rate != nil {
			var n float64
			var unit string
			if _, err := fmt.Sscanf(*z.Rate, "%fr/%1s", &n, &unit); err == nil {
				if unit == "m" {
					return n / 60
				}
				return n
			}
		}
	}
	return 0
}

// allowed is the most requests a limiter may admit in a burst of elapsed time: the burst allowance
// (1 + burst) plus what the bucket refills meanwhile, plus 2 for scheduling jitter.
func allowed(burst int, perSec float64, elapsed time.Duration) int {
	return burst + 1 + int(perSec*elapsed.Seconds()+0.999) + 2
}
