param([string]$Fnpack = 'fnpack')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location (Join-Path $root 'frontend')
try {
  npm ci
  if ($LASTEXITCODE) { throw 'npm ci failed' }
  npm run typecheck
  if ($LASTEXITCODE) { throw 'frontend typecheck failed' }
  npm run build
  if ($LASTEXITCODE) { throw 'frontend build failed' }
} finally { Pop-Location }
Push-Location $root
try {
  Copy-Item -Path 'frontend/dist/*' -Destination 'internal/web/dist' -Recurse -Force
  go test ./...
  if ($LASTEXITCODE) { throw 'Go tests failed' }
  go run ./cmd/package --fnpack $Fnpack
  if ($LASTEXITCODE) { throw 'FPK build failed' }
} finally { Pop-Location }
