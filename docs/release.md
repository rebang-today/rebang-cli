# 发版说明

发版流程见决策 D-50 ~ D-56，流水线定义在 `.github/workflows/release.yml` 与 `.goreleaser.yaml`。

## 一次性准备

### 1. 生成 minisign 密钥

在可信的机器上执行（任选其一：`brew install minisign`，或 `go install aead.dev/minisign/cmd/minisign@v0.3.0`）：

```bash
minisign -G -p minisign.pub -s minisign.key
```

- 把 `minisign.pub` 的内容提交到 `internal/update/minisign.pub`。公钥为空时程序拒绝自升级，发版流水线也会直接失败。
- `minisign.key` 与密码只放进 GitHub Secrets 和离线备份，不要提交。私钥丢失后，已发布的程序无法再验证新版本，只能通过安装脚本重新安装。

### 2. GitHub 配置

在仓库 Settings → Environments 中创建名为 `release` 的 Environment，并配置：

| 类型 | 名称 | 说明 |
| --- | --- | --- |
| Secret | `MINISIGN_SECRET_KEY` | `minisign.key` 文件的完整内容 |
| Secret | `MINISIGN_PASSWORD` | 私钥密码 |
| Secret | `HOMEBREW_TAP_TOKEN` | 只对 `rebang-today/homebrew-tap` 有 Contents 写权限的 fine-grained token |
| Secret | `OSS_ACCESS_KEY_ID` | OSS 子账号 AccessKey，权限仅限 Bucket 的 `cli/` 前缀写入 |
| Secret | `OSS_ACCESS_KEY_SECRET` | 同上 |
| Variable | `OSS_BUCKET` | Bucket 名称 |
| Variable | `OSS_ENDPOINT` | OSS 的 S3 兼容访问地址，如 `https://oss-cn-<地域>.aliyuncs.com` |
| Variable | `OSS_REGION` | 地域，如 `cn-<地域>` |
| Variable | `CDN_BASE_URL` | `cli/` 目录的公开 https 地址，必须与程序内置的 manifest 地址、安装脚本中的默认地址一致 |

另外需要创建空仓库 `rebang-today/homebrew-tap`。

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
5. 上传安装包、签名、`checksums.txt` 到 `cli/v<版本>/`，上传 `cli/install.sh`、`cli/install.ps1`；
6. **最后**生成并上传 `cli/manifest.json`。此前确认 CDN 上的安装包已可下载。

带 `-` 的预发布版本（如 `v0.2.0-rc.1`）只发布到 GitHub Releases，不上传 CDN、不更新 manifest，已安装的用户不会收到升级。

CDN 上的目录结构：

```
cli/
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

`min_version` 默认沿用线上 manifest 中的值。需要强制所有用户升级时，手动修改线上 `cli/manifest.json` 的 `min_version`（D-51）。低于该版本的程序在交互界面检查到后提示升级，非交互命令依据最近一次检查结果拒绝执行（退出码 7）。

## 本地试跑

```bash
make snapshot                                      # 构建全部平台，不签名、不发布
go run ./tools/manifestgen -version <版本> -base-url https://example.invalid/cli
```
