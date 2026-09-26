param(
    [string]$SourceProto = "..\annw-mp\src\XingyiStarry.Mp.Protocol\Protos\multiplayer.proto"
)

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot
$protoc = (Get-Command protoc -ErrorAction Stop).Source
$protocGenGo = (Get-Command protoc-gen-go -ErrorAction Stop).Source

$sourcePath = [IO.Path]::GetFullPath($(if ([IO.Path]::IsPathRooted($SourceProto)) { $SourceProto } else { Join-Path $projectRoot $SourceProto }))
if (-not (Test-Path -LiteralPath $sourcePath)) { throw "Canonical plugin proto not found: $sourcePath" }
$protoRoot = Split-Path -Parent $sourcePath
$protoName = Split-Path -Leaf $sourcePath

Push-Location $protoRoot
try {
    & $protoc --plugin="protoc-gen-go=$protocGenGo" --proto_path=$protoRoot --go_out=$projectRoot --go_opt=module=mprelay $protoName
    if ($LASTEXITCODE -ne 0) { throw "protoc failed with exit code $LASTEXITCODE" }
}
finally {
    Pop-Location
}
