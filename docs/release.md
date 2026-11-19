# 发版说明

发版流程见决策 D-50 ~ D-56，流水线定义在 `.github/workflows/release.yml` 与 `.goreleaser.yaml`。

## 一次性准备

### 1. 生成 minisign 密钥

在可信的机器上执行（任选其一：`brew install minisign`，或 `go install aead.dev/minisign/cmd/minisign@v0.3.0`）：

```bash
minisign -G -p minisign.pub -s minisign.key
```

- 把 `minisign.pub` 的内容提交到 `internal/update/minisign.pub`（当前正式公钥 ID 为 `05B5851321C39815`，测试会校验）。公钥为空时程序拒绝自升级，发版流水线也会直接失败。
- `minisign.key` 与密码只放进 GitHub Secrets 和离线备份，不要提交。私钥丢失后，已发布的程序无法再验证新版本，只能通过安装脚本重新安装。

### 2. 下载站 cli.rebang.today

下载站是独立域名 `cli.rebang.today`，直接绑定 OSS Bucket `rebang-cli`，文件放在 **Bucket 根目录**，服务器不参与：

```
cli.rebang.today  ──CNAME──▶  OSS Bucket rebang-cli（公共读，根目录）
```

以后可以在前面接 CDN，对外地址不变。不要用 `rebang.today/cli` 或 `cdn.rebang.today`（后者经 rebang-static 回源并永久缓存，manifest 不会刷新）。

一次性准备：

1. 在 OSS 控制台创建 Bucket `rebang-cli`，读写权限设为“公共读”（不开放列举）。
2. 在 Bucket 的“域名管理”中绑定自定义域名 `cli.rebang.today`，上传证书或使用托管证书，开启 HTTPS。
3. 在 DNS 中添加 CNAME：`cli` → OSS 给出的 Bucket 域名。
4. 创建 RAM 子账号，授权策略只允许写入该 Bucket（`oss:PutObject`，资源 `acs:oss:*:*:rebang-cli/*`；流水线读取线上 manifest 走公开地址，不需要读权限），生成 AccessKey 填入下面的 Secrets。
5. 在 Bucket 的“静态页面”设置中把默认首页设为 `index.html`，访问 `https://cli.rebang.today/` 即显示安装教程页。
6. 验证：`curl -I https://cli.rebang.today/manifest.json` 在首次发版后返回 200 与 `Cache-Control: max-age=60`。

### 3. GitHub 配置

在仓库 Settings → Environments 中创建名为 `release` 的 Environment，部署策略只允许 `v*` tag（流水线只在推送 `v*` tag 时运行），并配置：

| 类型 | 名称 | 说明 |
| --- | --- | --- |
| Secret | `MINISIGN_SECRET_KEY` | `minisign.key` 文件的完整内容 |
| Secret | `MINISIGN_PASSWORD` | 私钥密码 |
| Secret | `HOMEBREW_TAP_DEPLOY_KEY` | ed25519 私钥内容；对应公钥作为可写 deploy key 加到 `rebang-today/homebrew-tap`，goreleaser 经 SSH 推送 Formula |
| Secret | `OSS_ACCESS_KEY_ID` | OSS 子账号 AccessKey，权限仅限写入 Bucket `rebang-cli` |
| Secret | `OSS_ACCESS_KEY_SECRET` | 同上 |
| Variable | `OSS_BUCKET` | `rebang-cli` |
| Variable | `OSS_ENDPOINT` | OSS 的 S3 兼容访问地址，如 `https://oss-cn-<地域>.aliyuncs.com` |
| Variable | `OSS_REGION` | 地域，如 `cn-<地域>` |
| Variable | `CDN_BASE_URL` | `https://cli.rebang.today`，必须与程序内置的 manifest 地址、安装脚本中的默认地址一致 |

另外需要创建空仓库 `rebang-today/homebrew-tap`，生成 ed25519 密钥对（`ssh-keygen -t ed25519 -N "" -f tap_deploy_key`），把公钥作为允许写入的 deploy key 加到该仓库，私钥内容填入 `HOMEBREW_TAP_DEPLOY_KEY`。

## 发布

```bash
git tag v0.1.0
git push origin v0.1.0
```

流水线依次执行：

1. goreleaser 运行测试，构建 darwin、linux、windows 的 amd64 与 arm64 版本；
2. 生成 `checksums.txt`（sha256）与每个安装包的 `.minisig` 签名；
3. 发布 GitHub Releases，更新 Homebrew tap；
4. 用内置公钥重新校验全部签名；
5. 上传安装包、签名、`checksums.txt` 到 Bucket 的 `v<版本>/`（`Cache-Control: public, max-age=31536000, immutable`），上传根目录的 `install.sh`、`install.ps1`（`max-age=60`）、首页 `index.html`（`site/index.html`）与CLI 安装指南 `install.md`（`site/install.md`），后两者 `max-age=300`；
6. **最后**生成并上传根目录的 `manifest.json`（`max-age=60`）。此前确认下载站上的安装包已可下载。

带 `-` 的预发布版本（如 `v0.2.0-rc.1`）只发布到 GitHub Releases，不上传下载站、不更新 manifest，已安装的用户不会收到升级。

下载站（Bucket `rebang-cli` 根目录）的结构：

```
https://cli.rebang.today/
├── index.html        # 首页，源文件 site/index.html
├── install.md        # CLI 安装指南，源文件 site/install.md
├── manifest.json
├── install.sh
├── install.ps1
└── v0.1.0/
    ├── checksums.txt
    ├── rebang_0.1.0_darwin_arm64.tar.gz
    ├── rebang_0.1.0_darwin_arm64.tar.gz.minisig
    └── …
```

## 强制升级

`min_version` 默认沿用线上 manifest 中的值。需要强制所有用户升级时，手动修改线上 `https://cli.rebang.today/manifest.json` 的 `min_version`（上传时保留 `Cache-Control: max-age=60`）（D-51）。低于该版本的程序在交互界面检查到后提示升级，非交互命令依据最近一次检查结果拒绝执行（退出码 7）。

## 本地试跑

```bash
make snapshot                                      # 构建全部平台，不签名、不发布
go run ./tools/manifestgen -version <版本> -base-url https://example.invalid
```
