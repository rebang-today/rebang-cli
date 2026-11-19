# 今日热榜 CLI 安装脚本（Windows）
#
#   irm https://cli.rebang.today/install.ps1 | iex
#   & ([scriptblock]::Create((irm https://cli.rebang.today/install.ps1))) --with-skill
#
# 参数：
#   --with-skill        同时安装 Agent Skill，不询问
#   --no-skill          不安装 Agent Skill，不询问
#
# 环境变量：
#   REBANG_INSTALL_DIR  安装目录，默认 %LOCALAPPDATA%\Programs\rebang
#   REBANG_CDN          下载地址，默认 https://cli.rebang.today
#   REBANG_VERSION      指定版本（如 0.1.0），默认最新版
#   REBANG_INSTALL_SKILL  1 安装 Agent Skill、0 不安装；不设置时检测到 Agent 才询问
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

# 列出本机已安装的 Agent（与 rebang skill install 的默认目标一致）。
function Get-RebangAgents {
    $names = @()
    if (Test-Path (Join-Path $HOME '.claude')) { $names += 'Claude Code' }
    if (Test-Path (Join-Path $HOME '.codex')) { $names += 'Codex' }
    if (Test-Path (Join-Path $HOME '.agents')) { $names += '~/.agents' }
    if (Test-Path (Join-Path $HOME '.cursor')) { $names += 'Cursor' }
    if (Test-Path (Join-Path $HOME '.gemini')) { $names += 'Gemini CLI' }
    return $names
}

# 决定是否安装 Agent Skill：参数与环境变量优先；否则检测到 Agent 时询问，默认是；
# 非交互（CI、输入被重定向）时不安装。返回 yes、no 或 skip（未检测到 Agent，不询问）。
function Get-SkillChoice([string]$choice) {
    if ($choice -eq '1') { return 'yes' }
    if ($choice -eq '0') { return 'no' }
    $agents = @(Get-RebangAgents)
    if ($agents.Count -eq 0) { return 'skip' }
    if (-not [Environment]::UserInteractive -or [Console]::IsInputRedirected) { return 'no' }
    $answer = Read-Host "检测到 $($agents -join '、')，是否安装 rebang Agent Skill？[Y/n]"
    if ($answer -match '^\s*(y|yes)?\s*$') { return 'yes' }
    return 'no'
}

function Install-Rebang([string[]]$opts) {
    $skillChoice = $env:REBANG_INSTALL_SKILL
    foreach ($o in $opts) {
        switch ($o) {
            '--with-skill' { $skillChoice = '1' }
            '--no-skill' { $skillChoice = '0' }
            default { throw "未知参数：${o}（可用 --with-skill、--no-skill）" }
        }
    }
    $cdn = if ($env:REBANG_CDN) { $env:REBANG_CDN.TrimEnd('/') } else { 'https://cli.rebang.today' }
    $dir = if ($env:REBANG_INSTALL_DIR) { $env:REBANG_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\rebang' }

    $arch = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "不支持的架构：$($env:PROCESSOR_ARCHITECTURE)" }
    }
    $platform = "windows/$arch"
    $headers = @{ 'User-Agent' = 'rebang-install' }

    if ($env:REBANG_VERSION) {
        $ver = $env:REBANG_VERSION.TrimStart('v')
        $name = "rebang_${ver}_windows_${arch}.zip"
        $url = "$cdn/v$ver/$name"
        $sums = (Invoke-WebRequest -UseBasicParsing -Headers $headers "$cdn/v$ver/checksums.txt").Content
        $want = ($sums -split "`n" | Where-Object { $_ -match "\s$([regex]::Escape($name))\s*$" } | ForEach-Object { ($_ -split '\s+')[0] }) | Select-Object -First 1
    } else {
        $manifest = Invoke-RestMethod -UseBasicParsing -Headers $headers "$cdn/manifest.json"
        $ver = $manifest.version
        $asset = $manifest.platforms.$platform
        if (-not $asset) { throw "没有 $platform 平台的安装包" }
        $url = $asset.url
        $want = $asset.sha256
    }
    if (-not $want) { throw "缺少安装包的 sha256" }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("rebang-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "正在下载 rebang ${ver}（${platform}）…"
        $zip = Join-Path $tmp 'rebang.zip'
        Invoke-WebRequest -UseBasicParsing -Headers $headers $url -OutFile $zip
        $got = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLower()
        if ($got -ne $want.ToLower()) { throw 'sha256 校验失败，已中止安装' }
        Expand-Archive -Path $zip -DestinationPath $tmp -Force

        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $target = Join-Path $dir 'rebang.exe'
        # 正在运行的程序不能覆盖，先改名。
        if (Test-Path $target) { Move-Item -Force $target "$target.old" }
        Copy-Item -Force (Join-Path $tmp 'rebang.exe') $target
        Remove-Item -Force "$target.old" -ErrorAction SilentlyContinue
        Write-Host "已安装到 $target"

        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not (($userPath -split ';') -contains $dir)) {
            [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $dir).TrimStart(';'), 'User')
            $env:Path = "$env:Path;$dir"
            Write-Host "已把 $dir 加入用户 PATH，重新打开终端后生效。"
        }

        $skill = Get-SkillChoice $skillChoice
        if ($skill -eq 'yes') {
            Write-Host ''
            # 失败不影响程序安装。
            & $target skill install -o table
            if ($LASTEXITCODE -ne 0) { Write-Host 'Agent Skill 安装失败，稍后可运行 rebang skill install 重试。' }
        }
        Write-Host ''
        Write-Host '开始使用：'
        Write-Host '  rebang               进入交互界面'
        if ($skill -ne 'yes') {
            Write-Host '  rebang skill install 安装 Agent Skill，让 Claude Code、Codex、Cursor 等使用今日热榜'
        }
        Write-Host ''
        Write-Host '可选：rebang auth login 登录后可用我的关注、内容监控等功能'
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

Install-Rebang $args
