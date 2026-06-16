@echo off
chcp 65001 >nul
echo ========================================
echo 古建筑木构件超声波探伤标注系统 - 前端启动
echo ========================================

cd /d "%~dp0frontend"

echo [1/3] 检查 Node.js 环境...
node --version
if errorlevel 1 (
    echo ERROR: 未检测到 Node.js 环境，请先安装 Node.js 18+
    pause
    exit /b 1
)

echo.
echo [2/3] 安装依赖 (首次运行)...
if not exist "node_modules" (
    call npm install
)

echo.
echo [3/3] 启动前端开发服务器 (端口 5173)...
echo 后端 API 将通过代理转发到 http://localhost:8080
echo.

call npm run dev

pause
