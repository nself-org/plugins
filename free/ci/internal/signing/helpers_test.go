package signing_test

import (
	"encoding/base64"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"
)

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func base64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

// fakeSig has the shape of a real signature: a ci-release key id and 64 bytes.
var fakeSig = sdk.EnvelopeSig{KeyID: "ci-release-0123456789abcdef", Sig: base64.StdEncoding.EncodeToString(make([]byte, 64))}
