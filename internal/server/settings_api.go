package server

import (
	"encoding/hex"
	"errors"
	"log"
	"net/http"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// Phase 9.3 — 全量设置 API(契约 runtime-knobs-web.md §3.6):global 生效行 GET/PUT、换主密钥
// rotate-master-key、公司覆盖行 GET/PUT/DELETE、公司机密白名单 GET/PUT/DELETE。
// 全部写操作走 service 层 *_As(带 actor=human:console 审计);校验失败 = ErrSettingsBadRequest → 400,
// 主密钥未注入/存储/DB 错误 = 500。console_token_hash 从不回给前端,PUT 合并保留(红线,E1 guard)。
// 机密明文只进 seal,list 只出掩码 {id,set,updated_at}。

// ---- helpers ----

// settingsAPIErr 把 service 错误映射 HTTP:ErrSettingsBadRequest(客户端输入非法)→ 400 bad_request;
// 其余交 handleServiceErr(404/500)。
func settingsAPIErr(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrSettingsBadRequest) {
		apiErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	handleServiceErr(w, err)
}

// globalSettingsDTO 全局生效行回包(含 console_token_set 布尔掩码;绝不含 console_token_hash)。
type globalSettingsDTO struct {
	EngineModeDefault string `json:"engine_mode_default"`
	AgentCLIDefault   string `json:"agent_cli_default"`
	DigestTime        string `json:"digest_time"`
	HTTPPort          int    `json:"http_port"`
	PollMin           int    `json:"poll_min"`
	QueueWork         bool   `json:"queue_work"`
	QueueIntervalSec  int    `json:"queue_interval_sec"`
	SchedulePollSec   int    `json:"schedule_poll_sec"` // 10.2:流水线到点触发轮询间隔(秒;0=关)——回包必须回显,Web 设置行依赖
	ConsoleTokenSet   bool   `json:"console_token_set"`
	UpdatedAt         int64  `json:"updated_at"`
}

func toGlobalSettingsDTO(a settings.AppSetting) globalSettingsDTO {
	return globalSettingsDTO{
		EngineModeDefault: a.EngineModeDefault,
		AgentCLIDefault:   a.AgentCLIDefault,
		DigestTime:        a.DigestTime,
		HTTPPort:          a.HTTPPort,
		PollMin:           a.PollMin,
		QueueWork:         a.QueueWork,
		QueueIntervalSec:  a.QueueIntervalSec,
		SchedulePollSec:   a.SchedulePollSec,
		ConsoleTokenSet:   a.ConsoleTokenHash != "",
		UpdatedAt:         a.UpdatedAt,
	}
}

// ---- GET/PUT /api/v1/settings ----

func (s *Server) handleGetGlobalSettings(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc.AppSetting(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, toGlobalSettingsDTO(a))
}

// handleUpdateGlobalSettings PUT /api/v1/settings:部分更新(AppSettingPatch 指针字段,缺省字段不改)。
// engine_mode_default 只收 "live";空 patch → 400。更新后重读并回全局生效行。
func (s *Server) handleUpdateGlobalSettings(w http.ResponseWriter, r *http.Request) {
	var patch settings.AppSettingPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	a, err := s.svc.UpdateGlobalSettingsAs(r.Context(), patch, consoleActor)
	if err != nil {
		settingsAPIErr(w, err)
		return
	}
	apiOK(w, toGlobalSettingsDTO(a))
}

// ---- POST /api/v1/settings/rotate-master-key(换主密钥;新 master_key 仅此一次返回)----

func (s *Server) handleRotateMasterKey(w http.ResponseWriter, r *http.Request) {
	init, err := s.Initialized(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	if !init {
		apiErr(w, http.StatusBadRequest, "not_initialized", "setup not completed: run /setup first")
		return
	}
	// masterKeyPath 必须先于任何写确认(rotate 是 DB+文件两处写),防 re-key 已提交但无法落盘。
	if s.masterKeyPath == "" {
		apiErr(w, http.StatusInternalServerError, "internal", "server not configured with a master key path")
		return
	}
	oldKey, ok := settings.MasterKey()
	if !ok {
		apiErr(w, http.StatusBadRequest, "master_key_not_loaded", "process has no master key loaded (start os server with the existing <db>.key)")
		return
	}
	newHex := settings.GenerateKey()
	newKey, _ := hex.DecodeString(newHex)
	// 双表 re-key(端点 token + 公司机密):先全量校验,任一失败零写 → 这里只可能因错 key/坏行失败。
	epN, secN, err := s.svc.RekeyAll(r.Context(), oldKey, newKey)
	if err != nil {
		apiErr(w, http.StatusBadRequest, "rekey_failed", "re-key failed (no writes applied): "+err.Error())
		return
	}
	if err := settings.SaveKey(s.masterKeyPath, newHex); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", "persist new master key: "+err.Error())
		return
	}
	// 进程内 holder 换新(免重启;旧 key 立即失效)。
	endpoint.UseMasterKey(newKey)
	settings.UseMasterKey(newKey)
	// 审计失败不阻断返回新 key(DB 已换新;再报错会让 Web 拿不到仅此一次的主密钥)。
	if err := s.svc.AuditRotateMasterKey(r.Context(), consoleActor, epN, secN); err != nil {
		log.Printf("rotate master key: audit failed (re-key already applied): %v", err)
	}
	apiOK(w, map[string]any{"master_key": newHex, "endpoints_rekeyed": epN, "secrets_rekeyed": secN})
}

// ---- GET/PUT/DELETE /api/v1/companies/{id}/settings(company 覆盖行)----

func (s *Server) handleGetCompanySettings(w http.ResponseWriter, r *http.Request) {
	companyID := pathParam(r, "id")
	cs, ok, err := s.svc.CompanySetting(r.Context(), companyID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	if !ok {
		cs = settings.CompanySetting{CompanyID: companyID} // 无覆盖行 = 全 null(整行继承 global)
	}
	apiOK(w, cs)
}

func (s *Server) handleUpdateCompanySettings(w http.ResponseWriter, r *http.Request) {
	var patch settings.CompanySettingPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	cs, err := s.svc.UpdateCompanySettingsAs(r.Context(), pathParam(r, "id"), patch, consoleActor)
	if err != nil {
		settingsAPIErr(w, err)
		return
	}
	apiOK(w, cs)
}

func (s *Server) handleResetCompanySettings(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.ResetCompanySettingsAs(r.Context(), pathParam(r, "id"), consoleActor); err != nil {
		settingsAPIErr(w, err)
		return
	}
	apiOK(w, map[string]any{"reset": true})
}

// ---- GET/PUT/DELETE /api/v1/companies/{id}/secrets(白名单机密,掩码回包)----

func (s *Server) handleListCompanySecrets(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.SecretMeta(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) handleSetCompanySecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	companyID, secretID := pathParam(r, "id"), pathParam(r, "secretID")
	if err := s.svc.SetCompanySecretAs(r.Context(), companyID, secretID, req.Value, consoleActor); err != nil {
		settingsAPIErr(w, err)
		return
	}
	apiOK(w, map[string]any{"id": secretID, "set": true})
}

func (s *Server) handleDeleteCompanySecret(w http.ResponseWriter, r *http.Request) {
	companyID, secretID := pathParam(r, "id"), pathParam(r, "secretID")
	if err := s.svc.DeleteCompanySecretAs(r.Context(), companyID, secretID, consoleActor); err != nil {
		settingsAPIErr(w, err)
		return
	}
	apiOK(w, map[string]any{"id": secretID, "deleted": true})
}
