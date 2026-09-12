$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$NoStart = $false
$SkipDeps = $false
$DatabaseRequest = ''
$DatabaseMode = ''
$CurrentPhase = 'argument parsing'
$PackageManager = ''
$AdminPasswordGenerated = $false
$ListenAllInterfaces = $true
$OwnerPasswordAction = ''
$OwnerTargetPassword = ''
$OwnerTargetUrl = ''
$OwnerStartupPassword = ''
$AppPasswordAction = ''
$AppTargetPassword = ''
$AppTargetUrl = ''
$DockerComposeExitCode = 0

function Fail([string]$Message, [int]$Code = 1) {
    [Console]::Error.WriteLine("ERROR: $Message")
    exit $Code
}

foreach ($Argument in $args) {
    switch ($Argument) {
        '--no-start' { $NoStart = $true }
        '--skip-deps' { $SkipDeps = $true }
        '--db=native' { $DatabaseRequest = 'native' }
        '--db=docker' { $DatabaseRequest = 'docker' }
        default {
            [Console]::Error.WriteLine('Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1 [--db=native|docker] [--no-start] [--skip-deps]')
            if ($Argument -like '--db=*') {
                Fail "Invalid database mode: $($Argument.Substring(5)) (expected native or docker)" 2
            }
            Fail "Unknown option: $Argument" 2
        }
    }
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = [IO.Path]::GetFullPath((Join-Path $ScriptDir '..'))
$EnvFile = Join-Path $Root '.env'
$ComposeFile = Join-Path $Root 'infra\docker-compose.yml'
$CertDir = Join-Path $Root 'infra\certs'
$CertFile = Join-Path $CertDir 'localhost.crt'
$KeyFile = Join-Path $CertDir 'localhost.key'
$WebDist = Join-Path $Root 'web\dist\web\browser'
$Utf8NoBom = New-Object Text.UTF8Encoding($false)

function Write-Heading([string]$Name) {
    $script:CurrentPhase = $Name
    Write-Host "`n==> $Name"
}

function Test-Command([string]$Name) {
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Refresh-Path {
    $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $env:Path = "$machinePath;$userPath"

    $candidates = @(
        (Join-Path $env:ProgramFiles 'OpenSSL-Win64\bin'),
        (Join-Path $env:ProgramFiles 'Tesseract-OCR'),
        (Join-Path $env:ProgramFiles 'Docker\Docker\resources\bin'),
        (Join-Path $env:ProgramFiles 'PostgreSQL\17\bin')
    )
    foreach ($candidate in $candidates) {
        if ((Test-Path -LiteralPath $candidate -PathType Container) -and ($env:Path -notlike "*$candidate*")) {
            $env:Path = "$candidate;$env:Path"
        }
    }
}

function Get-InstalledPostgresqlMajors {
    $postgresqlRoot = Join-Path $env:ProgramFiles 'PostgreSQL'
    if (-not (Test-Path -LiteralPath $postgresqlRoot -PathType Container)) { return @() }
    return @(Get-ChildItem -LiteralPath $postgresqlRoot -Directory -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^[0-9]+$' } |
        ForEach-Object { $_.Name })
}

function Test-NativePostgresqlReachable {
    $runningService = Get-Service -ErrorAction SilentlyContinue |
        Where-Object { ($_.Name -like 'postgresql*') -and ($_.Status -eq 'Running') } |
        Select-Object -First 1
    if ($null -eq $runningService) { return $false }
    foreach ($major in @(Get-InstalledPostgresqlMajors)) {
        $readinessTool = Join-Path $env:ProgramFiles "PostgreSQL\$major\bin\pg_isready.exe"
        if (Test-Path -LiteralPath $readinessTool -PathType Leaf) {
            & $readinessTool --host 127.0.0.1 --port 5432 --timeout=1 *> $null
            if ($LASTEXITCODE -eq 0) { return $true }
        }
    }
    return $false
}

function Select-DatabaseMode {
    if (-not [string]::IsNullOrEmpty($DatabaseRequest)) {
        $script:DatabaseMode = $DatabaseRequest
        Write-Host "Database mode: $DatabaseMode (selected by --db=$DatabaseMode)."
    } elseif (Test-NativePostgresqlReachable) {
        $script:DatabaseMode = 'native'
        Write-Host 'Database mode: native (a local PostgreSQL server is installed and reachable).'
    } else {
        $script:DatabaseMode = 'docker'
        Write-Host 'Database mode: docker (no installed native PostgreSQL server was reachable).'
    }
}

function Ensure-PostgresqlInstalled {
    Refresh-Path
    $installedMajors = @(Get-InstalledPostgresqlMajors)
    if ($installedMajors -contains '17') {
        if ((Test-Command 'postgres') -and (Test-Command 'psql') -and (Test-Command 'pg_isready')) { return }
        Fail 'PostgreSQL 17 is installed but its server and client tools are incomplete or not on PATH. Repair PostgreSQL 17, then rerun with --db=native --skip-deps.'
    }
    if ($installedMajors.Count -gt 0) {
        Fail "PostgreSQL server major version $($installedMajors -join ', ') is installed; version 17 is required. Remove it or migrate it to PostgreSQL 17 yourself, then rerun. This installer will not upgrade or downgrade PostgreSQL."
    }
    if ($SkipDeps) {
        Fail 'PostgreSQL 17 is missing while --skip-deps is set. Install it manually, then rerun with --db=native --skip-deps.'
    }
    if ($PackageManager -eq 'winget') {
        Fail "PostgreSQL 17 requires an interactive administrator install on Windows. Run 'winget install --id PostgreSQL.PostgreSQL.17 --exact --interactive', select port 5432 and record the postgres superuser password, then rerun with --db=native."
    }
    if ($PackageManager -eq 'choco') {
        Fail "PostgreSQL 17 requires an administrator install on Windows. Run 'choco install postgresql17 --yes', record the generated postgres superuser password, then rerun with --db=native."
    }
    Fail 'PostgreSQL 17 is not installed. Install the official PostgreSQL 17 Windows server, select port 5432, then rerun with --db=native --skip-deps.'
}

function Install-Package([string]$Dependency) {
    if ($SkipDeps) {
        Fail "$Dependency is missing while --skip-deps is set. Install $Dependency manually, then rerun with --skip-deps."
    }
    if ([string]::IsNullOrEmpty($PackageManager)) {
        Fail "No supported package manager was found. Install winget or Chocolatey, then rerun this command."
    }

    $wingetIds = @{
        'Go 1.27+' = 'GoLang.Go'
        'Node.js 22+ with npm' = 'OpenJS.NodeJS.LTS'
        'OpenSSL' = 'ShiningLight.OpenSSL'
        'Tesseract' = 'UB-Mannheim.TesseractOCR'
        'Poppler' = 'oschwartz10612.Poppler'
        'Docker Desktop' = 'Docker.DockerDesktop'
    }
    $chocoIds = @{
        'Go 1.27+' = 'golang'
        'Node.js 22+ with npm' = 'nodejs-lts'
        'OpenSSL' = 'openssl'
        'Tesseract' = 'tesseract'
        'Poppler' = 'poppler'
        'Docker Desktop' = 'docker-desktop'
    }

    if ($PackageManager -eq 'winget') {
        $id = $wingetIds[$Dependency]
        & winget install --id $id --exact --silent --accept-package-agreements --accept-source-agreements --disable-interactivity
        if ($LASTEXITCODE -ne 0) {
            Fail "winget could not install $Dependency. Run 'winget install --id $id --exact', resolve the reported problem, then rerun this command."
        }
    } else {
        $id = $chocoIds[$Dependency]
        & choco install $id --yes --no-progress
        if ($LASTEXITCODE -ne 0) {
            Fail "Chocolatey could not install $Dependency. Run 'choco install $id -y', resolve the reported problem, then rerun this command."
        }
    }
    Refresh-Path
}

function Test-GoVersion {
    if (-not (Test-Command 'go')) { return $false }
    $text = (& go version 2>$null | Out-String)
    $match = [regex]::Match($text, ' go([0-9]+)\.([0-9]+)')
    if (-not $match.Success) { return $false }
    $major = [int]$match.Groups[1].Value
    $minor = [int]$match.Groups[2].Value
    return ($major -gt 1) -or (($major -eq 1) -and ($minor -ge 27))
}

function Test-NodeVersion {
    if (-not (Test-Command 'node')) { return $false }
    $text = (& node --version 2>$null | Out-String).Trim()
    $match = [regex]::Match($text, '^v([0-9]+)')
    return $match.Success -and ([int]$match.Groups[1].Value -ge 22)
}

function Test-TesseractArabic {
    if (-not (Test-Command 'tesseract')) { return $false }
    $languages = (& tesseract --list-langs 2>$null | ForEach-Object { $_.Trim() })
    return $languages -contains 'ara'
}

function Test-OpenSsl {
    if (-not (Test-Command 'openssl')) { return $false }
    $helpText = (& openssl req -help 2>&1 | Out-String)
    return $helpText -match '-addext'
}

function Install-TesseractLanguages {
    if ($SkipDeps) {
        Fail "Tesseract Arabic data is missing while --skip-deps is set. Install ara and eng trained data, verify 'tesseract --list-langs' lists ara, then rerun with --skip-deps."
    }
    $tessdata = Join-Path $env:LOCALAPPDATA 'pms\tessdata'
    New-Item -ItemType Directory -Force -Path $tessdata | Out-Null
    $downloads = @{
        'ara.traineddata' = 'https://github.com/tesseract-ocr/tessdata_fast/raw/4.1.0/ara.traineddata'
        'eng.traineddata' = 'https://github.com/tesseract-ocr/tessdata_fast/raw/4.1.0/eng.traineddata'
    }
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    foreach ($name in $downloads.Keys) {
        $destination = Join-Path $tessdata $name
        if (-not (Test-Path -LiteralPath $destination -PathType Leaf)) {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri $downloads[$name] -OutFile $destination
            } catch {
                Fail "Could not download the official Tesseract $name file. Download it from $($downloads[$name]) to $destination, then rerun this command."
            }
        }
    }
    $env:TESSDATA_PREFIX = $tessdata
    [Environment]::SetEnvironmentVariable('TESSDATA_PREFIX', $tessdata, 'User')
}

function Ensure-Dependencies {
    if (-not (Test-GoVersion)) {
        Install-Package 'Go 1.27+'
    }
    if (-not (Test-GoVersion)) {
        Fail "Go 1.27 or newer is required. Install it from https://go.dev/doc/install, ensure 'go' is on PATH, then rerun with --skip-deps."
    }

    if ((-not (Test-NodeVersion)) -or (-not (Test-Command 'npm.cmd'))) {
        Install-Package 'Node.js 22+ with npm'
    }
    if ((-not (Test-NodeVersion)) -or (-not (Test-Command 'npm.cmd'))) {
        Fail "Node.js 22 or newer with npm is required. Install it from https://nodejs.org/en/download, ensure 'node' and 'npm' are on PATH, then rerun with --skip-deps."
    }

    if (-not (Test-OpenSsl)) { Install-Package 'OpenSSL' }
    if (-not (Test-OpenSsl)) {
        Fail "OpenSSL with 'req -addext' support is required. Install OpenSSL 3, ensure 'openssl' is on PATH, then rerun with --skip-deps."
    }

    if (-not (Test-Command 'tesseract')) { Install-Package 'Tesseract' }
    if (-not (Test-Command 'tesseract')) {
        Fail "Tesseract is required. Install it, ensure 'tesseract' is on PATH, then rerun with --skip-deps."
    }
    if (-not (Test-TesseractArabic)) { Install-TesseractLanguages }
    if (-not (Test-TesseractArabic)) {
        Fail "Tesseract Arabic data is missing. Verify 'tesseract --list-langs' lists ara, then rerun this command."
    }

    if ((-not (Test-Command 'pdftotext')) -or (-not (Test-Command 'pdftoppm')) -or (-not (Test-Command 'pdfinfo'))) {
        Install-Package 'Poppler'
    }
    if ((-not (Test-Command 'pdftotext')) -or (-not (Test-Command 'pdftoppm')) -or (-not (Test-Command 'pdfinfo'))) {
        Fail "Poppler is incomplete. Install pdftotext, pdftoppm, and pdfinfo, ensure all three are on PATH, then rerun with --skip-deps."
    }

    if ($DatabaseMode -eq 'native') {
        Ensure-PostgresqlInstalled
    } else {
        if (-not (Test-Command 'docker')) { Install-Package 'Docker Desktop' }
        if (-not (Test-Command 'docker')) {
            Fail "Docker Desktop was installed but Docker is not on PATH. Restart Windows to finish the installation, then rerun this command."
        }
        & docker compose version *> $null
        if ($LASTEXITCODE -ne 0) {
            Fail "Docker Compose v2 is missing. Install or repair Docker Desktop, restart Windows if requested, then rerun this command."
        }
    }
}

function Ensure-DockerRunning {
    & docker info *> $null
    if ($LASTEXITCODE -eq 0) { return }

    $desktop = Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'
    if (-not (Test-Path -LiteralPath $desktop -PathType Leaf)) {
        Fail "Docker Desktop is not fully installed. Restart Windows to finish its installation, then rerun this command."
    }
    try {
        Start-Process -FilePath $desktop | Out-Null
    } catch {
        Fail "Docker Desktop could not be started. Start it manually or restart Windows, wait until it is ready, then rerun this command."
    }
    for ($attempt = 1; $attempt -le 60; $attempt++) {
        & docker info *> $null
        if ($LASTEXITCODE -eq 0) { return }
        Start-Sleep -Seconds 1
    }
    Fail "Docker Desktop did not become ready within 60 seconds. Start it manually or restart Windows, verify 'docker info' succeeds, then rerun this command."
}

function Read-DotEnv([string]$Path) {
    $dotenvValues = @{}
    foreach ($line in [IO.File]::ReadAllLines($Path)) {
        $match = [regex]::Match($line, '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$')
        if (-not $match.Success) { continue }
        $name = $match.Groups[1].Value
        $raw = $match.Groups[2].Value
        if (($raw.Length -ge 2) -and $raw.StartsWith("'") -and $raw.EndsWith("'")) {
            $raw = $raw.Substring(1, $raw.Length - 2).Replace("'\''", "'")
        } elseif (($raw.Length -ge 2) -and $raw.StartsWith('"') -and $raw.EndsWith('"')) {
            $raw = $raw.Substring(1, $raw.Length - 2)
        }
        $dotenvValues[$name] = $raw
    }
    return $dotenvValues
}

function Quote-DotEnv([string]$RawValue) {
    return "'" + $RawValue.Replace("'", "'\''") + "'"
}

function Set-DotEnvIfEmpty([hashtable]$DotEnvValues, [string]$Name, [string]$EnvValue) {
    if ($DotEnvValues.ContainsKey($Name) -and -not [string]::IsNullOrEmpty([string]$DotEnvValues[$Name])) {
        Set-Item -Path "Env:$Name" -Value ([string]$DotEnvValues[$Name])
        return
    }
    $content = [IO.File]::ReadAllText($EnvFile)
    $prefix = ''
    if (($content.Length -gt 0) -and -not $content.EndsWith("`n")) { $prefix = "`r`n" }
    [IO.File]::AppendAllText($EnvFile, $prefix + $Name + '=' + (Quote-DotEnv $EnvValue) + "`r`n", $Utf8NoBom)
    $DotEnvValues[$Name] = $EnvValue
    Set-Item -Path "Env:$Name" -Value $EnvValue
}

function Rewrite-DotEnvValue([string]$Name, [string]$EnvValue) {
    $lines = [IO.File]::ReadAllLines($EnvFile)
    $assignmentPattern = '^\s*(?:export\s+)?' + [regex]::Escape($Name) + '\s*='
    $replacement = $Name + '=' + (Quote-DotEnv $EnvValue)
    $found = $false
    for ($index = 0; $index -lt $lines.Length; $index++) {
        if ($lines[$index] -match $assignmentPattern) {
            $lines[$index] = $replacement
            $found = $true
        }
    }
    if (-not $found) { $lines += $replacement }
    [IO.File]::WriteAllLines($EnvFile, $lines, $Utf8NoBom)
    Set-Item -Path "Env:$Name" -Value $EnvValue
    Protect-FileForCurrentUser $EnvFile
}

function Get-DsnEncodedPassword([string]$Dsn) {
    $match = [regex]::Match($Dsn, '^[^:]+://[^:]+:([^@]*)@')
    if (-not $match.Success) { return '' }
    return $match.Groups[1].Value
}

function New-DatabasePassword([string]$RoleName) {
    $password = (& openssl rand -hex 24 2>$null | Out-String).Trim()
    if (($LASTEXITCODE -ne 0) -or ($password -notmatch '^[0-9a-f]{48}$')) {
        Fail "OpenSSL could not generate the $RoleName database password. Repair OpenSSL, then rerun this command."
    }
    return $password
}

function Prepare-DatabaseDsn([hashtable]$DotEnvValues, [string]$Name, [string]$RoleName, [string]$DefaultPassword) {
    $currentDsn = ''
    if ($DotEnvValues.ContainsKey($Name)) { $currentDsn = [string]$DotEnvValues[$Name] }
    $action = 'keep'
    if ([string]::IsNullOrEmpty($currentDsn)) {
        $generatedPassword = New-DatabasePassword $RoleName
        $currentDsn = "postgres://${RoleName}:$generatedPassword@127.0.0.1:5432/pms?sslmode=disable"
        Set-DotEnvIfEmpty $DotEnvValues $Name $currentDsn
        $action = 'fresh'
    }
    $encodedPassword = Get-DsnEncodedPassword $currentDsn
    try {
        $currentPassword = [Uri]::UnescapeDataString($encodedPassword)
    } catch {
        Fail "$Name has an invalid percent-encoded password component. Correct it in .env, then rerun."
    }
    if ($encodedPassword -eq $DefaultPassword) {
        $targetPassword = New-DatabasePassword $RoleName
        return @{ Action = 'rotate'; Password = $targetPassword; Url = "postgres://${RoleName}:$targetPassword@127.0.0.1:5432/pms?sslmode=disable" }
    }
    return @{ Action = $action; Password = $currentPassword; Url = $currentDsn }
}

function Protect-FileForCurrentUser([string]$Path) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $security = New-Object Security.AccessControl.FileSecurity
    $security.SetOwner($identity)
    $security.SetAccessRuleProtection($true, $false)
    $rule = New-Object Security.AccessControl.FileSystemAccessRule($identity, 'FullControl', 'Allow')
    $security.AddAccessRule($rule)
    Set-Acl -LiteralPath $Path -AclObject $security
}

function Protect-DirectoryForCurrentUser([string]$Path) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $security = New-Object Security.AccessControl.DirectorySecurity
    $security.SetOwner($identity)
    $security.SetAccessRuleProtection($true, $false)
    $inheritance = [Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit'
    $propagation = [Security.AccessControl.PropagationFlags]::None
    $rule = New-Object Security.AccessControl.FileSystemAccessRule($identity, 'FullControl', $inheritance, $propagation, 'Allow')
    $security.AddAccessRule($rule)
    Set-Acl -LiteralPath $Path -AclObject $security
}

function Configure-Environment {
    if (-not (Test-Path -LiteralPath $EnvFile -PathType Leaf)) {
        Copy-Item -LiteralPath (Join-Path $Root '.env.example') -Destination $EnvFile
    }
    Protect-FileForCurrentUser $EnvFile
    $dotenvValues = Read-DotEnv $EnvFile

    if ((-not $dotenvValues.ContainsKey('PMS_KEK')) -or [string]::IsNullOrEmpty([string]$dotenvValues['PMS_KEK'])) {
        $kek = (& openssl rand -base64 32 2>$null | Out-String).Trim()
        if ($LASTEXITCODE -ne 0) { Fail "OpenSSL could not generate PMS_KEK. Repair OpenSSL, then rerun this command." }
        Set-DotEnvIfEmpty $dotenvValues 'PMS_KEK' $kek
    } else {
        Set-DotEnvIfEmpty $dotenvValues 'PMS_KEK' ([string]$dotenvValues['PMS_KEK'])
    }
    $ownerDsn = Prepare-DatabaseDsn $dotenvValues 'PMS_DATABASE_OWNER_URL' 'pms_owner' 'dev_only_owner_pw'
    $script:OwnerPasswordAction = $ownerDsn.Action
    $script:OwnerTargetPassword = $ownerDsn.Password
    $script:OwnerTargetUrl = $ownerDsn.Url
    if ($OwnerPasswordAction -eq 'rotate') {
        $script:OwnerStartupPassword = 'dev_only_owner_pw'
    } else {
        $script:OwnerStartupPassword = $OwnerTargetPassword
    }
    $appDsn = Prepare-DatabaseDsn $dotenvValues 'PMS_DATABASE_APP_URL' 'pms_app' 'dev_only_app_pw'
    $script:AppPasswordAction = $appDsn.Action
    $script:AppTargetPassword = $appDsn.Password
    $script:AppTargetUrl = $appDsn.Url
    Set-DotEnvIfEmpty $dotenvValues 'PMS_TLS_CERT_PATH' $CertFile
    Set-DotEnvIfEmpty $dotenvValues 'PMS_TLS_KEY_PATH' $KeyFile
    Set-DotEnvIfEmpty $dotenvValues 'PMS_LISTEN_ADDR' '0.0.0.0:8443'
    Set-DotEnvIfEmpty $dotenvValues 'PMS_ATTACHMENT_STORE' (Join-Path $env:LOCALAPPDATA 'pms\attachments')
    Set-DotEnvIfEmpty $dotenvValues 'PMS_WEB_DIST' $WebDist

    if ((-not $dotenvValues.ContainsKey('PMS_ADMIN_PASSWORD')) -or [string]::IsNullOrEmpty([string]$dotenvValues['PMS_ADMIN_PASSWORD'])) {
        $password = (& openssl rand -base64 18 2>$null | Out-String).Trim()
        if (($LASTEXITCODE -ne 0) -or ($password.Length -ne 24)) {
            Fail "OpenSSL could not generate the administrator password. Repair OpenSSL, then rerun this command."
        }
        Set-DotEnvIfEmpty $dotenvValues 'PMS_ADMIN_PASSWORD' $password
        $script:AdminPasswordGenerated = $true
    }

    foreach ($entry in $dotenvValues.GetEnumerator()) {
        Set-Item -Path ("Env:" + $entry.Key) -Value ([string]$entry.Value)
    }
    if (-not [IO.Path]::IsPathRooted($env:PMS_ATTACHMENT_STORE)) {
        Fail "PMS_ATTACHMENT_STORE must be absolute. Set an absolute directory outside the repository in .env, then rerun."
    }
    New-Item -ItemType Directory -Force -Path $env:PMS_ATTACHMENT_STORE | Out-Null
    if (-not (Test-Path -LiteralPath $env:PMS_ATTACHMENT_STORE -PathType Container)) {
        Fail "PMS_ATTACHMENT_STORE is not a directory. Set it to an absolute directory outside the repository in .env, then rerun."
    }
    $storePath = [IO.Path]::GetFullPath($env:PMS_ATTACHMENT_STORE).TrimEnd('\')
    $rootPrefix = $Root.TrimEnd('\') + '\'
    if (($storePath -eq $Root.TrimEnd('\')) -or $storePath.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        Fail "PMS_ATTACHMENT_STORE is inside the repository. Set it to an absolute directory outside $Root in .env, then rerun."
    }
    if (-not [IO.Path]::IsPathRooted($env:PMS_WEB_DIST)) {
        Fail "PMS_WEB_DIST must be absolute. Set it to the Angular production output directory in .env, then rerun."
    }
    if (($env:PMS_WEB_DIST -ne $WebDist) -and (-not (Test-Path -LiteralPath $env:PMS_WEB_DIST -PathType Container))) {
        Fail "PMS_WEB_DIST does not exist and is not the bundle this setup builds. Correct it in .env, then rerun."
    }
    if (Test-Path -LiteralPath $env:PMS_WEB_DIST -PathType Container) {
        $webDistPath = [IO.Path]::GetFullPath($env:PMS_WEB_DIST).TrimEnd('\')
        $webPrefix = $webDistPath + '\'
        $storePrefix = $storePath + '\'
        if (($storePath -eq $webDistPath) -or
            $storePath.StartsWith($webPrefix, [StringComparison]::OrdinalIgnoreCase) -or
            $webDistPath.StartsWith($storePrefix, [StringComparison]::OrdinalIgnoreCase)) {
            Fail "PMS_WEB_DIST and PMS_ATTACHMENT_STORE overlap. Set them to separate directories in .env, then rerun."
        }
    }
    try {
        $decodedKek = [Convert]::FromBase64String($env:PMS_KEK)
    } catch {
        Fail "PMS_KEK must be base64 for exactly 32 bytes. Restore the correct key in .env; do not generate a replacement for existing encrypted data."
    }
    if ($decodedKek.Length -ne 32) {
        Fail "PMS_KEK must be base64 for exactly 32 bytes. Restore the correct key in .env; do not generate a replacement for existing encrypted data."
    }
    if ($env:PMS_ADMIN_PASSWORD.Length -lt 12) {
        Fail "PMS_ADMIN_PASSWORD must be at least 12 characters. Set a longer value in .env, then rerun."
    }
    $null = Get-ListenPort
    # A narrower bind is a legitimate operator choice for a local-only install,
    # so it is a warning rather than a refusal; the summary then omits the LAN
    # URL instead of printing one that cannot be reached.
    if ($env:PMS_LISTEN_ADDR -notmatch '^(?::|0\.0\.0\.0:|\[::\]:)[0-9]+$') {
        $configuredPort = $env:PMS_LISTEN_ADDR.Substring($env:PMS_LISTEN_ADDR.LastIndexOf(':') + 1)
        $script:ListenAllInterfaces = $false
        Write-Host "Note: PMS_LISTEN_ADDR is $($env:PMS_LISTEN_ADDR), so only this machine can reach the server. Set it to 0.0.0.0:$configuredPort in .env for LAN access."
    } else {
        $script:ListenAllInterfaces = $true
    }
    Protect-DirectoryForCurrentUser $env:PMS_ATTACHMENT_STORE
    Protect-FileForCurrentUser $EnvFile
}

function Ensure-Tls([string]$HostName, [string]$LanIp) {
    New-Item -ItemType Directory -Force -Path $CertDir | Out-Null
    $certExists = Test-Path -LiteralPath $CertFile -PathType Leaf
    $keyExists = Test-Path -LiteralPath $KeyFile -PathType Leaf
    if ($certExists -and $keyExists) {
        Write-Host 'Reusing existing TLS certificate.'
    } elseif ($certExists -or $keyExists) {
        Fail "Only one of $CertFile and $KeyFile exists. Restore the missing matching file or move the remaining file aside, then rerun."
    } else {
        $san = "subjectAltName=DNS:localhost,DNS:$HostName,IP:127.0.0.1,IP:$LanIp"
        & openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 3650 -keyout $KeyFile -out $CertFile -subj '/CN=localhost' -addext $san *> $null
        if ($LASTEXITCODE -ne 0) {
            Fail "OpenSSL could not create the TLS certificate. Run 'openssl version', repair OpenSSL, then rerun this command."
        }
        Write-Host "Created a self-signed TLS certificate for localhost, $HostName, and $LanIp."
    }
    if (-not (Test-Path -LiteralPath $env:PMS_TLS_CERT_PATH -PathType Leaf)) {
        Fail "PMS_TLS_CERT_PATH does not exist. Put the certificate at the path configured in .env, then rerun."
    }
    if (-not (Test-Path -LiteralPath $env:PMS_TLS_KEY_PATH -PathType Leaf)) {
        Fail "PMS_TLS_KEY_PATH does not exist. Put the matching private key at the path configured in .env, then rerun."
    }
}

function Invoke-DockerCompose([string[]]$ComposeArguments) {
    $passwordWasSet = Test-Path Env:POSTGRES_PASSWORD
    $savedPassword = $null
    if ($passwordWasSet) { $savedPassword = $env:POSTGRES_PASSWORD }
    $env:POSTGRES_PASSWORD = $OwnerStartupPassword
    try {
        & docker compose --env-file $EnvFile -f $ComposeFile @ComposeArguments
        $script:DockerComposeExitCode = $LASTEXITCODE
    } finally {
        if ($passwordWasSet) { $env:POSTGRES_PASSWORD = $savedPassword } else { Remove-Item Env:POSTGRES_PASSWORD -ErrorAction SilentlyContinue }
    }
}

function Get-NativePostgresqlService {
    $service = Get-Service -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -like 'postgresql*-17' } |
        Select-Object -First 1
    return $service
}

function Start-NativePostgresqlService {
    $service = Get-NativePostgresqlService
    if ($null -eq $service) {
        Fail "PostgreSQL 17 is installed but its Windows service was not found. Repair it with 'winget install --id PostgreSQL.PostgreSQL.17 --exact --interactive', then rerun with --db=native."
    }
    try {
        $serviceDetails = Get-CimInstance Win32_Service -Filter "Name = '$($service.Name)'"
        if ($serviceDetails.StartMode -ne 'Auto') { Set-Service -Name $service.Name -StartupType Automatic }
        if ($service.Status -ne 'Running') { Start-Service -Name $service.Name }
    } catch {
        Fail "PostgreSQL 17 could not be enabled and started. Open PowerShell as Administrator, run 'Set-Service -Name $($service.Name) -StartupType Automatic; Start-Service -Name $($service.Name)', then rerun with --db=native."
    }
}

function Test-NativePostgresqlSuperuser {
    'SELECT 1;' | & psql -X --host 127.0.0.1 --port 5432 --username postgres --dbname postgres --no-password -Atq *> $null
    return $LASTEXITCODE -eq 0
}

function Require-NativePostgresqlSuperuser {
    if (Test-NativePostgresqlSuperuser) { return }
    Fail "PostgreSQL is running, but bootstrap access as postgres failed. From this repository run '`$credential = Get-Credential -UserName postgres; `$env:PGPASSWORD = `$credential.GetNetworkCredential().Password; powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1 --db=native; Remove-Item Env:PGPASSWORD'."
}

function Invoke-NativePostgresqlQuery([string]$Sql) {
    $queryOutput = ($Sql | & psql -X --host 127.0.0.1 --port 5432 --username postgres --dbname postgres --no-password -v ON_ERROR_STOP=1 -Atq 2>$null | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { Fail 'A native PostgreSQL bootstrap query failed. Verify postgres superuser access, then rerun with --db=native.' }
    return $queryOutput
}

function Invoke-NativePostgresqlSql([string]$Sql) {
    $Sql | & psql -X --host 127.0.0.1 --port 5432 --username postgres --dbname postgres --no-password -v ON_ERROR_STOP=1 -q *> $null
    if ($LASTEXITCODE -ne 0) { Fail 'A native PostgreSQL bootstrap command failed. Verify postgres superuser access, then rerun with --db=native.' }
}

function Restart-NativePostgresqlService {
    $service = Get-NativePostgresqlService
    try {
        Restart-Service -Name $service.Name
    } catch {
        Fail "PostgreSQL 17 could not be restarted. Open PowerShell as Administrator, run 'Restart-Service -Name $($service.Name)', then rerun with --db=native."
    }
}

function Configure-NativePostgresql {
    Invoke-NativePostgresqlSql "ALTER SYSTEM SET listen_addresses TO '127.0.0.1';`nALTER SYSTEM SET port TO '5432';"
    Restart-NativePostgresqlService
    Require-NativePostgresqlSuperuser
}

function Ensure-NativeOwnerRole {
    $roleExists = Invoke-NativePostgresqlQuery "SELECT 1 FROM pg_roles WHERE rolname = 'pms_owner'"
    if ([string]::IsNullOrEmpty($roleExists)) {
        if ($OwnerPasswordAction -eq 'keep') {
            Fail 'PMS_DATABASE_OWNER_URL contains an operator-chosen password, but pms_owner does not exist. Create pms_owner as a LOGIN SUPERUSER with that password, then rerun; setup will not replace an operator-chosen credential.'
        }
        Invoke-NativePostgresqlSql "CREATE ROLE pms_owner LOGIN SUPERUSER PASSWORD '$OwnerStartupPassword';"
    } else {
        Invoke-NativePostgresqlSql 'ALTER ROLE pms_owner LOGIN SUPERUSER;'
        if ($OwnerPasswordAction -eq 'fresh') { Invoke-NativePostgresqlSql "ALTER ROLE pms_owner PASSWORD '$OwnerTargetPassword';" }
    }
}

function Ensure-NativeDatabase {
    Ensure-NativeOwnerRole
    $databaseExists = Invoke-NativePostgresqlQuery "SELECT 1 FROM pg_database WHERE datname = 'pms'"
    if ([string]::IsNullOrEmpty($databaseExists)) {
        Invoke-NativePostgresqlSql 'CREATE DATABASE pms OWNER pms_owner;'
    } else {
        Invoke-NativePostgresqlSql 'ALTER DATABASE pms OWNER TO pms_owner;'
    }
}

function Start-Database {
    if ($DatabaseMode -eq 'docker') {
        Invoke-DockerCompose @('up', '-d')
        if ($DockerComposeExitCode -ne 0) {
            Fail "PostgreSQL could not be started. Run 'docker compose --env-file `"$EnvFile`" -f `"$ComposeFile`" up -d', fix the reported problem, then rerun."
        }
        if ($OwnerPasswordAction -eq 'fresh') {
            Set-DatabaseRolePassword 'pms_owner' $OwnerTargetPassword
        }
        return
    }
    Start-NativePostgresqlService
    Require-NativePostgresqlSuperuser
    $serverVersion = Invoke-NativePostgresqlQuery 'SHOW server_version'
    $serverMajor = ([regex]::Match($serverVersion, '^([0-9]+)')).Groups[1].Value
    if ($serverMajor -ne '17') {
        Fail "The reachable native PostgreSQL server is major version $serverMajor; version 17 is required. Stop that server and make PostgreSQL 17 the local server, then rerun. Setup will not upgrade or downgrade it."
    }
    Configure-NativePostgresql
    Ensure-NativeDatabase
}

function Test-DatabaseReady {
    if ($DatabaseMode -eq 'docker') {
        Invoke-DockerCompose @('exec', '-T', 'postgres', 'pg_isready', '-U', 'pms_owner', '-d', 'pms') *> $null
        return $DockerComposeExitCode -eq 0
    } else {
        & pg_isready --host 127.0.0.1 --port 5432 --dbname pms *> $null
        return $LASTEXITCODE -eq 0
    }
}

function Invoke-Migrations {
    for ($attempt = 1; $attempt -le 30; $attempt++) {
        if (Test-DatabaseReady) {
            $bootstrapPassword = $null
            if (($DatabaseMode -eq 'native') -and (Test-Path Env:PGPASSWORD)) {
                $bootstrapPassword = $env:PGPASSWORD
                Remove-Item Env:PGPASSWORD
            }
            Push-Location (Join-Path $Root 'api')
            try {
                & go run ./cmd/migrate up
                if ($LASTEXITCODE -ne 0) {
                    Fail "Migrations failed. Load .env, run 'cd api; go run ./cmd/migrate up', fix the reported problem, then rerun."
                }
            } finally {
                Pop-Location
                if ($null -ne $bootstrapPassword) { $env:PGPASSWORD = $bootstrapPassword }
            }
            return
        }
        Start-Sleep -Seconds 2
    }
    if ($DatabaseMode -eq 'docker') {
        Fail "PostgreSQL did not become ready within 60 seconds. Run 'docker compose --env-file `"$EnvFile`" -f `"$ComposeFile`" logs postgres', fix the reported problem, then rerun."
    }
    Fail 'Native PostgreSQL did not become ready on 127.0.0.1:5432 within 60 seconds. Inspect its Windows service, then rerun with --db=native.'
}

function Invoke-DatabaseSql([string]$Sql) {
    if ($DatabaseMode -eq 'native') {
        Invoke-NativePostgresqlSql $Sql
        return
    }
    $passwordWasSet = Test-Path Env:POSTGRES_PASSWORD
    $savedPassword = $null
    if ($passwordWasSet) { $savedPassword = $env:POSTGRES_PASSWORD }
    $env:POSTGRES_PASSWORD = $OwnerStartupPassword
    try {
        $Sql | & docker compose --env-file $EnvFile -f $ComposeFile exec -T postgres psql -X --username pms_owner --dbname postgres -v ON_ERROR_STOP=1 -q *> $null
        if ($LASTEXITCODE -ne 0) { Fail 'A Docker PostgreSQL password update failed. Inspect the postgres container logs, then rerun.' }
    } finally {
        if ($passwordWasSet) { $env:POSTGRES_PASSWORD = $savedPassword } else { Remove-Item Env:POSTGRES_PASSWORD -ErrorAction SilentlyContinue }
    }
}

function Set-DatabaseRolePassword([string]$RoleName, [string]$Password) {
    Invoke-DatabaseSql "ALTER ROLE $RoleName PASSWORD '$Password';"
}

function Converge-DatabasePasswords {
    if ($OwnerPasswordAction -ne 'keep') {
        Set-DatabaseRolePassword 'pms_owner' $OwnerTargetPassword
        if ($OwnerPasswordAction -eq 'rotate') {
            Rewrite-DotEnvValue 'PMS_DATABASE_OWNER_URL' $OwnerTargetUrl
            Write-Host 'Rotated the default pms_owner database password.'
        }
    }
    if ($AppPasswordAction -ne 'keep') {
        Set-DatabaseRolePassword 'pms_app' $AppTargetPassword
        if ($AppPasswordAction -eq 'rotate') {
            Rewrite-DotEnvValue 'PMS_DATABASE_APP_URL' $AppTargetUrl
            Write-Host 'Rotated the default pms_app database password.'
        }
    }
}

function Invoke-AdminTool([string[]]$Arguments) {
    Push-Location (Join-Path $Root 'api')
    try {
        $output = (& go run ./cmd/admintool @Arguments 2>&1 | Out-String)
        return @{ ExitCode = $LASTEXITCODE; Output = $output }
    } finally {
        Pop-Location
    }
}

function Seed-Administrator {
    $username = 'admin'
    if ((Test-Path Env:SEED_USERNAME) -and -not [string]::IsNullOrEmpty($env:SEED_USERNAME)) { $username = $env:SEED_USERNAME }
    $seed = Invoke-AdminTool @('seed-admin', '--username', $username, '--password-env', 'PMS_ADMIN_PASSWORD')
    if ($seed.ExitCode -ne 0 -and $seed.Output -notlike '*an account with this canonical username already exists*') {
        Fail "The administrator could not be created. Load .env and run 'cd api; go run ./cmd/admintool seed-admin --username $username --password-env PMS_ADMIN_PASSWORD', fix the reported problem, then rerun."
    }
    # A newly created account carries must_change_password = true (the account
    # table's default), which would make the .env password unusable until the
    # operator changed it interactively. Clearing it here is what makes the
    # promise of this script true: one command, then sign in with the password
    # already in .env. The same call reconciles a pre-existing account, so both
    # paths end in the same state.
    $reset = Invoke-AdminTool @('reset-admin', '--username', $username, '--password-env', 'PMS_ADMIN_PASSWORD', '--must-change=false')
    if ($reset.ExitCode -ne 0) {
        Fail "The administrator password could not be set. Load .env and run 'cd api; go run ./cmd/admintool reset-admin --username $username --password-env PMS_ADMIN_PASSWORD --must-change=false', then rerun."
    }
    if ($AdminPasswordGenerated) {
        Write-Host 'The generated administrator password was written to .env.'
    }
}

function Build-WebApplication {
    Push-Location (Join-Path $Root 'web')
    try {
        if (Test-Path -LiteralPath 'package-lock.json' -PathType Leaf) {
            & npm.cmd ci
        } else {
            & npm.cmd install
        }
        if ($LASTEXITCODE -ne 0) { Fail "Web dependencies could not be installed. Resolve the npm error, then rerun this command." }
        & npm.cmd run build -- --configuration production
        if ($LASTEXITCODE -ne 0) { Fail "The production web build failed. Resolve the npm error, then rerun this command." }
    } finally {
        Pop-Location
    }
}

function Build-ApiApplication {
    $apiBin = Join-Path $Root 'api\bin'
    New-Item -ItemType Directory -Force -Path $apiBin | Out-Null
    Push-Location (Join-Path $Root 'api')
    try {
        & go build -o (Join-Path $apiBin 'pms-api.exe') ./cmd/server
        if ($LASTEXITCODE -ne 0) { Fail "The API build failed. Resolve the Go error, then rerun this command." }
    } finally {
        Pop-Location
    }
}

function Build-Application {
    $secretNames = @('PMS_KEK', 'PMS_DATABASE_OWNER_URL', 'PMS_DATABASE_APP_URL', 'PMS_ADMIN_PASSWORD', 'DEV_ADMIN_PASSWORD')
    $savedSecrets = @{}
    foreach ($secretName in $secretNames) {
        if (Test-Path "Env:$secretName") {
            $savedSecrets[$secretName] = (Get-Item "Env:$secretName").Value
            Remove-Item "Env:$secretName"
        }
    }
    try {
        Build-WebApplication
        Build-ApiApplication
    } finally {
        foreach ($secretName in $savedSecrets.Keys) {
            Set-Item -Path "Env:$secretName" -Value $savedSecrets[$secretName]
        }
    }
}

function Get-ListenPort {
    $match = [regex]::Match($env:PMS_LISTEN_ADDR, ':([0-9]+)$')
    if (-not $match.Success) {
        Fail "PMS_LISTEN_ADDR must end in a valid TCP port. Fix it in .env, then rerun."
    }
    $port = [int]$match.Groups[1].Value
    if (($port -lt 1) -or ($port -gt 65535)) {
        Fail "PMS_LISTEN_ADDR must end in a valid TCP port. Fix it in .env, then rerun."
    }
    return $port
}

function Get-PortOwner([int]$Port) {
    if (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue) {
        $connection = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $connection) {
            $process = Get-Process -Id $connection.OwningProcess -ErrorAction SilentlyContinue
            if ($null -ne $process) { return $process.ProcessName }
            return 'another process'
        }
        return $null
    }
    $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, $Port)
    try {
        $listener.Start()
        return $null
    } catch {
        return 'another process'
    } finally {
        $listener.Stop()
    }
}

function Stop-ManagedServer([string]$PidFile, [string]$ApiBinary) {
    if (-not (Test-Path -LiteralPath $PidFile -PathType Leaf)) { return }
    $pidText = [IO.File]::ReadAllText($PidFile).Trim()
    $savedPid = 0
    if (-not [int]::TryParse($pidText, [ref]$savedPid)) {
        Remove-Item -LiteralPath $PidFile -Force
        return
    }
    $process = Get-Process -Id $savedPid -ErrorAction SilentlyContinue
    if ($null -eq $process) {
        Remove-Item -LiteralPath $PidFile -Force
        return
    }
    $details = Get-CimInstance Win32_Process -Filter "ProcessId = $savedPid" -ErrorAction SilentlyContinue
    if (($null -eq $details) -or ([string]::IsNullOrEmpty($details.ExecutablePath)) -or ($details.ExecutablePath -ne $ApiBinary)) {
        Fail "The saved server PID $savedPid belongs to another process. Remove $PidFile only after verifying that process, then rerun."
    }
    Stop-Process -Id $savedPid
    try {
        Wait-Process -Id $savedPid -Timeout 10 -ErrorAction Stop
    } catch {
        Fail "The existing PMS server did not stop. Stop PID $savedPid manually, then rerun this command."
    }
    Remove-Item -LiteralPath $PidFile -Force
}

function Test-Health([int]$Port) {
    $oldCallback = [Net.ServicePointManager]::ServerCertificateValidationCallback
    try {
        [Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
        $response = Invoke-WebRequest -UseBasicParsing -Uri "https://127.0.0.1:$Port/api/v1/health" -TimeoutSec 2
        return $response.StatusCode -eq 200
    } catch {
        return $false
    } finally {
        [Net.ServicePointManager]::ServerCertificateValidationCallback = $oldCallback
    }
}

function Start-Application([string]$LanIp) {
    $stateDir = Join-Path $env:LOCALAPPDATA 'pms'
    New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
    Protect-DirectoryForCurrentUser $stateDir
    $pidFile = Join-Path $stateDir 'pms-api.pid'
    $stdoutLog = Join-Path $stateDir 'pms-api.stdout.log'
    $stderrLog = Join-Path $stateDir 'pms-api.stderr.log'
    $apiBinary = Join-Path $Root 'api\bin\pms-api.exe'
    Stop-ManagedServer $pidFile $apiBinary

    $port = Get-ListenPort
    $owner = Get-PortOwner $port
    if ($null -ne $owner) {
        Fail "PMS_LISTEN_ADDR port $port is already in use by $owner. Stop that process or change PMS_LISTEN_ADDR in .env, then rerun."
    }

    $ownerUrl = $env:PMS_DATABASE_OWNER_URL
    $adminPassword = $env:PMS_ADMIN_PASSWORD
    $devPasswordPresent = Test-Path Env:DEV_ADMIN_PASSWORD
    $devPassword = $null
    if ($devPasswordPresent) { $devPassword = $env:DEV_ADMIN_PASSWORD }
    Remove-Item Env:PMS_DATABASE_OWNER_URL -ErrorAction SilentlyContinue
    Remove-Item Env:PMS_ADMIN_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:DEV_ADMIN_PASSWORD -ErrorAction SilentlyContinue
    try {
        $process = Start-Process -FilePath $apiBinary -WorkingDirectory (Join-Path $Root 'api') -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog -PassThru
    } finally {
        $env:PMS_DATABASE_OWNER_URL = $ownerUrl
        $env:PMS_ADMIN_PASSWORD = $adminPassword
        if ($devPasswordPresent) { $env:DEV_ADMIN_PASSWORD = $devPassword }
    }
    [IO.File]::WriteAllText($pidFile, [string]$process.Id, $Utf8NoBom)

    for ($attempt = 1; $attempt -le 60; $attempt++) {
        $process.Refresh()
        if ($process.HasExited) {
            Remove-Item -LiteralPath $pidFile -Force
            Fail "The PMS server exited during startup. Inspect $stdoutLog and $stderrLog, fix the reported problem, then rerun."
        }
        if (Test-Health $port) {
            $username = 'admin'
            if ((Test-Path Env:SEED_USERNAME) -and -not [string]::IsNullOrEmpty($env:SEED_USERNAME)) { $username = $env:SEED_USERNAME }
            Write-Host "`nSign in at: https://localhost:$port"
            if ($script:ListenAllInterfaces) {
                Write-Host "LAN URL: https://${LanIp}:$port"
            }
            Write-Host "Administrator username: $username"
            Write-Host 'The administrator password is in .env.'
            return
        }
        Start-Sleep -Seconds 1
    }
    Stop-Process -Id $process.Id -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $pidFile -Force
    Fail "The PMS server did not answer its health check within 60 seconds. Inspect $stdoutLog and $stderrLog, fix the reported problem, then rerun."
}

try {
    Write-Heading 'Preflight'
    if (Test-Command 'winget') {
        $PackageManager = 'winget'
    } elseif (Test-Command 'choco') {
        $PackageManager = 'choco'
    }
    $hostName = $env:COMPUTERNAME
    if ([string]::IsNullOrEmpty($hostName)) { $hostName = [Net.Dns]::GetHostName() }
    if ($hostName -notmatch '^[A-Za-z0-9.-]+$') {
        Fail "The machine hostname cannot be placed in a TLS certificate. Set a hostname containing only letters, digits, dots, and hyphens, then rerun."
    }
    $lanAddress = Get-NetIPConfiguration -ErrorAction SilentlyContinue |
        Where-Object { ($null -ne $_.IPv4DefaultGateway) -and ($null -ne $_.IPv4Address) } |
        ForEach-Object { $_.IPv4Address.IPAddress } |
        Where-Object { $_ -and ($_ -notlike '169.254.*') } |
        Select-Object -First 1
    if ([string]::IsNullOrEmpty($lanAddress)) {
        Fail "Could not determine the primary LAN IPv4 address. Connect this machine to the LAN, then rerun this command."
    }
    Write-Host "Detected: Windows ($env:PROCESSOR_ARCHITECTURE); package manager: $(if ($PackageManager) { $PackageManager } else { 'none' })"
    Refresh-Path
    Select-DatabaseMode

    Write-Heading 'Dependencies'
    if ($SkipDeps) { Write-Host 'Dependency installation skipped; verifying required tools.' }
    Ensure-Dependencies
    if ($DatabaseMode -eq 'docker') { Ensure-DockerRunning }
    Write-Host 'All required dependencies are available.'

    Write-Heading 'Configuration'
    Configure-Environment
    Write-Host 'Configuration is ready and existing values were preserved.'

    Write-Heading 'TLS'
    Ensure-Tls $hostName $lanAddress

    Write-Heading 'Database'
    Start-Database
    Invoke-Migrations
    Converge-DatabasePasswords
    if ($DatabaseMode -eq 'native') { Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue }
    Write-Host 'Database is ready and migrations are current.'

    Write-Heading 'Seed administrator'
    Seed-Administrator
    Write-Host 'Administrator account is ready; no demonstration data was loaded.'

    Write-Heading 'Build'
    Build-Application
    Write-Host 'Production API and web bundle were built.'

    Write-Heading 'Start'
    if ($NoStart) {
        Write-Host 'Server start skipped by --no-start.'
    } else {
        Start-Application $lanAddress
    }
} catch {
    [Console]::Error.WriteLine("ERROR: Setup failed during $CurrentPhase. $($_.Exception.Message) Fix that error, then rerun this command.")
    exit 1
}
