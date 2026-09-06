package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

// Phase 9.1 配置存储 repo 层(契约 settings-foundation.md §3.4)。
// Upsert = Get 判存在 → Update / Insert(sqlc SQLite 语法不支持 upsert RETURNING)。
// queue_work INTEGER↔bool:domain bool,mapper 转 1/0(§六 风险)。

// GetAppSetting 读全局配置行。无行 → sql.ErrNoRows(service 层回落 DefaultAppSetting)。
func (s *Store) GetAppSetting(ctx context.Context) (settings.AppSetting, error) {
	row, err := s.q.GetAppSetting(ctx, settings.AppSettingID)
	if err != nil {
		return settings.AppSetting{}, err
	}
	return toAppSetting(row), nil
}

// UpsertAppSetting 写全局配置行(有更新 / 无插入)。字段零值即覆写该列(调用方给全量行)。
func (s *Store) UpsertAppSetting(ctx context.Context, a settings.AppSetting) error {
	if _, err := s.q.GetAppSetting(ctx, settings.AppSettingID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err := s.q.InsertAppSetting(ctx, appSettingInsertParams(a))
		return err
	}
	_, err := s.q.UpdateAppSetting(ctx, appSettingUpdateParams(a))
	return err
}

// GetCompanySetting 读公司覆盖行。无行 → sql.ErrNoRows(= 全继承 global)。
func (s *Store) GetCompanySetting(ctx context.Context, companyID string) (settings.CompanySetting, error) {
	row, err := s.q.GetCompanySetting(ctx, companyID)
	if err != nil {
		return settings.CompanySetting{}, err
	}
	return toCompanySetting(row), nil
}

// UpsertCompanySetting 写公司覆盖行(有更新 / 无插入)。nil 指针字段写 NULL(= 该维度继承 global)。
func (s *Store) UpsertCompanySetting(ctx context.Context, cs settings.CompanySetting) error {
	if _, err := s.q.GetCompanySetting(ctx, cs.CompanyID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err := s.q.InsertCompanySetting(ctx, companySettingInsertParams(cs))
		return err
	}
	_, err := s.q.UpdateCompanySetting(ctx, companySettingUpdateParams(cs))
	return err
}

// DeleteCompanySetting 删公司覆盖行 → 整行回退继承 global。
func (s *Store) DeleteCompanySetting(ctx context.Context, companyID string) error {
	return s.q.DeleteCompanySetting(ctx, companyID)
}

// UpsertSecret 写公司机密密文(有更新 / 无插入)。Cipher 必须已 SealSecret(明文不出 repo)。
func (s *Store) UpsertSecret(ctx context.Context, sec settings.Secret) error {
	if _, err := s.q.GetSecret(ctx, query.GetSecretParams{CompanyID: sec.CompanyID, ID: sec.ID}); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err := s.q.InsertSecret(ctx, query.InsertSecretParams{
			CompanyID: sec.CompanyID, ID: sec.ID, Cipher: sec.Cipher, UpdatedAt: now(),
		})
		return err
	}
	_, err := s.q.UpdateSecret(ctx, query.UpdateSecretParams{
		Cipher: sec.Cipher, UpdatedAt: now(), CompanyID: sec.CompanyID, ID: sec.ID,
	})
	return err
}

// GetSecret 读单条公司机密。无 → sql.ErrNoRows。
func (s *Store) GetSecret(ctx context.Context, companyID, id string) (settings.Secret, error) {
	row, err := s.q.GetSecret(ctx, query.GetSecretParams{CompanyID: companyID, ID: id})
	if err != nil {
		return settings.Secret{}, err
	}
	return toSecret(row), nil
}

// ListSecrets 列公司全部机密(按 id 升序)。
func (s *Store) ListSecrets(ctx context.Context, companyID string) ([]settings.Secret, error) {
	rows, err := s.q.ListSecrets(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]settings.Secret, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSecret(r))
	}
	return out, nil
}

// DeleteSecret 删公司单条机密。
func (s *Store) DeleteSecret(ctx context.Context, companyID, id string) error {
	return s.q.DeleteSecret(ctx, query.DeleteSecretParams{CompanyID: companyID, ID: id})
}

// ---- mappers ----

func appSettingInsertParams(a settings.AppSetting) query.InsertAppSettingParams {
	return query.InsertAppSettingParams{
		ID: a.ID, EngineModeDefault: a.EngineModeDefault, AgentCliDefault: a.AgentCLIDefault,
		ConsoleTokenHash: a.ConsoleTokenHash, DigestTime: a.DigestTime, HttpPort: int64(a.HTTPPort),
		PollMin: int64(a.PollMin), QueueWork: boolInt(a.QueueWork), QueueIntervalSec: int64(a.QueueIntervalSec),
		UpdatedAt: a.UpdatedAt,
	}
}

func appSettingUpdateParams(a settings.AppSetting) query.UpdateAppSettingParams {
	return query.UpdateAppSettingParams{
		EngineModeDefault: a.EngineModeDefault, AgentCliDefault: a.AgentCLIDefault,
		ConsoleTokenHash: a.ConsoleTokenHash, DigestTime: a.DigestTime, HttpPort: int64(a.HTTPPort),
		PollMin: int64(a.PollMin), QueueWork: boolInt(a.QueueWork), QueueIntervalSec: int64(a.QueueIntervalSec),
		UpdatedAt: a.UpdatedAt, ID: a.ID,
	}
}

func companySettingInsertParams(cs settings.CompanySetting) query.InsertCompanySettingParams {
	return query.InsertCompanySettingParams{
		CompanyID: cs.CompanyID, EngineMode: ptrToNull(cs.EngineMode), AgentCli: ptrToNull(cs.AgentCLI),
		IssueSource: ptrToNull(cs.IssueSource), IssueFixturePath: ptrToNull(cs.IssueFixturePath),
		UpdatedAt: now(),
	}
}

func companySettingUpdateParams(cs settings.CompanySetting) query.UpdateCompanySettingParams {
	return query.UpdateCompanySettingParams{
		EngineMode: ptrToNull(cs.EngineMode), AgentCli: ptrToNull(cs.AgentCLI),
		IssueSource: ptrToNull(cs.IssueSource), IssueFixturePath: ptrToNull(cs.IssueFixturePath),
		UpdatedAt: now(), CompanyID: cs.CompanyID,
	}
}

func toAppSetting(r query.AppSetting) settings.AppSetting {
	return settings.AppSetting{
		ID: r.ID, EngineModeDefault: r.EngineModeDefault, AgentCLIDefault: r.AgentCliDefault,
		ConsoleTokenHash: r.ConsoleTokenHash, DigestTime: r.DigestTime, HTTPPort: int(r.HttpPort),
		PollMin: int(r.PollMin), QueueWork: r.QueueWork != 0, QueueIntervalSec: int(r.QueueIntervalSec),
		UpdatedAt: r.UpdatedAt,
	}
}

func toCompanySetting(r query.CompanySetting) settings.CompanySetting {
	return settings.CompanySetting{
		CompanyID: r.CompanyID, EngineMode: nullToPtr(r.EngineMode), AgentCLI: nullToPtr(r.AgentCli),
		IssueSource: nullToPtr(r.IssueSource), IssueFixturePath: nullToPtr(r.IssueFixturePath),
		UpdatedAt: r.UpdatedAt,
	}
}

func toSecret(r query.Secret) settings.Secret {
	return settings.Secret{CompanyID: r.CompanyID, ID: r.ID, Cipher: r.Cipher, UpdatedAt: r.UpdatedAt}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
