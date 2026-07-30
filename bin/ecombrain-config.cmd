@echo off
setlocal
set "EB_ARCH=amd64"
if /i "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "EB_ARCH=arm64"
set "EB_EXE=%~dp0..\libexec\ecombrain-windows-%EB_ARCH%.exe"
if not exist "%EB_EXE%" (
  echo ecombrain: no bundled binary for windows/%EB_ARCH%. 1>&2
  exit /b 1
)
"%EB_EXE%" config %*
