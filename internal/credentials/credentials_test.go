package credentials

import (
	"os"
	"runtime"
	"testing"
)

func TestTokenPriorityAndPersistence(t *testing.T) {
	t.Setenv("REBANG_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvToken, "")
	if tok, src := Token(); tok != "" || src != SourceNone {
		t.Fatalf("expected none, got %q %s", tok, src)
	}
	if err := SetToken("rb_cli_filetoken"); err != nil {
		t.Fatal(err)
	}
	if tok, src := Token(); tok != "rb_cli_filetoken" || src != SourceFile {
		t.Fatalf("got %q %s", tok, src)
	}
	t.Setenv(EnvToken, "rb_cli_env")
	if tok, src := Token(); tok != "rb_cli_env" || src != SourceEnv {
		t.Fatalf("env must win, got %q %s", tok, src)
	}
	p, _ := Path()
	if runtime.GOOS != "windows" {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("perm %v", st.Mode().Perm())
		}
	}
}

func TestClearKeepsDeviceID(t *testing.T) {
	t.Setenv("REBANG_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvToken, "")
	id1, err := DeviceID()
	if err != nil || len(id1) != 32 {
		t.Fatalf("device id %q %v", id1, err)
	}
	if err := SetToken("rb_cli_x"); err != nil {
		t.Fatal(err)
	}
	if err := ClearToken(); err != nil {
		t.Fatal(err)
	}
	f, _ := Load()
	if f.Token != "" || f.DeviceID != id1 || f.Version != FormatVersion {
		t.Fatalf("got %+v", f)
	}
}

func TestRejectsNewerVersion(t *testing.T) {
	t.Setenv("REBANG_CONFIG_DIR", t.TempDir())
	p, _ := Path()
	os.MkdirAll(os.Getenv("REBANG_CONFIG_DIR"), 0o700)
	os.WriteFile(p, []byte(`{"version":99,"token":"x"}`), 0o600)
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestMask(t *testing.T) {
	if got := Mask("rb_cli_abcdefghijklmnop"); got != "rb_cli_…mnop" {
		t.Fatal(got)
	}
	if Mask("short") != "****" {
		t.Fatal("short")
	}
}
