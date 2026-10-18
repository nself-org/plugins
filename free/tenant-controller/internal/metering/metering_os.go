package metering

import "os"

// osGetenv is the real os.Getenv. Tests override getenv directly.
func osGetenv(key string) string {
	return os.Getenv(key)
}
