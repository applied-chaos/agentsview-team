$ErrorActionPreference = 'Stop'
Import-Module /psa/PSScriptAnalyzer.psd1
$found = Invoke-ScriptAnalyzer -Path /scripts -Recurse -Severity Warning, Error
$found | Format-List RuleName, Severity, ScriptName, Line, Message
if ($found) { exit 1 }
Write-Output 'PSScriptAnalyzer: clean'
