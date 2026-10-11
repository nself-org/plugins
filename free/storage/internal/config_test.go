package internal

import "testing"

func TestLoadConfig_limitsAndSecret(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		wantErr    bool
		wantMax    int64
		wantQuota  int64
		wantSecret string
	}{
		{"defaults", nil, false, 5 << 30, 0, ""},
		{"empty values are defaults", map[string]string{"STORAGE_MAX_OBJECT_BYTES": "", "STORAGE_ACCOUNT_QUOTA_BYTES": ""}, false, 5 << 30, 0, ""},
		{"max set", map[string]string{"STORAGE_MAX_OBJECT_BYTES": "1048576"}, false, 1048576, 0, ""},
		{"quota set", map[string]string{"STORAGE_ACCOUNT_QUOTA_BYTES": "1073741824"}, false, 5 << 30, 1 << 30, ""},
		{"quota zero is off", map[string]string{"STORAGE_ACCOUNT_QUOTA_BYTES": "0"}, false, 5 << 30, 0, ""},
		{"secret", map[string]string{"PLUGIN_INTERNAL_SECRET": "test-internal-secret"}, false, 5 << 30, 0, "test-internal-secret"},
		{"max zero", map[string]string{"STORAGE_MAX_OBJECT_BYTES": "0"}, true, 0, 0, ""},
		{"max negative", map[string]string{"STORAGE_MAX_OBJECT_BYTES": "-5"}, true, 0, 0, ""},
		{"max not a number", map[string]string{"STORAGE_MAX_OBJECT_BYTES": "5GiB"}, true, 0, 0, ""},
		{"max overflow", map[string]string{"STORAGE_MAX_OBJECT_BYTES": "99999999999999999999"}, true, 0, 0, ""},
		{"quota negative", map[string]string{"STORAGE_ACCOUNT_QUOTA_BYTES": "-1"}, true, 0, 0, ""},
		{"quota not a number", map[string]string{"STORAGE_ACCOUNT_QUOTA_BYTES": "1.5"}, true, 0, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, k := range []string{"STORAGE_MAX_OBJECT_BYTES", "STORAGE_ACCOUNT_QUOTA_BYTES", "PLUGIN_INTERNAL_SECRET"} {
				t.Setenv(k, "")
			}
			t.Setenv("NSELF_DB_URL", "postgresql://test:test@localhost:5432/test")
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			cfg, err := LoadConfig()
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil {
				return
			}
			if cfg.MaxObjectBytes != c.wantMax || cfg.AccountQuotaBytes != c.wantQuota || cfg.InternalSecret != c.wantSecret {
				t.Errorf("got max %d quota %d secret %q", cfg.MaxObjectBytes, cfg.AccountQuotaBytes, cfg.InternalSecret)
			}
		})
	}
}
