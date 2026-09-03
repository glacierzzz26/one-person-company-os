# One-Person Company OS — 阶段化构建入口(Phase 7.3 起含 Web 控制台嵌入)
#
# 单二进制定位:make build = 先 make ui(web/ → internal/console/ui),再 go build -o bin/os。
# 全新检出无 node 环境时 go build ./... 仍绿(内嵌目录只含已提交占位 index.html)。
UI_SRC := web
UI_DST := internal/console/ui

.PHONY: ui build dev test clean

# ui:构建 React 控制台并拷入 go:embed 目录。产物(assets/* + 真实 index.html)不入库;
#     提交前须还原占位:make clean 或 git checkout -- internal/console/ui/index.html。
ui:
	cd $(UI_SRC) && npm ci && npm run build
	rm -rf $(UI_DST)/assets
	cp -r $(UI_SRC)/dist/. $(UI_DST)/

# build:单二进制(内嵌控制台)+ CLI 全命令。产物 bin/os。
build: ui
	go build -o bin/os ./cmd/os

# dev:后端 :8787 + 前端 Vite(代理 /api、/healthz → :8787)双进程。
dev:
	@echo "终端1 — 后端:go run ./cmd/os --db ./os.db server --port 8787 [--queue-work --queue-interval 5s --digest off]"
	@echo "终端2 — 前端:cd $(UI_SRC) && npm run dev(开 http://127.0.0.1:5173)"

# test:Go 全量测试(含 console 嵌入 httptest)。
test:
	go test ./...

# clean:清二进制与内嵌构建产物,并把内嵌目录还原为占位 index.html。
clean:
	rm -rf bin/os
	rm -rf $(UI_DST)/assets
	git checkout -- $(UI_DST)/index.html
