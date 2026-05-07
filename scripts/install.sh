#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
用法:
  install.sh server|node [version] [选项]

选项（node 专用）:
  --server <URL>      控制面板地址（由控制面板生成时自动填入）
  --node-id <ID>      节点 ID（由控制面板生成时自动填入）
  --token <TOKEN>     一次性 enroll token（由控制面板生成时自动填入）
  --token-file <PATH> 从文件读取 token，'-' 表示 stdin（与 --token 二选一）
  --insecure          enroll 时跳过控制面 TLS 校验（默认开启，首次 enroll 必需；
                      TODO: 待实现 --server-fingerprint 后改为默认关闭）
  --cert <BASE64>     [已废弃] 旧的 server 客户端证书 base64，仅向后兼容（会被忽略）

环境变量:
  PULSE_INSTALL_BIN     二进制安装目录，默认 /usr/local/bin
  PULSE_INSTALL_ETC     配置安装目录，默认 /etc/pulse
  PULSE_INSTALL_LIB     systemd 安装目录，默认 /etc/systemd/system
  PULSE_INSTALL_INITD   OpenRC 安装目录，默认 /etc/init.d
  PULSE_STATE_DIR       工作目录，默认 /var/lib/pulse
  PULSE_SERVER_ADDR     server 监听地址，不指定则随机端口（格式 :端口）
  PULSE_DATABASE_URL    PostgreSQL 连接串（postgres://user:pass@host:5432/db?sslmode=disable）
  PULSE_DATA_DIR        工作目录（geoip、上传文件等），默认 /var/lib/pulse
  PULSE_INSTALL_DRY_RUN 设为 1 时跳过下载、特权操作和 enroll 调用（仅用于本地 sanity check）

示例:
  # 安装控制面板（server）
  curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server

  # 重置密码
  curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | \
    sh -s -- server --reset-password

  # 安装节点（推荐：从控制面板"添加节点"页面复制生成的命令）
  bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) node \
    --server https://<控制面板地址> \
    --node-id <节点ID> \
    --token <ENROLL_TOKEN>

  # 从 stdin 传 token（避免命令行参数泄露到 shell history）
  echo "$ENROLL_TOKEN" | bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) node \
    --server https://<控制面板地址> --node-id <节点ID> --token-file -

EOF
}

tty_available() {
  [ -r /dev/tty ] && [ -w /dev/tty ]
}

prompt_node_client_cert_pem() {
  # Deprecated stub: pre-pasted client cert flow has been replaced by
  # `pulse-node enroll`. Kept only to avoid breaking older copies of the
  # script that may still source/call this function.
  echo "[deprecated] prompt_node_client_cert_pem 已废弃，新流程请使用 'pulse-node enroll'。" >&2
  return 0
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

pg_is_ready() {
  command -v pg_isready >/dev/null 2>&1 && pg_isready -q 2>/dev/null
}

pg_exec_sql_file() {
  _f="$1"
  chmod 0644 "$_f"
  if [ "$(id -u)" -eq 0 ]; then
    su postgres -s /bin/sh -c "psql -v ON_ERROR_STOP=0 -f '${_f}'" 2>/dev/null || true
  else
    sudo -u postgres psql -v ON_ERROR_STOP=0 -f "$_f" 2>/dev/null || true
  fi
}

install_postgres_native() {
  echo "正在安装 PostgreSQL ..."
  if command -v apt-get >/dev/null 2>&1; then
    run_as_root apt-get install -y postgresql
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y postgresql-server postgresql
    run_as_root postgresql-setup --initdb 2>/dev/null || \
      run_as_root postgresql-setup initdb 2>/dev/null || true
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache postgresql postgresql-client
    if id -u postgres >/dev/null 2>&1 && [ ! -f /var/lib/postgresql/data/PG_VERSION ]; then
      run_as_root install -d -m 0700 -o postgres -g postgres /var/lib/postgresql/data
      su postgres -s /bin/sh -c "initdb -D /var/lib/postgresql/data" 2>/dev/null || true
    fi
  else
    echo "不支持的包管理器，请手动安装 PostgreSQL 后重新运行安装脚本" >&2
    exit 1
  fi
  run_as_root systemctl start postgresql 2>/dev/null || \
    run_as_root service postgresql start 2>/dev/null || \
    run_as_root rc-service postgresql start 2>/dev/null || true
  run_as_root systemctl enable postgresql 2>/dev/null || \
    run_as_root rc-update add postgresql default 2>/dev/null || true
  _wait=15
  while [ "$_wait" -gt 0 ]; do
    pg_is_ready && break
    _wait=$((_wait - 1))
    sleep 1
  done
}

install_postgres_docker() {
  _user="$1" _pass="$2" _db="$3"
  need_cmd docker
  echo "正在通过 Docker 启动 PostgreSQL ..."
  docker run -d --name pulse-postgres --restart always \
    -e POSTGRES_USER="$_user" \
    -e POSTGRES_PASSWORD="$_pass" \
    -e POSTGRES_DB="$_db" \
    -p 127.0.0.1:5432:5432 \
    postgres:16-alpine
  _wait=30
  while [ "$_wait" -gt 0 ]; do
    docker exec pulse-postgres pg_isready -U "$_user" -q 2>/dev/null && break
    _wait=$((_wait - 1))
    sleep 1
  done
}

create_postgres_db() {
  _user="$1" _pass="$2" _db="$3"
  echo "正在创建数据库 '${_db}' 和用户 '${_user}' ..."
  _sql_tmp="$(mktemp)"
  chmod 0644 "$_sql_tmp"
  printf "CREATE USER %s WITH PASSWORD '%s';\nCREATE DATABASE %s OWNER %s;\n" \
    "$_user" "$_pass" "$_db" "$_user" > "$_sql_tmp"
  pg_exec_sql_file "$_sql_tmp"
  rm -f "$_sql_tmp"
}

setup_database() {
  _env_file="$1"

  [ "${PULSE_INSTALL_DRY_RUN:-0}" = "1" ] && { echo "[dry-run] 跳过数据库配置" >&2; return 0; }

  # 已通过环境变量传入
  if [ "${PULSE_DATABASE_URL+x}" = "x" ]; then
    set_env_file_value "$_env_file" "PULSE_DATABASE_URL" "$PULSE_DATABASE_URL"
    return 0
  fi

  # 重装/升级场景：配置文件中已有连接串，跳过
  _existing="$(grep '^PULSE_DATABASE_URL=' "$_env_file" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
  [ -n "$_existing" ] && return 0

  echo ""
  _url=""
  if tty_available; then
    printf "PostgreSQL 连接串（留空自动安装）: " >/dev/tty
    read -r _url </dev/tty
  fi

  if [ -n "$_url" ]; then
    set_env_file_value "$_env_file" "PULSE_DATABASE_URL" "$_url"
    return 0
  fi

  # 选择安装方式
  _method="1"
  if tty_available; then
    printf "安装方式:\n  1) 原生安装（apt/yum/apk）\n  2) Docker\n选择 [1]: " >/dev/tty
    read -r _method </dev/tty
    _method="${_method:-1}"
  fi

  _db_user="pulse"
  _db_pass="$(random_password)"
  _db_name="pulse"

  if [ "$_method" = "2" ]; then
    install_postgres_docker "$_db_user" "$_db_pass" "$_db_name"
  else
    install_postgres_native
    create_postgres_db "$_db_user" "$_db_pass" "$_db_name"
  fi

  set_env_file_value "$_env_file" "PULSE_DATABASE_URL" \
    "postgres://${_db_user}:${_db_pass}@localhost:5432/${_db_name}?sslmode=disable"
  echo "数据库配置完成。"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "缺少命令: $1" >&2
    exit 1
  }
}

run_as_root() {
  if [ "${PULSE_INSTALL_DRY_RUN:-0}" = "1" ]; then
    echo "[dry-run] run_as_root: $*" >&2
    return 0
  fi
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
component=""
version="latest"
server_url=""
node_id=""
token=""
token_file=""
cert_b64=""
insecure=1
_next_is_server=0
_next_is_node_id=0
_next_is_cert=0
_next_is_token=0
_next_is_token_file=0
for _arg in "$@"; do
  # 支持 --key=value 形式
  case "$_arg" in
    --server=*)     server_url="${_arg#*=}";    continue ;;
    --node-id=*)    node_id="${_arg#*=}";       continue ;;
    --cert=*)       cert_b64="${_arg#*=}";      continue ;;
    --token=*)      token="${_arg#*=}";         continue ;;
    --token-file=*) token_file="${_arg#*=}";    continue ;;
  esac
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
  if [ "$_next_is_token" = "1" ]; then
    token="$_arg"
    _next_is_token=0
    continue
  fi
  if [ "$_next_is_token_file" = "1" ]; then
    token_file="$_arg"
    _next_is_token_file=0
    continue
  fi
  case "$_arg" in
    --force|-f) force=1 ;;
    -h|--help) usage; exit 0 ;;
    --server) _next_is_server=1 ;;
    --node-id) _next_is_node_id=1 ;;
    --cert) _next_is_cert=1 ;;
    --token) _next_is_token=1 ;;
    --token-file) _next_is_token_file=1 ;;
    --insecure) insecure=1 ;;
    --no-insecure) insecure=0 ;;
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

# Silence shellcheck SC2034: force is referenced indirectly when callers
# adopt new flow but legacy --force semantics remain reserved.
: "${force}"

if [ -n "$cert_b64" ]; then
  cat >&2 <<'EOF'
[deprecated] --cert 已废弃，新流程不再需要预粘贴 server 客户端证书。
  请改用控制面板"添加节点"生成的新命令（包含 --token <ENROLL_TOKEN>）。
  本次将忽略 --cert 的值，继续按新流程执行。
EOF
  cert_b64=""
fi

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
case "$cpu" in
  x86_64|amd64) cpu="amd64" ;;
  aarch64|arm64) cpu="arm64" ;;
  *) echo "不支持的 CPU 架构: $cpu" >&2; exit 1 ;;
esac
asset="pulse-${component}-${os}-${cpu}.tar.gz"

# Non-token fallback URL (public repos)
if [ "$version" = "latest" ]; then
  download_url="https://github.com/${repo}/releases/latest/download/${asset}"
else
  download_url="https://github.com/${repo}/releases/download/${version}/${asset}"
fi

# 镜像支持：设置 PULSE_DOWNLOAD_MIRROR 可在前缀拼接镜像
# 例如 PULSE_DOWNLOAD_MIRROR=https://ghfast.top/ 会拼成
# https://ghfast.top/https://github.com/...
if [ -n "${PULSE_DOWNLOAD_MIRROR:-}" ]; then
  mirror="${PULSE_DOWNLOAD_MIRROR%/}/"
  download_url="${mirror}${download_url}"
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

package_dir="${tmp_dir}/pulse-${component}-${os}-${cpu}"

if [ "${PULSE_INSTALL_DRY_RUN:-0}" = "1" ]; then
  echo "[dry-run] 跳过下载: ${download_url}"
  # Stage a fake package layout so downstream `install` calls have something to
  # reference (run_as_root is a no-op under dry-run, so missing payloads are
  # tolerated).
  mkdir -p "${package_dir}/bin" \
           "${package_dir}/etc/pulse" \
           "${package_dir}/lib/systemd/system" \
           "${package_dir}/etc/init.d"
  : > "${package_dir}/bin/pulse-${component}"
  : > "${package_dir}/etc/pulse/pulse-${component}.env.example"
  : > "${package_dir}/lib/systemd/system/pulse-${component}.service"
  : > "${package_dir}/etc/init.d/pulse-${component}"
else
  echo "下载 ${asset} ..."
  if ! curl -fsSL "$download_url" -o "${tmp_dir}/${asset}"; then
    echo "下载失败: ${download_url}" >&2
    echo "如网络无法直连 GitHub，可设置 PULSE_DOWNLOAD_MIRROR 使用镜像，例如：" >&2
    echo "  PULSE_DOWNLOAD_MIRROR=https://ghfast.top/ bash <(...) ..." >&2
    exit 1
  fi
  tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"

  if [ ! -d "$package_dir" ]; then
    echo "安装包内容异常: ${package_dir} 不存在" >&2
    exit 1
  fi
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
    if [ "${PULSE_SERVER_ADDR+x}" != "x" ]; then
      PULSE_SERVER_ADDR=":$(random_port)"
    fi
  fi
  if [ "${PULSE_SERVER_ADDR+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_SERVER_ADDR" "$PULSE_SERVER_ADDR"
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
  setup_database "$env_target"
  if [ "${PULSE_DATA_DIR+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_DATA_DIR" "$PULSE_DATA_DIR"
  fi
  if [ "${PULSE_STRIPE_SECRET_KEY+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_STRIPE_SECRET_KEY" "$PULSE_STRIPE_SECRET_KEY"
  fi
  if [ "${PULSE_STRIPE_WEBHOOK_SECRET+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_STRIPE_WEBHOOK_SECRET" "$PULSE_STRIPE_WEBHOOK_SECRET"
  fi
  # 新安装时自动填写 PULSE_NODE_GRPC_URL，避免节点 enroll 拿到 localhost:8082
  if [ "$is_new_install" = "1" ] && [ "${PULSE_NODE_GRPC_URL+x}" != "x" ]; then
    _grpc_addr="${PULSE_NODE_GRPC_ADDR:-:8082}"
    _grpc_port="${_grpc_addr#:}"
    _public_ip="$(ip -4 addr show scope global 2>/dev/null | awk '/inet/{gsub(/\/.*/, "", $2); print $2; exit}' \
                || hostname -I 2>/dev/null | awk '{print $1}')"
    if [ -n "$_public_ip" ]; then
      PULSE_NODE_GRPC_URL="https://${_public_ip}:${_grpc_port}"
    fi
  fi
  if [ "${PULSE_NODE_GRPC_URL+x}" = "x" ]; then
    set_env_file_value "$env_target" "PULSE_NODE_GRPC_URL" "$PULSE_NODE_GRPC_URL"
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

  # 校验新流程必需参数（--server/--node-id/--token 由控制面板生成的命令带入；
  # 也允许通过 --token-file 从文件或 stdin 读取 token）
  if [ -z "$server_url" ]; then
    echo "缺少 --server <URL>，请从控制面板'添加节点'页面复制完整命令" >&2
    exit 1
  fi
  if [ -z "$node_id" ]; then
    echo "缺少 --node-id <ID>，请从控制面板'添加节点'页面复制完整命令" >&2
    exit 1
  fi

  if [ -n "$token" ] && [ -n "$token_file" ]; then
    echo "--token 与 --token-file 互斥" >&2
    exit 1
  fi
  if [ -z "$token" ] && [ -z "$token_file" ]; then
    echo "缺少 --token <ENROLL_TOKEN>（或 --token-file <PATH>）" >&2
    exit 1
  fi

  # 把 token 收敛到一个临时文件，便于以 --token-file 形式喂给 pulse-node enroll，
  # 避免 token 出现在 ps/审计日志里。
  token_tmp="$(mktemp)"
  chmod 0600 "$token_tmp"
  if [ -n "$token_file" ]; then
    if [ "$token_file" = "-" ]; then
      cat > "$token_tmp"
    else
      cat "$token_file" > "$token_tmp"
    fi
  else
    printf "%s" "$token" > "$token_tmp"
  fi
  if [ ! -s "$token_tmp" ]; then
    rm -f "$token_tmp"
    echo "token 内容为空" >&2
    exit 1
  fi

  # 旧的 server 客户端证书不再使用；若残留则重命名为 .deprecated 防止混淆
  legacy_cert="${etc_dir}/server_client_cert.pem"
  if [ -e "$legacy_cert" ]; then
    echo "[notice] 发现旧的 ${legacy_cert}，重命名为 ${legacy_cert}.deprecated" >&2
    run_as_root mv "$legacy_cert" "${legacy_cert}.deprecated"
  fi

  enroll_args="--server=$server_url --node-id=$node_id --token-file=$token_tmp --out=$etc_dir"
  if [ "$insecure" = "1" ]; then
    # TODO: 待 pulse-node enroll 支持 --server-fingerprint 后，把默认改为 fingerprint pinning
    enroll_args="$enroll_args --insecure"
  fi

  enroll_log="$(mktemp)"
  echo "执行 pulse-node enroll ..."
  if [ "${PULSE_INSTALL_DRY_RUN:-0}" = "1" ]; then
    echo "[dry-run] ${bin_dir}/pulse-node enroll $enroll_args" >&2
    # 模拟产物以便后续步骤继续
    : > "${tmp_dir}/node_cert.pem"
    : > "${tmp_dir}/node_key.pem"
    : > "${tmp_dir}/node_ca.pem"
    cert_target="${tmp_dir}/node_cert.pem"
    key_target="${tmp_dir}/node_key.pem"
    ca_target="${tmp_dir}/node_ca.pem"
    grpc_url=""
  else
    # shellcheck disable=SC2086 # 我们故意拆词以便把多个 flag 传给 enroll
    if ! run_as_root "${bin_dir}/pulse-node" enroll $enroll_args | tee "$enroll_log"; then
      rm -f "$token_tmp"
      echo "pulse-node enroll 失败，安装终止" >&2
      exit 1
    fi
    cert_target="${etc_dir}/node_cert.pem"
    key_target="${etc_dir}/node_key.pem"
    ca_target="${etc_dir}/node_ca.pem"
    if [ ! -s "$cert_target" ] || [ ! -s "$key_target" ] || [ ! -s "$ca_target" ]; then
      rm -f "$token_tmp" "$enroll_log"
      echo "enroll 完成但缺少证书文件 (${cert_target} / ${key_target} / ${ca_target})" >&2
      exit 1
    fi
    # enroll 输出形如 'GRPC server: https://host:port'
    grpc_url="$(awk -F': ' '/^GRPC server: /{print $2}' "$enroll_log" | tail -n1)"
  fi
  rm -f "$token_tmp" "$enroll_log"

  # 修正权限（enroll 自身已写 0644/0600/0644，这里再保险一遍）
  if [ "${PULSE_INSTALL_DRY_RUN:-0}" != "1" ]; then
    run_as_root chmod 0644 "$cert_target" "$ca_target"
    run_as_root chmod 0600 "$key_target"
    if id -u pulse >/dev/null 2>&1; then
      run_as_root chown pulse:pulse "$key_target" 2>/dev/null || true
    fi
  fi

  set_env_file_value "$env_target" "PULSE_NODE_ID" "$node_id"
  set_env_file_value "$env_target" "PULSE_NODE_CLIENT_CERT_FILE" "$cert_target"
  set_env_file_value "$env_target" "PULSE_NODE_CLIENT_KEY_FILE" "$key_target"
  set_env_file_value "$env_target" "PULSE_NODE_SERVER_CA_FILE" "$ca_target"
  if [ -n "$grpc_url" ]; then
    set_env_file_value "$env_target" "PULSE_NODE_GRPC_URL" "$grpc_url"
    # PULSE_NODE_SERVER_ADDR = host:port，从 grpc_url 解析
    grpc_hostport="${grpc_url#*://}"
    grpc_hostport="${grpc_hostport%%/*}"
    set_env_file_value "$env_target" "PULSE_NODE_SERVER_ADDR" "$grpc_hostport"
  fi

  # 移除已废弃的环境变量（HTTP listener / 旧 mTLS 流程残留）
  for legacy_var in PULSE_NODE_TLS_CERT_FILE PULSE_NODE_TLS_KEY_FILE \
                    PULSE_NODE_TLS_CLIENT_CERT_FILE PULSE_NODE_CA_FILE \
                    PULSE_NODE_ADDR PULSE_NODE_PORT; do
    if grep -q "^${legacy_var}=" "$env_target" 2>/dev/null; then
      tmp_env="$(mktemp)"
      grep -v "^${legacy_var}=" "$env_target" > "$tmp_env" || true
      run_as_root install -m 0644 "$tmp_env" "$env_target"
      rm -f "$tmp_env"
    fi
  done

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
  _addr="$(grep '^PULSE_SERVER_ADDR=' "$env_target" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
  _port="${_addr#:}"
  _ip="$(ip -4 addr show scope global 2>/dev/null | awk '/inet/{gsub(/\/.*/, "", $2); print $2; exit}' \
        || hostname -I 2>/dev/null | awk '{print $1}' \
        || echo "<your-ip>")"
  _grpc_url="$(grep '^PULSE_NODE_GRPC_URL=' "$env_target" 2>/dev/null | cut -d= -f2- | tr -d "'" | tr -d '"')"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  面板地址: http://${_ip}:${_port}"
  if [ -n "$_grpc_url" ]; then
    echo "  节点 gRPC: ${_grpc_url}"
  fi
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
else
  _ip="$(ip -4 addr show scope global 2>/dev/null | awk '/inet/{gsub(/\/.*/, "", $2); print $2; exit}' \
        || hostname -I 2>/dev/null | awk '{print $1}' \
        || echo "<your-ip>")"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  节点 ID:   ${node_id}"
  echo "  节点出口:  ${_ip}"
  echo "  控制面 gRPC: ${grpc_url:-(见 ${env_target} 中的 PULSE_NODE_GRPC_URL)}"
  echo ""
  echo "  pulse-node 不再监听 HTTP 端口，所有指令通过 gRPC 长连接由控制面下发。"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi
