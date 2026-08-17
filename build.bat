@echo off
REM Builds backend (Go) and frontend (Vite) for deployment.
REM Usage:
REM   build.bat        -> backend for Windows (callcentrix.exe) + frontend (dist/)
REM   build.bat linux  -> backend for Linux (callcentrix-linux), for server deployment

setlocal

set ROOT=%~dp0
set BACKEND_DIR=%ROOT%callcentrix-backend
set FRONTEND_DIR=%ROOT%callcenter.tj-ui

echo === Backend (Go) ===
pushd "%BACKEND_DIR%"
if /i "%~1"=="linux" (
    echo Target platform: linux/amd64
    set GOOS=linux
    set GOARCH=amd64
    go build -o callcentrix-linux ./cmd/server
) else (
    go build -o callcentrix.exe ./cmd/server
)
if errorlevel 1 (
    echo [ERROR] Backend build failed.
    popd
    exit /b 1
)
popd
echo Backend built in "%BACKEND_DIR%"

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
echo Frontend built in "%FRONTEND_DIR%\dist"

echo.
echo Build finished successfully.

endlocal
