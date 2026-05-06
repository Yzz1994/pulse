package jobs

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"pulse/internal/backup"
)

// BackupSettingsStore 提供备份所需的配置读写能力。
type BackupSettingsStore interface {
	GetSetting(key string) (string, bool)
	SetSetting(key, value string) error
}

// BackupDB 使用 pgx COPY TO STDOUT 创建 PostgreSQL 数据库备份（zip 格式）并上传到 Cloudflare R2。
// 若未配置 R2 凭据，立即返回 nil。
func BackupDB(ctx context.Context, databaseURL string, settings BackupSettingsStore) error {
	cfg := backup.ConfigFromSettings(settings)
	if !cfg.Valid() {
		return nil
	}

	data, err := backup.CreatePgDump(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("创建备份: %w", err)
	}

	objectKey := fmt.Sprintf("pulse_backup_%s.zip", time.Now().UTC().Format("20060102_150405"))
	if err := backup.UploadToR2(ctx, cfg, objectKey, data); err != nil {
		return fmt.Errorf("上传到 R2: %w", err)
	}

	_ = settings.SetSetting("backup_last_at", time.Now().UTC().Format(time.RFC3339))

	keepStr, _ := settings.GetSetting("backup_keep_count")
	keepCount, _ := strconv.Atoi(keepStr)
	_ = backup.PruneOldBackups(ctx, cfg, keepCount)

	return nil
}
