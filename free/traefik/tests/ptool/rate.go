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
