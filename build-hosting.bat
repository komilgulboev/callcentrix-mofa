@echo off
REM Builds backend (Go, for both Windows and Linux) and frontend (Vite), then packages
REM everything into cc_hosting_version/ ready to copy to a server as-is. The backend serves
REM the frontend itself (STATIC_DIR=./web by default), so cc_hosting_version/web must sit
REM next to the executable.
REM
REM Usage:
REM   build-hosting.bat        -> builds both callcentrix_mofa.exe (Windows) and
REM                                callcentrix-linux (Linux/amd64)
REM   build-hosting.bat win    -> builds only the Windows binary
REM   build-hosting.bat linux  -> builds only the Linux binary

setlocal

set ROOT=%~dp0
set BACKEND_DIR=%ROOT%callcentrix-backend
set FRONTEND_DIR=%ROOT%callcenter.tj-ui
set OUT_DIR=%ROOT%cc_hosting_version

set WIN_BINARY=callcentrix_mofa.exe
set LINUX_BINARY=callcentrix-linux

set BUILD_WIN=1
set BUILD_LINUX=1
if /i "%~1"=="win" set BUILD_LINUX=0
if /i "%~1"=="windows" set BUILD_LINUX=0
if /i "%~1"=="linux" set BUILD_WIN=0

for /f %%i in ('git rev-parse --short HEAD 2^>nul') do set GIT_COMMIT=%%i
if "%GIT_COMMIT%"=="" set GIT_COMMIT=unknown
for /f %%i in ('powershell -NoProfile -Command "Get-Date -Format o"') do set BUILD_TIME=%%i
set LDFLAGS=-X main.buildCommit=%GIT_COMMIT% -X main.buildTime=%BUILD_TIME%
echo Build info: commit=%GIT_COMMIT% time=%BUILD_TIME%

echo === Backend (Go) ===
pushd "%BACKEND_DIR%"

if "%BUILD_WIN%"=="1" (
    echo Target platform: windows/amd64
    set GOOS=windows
    set GOARCH=amd64
    go build -ldflags "%LDFLAGS%" -o %WIN_BINARY% ./cmd/server
    if errorlevel 1 (
        echo [ERROR] Windows backend build failed.
        popd
        exit /b 1
    )
    echo Backend built: %BACKEND_DIR%\%WIN_BINARY%
)

if "%BUILD_LINUX%"=="1" (
    echo Target platform: linux/amd64
    set GOOS=linux
    set GOARCH=amd64
    go build -ldflags "%LDFLAGS%" -o %LINUX_BINARY% ./cmd/server
    if errorlevel 1 (
        echo [ERROR] Linux backend build failed.
        popd
        exit /b 1
    )
    echo Backend built: %BACKEND_DIR%\%LINUX_BINARY%
)

REM Reset cross-compile env vars so nothing downstream inherits them.
set GOOS=
set GOARCH=

popd

echo.
echo === Frontend (Vite) ===
pushd "%FRONTEND_DIR%"
if not exist node_modules (
    echo Installing frontend dependencies...
    call npm install
    if errorlevel 1 (
        echo [ERROR] npm install failed.
        popd
        exit /b 1
    )
)
call npm run build
if errorlevel 1 (
    echo [ERROR] Frontend build failed.
    popd
    exit /b 1
)
popd
echo Frontend built: %FRONTEND_DIR%\dist

echo.
echo === Packaging "%OUT_DIR%" ===
if exist "%OUT_DIR%" rmdir /s /q "%OUT_DIR%"
mkdir "%OUT_DIR%"
mkdir "%OUT_DIR%\web"

if "%BUILD_WIN%"=="1" copy /y "%BACKEND_DIR%\%WIN_BINARY%" "%OUT_DIR%\%WIN_BINARY%" >nul
if "%BUILD_LINUX%"=="1" copy /y "%BACKEND_DIR%\%LINUX_BINARY%" "%OUT_DIR%\%LINUX_BINARY%" >nul
xcopy /e /i /y "%FRONTEND_DIR%\dist\*" "%OUT_DIR%\web\" >nul

echo.
echo Build finished successfully.
echo Package ready in "%OUT_DIR%"
if "%BUILD_WIN%"=="1" echo   - %WIN_BINARY%    (Windows)
if "%BUILD_LINUX%"=="1" echo   - %LINUX_BINARY%      (Linux/amd64, chmod +x before running)
echo NOTE: copy your .env file into "%OUT_DIR%" before running the binary there.

endlocal
