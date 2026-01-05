# Local build and act installer script
# Run from repo root via PowerShell

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir '..')
Set-Location $repoRoot

Write-Output "Repository root: $(Get-Location)"

Write-Output "\n-- GO BUILD --"
if (Get-Command go -ErrorAction SilentlyContinue) {
    go version
    go build -v -o github-analyzer ./cmd/server
} else {
    Write-Output 'go not found in PATH'
}

Write-Output "\n-- TAILWIND CSS BUILD --"
if (Test-Path 'tailwind') {
    Set-Location 'tailwind'
    if (Get-Command npm -ErrorAction SilentlyContinue) {
        npm ci
        npm run build:css
    } else {
        Write-Output 'npm not found in PATH'
    }
    Set-Location $repoRoot
} else {
    Write-Output 'tailwind directory not found'
}

Write-Output "\n-- ATTEMPT INSTALL act --"
if (Get-Command act -ErrorAction SilentlyContinue) {
    Write-Output "act already installed: $(act --version)"
    exit 0
}

if (Get-Command choco -ErrorAction SilentlyContinue) {
    Write-Output 'Installing act via chocolatey...'
    choco install act -y
    exit 0
}

if (Get-Command scoop -ErrorAction SilentlyContinue) {
    Write-Output 'Installing act via scoop...'
    scoop install act
    exit 0
}

try {
    $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/nektos/act/releases/latest' -UseBasicParsing
    $asset = $rel.assets | Where-Object { $_.name -like '*Windows_x86_64.zip' } | Select-Object -First 1
    if ($null -eq $asset) { Write-Output 'No Windows asset found in latest release'; exit 0 }
    $url = $asset.browser_download_url
    $dest = Join-Path $env:USERPROFILE 'Downloads\act.zip'
    Write-Output "Downloading $url to $dest"
    Invoke-WebRequest -Uri $url -OutFile $dest
    $out = Join-Path $env:USERPROFILE 'bin'
    New-Item -ItemType Directory -Force -Path $out | Out-Null
    Expand-Archive -Path $dest -DestinationPath $out -Force
    $exe = Get-ChildItem $out -Filter act.exe -Recurse | Select-Object -First 1
    if ($exe) {
        Copy-Item $exe.FullName -Destination (Join-Path $out 'act.exe') -Force
        Write-Output "Installed act to $out"
        & (Join-Path $out 'act.exe') --version
    } else {
        Write-Output 'Failed to find act.exe in archive'
    }
} catch {
    Write-Output "Error installing act: $_"
}
