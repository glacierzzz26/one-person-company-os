package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// Phase 9.1 配置存储 service 层(契约 settings-foundation.md §3.5,纯地基:不接运行时消费点/不接 Web,
// 只把「读得到的设置」与「生效解析」立起来;运行时 9.3 统一切 DB)。

// AppSetting 读全局设置。无行 → 内置默认(DefaultAppSetting,UpdatedAt=0 表未落库)。
func (s *Service) AppSetting(ctx context.Context) (settings.AppSetting, error) {
	a, err := s.store.GetAppSetting(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.DefaultAppSetting(), nil
	}
	return a, err
}

// UpsertAppSetting 写全局设置(整行覆盖,调用方给全量值)。
func (s *Service) UpsertAppSetting(ctx context.Context, a settings.AppSetting) error {
	a.ID = settings.AppSettingID
	a.UpdatedAt = time.Now().Unix()
	return s.store.UpsertAppSetting(ctx, a)
}

// CompanySetting 读公司覆盖行。ok=false = 无行(整行继承 global)。
func (s *Service) CompanySetting(ctx context.Context, companyID string) (settings.CompanySetting, bool, error) {
	cs, err := s.store.GetCompanySetting(ctx, companyID)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.CompanySetting{}, false, nil
	}
	if err != nil {
		return settings.CompanySetting{}, false, err
	}
	return cs, true, nil
}

// UpsertCompanySetting 写公司覆盖行(nil 指针字段 = 该维度继承 global)。
func (s *Service) UpsertCompanySetting(ctx context.Context, cs settings.CompanySetting) error {
	cs.UpdatedAt = time.Now().Unix()
	return s.store.UpsertCompanySetting(ctx, cs)
}

// DeleteCompanySetting 删公司覆盖行 → 整行回退继承 global。
func (s *Service) DeleteCompanySetting(ctx context.Context, companyID string) error {
	return s.store.DeleteCompanySetting(ctx, companyID)
}

// SetSecret 存公司机密:SealSecret(key, plain) 得密文 → UpsertSecret(明文不出 service)。
func (s *Service) SetSecret(ctx context.Context, companyID, id, plain string, key []byte) error {
	if companyID == "" || id == "" {
		return fmt.Errorf("settings: companyID and secret id are required")
	}
	cipher, err := settings.SealSecret(key, plain)
	if err != nil {
		return err
	}
	return s.store.UpsertSecret(ctx, settings.Secret{CompanyID: companyID, ID: id, Cipher: cipher})
}

// OpenSecret 读公司机密明文。ok=false = 无该 secret。
func (s *Service) OpenSecret(ctx context.Context, companyID, id string, key []byte) (string, bool, error) {
	sec, err := s.store.GetSecret(ctx, companyID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plain, err := settings.OpenSecret(key, sec.Cipher)
	if err != nil {
		return "", false, err
	}
	return plain, true, nil
}

// EngineModeFor 公司生效 engine 模式(company 覆盖 → global 默认 → 内置 "live")。
// 判读消费点(engine.go 等)在 9.3 切到本函数;9.1 只立解析,不改调用。
func (s *Service) EngineModeFor(ctx context.Context, companyID string) (string, error) {
	if cs, ok, err := s.CompanySetting(ctx, companyID); err != nil {
		return "", err
	} else if ok && cs.EngineMode != nil && *cs.EngineMode != "" {
		return *cs.EngineMode, nil
	}
	app, err := s.AppSetting(ctx)
	if err != nil {
		return "", err
	}
	if app.EngineModeDefault == "" {
		return settings.DefaultEngineMode, nil
	}
	return app.EngineModeDefault, nil
}

// RekeyEndpointTokens 全量端点 token re-key(enc:v1 → 新 key 的 enc:v1)。
// 两遍:第一遍 ListCompanies→ListEndpoints 解密校验并收集明文(空 token_enc 端点跳过);
// 全量成功后才第二遍 SealTokenWith(newKey)+SetEndpointToken 写回;任何一处失败 → error 且零写回。
// 返回 re-key 端点数。
func (s *Service) RekeyEndpointTokens(ctx context.Context, oldKey, newKey []byte) (int, error) {
	type target struct{ id, plain string }
	var targets []target

	companies, err := s.store.ListCompanies(ctx)
	if err != nil {
		return 0, err
	}
	for _, c := range companies {
		eps, err := s.store.ListEndpoints(ctx, c.ID)
		if err != nil {
			return 0, err
		}
		for _, e := range eps {
			if e.TokenEnc == "" {
				continue // 无鉴权端点,无需 re-key
			}
			plain, err := endpoint.OpenTokenWith(oldKey, e.TokenEnc)
			if err != nil {
				return 0, fmt.Errorf("rekey %s: %w", e.ID, err)
			}
			targets = append(targets, target{id: e.ID, plain: plain})
		}
	}

	for _, t := range targets {
		cipher, err := endpoint.SealTokenWith(newKey, t.plain)
		if err != nil {
			return 0, fmt.Errorf("rekey seal %s: %w", t.id, err)
		}
		if _, err := s.store.SetEndpointToken(ctx, t.id, cipher); err != nil {
			return 0, err
		}
	}
	return len(targets), nil
}

// ConsoleTokenHash 读全局 console 令牌哈希(”=未初始化,/setup 开放)。契约 console-access.md §3.3。
func (s *Service) ConsoleTokenHash(ctx context.Context) (string, error) {
	a, err := s.AppSetting(ctx)
	if err != nil {
		return "", err
	}
	return a.ConsoleTokenHash, nil
}

// SetConsoleTokenAs 设全局 console 令牌(DB 只存 sha256 hex 哈希)。空 plain 拒绝;
// 读现 global(Upsert 前保留其余字段)后落哈希;写审计。actor 语义与 AddEndpointAs 等一致
// (server 传 consoleActor;CLI/测试传 "human:cli")。
func (s *Service) SetConsoleTokenAs(ctx context.Context, plain, actor string) error {
	if plain == "" {
		return fmt.Errorf("settings: console token must not be empty")
	}
	a, err := s.AppSetting(ctx)
	if err != nil {
		return err
	}
	a.ConsoleTokenHash = settings.HashConsoleToken(plain)
	if err := s.UpsertAppSetting(ctx, a); err != nil {
		return err
	}
	_, err = s.audit(ctx, "settings", "global", "set-console-token", actor, "")
	return err
}

// SetConsoleToken 便捷写控制台令牌(actor="human:cli"),供测试与 CLI 用。
func (s *Service) SetConsoleToken(ctx context.Context, plain string) error {
	return s.SetConsoleTokenAs(ctx, plain, "human:cli")
}

// ---- Phase 9.3:机密白名单 / Current 族(主密钥 holder 运行期读写)+ re-key / 设置写审计(契约 §3.2/§3.6) ----

// ErrSettingsBadRequest 标记 settings 写接口的客户端输入错误(校验失败)。settings API handler
// errors.Is → HTTP 400;未包装的内部/存储/主密钥未注入错误走 500。9.3 新增,无存量断言依赖文案。
var ErrSettingsBadRequest = errors.New("settings: bad request")

// settingsBadRequestf 构造带 ErrSettingsBadRequest 的校验错误(客户端输入非法,非内部故障)。
// 消息 = sentinel 文案 + 原文;调用方错误字面量写法不变,只换构造器。
func settingsBadRequestf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSettingsBadRequest, fmt.Sprintf(format, args...))
}

// 公司机密 id 白名单(Web/API 只收这些;审计只记 id+掩码,值明文不入 audit)。
const (
	SecretGitHubToken   = "github_token"   // 通道 B issue 源(GitHub REST,issueSourceFor 消费)
	SecretFeishuWebhook = "feishu_webhook" // 通知/摘要 sink(notifyCompany/digest fan-out)
	SecretFeishuSecret  = "feishu_secret"  // 飞书自定义机器人加签(可选)
)

// KnownSecretIDs 白名单(确定性顺序,SecretMeta/校验复用)。
var KnownSecretIDs = []string{SecretGitHubToken, SecretFeishuWebhook, SecretFeishuSecret}

// Phase 10.5 项目机密(契约 github-roundtrip-pr.md §四 B)。每项目一个 GitHub token —— 写路径
// (git push + 开 PR)唯一凭据;读路径项目 token → 公司回退(githubTokenFor, runtime.go)。
const (
	ProjectSecretGitHubToken = "github_token"
)

// KnownProjectSecretIDs 项目机密白名单(确定性顺序;暂仅 github_token)。
var KnownProjectSecretIDs = []string{ProjectSecretGitHubToken}

// SetSecretCurrent 以当前注入主密钥存公司机密(运行期写入走 holder)。主密钥未注入 → 明确报错。
func (s *Service) SetSecretCurrent(ctx context.Context, companyID, id, plain string) error {
	key, ok := settings.MasterKey()
	if !ok {
		return fmt.Errorf("settings: master key not loaded (cannot seal secret; run /setup or start os with a .key file)")
	}
	return s.SetSecret(ctx, companyID, id, plain, key)
}

// OpenSecretCurrent 以当前注入主密钥读公司机密明文。ok=false = 无该 secret。主密钥未注入 → 报错。
func (s *Service) OpenSecretCurrent(ctx context.Context, companyID, id string) (string, bool, error) {
	key, ok := settings.MasterKey()
	if !ok {
		return "", false, fmt.Errorf("settings: master key not loaded (cannot open secret; run /setup or start os with a .key file)")
	}
	return s.OpenSecret(ctx, companyID, id, key)
}

// DeleteSecret 删公司机密行(幂等:不存在也返回 nil)。
func (s *Service) DeleteSecret(ctx context.Context, companyID, id string) error {
	if companyID == "" || id == "" {
		return fmt.Errorf("settings: companyID and secret id are required")
	}
	return s.store.DeleteSecret(ctx, companyID, id)
}

// SecretMeta 机密元数据(API 只出掩码:{id,set,updated_at},明文永不出)。白名单顺序 + 存量残留行排后。
type SecretMeta struct {
	ID        string `json:"id"`
	Set       bool   `json:"set"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

func (s *Service) SecretMeta(ctx context.Context, companyID string) ([]SecretMeta, error) {
	secs, err := s.store.ListSecrets(ctx, companyID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]settings.Secret, len(secs))
	for _, sec := range secs {
		byID[sec.ID] = sec
	}
	out := make([]SecretMeta, 0, len(secs))
	for _, id := range KnownSecretIDs {
		if sec, ok := byID[id]; ok {
			out = append(out, SecretMeta{ID: id, Set: true, UpdatedAt: sec.UpdatedAt})
		} else {
			out = append(out, SecretMeta{ID: id})
		}
		delete(byID, id)
	}
	extras := make([]SecretMeta, 0, len(byID))
	for _, sec := range byID { // 白名单外残留(手工/历史),列出元数据供处置
		extras = append(extras, SecretMeta{ID: sec.ID, Set: true, UpdatedAt: sec.UpdatedAt})
	}
	sort.Slice(extras, func(i, j int) bool { return extras[i].ID < extras[j].ID })
	return append(out, extras...), nil
}

// RekeySecrets 全量公司机密 re-key(enc:v2 → 新 key 的 enc:v2)。两遍同 RekeyEndpointTokens:
// 第一遍 ListCompanies→ListSecrets 解密校验收集明文(空 cipher 跳过);全量成功后才第二遍写回。
// 返回 re-key 机密数。
func (s *Service) RekeySecrets(ctx context.Context, oldKey, newKey []byte) (int, error) {
	type target struct{ companyID, id, plain string }
	var targets []target
	companies, err := s.store.ListCompanies(ctx)
	if err != nil {
		return 0, err
	}
	for _, c := range companies {
		secs, err := s.store.ListSecrets(ctx, c.ID)
		if err != nil {
			return 0, err
		}
		for _, sec := range secs {
			if sec.Cipher == "" {
				continue
			}
			plain, err := settings.OpenSecret(oldKey, sec.Cipher)
			if err != nil {
				return 0, fmt.Errorf("rekey secret %s/%s: %w", short8(c.ID), sec.ID, err)
			}
			targets = append(targets, target{companyID: c.ID, id: sec.ID, plain: plain})
		}
	}
	for _, t := range targets {
		cipher, err := settings.SealSecret(newKey, t.plain)
		if err != nil {
			return 0, fmt.Errorf("rekey seal secret %s/%s: %w", short8(t.companyID), t.id, err)
		}
		if err := s.store.UpsertSecret(ctx, settings.Secret{CompanyID: t.companyID, ID: t.id, Cipher: cipher}); err != nil {
			return 0, err
		}
	}
	return len(targets), nil
}

// RekeyAll 端点 token + 公司机密双表 re-key(换主密钥 rotate-master-key 用)。比单表 re-key 更稳:
// 第一遍把两张表全部解密校验(oldKey 错 / 任一行不可解 → error 且零写),校验全过后才写:
// 先端点后机密。返回各自 re-key 数。
func (s *Service) RekeyAll(ctx context.Context, oldKey, newKey []byte) (epN, secN int, err error) {
	type epTarget struct{ id, plain string }
	type secTarget struct{ companyID, id, plain string }
	var eps []epTarget
	var secs []secTarget

	companies, err := s.store.ListCompanies(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range companies {
		elist, err := s.store.ListEndpoints(ctx, c.ID)
		if err != nil {
			return 0, 0, err
		}
		for _, e := range elist {
			if e.TokenEnc == "" {
				continue
			}
			plain, err := endpoint.OpenTokenWith(oldKey, e.TokenEnc)
			if err != nil {
				return 0, 0, fmt.Errorf("rekey endpoint %s: %w", e.ID, err)
			}
			eps = append(eps, epTarget{id: e.ID, plain: plain})
		}
		slist, err := s.store.ListSecrets(ctx, c.ID)
		if err != nil {
			return 0, 0, err
		}
		for _, sec := range slist {
			if sec.Cipher == "" {
				continue
			}
			plain, err := settings.OpenSecret(oldKey, sec.Cipher)
			if err != nil {
				return 0, 0, fmt.Errorf("rekey secret %s/%s: %w", short8(c.ID), sec.ID, err)
			}
			secs = append(secs, secTarget{companyID: c.ID, id: sec.ID, plain: plain})
		}
	}
	for _, t := range eps {
		cipher, err := endpoint.SealTokenWith(newKey, t.plain)
		if err != nil {
			return 0, 0, err
		}
		if _, err := s.store.SetEndpointToken(ctx, t.id, cipher); err != nil {
			return 0, 0, err
		}
	}
	for _, t := range secs {
		cipher, err := settings.SealSecret(newKey, t.plain)
		if err != nil {
			return 0, 0, err
		}
		if err := s.store.UpsertSecret(ctx, settings.Secret{CompanyID: t.companyID, ID: t.id, Cipher: cipher}); err != nil {
			return 0, 0, err
		}
	}
	return len(eps), len(secs), nil
}

// UpdateGlobalSettingsAs 部分更新全局设置(contract §3.6)。patch nil 维度不改;engine_mode_default 只收
// "live"(scripted 拒收,Web 唯一写入口治理);agent_cli ∈ {claude,codex};digest_time ""/"HH:MM";范围防护。
// 合并后**保留现有 console_token_hash**(9.1 Upsert 为全列覆盖,漏 hash 会清空致鉴权回落——红线)。写审计。
func (s *Service) UpdateGlobalSettingsAs(ctx context.Context, p settings.AppSettingPatch, actor string) (settings.AppSetting, error) {
	if appSettingPatchEmpty(p) {
		return settings.AppSetting{}, settingsBadRequestf("settings: empty update (no fields provided)")
	}
	a, err := s.AppSetting(ctx)
	if err != nil {
		return settings.AppSetting{}, err
	}
	if p.EngineModeDefault != nil {
		if v := strings.TrimSpace(*p.EngineModeDefault); v != "" && v != "live" {
			return settings.AppSetting{}, settingsBadRequestf("engine_mode_default must be %q (Web only manages live; scripted is an offline CLI/env test seam)", "live")
		}
	}
	if p.AgentCLIDefault != nil {
		if v := strings.TrimSpace(*p.AgentCLIDefault); v != "" && v != "claude" && v != "codex" {
			return settings.AppSetting{}, settingsBadRequestf("agent_cli_default must be claude|codex (got %q)", v)
		}
	}
	if p.DigestTime != nil {
		if err := validateDigestTime(*p.DigestTime); err != nil {
			return settings.AppSetting{}, settingsBadRequestf("%s", err.Error())
		}
	}
	if p.HTTPPort != nil && (*p.HTTPPort < 1 || *p.HTTPPort > 65535) {
		return settings.AppSetting{}, settingsBadRequestf("http_port must be 1..65535 (got %d)", *p.HTTPPort)
	}
	if p.PollMin != nil && *p.PollMin < 1 {
		return settings.AppSetting{}, settingsBadRequestf("poll_min must be >= 1")
	}
	if p.QueueIntervalSec != nil && *p.QueueIntervalSec < 1 {
		return settings.AppSetting{}, settingsBadRequestf("queue_interval_sec must be >= 1")
	}
	if p.SchedulePollSec != nil && *p.SchedulePollSec < 0 {
		return settings.AppSetting{}, settingsBadRequestf("schedule_poll_sec must be >= 0 (0 = scheduling off)")
	}
	changed := applyAppSettingPatch(&a, p)
	if err := s.UpsertAppSetting(ctx, a); err != nil {
		return settings.AppSetting{}, err
	}
	_, err = s.audit(ctx, "settings", "global", "update", actor, "dims="+strings.Join(changed, ","))
	return a, err
}

// UpdateCompanySettingsAs 部分更新公司覆盖行(contract §3.6)。patch nil 维度不改;显式空串 = 清该维度
// (回退继承 global);engine_mode 只收 "live";issue_source ∈ {github,fixture};fixture 需已有/本次给
// issue_fixture_path;全维度清空且无既有行 → 空操作(等价整行重置)。写审计。
func (s *Service) UpdateCompanySettingsAs(ctx context.Context, companyID string, p settings.CompanySettingPatch, actor string) (settings.CompanySetting, error) {
	if companySettingPatchEmpty(p) {
		return settings.CompanySetting{}, settingsBadRequestf("settings: empty update (no fields provided)")
	}
	cs, ok, err := s.CompanySetting(ctx, companyID)
	if err != nil {
		return settings.CompanySetting{}, err
	}
	if !ok {
		cs = settings.CompanySetting{CompanyID: companyID}
	}
	if err := applyCompanySettingPatch(&cs, p); err != nil {
		return settings.CompanySetting{}, settingsBadRequestf("%s", err.Error())
	}
	// 校验 fixture 需路径(生效 source = 覆盖值,缺省 github 不需要路径)。
	if cs.IssueSource != nil && *cs.IssueSource == "fixture" &&
		(cs.IssueFixturePath == nil || strings.TrimSpace(*cs.IssueFixturePath) == "") {
		return settings.CompanySetting{}, settingsBadRequestf("issue_source=fixture requires issue_fixture_path")
	}
	// 覆盖行被清空(全维度 nil)→ 等价整行重置(继承),不落空行。
	if companySettingAllNil(cs) {
		if ok {
			if err := s.DeleteCompanySetting(ctx, companyID); err != nil {
				return settings.CompanySetting{}, err
			}
		}
		_, err := s.audit(ctx, "company_settings", companyID, "update", actor, "dims=reset(empty override)")
		return settings.CompanySetting{CompanyID: companyID}, err
	}
	if err := s.UpsertCompanySetting(ctx, cs); err != nil {
		return settings.CompanySetting{}, err
	}
	_, err = s.audit(ctx, "company_settings", companyID, "update", actor, "dims="+strings.Join(companySettingPresentDims(cs), ","))
	return cs, err
}

// ResetCompanySettingsAs 删除公司覆盖行 → 整行回退继承 global(幂等)。写审计。
func (s *Service) ResetCompanySettingsAs(ctx context.Context, companyID, actor string) error {
	if err := s.DeleteCompanySetting(ctx, companyID); err != nil {
		return err
	}
	_, err := s.audit(ctx, "company_settings", companyID, "reset", actor, "")
	return err
}

// AuditRotateMasterKey 记录换主密钥审计(entity=settings/global)。rotate 编排在 server 层
// (需主密钥文件路径 + 进程 holder 换新),service 只补审计 + re-key 计数 detail。
func (s *Service) AuditRotateMasterKey(ctx context.Context, actor string, epN, secN int) error {
	_, err := s.audit(ctx, "settings", "global", "rotate_master_key", actor, fmt.Sprintf("endpoints=%d secrets=%d", epN, secN))
	return err
}

// SetCompanySecretAs 设公司机密(白名单校验 + 主密钥 holder 落密文)。值明文只进 seal,不入 audit。
func (s *Service) SetCompanySecretAs(ctx context.Context, companyID, id, value, actor string) error {
	if err := validateSecretID(id); err != nil {
		return settingsBadRequestf("%s", err.Error())
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return settingsBadRequestf("secret %s: value must not be empty", id)
	}
	if err := s.SetSecretCurrent(ctx, companyID, id, value); err != nil {
		return err
	}
	_, err := s.audit(ctx, "secret", id, "set", actor, "company="+short8(companyID))
	return err
}

// DeleteCompanySecretAs 删公司机密(白名单校验;幂等)。写审计。
func (s *Service) DeleteCompanySecretAs(ctx context.Context, companyID, id, actor string) error {
	if err := validateSecretID(id); err != nil {
		return settingsBadRequestf("%s", err.Error())
	}
	if err := s.DeleteSecret(ctx, companyID, id); err != nil {
		return err
	}
	_, err := s.audit(ctx, "secret", id, "delete", actor, "company="+short8(companyID))
	return err
}

// ---- Phase 10.5 项目机密(github-roundtrip-pr.md §四 B;镜像公司 secret 的 Current/As 形状)----

// SetProjectSecretCurrent 以当前主密钥存项目机密。主密钥未注入 → 明确报错。
func (s *Service) SetProjectSecretCurrent(ctx context.Context, projectID, id, plain string) error {
	key, ok := settings.MasterKey()
	if !ok {
		return fmt.Errorf("settings: master key not loaded (cannot seal secret; run /setup or start os with a .key file)")
	}
	if projectID == "" || id == "" {
		return fmt.Errorf("settings: projectID and secret id are required")
	}
	cipher, err := settings.SealSecret(key, plain)
	if err != nil {
		return err
	}
	return s.store.UpsertProjectSecret(ctx, settings.ProjectSecret{ProjectID: projectID, ID: id, Cipher: cipher})
}

// OpenProjectSecretCurrent 以当前主密钥读项目机密明文。ok=false = 无该项目 secret。
func (s *Service) OpenProjectSecretCurrent(ctx context.Context, projectID, id string) (string, bool, error) {
	key, ok := settings.MasterKey()
	if !ok {
		return "", false, fmt.Errorf("settings: master key not loaded (cannot open secret; run /setup or start os with a .key file)")
	}
	sec, err := s.store.GetProjectSecret(ctx, projectID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plain, err := settings.OpenSecret(key, sec.Cipher)
	if err != nil {
		return "", false, err
	}
	return plain, true, nil
}

// ProjectSecretMeta 项目机密元数据(与 SecretMeta 同形状;白名单顺序 + 存量残留排后)。
func (s *Service) ProjectSecretMeta(ctx context.Context, projectID string) ([]SecretMeta, error) {
	secs, err := s.store.ListProjectSecrets(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]settings.ProjectSecret, len(secs))
	for _, sec := range secs {
		byID[sec.ID] = sec
	}
	out := make([]SecretMeta, 0, len(secs))
	for _, id := range KnownProjectSecretIDs {
		if sec, ok := byID[id]; ok {
			out = append(out, SecretMeta{ID: id, Set: true, UpdatedAt: sec.UpdatedAt})
		} else {
			out = append(out, SecretMeta{ID: id})
		}
		delete(byID, id)
	}
	extras := make([]SecretMeta, 0, len(byID))
	for _, sec := range byID {
		extras = append(extras, SecretMeta{ID: sec.ID, Set: true, UpdatedAt: sec.UpdatedAt})
	}
	sort.Slice(extras, func(i, j int) bool { return extras[i].ID < extras[j].ID })
	return append(out, extras...), nil
}

// DeleteProjectSecretAs 删项目机密(白名单校验;幂等)。写审计(entity=secret,detail 带 project=,与公司机密同筛)。
func (s *Service) DeleteProjectSecretAs(ctx context.Context, projectID, id, actor string) error {
	if err := validateProjectSecretID(id); err != nil {
		return settingsBadRequestf("%s", err.Error())
	}
	if err := s.store.DeleteProjectSecret(ctx, projectID, id); err != nil {
		return err
	}
	_, err := s.audit(ctx, "secret", id, "delete", actor, "project="+short8(projectID))
	return err
}

// SetProjectSecretAs 设项目机密(白名单校验 + 主密钥 holder 落密文)。值明文只进 seal,不入 audit。
func (s *Service) SetProjectSecretAs(ctx context.Context, projectID, id, value, actor string) error {
	if err := validateProjectSecretID(id); err != nil {
		return settingsBadRequestf("%s", err.Error())
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return settingsBadRequestf("secret %s: value must not be empty", id)
	}
	if err := s.SetProjectSecretCurrent(ctx, projectID, id, value); err != nil {
		return err
	}
	_, err := s.audit(ctx, "secret", id, "set", actor, "project="+short8(projectID))
	return err
}

// ---- 私有 helpers ----

func validateSecretID(id string) error {
	for _, k := range KnownSecretIDs {
		if id == k {
			return nil
		}
	}
	return fmt.Errorf("unknown secret id %q (known: %s)", id, strings.Join(KnownSecretIDs, ", "))
}

// validateProjectSecretID 校验项目机密 id(暂仅 github_token)。
func validateProjectSecretID(id string) error {
	for _, k := range KnownProjectSecretIDs {
		if id == k {
			return nil
		}
	}
	return fmt.Errorf("unknown project secret id %q (known: %s)", id, strings.Join(KnownProjectSecretIDs, ", "))
}

// validateDigestTime 校验摘要时刻:"" = 关闭(off),否则 "HH:MM"(00-23:00-59)。
func validateDigestTime(v string) error {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, "off") {
		return nil
	}
	if _, err := time.Parse("15:04", v); err != nil {
		return fmt.Errorf("digest_time must be %q (HH:MM) or empty to disable (got %q)", "HH:MM", v)
	}
	return nil
}

func appSettingPatchEmpty(p settings.AppSettingPatch) bool {
	return p.EngineModeDefault == nil && p.AgentCLIDefault == nil && p.DigestTime == nil &&
		p.HTTPPort == nil && p.PollMin == nil && p.QueueWork == nil && p.QueueIntervalSec == nil &&
		p.SchedulePollSec == nil
}

func companySettingPatchEmpty(p settings.CompanySettingPatch) bool {
	return p.EngineMode == nil && p.AgentCLI == nil && p.IssueSource == nil && p.IssueFixturePath == nil
}

func companySettingAllNil(cs settings.CompanySetting) bool {
	return cs.EngineMode == nil && cs.AgentCLI == nil && cs.IssueSource == nil && cs.IssueFixturePath == nil
}

func companySettingPresentDims(cs settings.CompanySetting) []string {
	var dims []string
	for _, d := range []struct {
		name string
		v    *string
	}{{"engine_mode", cs.EngineMode}, {"agent_cli", cs.AgentCLI}, {"issue_source", cs.IssueSource}, {"issue_fixture_path", cs.IssueFixturePath}} {
		if d.v != nil {
			dims = append(dims, d.name)
		}
	}
	return dims
}

func applyAppSettingPatch(a *settings.AppSetting, p settings.AppSettingPatch) []string {
	var changed []string
	if p.EngineModeDefault != nil {
		if v := strings.TrimSpace(*p.EngineModeDefault); v != "" {
			a.EngineModeDefault = v
			changed = append(changed, "engine_mode_default")
		}
	}
	if p.AgentCLIDefault != nil {
		if v := strings.TrimSpace(*p.AgentCLIDefault); v != "" {
			a.AgentCLIDefault = v
			changed = append(changed, "agent_cli_default")
		}
	}
	if p.DigestTime != nil {
		a.DigestTime = strings.TrimSpace(*p.DigestTime)
		if strings.EqualFold(a.DigestTime, "off") {
			a.DigestTime = ""
		}
		changed = append(changed, "digest_time")
	}
	if p.HTTPPort != nil {
		a.HTTPPort = *p.HTTPPort
		changed = append(changed, "http_port")
	}
	if p.PollMin != nil {
		a.PollMin = *p.PollMin
		changed = append(changed, "poll_min")
	}
	if p.QueueWork != nil {
		a.QueueWork = *p.QueueWork
		changed = append(changed, "queue_work")
	}
	if p.QueueIntervalSec != nil {
		a.QueueIntervalSec = *p.QueueIntervalSec
		changed = append(changed, "queue_interval_sec")
	}
	if p.SchedulePollSec != nil {
		a.SchedulePollSec = *p.SchedulePollSec
		changed = append(changed, "schedule_poll_sec")
	}
	return changed
}

func applyCompanySettingPatch(cs *settings.CompanySetting, p settings.CompanySettingPatch) error {
	if p.EngineMode != nil {
		v := strings.TrimSpace(*p.EngineMode)
		if v != "" && v != "live" {
			return settingsBadRequestf("engine_mode must be %q (Web only manages live; scripted is an offline CLI/env test seam)", "live")
		}
		if v == "" {
			cs.EngineMode = nil
		} else {
			cs.EngineMode = &v
		}
	}
	if p.AgentCLI != nil {
		v := strings.TrimSpace(*p.AgentCLI)
		if v != "" && v != "claude" && v != "codex" {
			return settingsBadRequestf("agent_cli must be claude|codex (got %q)", v)
		}
		if v == "" {
			cs.AgentCLI = nil
		} else {
			cs.AgentCLI = &v
		}
	}
	if p.IssueSource != nil {
		v := strings.ToLower(strings.TrimSpace(*p.IssueSource))
		if v != "" && v != "github" && v != "fixture" {
			return settingsBadRequestf("issue_source must be github|fixture (got %q)", v)
		}
		if v == "" {
			cs.IssueSource = nil
		} else {
			cs.IssueSource = &v
		}
	}
	if p.IssueFixturePath != nil {
		v := strings.TrimSpace(*p.IssueFixturePath)
		if v == "" {
			cs.IssueFixturePath = nil
		} else {
			cs.IssueFixturePath = &v
		}
	}
	return nil
}
