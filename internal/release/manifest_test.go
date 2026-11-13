package release

import (
	"strings"
	"testing"
	"time"
)

const sums = `0000000000000000000000000000000000000000000000000000000000000001  rebang_0.1.0_darwin_arm64.tar.gz
0000000000000000000000000000000000000000000000000000000000000002  rebang_0.1.0_linux_amd64.tar.gz
0000000000000000000000000000000000000000000000000000000000000003  rebang_0.1.0_windows_amd64.zip
0000000000000000000000000000000000000000000000000000000000000004  rebang_0.1.0_checksums.txt.minisig
`

func TestBuild(t *testing.T) {
	m, err := Build(Params{Version: "0.1.0", MinVersion: "0.1.0", BaseURL: "https://cdn.example.com/cli/", Now: time.Unix(0, 0)}, strings.NewReader(sums))
	if err != nil {
		t.Fatal(err)
	}
	if got := Platforms(m); strings.Join(got, ",") != "darwin/arm64,linux/amd64,windows/amd64" {
		t.Fatal(got)
	}
	a := m.Platforms["windows/amd64"]
	if a.URL != "https://cdn.example.com/cli/v0.1.0/rebang_0.1.0_windows_amd64.zip" || a.SignatureURL != a.URL+".minisig" || !strings.HasSuffix(a.SHA256, "3") {
		t.Fatalf("%+v", a)
	}
	if m.ReleasedAt != "1970-01-01T00:00:00Z" {
		t.Fatal(m.ReleasedAt)
	}
}

func TestBuildRejects(t *testing.T) {
	cases := []Params{
		{Version: "x", BaseURL: "https://example.com"},
		{Version: "0.1.0", BaseURL: "http://example.com"},
		{Version: "0.1.0", MinVersion: "0.2.0", BaseURL: "https://example.com"},
		{Version: "0.2.0", BaseURL: "https://example.com"}, // 与安装包版本不一致
	}
	for _, p := range cases {
		if _, err := Build(p, strings.NewReader(sums)); err == nil {
			t.Errorf("%+v 应报错", p)
		}
	}
	if _, err := Build(Params{Version: "0.1.0", BaseURL: "https://example.com"}, strings.NewReader("")); err == nil {
		t.Error("空 checksums 应报错")
	}
}
