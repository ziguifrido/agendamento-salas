package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# comentário\nSALAS_TEST_FILE=from-file\nexport SALAS_TEST_QUOTED=\"quoted value\"\nSALAS_TEST_EXISTING=from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"SALAS_TEST_FILE", "SALAS_TEST_QUOTED"} {
		old, exists := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if exists {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
	}
	t.Setenv("SALAS_TEST_EXISTING", "from-environment")

	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("SALAS_TEST_FILE"); got != "from-file" {
		t.Fatalf("file value not loaded: %q", got)
	}
	if got := os.Getenv("SALAS_TEST_QUOTED"); got != "quoted value" {
		t.Fatalf("quoted value not loaded: %q", got)
	}
	if got := os.Getenv("SALAS_TEST_EXISTING"); got != "from-environment" {
		t.Fatalf("environment value overwritten: %q", got)
	}
	if err := loadEnvFile(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("missing file returned an error: %v", err)
	}
}
