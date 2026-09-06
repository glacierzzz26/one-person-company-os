package server

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// Phase 9.2 — 控制台访问治理 /setup(契约 console-access.md §3.4):主密钥 Web 首启生成-显示一次-落盘
// 0600(方向 D1)+ 存量 enc:v1 端点 token 一次性 re-key(D2)+ console 令牌哈希化鉴权。

// handleSetupStatus GET /api/v1/setup/status(免 bearer,见 Handler 挂载):SPA 启动判定是否进 /setup 向导。
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	init, err := s.Initialized(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	apiOK(w, map[string]any{"initialized": init})
}

type setupReq struct {
	ConsoleToken   string `json:"console_token"`    // 必填(≥8 字符);只存 sha256 哈希
	OldEndpointKey string `json:"old_endpoint_key"` // 可选:存量 enc:v1 端点旧密钥(OS_ENDPOINT_KEY),首启一次性 re-key
}

// handleSetup POST /api/v1/setup:完成首启初始化。免 bearer(未初始化阶段无令牌可用)。
// 流程(任一步失败 → 错误响应且停留在未初始化态;已初始化 → 409,绝不覆盖既有密钥/令牌):
//
//	生成主密钥 → 落盘 <db>.key(0600)→ 可选旧 key re-key(两遍,错 key 零写回)→ 进程内 UseMasterKey → 设 console 令牌哈希。
//
// 成功返回 {initialized:true, master_key},master_key 仅此一次(前端即显示,之后无通道可取回)。
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	init, err := s.Initialized(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if init {
		apiErr(w, http.StatusConflict, "already_initialized", "setup already completed")
		return
	}

	var req setupReq
	if !decodeJSON(w, r, &req) {
		return
	}
	token := strings.TrimSpace(req.ConsoleToken)
	if token == "" || len(token) < 8 {
		apiErr(w, http.StatusBadRequest, "bad_request", "console_token is required (at least 8 characters)")
		return
	}
	// 落盘路径必须先于任何写(re-key 也是写)确认,防「re-key 已提交但主密钥无人持有」。
	if s.masterKeyPath == "" {
		apiErr(w, http.StatusInternalServerError, "internal", "server not configured with a master key path")
		return
	}
	// 旧 key 可选:给了须 32 字节 hex(64 hex chars);不给 = D2「dev/scratch 跳过」。
	var oldKey []byte
	if old := strings.TrimSpace(req.OldEndpointKey); old != "" {
		oldKey, err = hex.DecodeString(old)
		if err != nil || len(oldKey) != 32 {
			apiErr(w, http.StatusBadRequest, "bad_request", "old_endpoint_key must be 32 bytes hex (64 hex characters)")
			return
		}
	}

	master := settings.GenerateKey()
	newKey, _ := hex.DecodeString(master)

	if oldKey != nil {
		if _, err := s.svc.RekeyEndpointTokens(r.Context(), oldKey, newKey); err != nil {
			apiErr(w, http.StatusBadRequest, "rekey_failed", "re-keying endpoints with old_endpoint_key failed: "+err.Error())
			return
		}
	}
	if err := settings.SaveKey(s.masterKeyPath, master); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", "persist master key: "+err.Error())
		return
	}
	endpoint.UseMasterKey(newKey) // 进程内生效,免重启
	if err := s.svc.SetConsoleTokenAs(r.Context(), token, consoleActor); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	apiOK(w, map[string]any{"initialized": true, "master_key": master}) // master_key 仅此一次
}

// handleRotateConsoleToken PUT /api/v1/settings/console-token(Bearer 鉴权后;未初始化 → 409):
// 换新令牌哈希,旧令牌即失效。自守卫初始化态(未初始化无令牌可轮换,须先 /setup)。
func (s *Server) handleRotateConsoleToken(w http.ResponseWriter, r *http.Request) {
	init, err := s.Initialized(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if !init {
		apiErr(w, http.StatusConflict, "not_initialized", "console token not set yet: run /setup first")
		return
	}
	var req struct {
		NewToken string `json:"new_token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	token := strings.TrimSpace(req.NewToken)
	if token == "" || len(token) < 8 {
		apiErr(w, http.StatusBadRequest, "bad_request", "new_token is required (at least 8 characters)")
		return
	}
	if err := s.svc.SetConsoleTokenAs(r.Context(), token, consoleActor); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	apiOK(w, map[string]any{"rotated": true})
}
