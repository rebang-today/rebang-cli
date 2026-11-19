# 浏览热榜：menus、tabs、nodes、list、open

热榜按 **菜单 → Tab → 二级 Tab / 节点** 组织：

- 菜单（menu）：综合、科技、娱乐、财经等大类，用 ID、`menu_key` 或名称指定。
- Tab：一个平台或一个聚合榜，如 `weibo`（微博）、`zhihu`（知乎）、`github`。Tab KEY 同时用于 `list --tab` 与 `search --source`。
- 二级 Tab（sub-tab）：Tab 下的分榜，如 `github` 下的 `daily`。
- 节点（node）：一个可以直接查看的榜单，用 `node_id` 指定。

不确定 KEY 时不要猜，先用 `rebang tabs --all` 查。

## rebang menus

列出菜单。无参数。

JSON：`menus[]`，每项 `id`、`menu_key`、`menu_name`、`menu_path`。

## rebang tabs

```bash
rebang tabs --all -o json        # 全部菜单的 Tab，按 KEY 去重
rebang tabs --menu tech -o json  # 某个菜单下的 Tab
```

| 参数 | 说明 |
| --- | --- |
| `--all` | 列出全部菜单的 Tab。会对每个菜单各请求一次，结果可在本次对话中复用 |
| `--menu <ID、KEY 或名称>` | 只看某个菜单；与 `--all` 二选一 |

JSON：`tabs[]`，每项 `key`、`name`、`child[]`（二级 Tab：`key`、`name`）。`--all` 时每项还有 `menus`（所属菜单 KEY）；`--menu` 时顶层还有 `menu_id`。

用户说的平台名（如“小红书”“B 站”）先在 `name` 中匹配，再用对应的 `key`。

## rebang nodes

```bash
rebang nodes --menu tech     # 菜单下可直接查看的榜单节点
rebang nodes --tab github    # Tab 下的二级 Tab，等价于可查看的榜单
```

| 参数 | 说明 |
| --- | --- |
| `--menu <ID、KEY 或名称>` | 菜单下的节点 |
| `--tab <KEY>` | 一级 Tab 下的二级 Tab |

两个参数必须且只能指定一个。

JSON：`--menu` 时为 `nodes[]`（`node_id`、`name`、`subtitle`、`desc`、`source_website`）；`--tab` 时为 `nodes[]`（`tab`、`sub_tab`、`name`）。

## rebang list

查看一个榜单的实时内容。

```bash
rebang list --tab weibo -n 20 -o json
rebang list --tab github --sub-tab daily -n 10 -o json
rebang list --node <NODE_ID> -o json
```

| 参数 | 说明 |
| --- | --- |
| `--tab <KEY>` | 一级 Tab |
| `--sub-tab <KEY>` | 二级 Tab，配合 `--tab`；不指定时使用默认分榜 |
| `--node <NODE_ID>` | 节点，与 `--tab` 二选一 |
| `-n, --limit <N>` | 最多返回条数，1~500，默认 30 |

JSON：

- `items[]`：`rank`、`title`、`url`、`mobile_url`、`desc`（简介）、`heat`（热度文本，可能为空）、`label`（如“热”“新”）、`published_at`、`item_key`（用于 `summary`）、`extra_info`（平台原始字段，不稳定，尽量不用）。
- `is_ranking`：是否是有名次的排行榜；为 false 时 `rank` 只是顺序。
- `node_id`、`tab`、`sub_tab`、`snapshot_id`、`next_refresh_at`（下次刷新时间）。
- `content_access`：`mode` 为 `preview` 时非会员只能看到前 `preview_limit` 条，需要如实说明结果不完整；榜单完全仅对会员开放时退出码为 4。

## rebang open

用浏览器打开链接，或打开榜单中的第 N 条。你通常只需要链接，用 `--print`：

```bash
rebang open 3 --tab weibo --print -o json   # 只输出第 3 条的链接，不打开浏览器
rebang open https://example.com/a           # 在用户的浏览器中打开
```

| 参数 | 说明 |
| --- | --- |
| `--node`、`--tab`、`--sub-tab` | 参数是序号时指定榜单，与 `list` 相同 |
| `--print` | 只输出链接，不打开浏览器 |
| `--mobile` | 优先使用移动版链接 |

JSON：`url`、`title`、`opened`。只在用户明确要求打开时才去掉 `--print`。
