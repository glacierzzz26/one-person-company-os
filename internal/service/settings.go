package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
