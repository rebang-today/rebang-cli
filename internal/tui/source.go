package tui

import (
	"context"
	"strconv"
	"time"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/update"
)

// Source 是交互界面的数据来源，测试时可替换为内存实现。
type Source interface {
	Menus(ctx context.Context) ([]api.Menu, error)
	MenuData(ctx context.Context, menuID int64) (*menuData, error)
	List(ctx context.Context, t listTarget, limit int) (*cli.ListResult, error)
	Search(ctx context.Context, q string, limit int) (*cli.SearchResult, error)
	Account(ctx context.Context) (*cli.AccountView, error)
	// InsightShorts 批量查询一句话总结（需登录），返回有总结的 item_key。
	InsightShorts(ctx context.Context, keys []string) (map[string]string, error)
	// Insight 查询单条完整 AI 总结（需登录），没有总结时返回 errNoSummary。
	Insight(ctx context.Context, key string) (*api.Insight, error)
	// CheckUpdate 返回可用的新版本，没有时返回 nil。
	CheckUpdate(ctx context.Context) (*update.Manifest, error)
	// Install 安装新版本。
	Install(ctx context.Context, m *update.Manifest) error
}

// apiSource 通过 api.Client 获取数据，与非交互命令共用同一套逻辑。
type apiSource struct {
	client      *api.Client
	web         string
	manifestURL string
	version     string
	now         func() time.Time
}

func (s *apiSource) Menus(ctx context.Context) ([]api.Menu, error) {
	m, err := s.client.Menus(ctx)
	return m, api.MapError(err)
}

func (s *apiSource) MenuData(ctx context.Context, menuID int64) (*menuData, error) {
	if menuID == followMenuID {
		// 订阅：用户订阅的 Tab（需登录），服务端已按用户设置排序。
		tabs, err := s.client.FollowingTabs(ctx)
		if err != nil {
			return nil, api.MapError(err)
		}
		return &menuData{tabs: tabs}, nil
	}
	tabs, err := s.client.MenuTabs(ctx, menuID)
	if err != nil {
		return nil, api.MapError(err)
	}
	d := &menuData{tabs: tabs}
	if len(tabs) == 0 {
		nodes, err := s.client.MenuNodes(ctx, menuID)
		if err != nil {
			return nil, api.MapError(err)
		}
		d.nodes = nodes
	}
	return d, nil
}

func (s *apiSource) List(ctx context.Context, t listTarget, limit int) (*cli.ListResult, error) {
	return cli.FetchList(ctx, s.client, t.node, t.tab, t.subTab, limit)
}

func (s *apiSource) Search(ctx context.Context, q string, limit int) (*cli.SearchResult, error) {
	return cli.DoSearch(ctx, s.client, q, 0, nil, "", limit, s.now())
}

func (s *apiSource) Account(ctx context.Context) (*cli.AccountView, error) {
	if s.client.Token == "" {
		return nil, nil
	}
	acct, err := s.client.Account(ctx)
	if err != nil {
		return nil, api.MapError(err)
	}
	v := cli.ToAccountView(acct, s.now(), s.web)
	return &v, nil
}

func (s *apiSource) InsightShorts(ctx context.Context, keys []string) (map[string]string, error) {
	res, err := s.client.InsightsBatch(ctx, keys)
	if err != nil {
		return nil, api.MapError(err)
	}
	out := make(map[string]string, len(res))
	for k, v := range res {
		out[k] = v.SummaryShort
	}
	return out, nil
}

func (s *apiSource) Insight(ctx context.Context, key string) (*api.Insight, error) {
	in, err := s.client.Insight(ctx, key)
	if api.IsNotFound(err) {
		return nil, errNoSummary
	}
	if err != nil {
		return nil, api.MapError(err)
	}
	return in, nil
}

func (s *apiSource) CheckUpdate(ctx context.Context) (*update.Manifest, error) {
	return update.CheckDaily(ctx, s.manifestURL, s.version, s.now())
}

func (s *apiSource) Install(ctx context.Context, m *update.Manifest) error {
	full, err := update.FetchManifest(ctx, s.manifestURL)
	if err != nil {
		return err
	}
	_, err = cli.InstallRelease(ctx, full)
	return err
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }
