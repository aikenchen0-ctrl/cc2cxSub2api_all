#!/bin/bash
# BidMonitor 启动脚本
# 用于手动启动服务（通常使用 setup.sh 一键部署）

cd /opt/bidmonitor/server

YIBIAO_PORT="${YIBIAO_PORT:-8081}"

# 停止已有进程
pkill -f "uvicorn app:app" 2>/dev/null || true
sleep 1

# 创建日志目录
mkdir -p logs

# 后台启动
echo "正在启动 BidMonitor 服务..."
nohup python3 -m uvicorn app:app --host 0.0.0.0 --port "$YIBIAO_PORT" > logs/server.log 2>&1 &
sleep 2

# 验证启动
if pgrep -f "uvicorn app:app" > /dev/null; then
    echo "✅ BidMonitor 已在后台启动"
    echo "📋 查看日志: tail -f /opt/bidmonitor/server/logs/server.log"
    echo "🌐 访问地址: http://$(hostname -I | awk '{print $1}'):$YIBIAO_PORT"
else
    echo "❌ 启动失败，查看日志:"
    cat logs/server.log
fi
