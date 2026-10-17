$ErrorActionPreference = 'Stop'
$jevbotRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$artifactDir = Join-Path $jevbotRoot '.artifacts'
New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
$packageFiles = @('manifest.json', 'background.js', 'core.js', 'api.js', 'history.js', 'page.js', 'browser.js', 'marks.js', 'settings.js', 'panel.html', 'panel.js', 'options.html', 'options.js', 'styles.css', 'README.md', 'IMPLEMENTATION_PLAN.md')
$absoluteFiles = $packageFiles | ForEach-Object { Join-Path $jevbotRoot $_ }
$zipPath = Join-Path $artifactDir 'jevbot.zip'
Compress-Archive -LiteralPath $absoluteFiles -DestinationPath $zipPath -Force
Write-Output $zipPath
