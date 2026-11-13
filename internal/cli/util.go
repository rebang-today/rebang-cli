package cli

import (
	"encoding/json"
	"fmt"
	"time"
)

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// cst 是服务端使用的时区（UTC+8），避免依赖系统时区数据库。
var cst = time.FixedZone("CST", 8*3600)

func formatUnix(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).In(cst).Format(time.RFC3339)
}

func formatUnixShort(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).In(cst).Format("2006-01-02 15:04")
}

func itoa(i int64) string { return fmt.Sprintf("%d", i) }

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
