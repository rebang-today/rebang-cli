// Command manifestgen 在发版流水线中生成 manifest.json，不随程序发布。
//
//	go run ./tools/manifestgen -version 0.1.0 -base-url https://example.com \
//	    -checksums dist/checksums.txt -o dist/manifest.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rebang-today/rebang-cli/internal/release"
)

func main() {
	var p release.Params
	var checksums, out string
	flag.StringVar(&p.Version, "version", "", "发布版本，可带 v 前缀")
	flag.StringVar(&p.MinVersion, "min-version", "", "最低支持版本，留空表示不强制升级")
	flag.StringVar(&p.BaseURL, "base-url", "", "下载站地址，如 https://cli.rebang.today")
	flag.StringVar(&p.NotesURL, "notes-url", "", "更新日志地址")
	flag.StringVar(&checksums, "checksums", "dist/checksums.txt", "goreleaser 生成的 checksums.txt")
	flag.StringVar(&out, "o", "dist/manifest.json", "输出文件")
	flag.Parse()
	p.Version = strings.TrimPrefix(p.Version, "v")
	p.MinVersion = strings.TrimPrefix(p.MinVersion, "v")
	p.Now = time.Now()

	f, err := os.Open(checksums)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	m, err := release.Build(p, f)
	if err != nil {
		fail(err)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "manifest %s: %v\n", m.Version, release.Platforms(m))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "manifestgen:", err)
	os.Exit(1)
}
