# 今日热榜 CLI 安装脚本（Windows）
#
#   irm https://rebang.today/cli/install.ps1 | iex
#
# 环境变量：
#   REBANG_INSTALL_DIR  安装目录，默认 %LOCALAPPDATA%\Programs\rebang
#   REBANG_CDN          下载地址，默认 https://rebang.today/cli
#   REBANG_VERSION      指定版本（如 0.1.0），默认最新版
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

function Install-Rebang {
    $cdn = if ($env:REBANG_CDN) { $env:REBANG_CDN.TrimEnd('/') } else { 'https://rebang.today/cli' }
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
        Write-Host "正在下载 rebang $ver（$platform）…"
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
        Write-Host ''
        Write-Host '开始使用：'
        Write-Host '  rebang               进入交互界面'
        Write-Host '  rebang auth login    登录'
        Write-Host '  rebang skill install 安装 Agent Skill'
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

Install-Rebang
