package update

import (
	"strconv"
	"strings"
)

// Version 是简化的语义化版本。
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

// ParseVersion 解析 v1.2.3 或 1.2.3-rc.1。
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v Version
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.Pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	nums := []*int{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, false
		}
		*nums[i] = n
	}
	return v, true
}

// Compare 比较两个版本，返回 -1、0、1；无法解析的版本视为最小。
func Compare(a, b string) int {
	va, oka := ParseVersion(a)
	vb, okb := ParseVersion(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for _, d := range [][2]int{{va.Major, vb.Major}, {va.Minor, vb.Minor}, {va.Patch, vb.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case va.Pre == vb.Pre:
		return 0
	case va.Pre == "":
		return 1
	case vb.Pre == "":
		return -1
	case va.Pre < vb.Pre:
		return -1
	default:
		return 1
	}
}

// IsDev 表示本地开发构建，不参与版本检查。
func IsDev(v string) bool {
	return strings.Contains(v, "dev") || v == "" || strings.HasPrefix(v, "0.0.0")
}
