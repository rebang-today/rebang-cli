package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aead.dev/minisign"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/update"
)

func setVersion(t *testing.T, v string) {
	old := buildinfo.Version
	buildinfo.Version = v
	t.Cleanup(func() { buildinfo.Version = old })
}

func TestMinVersionBlocksCommands(t *testing.T) {
	h := newHarness(t)
	setVersion(t, "0.1.0")
	update.SaveState(update.State{LatestVersion: "0.3.0", MinVersion: "0.2.0"})
	_, stderr, code := h.run(false, "menus")
	if code != errs.ExitUpgrade || !strings.Contains(stderr, "upgrade_required") {
		t.Fatalf("%d %s", code, stderr)
	}
	for _, args := range [][]string{{"version"}, {"help"}} {
		if _, _, code := h.run(false, args...); code != 0 {
			t.Errorf("%v blocked: %d", args, code)
		}
	}
}

func TestUpdateEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as fake binary")
	}
	h := newHarness(t)
	setVersion(t, "0.1.0")

	newBin := []byte("#!/bin/sh\necho '{\"version\":\"0.2.0\",\"updated\":[\"/x/rebang\"]}'\n")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "rebang", Mode: 0o755, Size: int64(len(newBin)), Typeflag: tar.TypeReg})
	tw.Write(newBin)
	tw.Close()
	gz.Close()
	archive := buf.Bytes()

	pub, priv, _ := minisign.GenerateKey(rand.Reader)
	pubText, _ := pub.MarshalText()
	old := update.PublicKey
	update.PublicKey = func() string { return string(pubText) }
	t.Cleanup(func() { update.PublicKey = old })
	sum := sha256.Sum256(archive)

	var cdn *httptest.Server
	cdn = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			json.NewEncoder(w).Encode(update.Manifest{Version: "0.2.0", MinVersion: "0.1.0", Platforms: map[string]update.Asset{
				buildinfo.Platform(): {URL: cdn.URL + "/a.tar.gz", SHA256: hex.EncodeToString(sum[:]), SignatureURL: cdn.URL + "/a.tar.gz.minisig"},
			}})
		case "/a.tar.gz":
			w.Write(archive)
		case "/a.tar.gz.minisig":
			w.Write(minisign.Sign(priv, archive))
		}
	}))
	defer cdn.Close()
	t.Setenv("REBANG_MANIFEST_URL", cdn.URL+"/manifest.json")

	exe := filepath.Join(t.TempDir(), "rebang")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	oldExe := Executable
	Executable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { Executable = oldExe })

	out, _, code := h.run(false, "update", "--check")
	if code != 0 || !strings.Contains(out, `"latest_version": "0.2.0"`) || !strings.Contains(out, `"updated": false`) {
		t.Fatal(out)
	}
	out, stderr, code := h.run(false, "update")
	if code != 0 {
		t.Fatalf("%d %s", code, stderr)
	}
	var res UpdateResult
	decode(t, out, &res)
	if !res.Updated || len(res.SkillsUpdated) != 1 {
		t.Fatal(out)
	}
	b, _ := os.ReadFile(exe)
	if !bytes.Equal(b, newBin) {
		t.Fatal("binary not replaced")
	}
}
