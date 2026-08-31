package internal

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	for _, k := range []string{"PORT", "DATABASE_URL", "POST_INTERNAL_SECRET", "POST_ENCRYPTION_KEY"} {
		os.Unsetenv(k)
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if c.Port != 3129 {
		t.Errorf("Port = %d; want 3129", c.Port)
	}
	if c.DatabaseURL != "" || c.InternalSecret != "" || c.EncryptionKey != "" {
		t.Errorf("expected blank secrets/db; got %+v", c)
	}
}

func TestLoadConfig_PortOverride(t *testing.T) {
	os.Setenv("PORT", "9000")
	defer os.Unsetenv("PORT")
	c, _ := LoadConfig()
	if c.Port != 9000 {
		t.Errorf("Port = %d; want 9000", c.Port)
	}
}

func TestLoadConfig_PortInvalidFalls(t *testing.T) {
	os.Setenv("PORT", "abc")
	defer os.Unsetenv("PORT")
	c, _ := LoadConfig()
	if c.Port != 3129 {
		t.Errorf("invalid PORT should fall back; got %d", c.Port)
	}
}

func TestLoadConfig_AllOverrides(t *testing.T) {
	os.Setenv("PORT", "1111")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("POST_INTERNAL_SECRET", "secret")
	os.Setenv("POST_ENCRYPTION_KEY", "key")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POST_INTERNAL_SECRET")
		os.Unsetenv("POST_ENCRYPTION_KEY")
	}()
	c, _ := LoadConfig()
	if c.Port != 1111 || c.DatabaseURL != "postgres://x" ||
		c.InternalSecret != "secret" || c.EncryptionKey != "key" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestParseEncryptionKey_Hex(t *testing.T) {
	raw := strings.Repeat("ab", 32) // 64 hex chars = 32 bytes
	k, err := ParseEncryptionKey(raw)
	if err != nil {
		t.Fatalf("hex 64 chars: %v", err)
	}
	want, _ := hex.DecodeString(raw)
	for i := 0; i < 32; i++ {
		if k[i] != want[i] {
			t.Errorf("byte[%d] = %x; want %x", i, k[i], want[i])
			break
		}
	}
}

func TestParseEncryptionKey_Base64(t *testing.T) {
	bytes := make([]byte, 32)
	for i := range bytes {
		bytes[i] = byte(i)
	}
	raw := base64.StdEncoding.EncodeToString(bytes) // 44 chars with padding
	k, err := ParseEncryptionKey(raw)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	for i := 0; i < 32; i++ {
		if k[i] != bytes[i] {
			t.Errorf("byte[%d] = %x; want %x", i, k[i], bytes[i])
			break
		}
	}
}

func TestParseEncryptionKey_HexWithBadChars(t *testing.T) {
	raw := strings.Repeat("zz", 32) // 64 chars but not hex; falls through to base64 (also fails)
	_, err := ParseEncryptionKey(raw)
	if err == nil {
		t.Error("expected error for non-hex 64-char string")
	}
}

func TestParseEncryptionKey_TooShort(t *testing.T) {
	_, err := ParseEncryptionKey("short")
	if err == nil {
		t.Error("expected error for short key")
	}
	if !strings.Contains(err.Error(), "POST_ENCRYPTION_KEY") {
		t.Errorf("error should mention env var name; got %v", err)
	}
}

func TestParseEncryptionKey_WrongBase64Length(t *testing.T) {
	// valid base64 but decodes to wrong length
	raw := base64.StdEncoding.EncodeToString([]byte("only sixteen byt"))
	_, err := ParseEncryptionKey(raw)
	if err == nil {
		t.Error("expected error for base64 decoding to <32 bytes")
	}
}
