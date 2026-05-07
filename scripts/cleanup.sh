#!/bin/sh
# cleanup.sh — 清理 Pulse 历史遗留文件和废弃环境变量
# 用法: curl -fsSL .../cleanup.sh | sh
#   或: bash cleanup.sh [--dry-run]

set -eu

dry_run=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) dry_run=1 ;;
  esac
done

etc_dir="/etc/pulse"
node_env="${etc_dir}/pulse-node.env"
server_env="${etc_dir}/pulse-server.env"

run() {
  if [ "$dry_run" = "1" ]; then
    echo "[dry-run] $*"
  else
    "$@"
  fi
}

need_root() {
  if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    "$@"
  fi
}

echo "=== Pulse 清理工具 ==="
[ "$dry_run" = "1" ] && echo "(dry-run 模式，不实际修改)"

# ── 1. 删除 .deprecated 文件 ─────────────────────────────────────
echo ""
echo "── 遗留文件 ──"
found_files=0
for f in "${etc_dir}"/*.deprecated; do
  [ -e "$f" ] || continue
  found_files=1
  echo "  删除: $f"
  run need_root rm -f "$f"
done
[ "$found_files" = "0" ] && echo "  无遗留文件"

# ── 2. 删除废弃的 node 环境变量 ──────────────────────────────────
node_legacy_vars="
PULSE_NODE_TLS_CERT_FILE
PULSE_NODE_TLS_KEY_FILE
PULSE_NODE_TLS_CLIENT_CERT_FILE
PULSE_NODE_CA_FILE
PULSE_NODE_ADDR
PULSE_NODE_PORT
PULSE_NODE_GRPC_ADDR
"

echo ""
echo "── Node 废弃环境变量（${node_env}）──"
if [ -f "$node_env" ]; then
  found_vars=0
  for var in $node_legacy_vars; do
    var="$(echo "$var" | tr -d ' \n')"
    [ -z "$var" ] && continue
    if grep -q "^${var}=" "$node_env" 2>/dev/null; then
      found_vars=1
      echo "  移除: ${var}"
      if [ "$dry_run" = "0" ]; then
        tmp="$(mktemp)"
        grep -v "^${var}=" "$node_env" > "$tmp" || true
        need_root install -m 0644 "$tmp" "$node_env"
        rm -f "$tmp"
      fi
    fi
  done
  [ "$found_vars" = "0" ] && echo "  无废弃变量"
else
  echo "  文件不存在，跳过"
fi

# ── 3. 删除废弃的 server 环境变量 ────────────────────────────────
server_legacy_vars="
PULSE_NODE_GRPC_ADDR
"

echo ""
echo "── Server 废弃环境变量（${server_env}）──"
if [ -f "$server_env" ]; then
  found_vars=0
  for var in $server_legacy_vars; do
    var="$(echo "$var" | tr -d ' \n')"
    [ -z "$var" ] && continue
    if grep -q "^${var}=" "$server_env" 2>/dev/null; then
      found_vars=1
      echo "  移除: ${var}"
      if [ "$dry_run" = "0" ]; then
        tmp="$(mktemp)"
        grep -v "^${var}=" "$server_env" > "$tmp" || true
        need_root install -m 0644 "$tmp" "$server_env"
        rm -f "$tmp"
      fi
    fi
  done
  [ "$found_vars" = "0" ] && echo "  无废弃变量"
else
  echo "  文件不存在，跳过"
fi

echo ""
echo "=== 清理完成 ==="
