@echo off
chcp 65001 >nul
echo ========================================
echo 古建筑木构件超声波探伤标注系统 - 后端启动
echo ========================================

cd /d "%~dp0backend"

echo [1/3] 检查 Go 环境...
go version
if errorlevel 1 (
    echo ERROR: 未检测到 Go 环境，请先安装 Go 1.22+
    pause
    exit /b 1
)

echo.
echo [2/3] 下载依赖...
go mod download

echo.
echo [3/3] 启动后端服务 (端口 8080)...
echo 请确保 PostgreSQL 和 Redis 服务已启动
echo 数据库配置: %DB_HOST%:%DB_PORT%/%DB_NAME%
echo.

go run cmd/server/main.go

pause
