VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
TARGET_OS ?= $(shell go env GOOS)
TARGET_ARCH ?= $(shell go env GOARCH)
DIST_DIR ?= dist
SINGBOX_TAGS ?= with_clash_api,with_utls,with_v2ray_api
LDFLAGS = -s -w -X pulse/internal/buildinfo.Version=$(VERSION) -X pulse/internal/buildinfo.Commit=$(COMMIT) -X pulse/internal/buildinfo.BuildDate=$(BUILD_DATE)

# 版本管理
CURRENT_VERSION := $(shell git tag --sort=-v:refname | grep '^v' | head -1 | sed 's/^v//')
CURRENT_VERSION := $(if $(CURRENT_VERSION),$(CURRENT_VERSION),0.0.0)
_VER_PARTS    = $(subst ., ,$(CURRENT_VERSION))
MAJOR        := $(word 1,$(_VER_PARTS))
MINOR        := $(word 2,$(_VER_PARTS))
PATCH        := $(word 3,$(_VER_PARTS))
NEXT_PATCH   := $(MAJOR).$(MINOR).$(shell echo $$(($(PATCH)+1)))
NEXT_MINOR   := $(MAJOR).$(shell echo $$(($(MINOR)+1))).0
NEXT_MAJOR   := $(shell echo $$(($(MAJOR)+1))).0.0

.PHONY: build build-server build-node build-cli build-spa test loadtest sqlc proto package-server package-node checksums clean clean-dev dev stop release _do_release _build-server-dev

build: build-cli build-server build-node

build-cli:
	CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/pulse ./cmd/pulse

build-spa:
	cd web/panel && bun install --frozen-lockfile && bun run build.ts

# CI 中前端已预先构建并下载到 web/panel/dist/，设置 SKIP_SPA=1 跳过重复构建
build-server:
ifeq ($(SKIP_SPA),1)
	@echo "Skipping SPA build (SKIP_SPA=1)"
else
	$(MAKE) build-spa
endif
	@CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/pulse-server ./cmd/pulse-server

build-node:
	@CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -tags "$(SINGBOX_TAGS)" -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/pulse-node ./cmd/pulse-node

test:
	go test ./...

# nodehub gRPC 并发压测（1000 个 nodeagent 同进程 bufconn）。
# 默认不进 CI（build tag `loadtest` 隔离）；在本机跑：
#   make loadtest
loadtest:
	go test -tags loadtest -count=1 -timeout 5m -v ./internal/nodehub/loadtest/...

sqlc:
	sqlc generate
	sqlc vet

# 生成 gRPC / protobuf 代码到 internal/pb/nodev1/。
# 依赖以下工具，请先安装到 PATH：
#   brew install protobuf                                   # protoc
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
proto:
	protoc --go_out=. --go_opt=module=pulse \
	       --go-grpc_out=. --go-grpc_opt=module=pulse \
	       proto/node/v1/node.proto

package-server: build-server
	rm -rf $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)
	mkdir -p $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/bin
	mkdir -p $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/etc/pulse
	mkdir -p $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/lib/systemd/system
	mkdir -p $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/etc/init.d
	cp $(DIST_DIR)/pulse-server $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/bin/pulse-server
	cp deploy/env/pulse-server.env.example $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/etc/pulse/pulse-server.env.example
	cp deploy/systemd/pulse-server.service $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/lib/systemd/system/pulse-server.service
	cp deploy/openrc/pulse-server $(DIST_DIR)/package/pulse-server-$(TARGET_OS)-$(TARGET_ARCH)/etc/init.d/pulse-server
	mkdir -p $(DIST_DIR)/release
	tar -C $(DIST_DIR)/package -czf $(DIST_DIR)/release/pulse-server-$(TARGET_OS)-$(TARGET_ARCH).tar.gz pulse-server-$(TARGET_OS)-$(TARGET_ARCH)

package-node: build-node
	rm -rf $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)
	mkdir -p $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/bin
	mkdir -p $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/etc/pulse
	mkdir -p $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/lib/systemd/system
	mkdir -p $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/etc/init.d
	cp $(DIST_DIR)/pulse-node $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/bin/pulse-node
	cp deploy/env/pulse-node.env.example $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/etc/pulse/pulse-node.env.example
	cp deploy/systemd/pulse-node.service $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/lib/systemd/system/pulse-node.service
	cp deploy/openrc/pulse-node $(DIST_DIR)/package/pulse-node-$(TARGET_OS)-$(TARGET_ARCH)/etc/init.d/pulse-node
	mkdir -p $(DIST_DIR)/release
	tar -C $(DIST_DIR)/package -czf $(DIST_DIR)/release/pulse-node-$(TARGET_OS)-$(TARGET_ARCH).tar.gz pulse-node-$(TARGET_OS)-$(TARGET_ARCH)

checksums:
	cd $(DIST_DIR)/release && shasum -a 256 *.tar.gz > checksums.txt

release:
	@printf "\n\033[1;34m  ◈ Pulse Release\033[0m\n"
	@printf "  \033[2m────────────────────────────\033[0m\n"
	@printf "  current   \033[33mv$(CURRENT_VERSION)\033[0m\n\n"
	@printf "  \033[36m1)\033[0m patch   \033[2m→\033[0m  \033[32mv$(NEXT_PATCH)\033[0m\n"
	@printf "  \033[36m2)\033[0m minor   \033[2m→\033[0m  \033[32mv$(NEXT_MINOR)\033[0m\n"
	@printf "  \033[36m3)\033[0m major   \033[2m→\033[0m  \033[32mv$(NEXT_MAJOR)\033[0m\n"
	@printf "  \033[2m────────────────────────────\033[0m\n\n"
	@read -p "  select [1/2/3]: " choice; \
	case $$choice in \
	  1) $(MAKE) _do_release V=$(NEXT_PATCH) ;; \
	  2) $(MAKE) _do_release V=$(NEXT_MINOR) ;; \
	  3) $(MAKE) _do_release V=$(NEXT_MAJOR) ;; \
	  *) printf "\n  \033[31m✗\033[0m 已取消\n\n"; exit 1 ;; \
	esac

_do_release:
	@printf "\n  \033[2m·\033[0m 运行测试...\n"
	@go test ./... || exit 1
	@printf "  \033[2m·\033[0m 推送 main...\n"
	@git push origin main
	@git tag v$(V)
	@printf "  \033[2m·\033[0m 推送 tag v$(V)...\n"
	@git push origin v$(V)
	@printf "\n  \033[1;32m✓\033[0m 已发布 \033[1mv$(V)\033[0m，CI 构建中\n\n"

stop:
	@-pkill -f 'dist/pulse-server' 2>/dev/null || true
	@-pkill -f 'dist/pulse-node' 2>/dev/null || true
	@-pkill -f 'bun run dev.ts' 2>/dev/null || true
	@echo "Dev processes stopped."

clean:
	rm -rf $(DIST_DIR)

clean-dev:
	rm -rf dev-data


_build-server-dev:
	@CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/pulse-server ./cmd/pulse-server

dev: _build-server-dev build-node
	@git config core.hooksPath .githooks
	@pkill -f 'dist/pulse-server' 2>/dev/null; pkill -f 'dist/pulse-node' 2>/dev/null; pkill -f 'bun run dev.ts' 2>/dev/null; sleep 0.3; true
	@mkdir -p dev-data/server dev-data/node
	@echo "→ server  http://localhost:8080  (admin/admin123)"
	@echo "→ grpc    :8082  (node hub, mTLS)"
	@echo "→ panel   http://localhost:3000  (React SPA)"
	@echo "→ node    pulse-node 不再监听端口；请通过控制面 enroll 流程注册节点"
	@echo "         (示例：./dist/pulse-node enroll --server=http://localhost:8080 \\)"
	@echo "                  --node-id=<ID> --token=<TOKEN> --insecure --out=./dev-data/node)"
	@( trap 'kill $$(jobs -p) 2>/dev/null; wait' INT TERM; \
	   PULSE_SERVER_ADDR=:8080 \
	   PULSE_ADMIN_USERNAME=admin \
	   PULSE_ADMIN_PASSWORD=admin123 \
	   PULSE_DATABASE_URL=postgresql://user:password@localhost:5432/pulse?sslmode=disable \
	   PULSE_NODE_CA_CERT_FILE=./dev-data/server/node_ca_cert.pem \
	   PULSE_NODE_CA_KEY_FILE=./dev-data/server/node_ca_key.pem \
	   PULSE_NODE_GRPC_URL=https://localhost:8082 \
	   PULSE_NODE_GRPC_ADDR=:8082 \
	   ./dist/pulse-server & \
	   sleep 1 && \
	   cd web/panel && bun run dev.ts & \
	   wait )
