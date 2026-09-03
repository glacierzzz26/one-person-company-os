// Package console 内嵌 Web 运营控制台(Phase 7.3,React+TS+AntD SPA)。
//
// 单二进制定位:构建时 `make ui` 把 web/dist 拷入本包 ui/ 目录,go:embed 进二进制,
// 由 os server 同源托管(Spa 与 /api/v1 JSON 同 origin,无 CORS)。全新检出时 ui/
// 只含已提交的占位 index.html(提示先 make ui),保证 go build ./... 恒绿。
//
// 路由语义:
//   - 已存在文件(assets)→ http.FileServer 原样返回(正确 Content-Type);
//   - 目录或不存在且无扩展名(客户端路由)→ 回 index.html(history fallback);
//   - /healthz 与 /api/* → 404(归 server 其余路由;兜底防吞 API 路径);
//   - 带扩展名缺失资源 → 404;非 GET/HEAD → 405。
package console

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed ui
var uiFS embed.FS

// Handler 返回内嵌 SPA 的 http.Handler(挂 os server 的 /* 兜底路由)。
func Handler() http.Handler {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		panic(err) // ui/ 恒含已提交 index.html,fs.Sub 不会失败
	}
	fsrv := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "healthz" || strings.HasPrefix(p, "api/") {
			http.NotFound(w, r)
			return
		}
		if f, err := sub.Open(p); err == nil {
			st, serr := f.Stat()
			f.Close()
			if serr == nil && !st.IsDir() {
				fsrv.ServeHTTP(w, r)
				return
			}
			// 目录:非根目录一律不当目录列表服务,落入下方 fallback/404。
		}
		// 不存在且无扩展名(或目录)= 客户端路由 → 回 index.html。
		if filepath.Ext(p) == "" {
			f, err := sub.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method == http.MethodGet {
				_, _ = io.Copy(w, f)
			}
			return
		}
		http.NotFound(w, r)
	})
}
