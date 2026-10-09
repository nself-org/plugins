package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
)

// Realm partitions identical content by the authority that owns it.
type Realm string

var realmPattern = regexp.MustCompile(`^(cache|artifacts)-[0-9a-f]{64}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var writerPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

var ErrInvalid = errors.New("cas: invalid path component")

func CacheRealm(namespace string) Realm    { return realm("cache", namespace) }
func ArtifactRealm(projectID string) Realm { return realm("artifacts", projectID) }

func realm(kind, value string) Realm {
	sum := sha256.Sum256([]byte(value))
	return Realm(kind + "-" + hex.EncodeToString(sum[:]))
}

func (r Realm) Validate() error {
	if !realmPattern.MatchString(string(r)) {
		return ErrInvalid
	}
	return nil
}

func validateDigest(digest string) error {
	if !digestPattern.MatchString(digest) {
		return ErrInvalid
	}
	return nil
}
