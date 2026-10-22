package main

import (
	"sync"
	"testing"
)

// A flag set under construction must never become a listener's source of values.
func TestUnparsedServeFlagsAreIsolated(t *testing.T) {
	for _, dir := range []string{"first-home", "second-home"} {
		t.Run(dir, func(t *testing.T) {
			fs, _, err := newServeV15Flags()
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < 1000; i++ {
					if err := fs.Set("data-dir", dir); err != nil {
						t.Error(err)
					}
				}
			}()
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < 1000; i++ {
					_ = serveHome()
				}
			}()
			close(start)
			wg.Wait()
		})
	}
}
