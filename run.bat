@echo off
REM Starts backend (Go) and frontend (Vite) in development mode.
REM Each service opens in its own console window - close the window or press Ctrl+C to stop it.

set ROOT=%~dp0
set BACKEND_DIR=%ROOT%callcentrix-backend
set FRONTEND_DIR=%ROOT%callcenter.tj-ui

if not exist "%BACKEND_DIR%\go.mod" (
    echo [ERROR] Backend not found at "%BACKEND_DIR%"
    exit /b 1
)
if not exist "%FRONTEND_DIR%\package.json" (
    echo [ERROR] Frontend not found at "%FRONTEND_DIR%"
    exit /b 1
)

echo Starting backend (http://localhost:8080)...
start "CallCentrix Backend" cmd /k "cd /d "%BACKEND_DIR%" && go run ./cmd/server"

echo Starting frontend (http://localhost:3000)...
start "CallCentrix Frontend" cmd /k "cd /d "%FRONTEND_DIR%" && (if not exist node_modules npm install) && npm run start"

echo.
echo Backend and frontend are starting in separate windows.
echo To stop them, close the windows or press Ctrl+C in each one.
