@echo off
call "%~dp0..\libexec\dispatch.cmd" config %*
exit /b %ERRORLEVEL%
