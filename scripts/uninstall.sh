#!/bin/sh
# 卸载 pulse-server、pulse-node 及所有相关数据（不可恢复）
set -eu

run_as_root() {
  if [ "$(id -u)" -eq 0 ]; then "$@"; return; fi
  if command -v sudo >/dev/null 2>&1; then sudo "$@"; return; fi
  echo "需要 root 权限: $*" >&2; exit 1
}

stop_and_disable() {
  svc="$1"
  if command -v systemctl >/dev/null 2>&1 && systemctl --version >/dev/null 2>&1; then
    systemctl is-active --quiet "$svc" 2>/dev/null && run_as_root systemctl stop "$svc" || true
    systemctl is-enabled --quiet "$svc" 2>/dev/null && run_as_root systemctl disable "$svc" || true
  elif command -v rc-service >/dev/null 2>&1; then
    rc-service "$svc" status >/dev/null 2>&1 && run_as_root rc-service "$svc" stop || true
    run_as_root rc-update del "$svc" default 2>/dev/null || true
  fi
}

bin_dir="${PULSE_INSTALL_BIN:-/usr/local/bin}"
etc_dir="${PULSE_INSTALL_ETC:-/etc/pulse}"
share_dir="${PULSE_INSTALL_SHARE:-/usr/local/share/pulse}"
lib_dir="${PULSE_INSTALL_LIB:-/etc/systemd/system}"
initd_dir="${PULSE_INSTALL_INITD:-/etc/init.d}"
state_dir="${PULSE_STATE_DIR:-/var/lib/pulse}"

# ── pulse-server ───────────────────────────────────────────────────────────────
stop_and_disable pulse-server
run_as_root rm -f "${lib_dir}/pulse-server.service"
run_as_root rm -f "${initd_dir}/pulse-server"
run_as_root rm -f "${bin_dir}/pulse-server"

# ── pulse-node ─────────────────────────────────────────────────────────────────
stop_and_disable pulse-node
run_as_root rm -f "${lib_dir}/pulse-node.service"
run_as_root rm -f "${initd_dir}/pulse-node"
run_as_root rm -f "${bin_dir}/pulse-node"

# systemd daemon-reload
command -v systemctl >/dev/null 2>&1 && run_as_root systemctl daemon-reload || true

# systemd journald 日志（节点/服务的历史日志，含订阅 token、IP 等敏感数据）
if command -v journalctl >/dev/null 2>&1; then
  run_as_root journalctl --vacuum-time=1s --unit=pulse-server >/dev/null 2>&1 || true
  run_as_root journalctl --vacuum-time=1s --unit=pulse-node   >/dev/null 2>&1 || true
fi

# ── pulse 配置与数据 ────────────────────────────────────────────────────────────
# 先读 PULSE_DATA_DIR（同时支持 server / node-only 安装），再删配置目录
data_dir="$(cat "${etc_dir}/pulse-server.env" "${etc_dir}/pulse-node.env" 2>/dev/null \
  | grep '^PULSE_DATA_DIR=' | tail -1 | cut -d= -f2- | tr -d "'\"")"

# OpenRC PID 文件（异常退出时可能残留）
run_as_root rm -f /run/pulse-server.pid /run/pulse-node.pid

run_as_root rm -rf "$etc_dir"
run_as_root rm -rf "$share_dir"
run_as_root rm -rf "$state_dir"

# certmgr 默认证书目录（节点启用 SNI Terminating / direct TLS 时使用，硬编码路径）
run_as_root rm -rf /var/lib/pulse-node

# PULSE_DATA_DIR 可能与 state_dir 不同（geoip、上传文件等）
if [ -n "$data_dir" ] && [ "$data_dir" != "$state_dir" ]; then
  run_as_root rm -rf "$data_dir"
fi

# OpenRC 日志（systemd 走 journald，无需清理）
run_as_root rm -f /var/log/pulse-server.log /var/log/pulse-node.log

echo ""
echo "卸载完成：pulse-server、pulse-node 及所有数据已删除"
