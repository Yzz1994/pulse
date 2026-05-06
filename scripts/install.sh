#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
用法:
  install.sh server|node [version] [选项]

选项（node 专用）:
  --server <URL>     控制面板地址，安装完成后自动注册节点地址（由控制面板生成时自动填入）
  --node-id <ID>     节点 ID（由控制面板生成时自动填入）
  --cert <BASE64>    server 客户端证书 base64，跳过交互式粘贴（由控制面板生成时自动填入）

选项（server 专用）:
  --reset-password   生成随机新密码并重启服务

环境变量:
  PULSE_INSTALL_BIN   二进制安装目录，默认 /usr/local/bin
  PULSE_INSTALL_ETC   配置安装目录，默认 /etc/pulse
  PULSE_INSTALL_LIB   systemd 安装目录，默认 /etc/systemd/system
  PULSE_INSTALL_INITD OpenRC 安装目录，默认 /etc/init.d
  PULSE_STATE_DIR     工作目录，默认 /var/lib/pulse
  PULSE_ADMIN_USERNAME server 安装时管理员用户名，默认 admin
  PULSE_ADMIN_PASSWORD server 安装时管理员密码，不指定则随机生成
  PULSE_SERVER_ADDR   server 监听地址，不指定则随机端口（格式 :端口）
  PULSE_NODE_ADDR     node 监听地址，默认 :8081（格式 :端口）
  PULSE_NODE_PORT     node 监听端口，优先于 PULSE_NODE_ADDR（纯数字）
  PULSE_DATABASE_URL  PostgreSQL 连接串（postgres://user:pass@host:5432/db?sslmode=disable）
  PULSE_DATA_DIR      工作目录（geoip、上传文件等），默认 /var/lib/pulse

示例:
  # 安装控制面板（server）
  curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server

  # 重置密码
  curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | \
    sh -s -- server --reset-password

  # 安装节点（推荐：从控制面板"添加节点"页面复制生成的命令，自动包含以下参数）
  bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) node \
    --server https://<控制面板地址> \
    --node-id <节点ID> \
    --cert <证书BASE64>

  # 手动安装节点（会交互式提示粘贴 server 客户端证书）
  curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- node

  # 指定 node 监听端口
  curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | \
    PULSE_NODE_PORT='9090' sh -s -- node
EOF
}

tty_available() {
  [ -r /dev/tty ] && [ -w /dev/tty ]
}

prompt_node_client_cert_pem() {
  target_file="$1"
  force="${2:-0}"

  # 更新场景：证书已存在且未强制，直接跳过
  if [ -f "$target_file" ] && [ "$force" != "1" ]; then
    return
  fi

  if ! tty_available; then
    echo "无法交互输入证书，请确保在终端中运行安装脚本" >&2
    exit 1
  fi

  cert_tmp="$(mktemp)"
  printf "请粘贴 node 需要信任的 server 客户端证书 PEM 内容。\n" > /dev/tty
  printf "粘贴完成后，按两次回车（输入空行）确认。\n" > /dev/tty
  : > "$cert_tmp"
  while IFS= read -r line < /dev/tty; do
    [ -z "$line" ] && break
    printf "%s\n" "$line" >> "$cert_tmp"
  done
  if [ ! -s "$cert_tmp" ]; then
    rm -f "$cert_tmp"
    echo "证书内容为空，安装终止" >&2
    exit 1
  fi

  run_as_root mkdir -p "$(dirname "$target_file")"
  run_as_root install -m 0644 "$cert_tmp" "$target_file"
  rm -f "$cert_tmp"
}

random_password() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 18 | tr -d '/+=' | head -c 24
  else
    tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 24
  fi
}

random_port() {
  awk 'BEGIN{srand(); print int(rand()*55535)+10000}'
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "缺少命令: $1" >&2
    exit 1
  }
}

run_as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return
  fi
  echo "需要 root 权限运行: $*" >&2
  exit 1
}

quote_env_value() {
  printf "'%s'" "$(printf "%s" "$1" | sed "s/'/'\\\\''/g")"
}

set_env_file_value() {
  file="$1"
  key="$2"
  value="$(quote_env_value "$3")"
  tmp_file="$(mktemp)"

  awk -v key="$key" -v value="$value" '
    BEGIN { updated = 0 }
    index($0, key "=") == 1 {
      print key "=" value
      updated = 1
      next
    }
    { print }
    END {
      if (!updated) {
        print key "=" value
      }
    }
  ' "$file" > "$tmp_file"

  run_as_root install -m 0644 "$tmp_file" "$file"
  rm -f "$tmp_file"
}

arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *)
      echo "不支持的架构: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

force=0
reset_password=0
component=""
version="latest"
server_url=""
node_id=""
cert_b64=""
_next_is_server=0
_next_is_node_id=0
_next_is_cert=0
for _arg in "$@"; do
  if [ "$_next_is_server" = "1" ]; then
    server_url="$_arg"
    _next_is_server=0
    continue
  fi
  if [ "$_next_is_node_id" = "1" ]; then
    node_id="$_arg"
    _next_is_node_id=0
    continue
  fi
  if [ "$_next_is_cert" = "1" ]; then
    cert_b64="$_arg"
    _next_is_cert=0
    continue
  fi
  case "$_arg" in
    --force|-f) force=1 ;;
    --reset-password) reset_password=1 ;;
    -h|--help) usage; exit 0 ;;
    --server) _next_is_server=1 ;;
    --node-id) _next_is_node_id=1 ;;
    --cert) _next_is_cert=1 ;;
    server|node)
      component="$_arg"
      ;;
    *)
      if [ -n "$component" ]; then
        version="$_arg"
      else
        echo "未知参数: $_arg" >&2
        usage
        exit 1
      fi
      ;;
  esac
done

if [ -z "$component" ]; then
  usage
  exit 0
fi

need_cmd curl
need_cmd tar
need_cmd install

repo="0xUnixIO/pulse"

os="linux"
cpu="$(arch)"
asset="pulse-${component}-${os}-${cpu}.tar.gz"

# Non-token fallback URL (public repos)
if [ "$version" = "latest" ]; then
  download_url="https://github.com/${repo}/releases/latest/download/${asset}"
else
  download_url="https://github.com/${repo}/releases/download/${version}/${asset}"
fi

bin_dir="${PULSE_INSTALL_BIN:-/usr/local/bin}"
etc_dir="${PULSE_INSTALL_ETC:-/etc/pulse}"
share_dir="${PULSE_INSTALL_SHARE:-/usr/local/share/pulse}"
lib_dir="${PULSE_INSTALL_LIB:-/etc/systemd/system}"
initd_dir="${PULSE_INSTALL_INITD:-/etc/init.d}"
state_dir="${PULSE_STATE_DIR:-/var/lib/pulse}"

# 检测 init 系统
if command -v systemctl >/dev/null 2>&1 && systemctl --version >/dev/null 2>&1; then
  init_system="systemd"
elif command -v rc-service >/dev/null 2>&1; then
  init_system="openrc"
else
  init_system="unknown"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

echo "下载 ${asset} ..."
curl -fsSL "$download_url" -o "${tmp_dir}/${asset}"
tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"

package_dir="${tmp_dir}/pulse-${component}-${os}-${cpu}"
if [ ! -d "$package_dir" ]; then
  echo "安装包内容异常: ${package_dir} 不存在" >&2
  exit 1
fi

run_as_root mkdir -p "$bin_dir" "$etc_dir" "$state_dir"
if [ "$init_system" = "systemd" ]; then
  run_as_root mkdir -p "$lib_dir"
elif [ "$init_system" = "openrc" ]; then
  run_as_root mkdir -p "$initd_dir"
fi
run_as_root install -m 0755 "${package_dir}/bin/pulse-${component}" "${bin_dir}/pulse-${component}"

if [ "$component" = "server" ]; then
  env_target="${etc_dir}/pulse-server.env"
  is_new_install=0
  if [ ! -f "$env_target" ]; then
    is_new_install=1
    run_as_root install -m 0644 "${package_dir}/etc/pulse/pulse-server.env.example" "$env_target"
  fi
  if [ "$is_new_install" = "1" ]; then
    if [ "${PULSE_ADMIN_PASSWORD+x}" != "x" ]; then
      PULSE_ADMIN_PASSWORD="$(random_password)"
    fi
    if [ "${PULSE_SERVER_ADDR+x}" != "x" ]; then
      PULSE_SERVER_ADDR=":$(random_port)"
    fi
  elif [ "$reset_password" = "1" ]; then
    PULSE_ADMIN_PASSWORD="$(random_password)"
  fi
  if [ "${PULSE_ADMIN_USERNAME+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_ADMIN_USERNAME" "$PULSE_ADMIN_USERNAME"
  fi
  if [ "${PULSE_ADMIN_PASSWORD+x}" = "x" ] || [ "$reset_password" = "1" ]; then
    set_env_file_value "$env_target" "PULSE_ADMIN_PASSWORD" "$PULSE_ADMIN_PASSWORD"
  fi
  if [ "${PULSE_SERVER_ADDR+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_SERVER_ADDR" "$PULSE_SERVER_ADDR"
  fi
  if [ "${PULSE_SERVER_NODE_CLIENT_CERT_FILE+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_SERVER_NODE_CLIENT_CERT_FILE" "$PULSE_SERVER_NODE_CLIENT_CERT_FILE"
  fi
  if [ "${PULSE_SERVER_NODE_CLIENT_KEY_FILE+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_SERVER_NODE_CLIENT_KEY_FILE" "$PULSE_SERVER_NODE_CLIENT_KEY_FILE"
  fi
  if [ "${PULSE_DISCOURSE_URL+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_DISCOURSE_URL" "$PULSE_DISCOURSE_URL"
  fi
  if [ "${PULSE_DISCOURSE_SSO_SECRET+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_DISCOURSE_SSO_SECRET" "$PULSE_DISCOURSE_SSO_SECRET"
  fi
  if [ "${PULSE_DISCOURSE_ADMIN_USERS+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_DISCOURSE_ADMIN_USERS" "$PULSE_DISCOURSE_ADMIN_USERS"
  fi
  if [ "${PULSE_DATABASE_URL+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_DATABASE_URL" "$PULSE_DATABASE_URL"
  fi
  if [ "${PULSE_DATA_DIR+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_DATA_DIR" "$PULSE_DATA_DIR"
  fi
  if [ "${PULSE_STRIPE_SECRET_KEY+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_STRIPE_SECRET_KEY" "$PULSE_STRIPE_SECRET_KEY"
  fi
  if [ "${PULSE_STRIPE_WEBHOOK_SECRET+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_STRIPE_WEBHOOK_SECRET" "$PULSE_STRIPE_WEBHOOK_SECRET"
  fi
  if [ "$init_system" = "systemd" ]; then
    run_as_root install -m 0644 "${package_dir}/lib/systemd/system/pulse-server.service" "${lib_dir}/pulse-server.service"
    run_as_root systemctl daemon-reload
    run_as_root systemctl enable pulse-server
    run_as_root systemctl restart pulse-server
  elif [ "$init_system" = "openrc" ]; then
    run_as_root install -m 0755 "${package_dir}/etc/init.d/pulse-server" "${initd_dir}/pulse-server"
    run_as_root rc-update add pulse-server default
    run_as_root rc-service pulse-server restart
  else
    echo "未检测到 systemd 或 OpenRC，请手动启动: ${bin_dir}/pulse-server"
  fi
else
  env_target="${etc_dir}/pulse-node.env"
  if [ ! -f "$env_target" ]; then
    run_as_root install -m 0644 "${package_dir}/etc/pulse/pulse-node.env.example" "$env_target"
  fi
  cert_target="${etc_dir}/server_client_cert.pem"
  if [ -n "$cert_b64" ]; then
    # --cert 参数直接写入证书（来自控制面板生成的安装命令）
    run_as_root mkdir -p "$(dirname "$cert_target")"
    printf "%s" "$cert_b64" | base64 -d | run_as_root tee "$cert_target" > /dev/null
  else
    prompt_node_client_cert_pem "$cert_target" "$force"
  fi
  # 无论新装还是更新，都确保 env 中记录证书路径
  set_env_file_value "$env_target" "PULSE_NODE_TLS_CLIENT_CERT_FILE" "$cert_target"
  if [ "${PULSE_NODE_ADDR+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_NODE_ADDR" "$PULSE_NODE_ADDR"
  fi
  if [ "${PULSE_NODE_PORT+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_NODE_PORT" "$PULSE_NODE_PORT"
  fi
  if [ "${PULSE_NODE_TLS_CERT_FILE+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_NODE_TLS_CERT_FILE" "$PULSE_NODE_TLS_CERT_FILE"
  fi
  if [ "${PULSE_NODE_TLS_KEY_FILE+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_NODE_TLS_KEY_FILE" "$PULSE_NODE_TLS_KEY_FILE"
  fi
  if [ "$init_system" = "systemd" ]; then
    run_as_root install -m 0644 "${package_dir}/lib/systemd/system/pulse-node.service" "${lib_dir}/pulse-node.service"
    run_as_root systemctl daemon-reload
    run_as_root systemctl enable pulse-node
    run_as_root systemctl restart pulse-node
  elif [ "$init_system" = "openrc" ]; then
    run_as_root install -m 0755 "${package_dir}/etc/init.d/pulse-node" "${initd_dir}/pulse-node"
    run_as_root rc-update add pulse-node default
    run_as_root rc-service pulse-node restart
  else
    echo "未检测到 systemd 或 OpenRC，请手动启动: ${bin_dir}/pulse-node"
  fi
fi

_installed_version="$("${bin_dir}/pulse-${component}" --version 2>/dev/null || echo "$version")"
echo ""
echo "安装完成: pulse-${component} ${_installed_version}"
echo "配置文件: ${env_target}"
echo "工作目录: ${state_dir}"
if [ "$component" = "server" ]; then
  # 从 env 文件读取实际值
  _addr="$(grep '^PULSE_SERVER_ADDR=' "$env_target" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
  _username="$(grep '^PULSE_ADMIN_USERNAME=' "$env_target" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
  _password="$(grep '^PULSE_ADMIN_PASSWORD=' "$env_target" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  _port="${_addr#:}"
  _ip="$(ip -4 addr show scope global 2>/dev/null | awk '/inet/{gsub(/\/.*/, "", $2); print $2; exit}' \
        || hostname -I 2>/dev/null | awk '{print $1}' \
        || echo "<your-ip>")"
  echo "  面板地址: http://${_ip}:${_port}"
  echo "  管理员:   ${_username:-admin}"
  echo "  密码:     ${_password:-(见 ${env_target})}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
else
  _port="$(grep '^PULSE_NODE_PORT=' "$env_target" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
  if [ -z "$_port" ]; then
    _addr="$(grep '^PULSE_NODE_ADDR=' "$env_target" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
    _addr="${_addr:-:8081}"
    _port="${_addr#:}"
  fi
  _ip="$(ip -4 addr show scope global 2>/dev/null | awk '/inet/{gsub(/\/.*/, "", $2); print $2; exit}' \
        || hostname -I 2>/dev/null | awk '{print $1}' \
        || echo "<your-ip>")"
  _node_base_url="https://${_ip}:${_port}"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  节点地址: ${_node_base_url}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  # 若由控制面板生成的命令提供了 server_url 和 node_id，则自动注册节点地址
  if [ -n "$server_url" ] && [ -n "$node_id" ]; then
    echo ""
    echo "正在向控制面板注册节点地址..."
    _register_url="${server_url%/}/v1/node-register"
    _register_body="{\"node_id\":\"${node_id}\",\"base_url\":\"${_node_base_url}\"}"
    if curl -fsSL -X POST "$_register_url" \
        -H "Content-Type: application/json" \
        -d "$_register_body" > /dev/null 2>&1; then
      echo "注册成功，节点已与控制面板连接。"
    else
      echo "注册请求失败，请在控制面板手动填写节点地址: ${_node_base_url}" >&2
    fi
  fi
fi
