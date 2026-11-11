#!/usr/bin/env bash
# mermaid-live-editor 一键安装/更新（生产部署）
# 用法：bash install.sh   （以普通用户运行；systemd 步骤需要 sudo）
# 产物：bin/mermaid-server + bin/static/，服务名 mermaid-server（0.0.0.0:3000）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

echo "==> 1/4 构建（生产运行不受影响：构建期间仍服务旧的 bin/static）"
pnpm build:prod

echo "==> 2/4 清理废弃单元"
for unit in mermaid-frontend mermaid-storage mermaid-live; do
  sudo systemctl disable --now "$unit" 2>/dev/null || true
  sudo rm -f "/etc/systemd/system/$unit.service"
done
rm -rf server-go/bin bin/mermaid-storage  # 旧位置/旧名二进制

echo "==> 3/4 安装 mermaid-server 单元"
sudo ln -sf "$ROOT/systemd/mermaid-server.service" /etc/systemd/system/mermaid-server.service
sudo systemctl daemon-reload
sudo systemctl enable mermaid-server

echo "==> 4/4 启动并验证"
sudo systemctl restart mermaid-server
sleep 1
curl -sf -o /dev/null http://localhost:3000/ && echo "  OK: / -> 200"
curl -s http://localhost:3000/api/auth/me | grep -q unauthorized && echo "  OK: /api/auth/me -> 401 (gated)"
systemctl is-active mermaid-server

echo "完成：http://$(hostname -I 2>/dev/null | awk '{print $1}'):3000"
