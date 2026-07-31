@echo off
call "%~dp0..\libexec\dispatch.cmd" login %*
exit /b %ERRORLEVEL%
