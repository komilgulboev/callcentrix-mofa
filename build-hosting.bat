@echo off
REM Builds backend (Go) and frontend (Vite), then packages both into cc_hosting_version/
REM ready to copy to a server as-is. The backend serves the frontend itself (STATIC_DIR=./web
REM by default), so cc_hosting_version/web must sit next to the executable.
REM
REM Usage:
REM   build-hosting.bat        -> Windows backend (callcentrix.exe)
REM   build-hosting.bat linux  -> Linux backend (callcentrix-linux)

setlocal

set ROOT=%~dp0
set BACKEND_DIR=%ROOT%callcentrix-backend
set FRONTEND_DIR=%ROOT%callcenter.tj-ui
set OUT_DIR=%ROOT%cc_hosting_version

if /i "%~1"=="linux" (
    set BINARY_NAME=callcentrix-linux
) else (
    set BINARY_NAME=callcentrix_mofa.exe
)

for /f %%i in ('git rev-parse --short HEAD 2^>nul') do set GIT_COMMIT=%%i
if "%GIT_COMMIT%"=="" set GIT_COMMIT=unknown
for /f %%i in ('powershell -NoProfile -Command "Get-Date -Format o"') do set BUILD_TIME=%%i
set LDFLAGS=-X main.buildCommit=%GIT_COMMIT% -X main.buildTime=%BUILD_TIME%
echo Build info: commit=%GIT_COMMIT% time=%BUILD_TIME%

echo === Backend (Go) ===
pushd "%BACKEND_DIR%"
if /i "%~1"=="linux" (
    echo Target platform: linux/amd64
    set GOOS=linux
    set GOARCH=amd64
    go build -ldflags "%LDFLAGS%" -o %BINARY_NAME% ./cmd/server
) else (
    go build -ldflags "%LDFLAGS%" -o %BINARY_NAME% ./cmd/server
)
if errorlevel 1 (
    echo [ERROR] Backend build failed.
    popd
    exit /b 1
)
popd
echo Backend built: %BACKEND_DIR%\%BINARY_NAME%

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

copy /y "%BACKEND_DIR%\%BINARY_NAME%" "%OUT_DIR%\%BINARY_NAME%" >nul
xcopy /e /i /y "%FRONTEND_DIR%\dist\*" "%OUT_DIR%\web\" >nul

echo.
echo Build finished successfully.
echo Package ready in "%OUT_DIR%"
echo NOTE: copy your .env file into "%OUT_DIR%" before running %BINARY_NAME% there.

endlocal
