@echo off
call "%~dp0..\libexec\dispatch.cmd" gql %*
exit /b %ERRORLEVEL%
