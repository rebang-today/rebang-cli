package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aead.dev/minisign"

	"github.com/rebang-today/rebang-cli/internal/errs"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0}, {"v0.2.0", "0.1.9", 1}, {"0.1.0", "0.10.0", -1},
		{"1.0.0-rc.1", "1.0.0", -1}, {"1.0.0", "1.0.0-rc.1", 1}, {"bad", "0.0.1", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s,%s)=%d", c.a, c.b, got)
		}
	}
	if !IsDev("0.0.0-dev") || IsDev("0.1.0") {
		t.Fatal("IsDev")
	}
}

func tarGz(t *testing.T, name string, content []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	tw.Write([]byte("hi"))
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	b, err := ExtractBinary("x.tar.gz", tarGz(t, "rebang_0.2.0/rebang", []byte("BIN")), "linux")
	if err != nil || string(b) != "BIN" {
		t.Fatal(err, string(b))
	}
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("rebang.exe")
	w.Write([]byte("EXE"))
	zw.Close()
	b, err = ExtractBinary("x.zip", zb.Bytes(), "windows")
	if err != nil || string(b) != "EXE" {
		t.Fatal(err)
	}
	if _, err := ExtractBinary("x.tar.gz", tarGz(t, "other", []byte("x")), "linux"); err == nil {
		t.Fatal("expected missing binary")
	}
}

// release 生成签名过的发布物并启动模拟 CDN。
type release struct {
	srv      *httptest.Server
	manifest *Manifest
	archive  []byte
}

func newRelease(t *testing.T, platform string, content []byte, tamper bool) *release {
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubText, _ := pub.MarshalText()
	old := PublicKey
	PublicKey = func() string { return "untrusted comment: minisign public key\n" + string(pubText) }
	t.Cleanup(func() { PublicKey = old })

	r := &release{archive: tarGz(t, "rebang", content)}
	sig := minisign.Sign(priv, r.archive)
	sum := sha256.Sum256(r.archive)
	served := r.archive
	if tamper {
		served = append([]byte{}, r.archive...)
		served[len(served)-1] ^= 0xff
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/a.tar.gz", func(w http.ResponseWriter, _ *http.Request) { w.Write(served) })
	mux.HandleFunc("/a.tar.gz.minisig", func(w http.ResponseWriter, _ *http.Request) { w.Write(sig) })
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	r.manifest = &Manifest{Version: "9.9.9", MinVersion: "0.1.0", Platforms: map[string]Asset{
		platform: {URL: r.srv.URL + "/a.tar.gz", SHA256: hex.EncodeToString(sum[:]), SignatureURL: r.srv.URL + "/a.tar.gz.minisig"},
	}}
	return r
}

func TestDownloadVerifies(t *testing.T) {
	r := newRelease(t, "linux/amd64", []byte("NEWBIN"), false)
	bin, err := Download(context.Background(), r.manifest, "linux/amd64")
	if err != nil || string(bin) != "NEWBIN" {
		t.Fatal(err)
	}
	if _, err := Download(context.Background(), r.manifest, "plan9/arm"); errs.From(err).Code != errs.CodeNotFound {
		t.Fatal(err)
	}
}

func TestDownloadRejectsTampered(t *testing.T) {
	r := newRelease(t, "linux/amd64", []byte("NEWBIN"), true)
	_, err := Download(context.Background(), r.manifest, "linux/amd64")
	if errs.From(err).Code != "checksum_mismatch" {
		t.Fatalf("got %v", err)
	}
}

func TestSignatureRejectsWrongKey(t *testing.T) {
	r := newRelease(t, "linux/amd64", []byte("NEWBIN"), false)
	other, _, _ := minisign.GenerateKey(rand.Reader)
	txt, _ := other.MarshalText()
	PublicKey = func() string { return string(txt) }
	_, err := Download(context.Background(), r.manifest, "linux/amd64")
	if errs.From(err).Code != "signature_invalid" {
		t.Fatalf("got %v", err)
	}
	PublicKey = func() string { return "" }
	if err := VerifySignature([]byte("x"), []byte("y")); errs.From(err).Code != "signature_unavailable" {
		t.Fatal(err)
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "rebang")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	if err := Replace(exe, []byte("NEW")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	st, _ := os.Stat(exe)
	if string(b) != "NEW" || (runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0) {
		t.Fatal(string(b), st.Mode())
	}
	if ManagedBy("/opt/homebrew/Cellar/rebang/0.1.0/bin/rebang") != "homebrew" || ManagedBy(exe) != "" {
		t.Fatal("ManagedBy")
	}
}

func TestMinVersionAndDailyCheck(t *testing.T) {
	t.Setenv("REBANG_CONFIG_DIR", t.TempDir())
	if err := EnforceMinVersion("0.1.0"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"version":"0.3.0","min_version":"0.2.0","platforms":{}}`))
	}))
	defer srv.Close()
	now := time.Now()
	m, err := CheckDaily(context.Background(), srv.URL, "0.1.0", now)
	if err != nil || m == nil || m.Version != "0.3.0" {
		t.Fatal(m, err)
	}
	// 24 小时内不再联网
	m, _ = CheckDaily(context.Background(), srv.URL, "0.1.0", now.Add(time.Hour))
	if calls != 1 || m == nil {
		t.Fatal(calls, m)
	}
	err = EnforceMinVersion("0.1.0")
	if errs.ExitCode(err) != errs.ExitUpgrade {
		t.Fatal(err)
	}
	if EnforceMinVersion("0.2.0") != nil || EnforceMinVersion("0.0.0-dev") != nil {
		t.Fatal("should pass")
	}
	if m, _ := CheckDaily(context.Background(), srv.URL, "0.0.0-dev", now); m != nil || calls != 1 {
		t.Fatal("dev builds never check")
	}
}
