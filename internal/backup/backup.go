package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Config R2 上传配置。
type Config struct {
	AccountID   string
	AccessKeyID string
	SecretKey   string
	BucketName  string
}

// Valid 报告配置是否完整。
func (c Config) Valid() bool {
	return c.AccountID != "" && c.AccessKeyID != "" && c.SecretKey != "" && c.BucketName != ""
}

// SettingsReader 从 KV 存储读取配置值。
type SettingsReader interface {
	GetSetting(key string) (string, bool)
}

// ConfigFromSettings 从 settings 存储构建 Config。
func ConfigFromSettings(s SettingsReader) Config {
	get := func(key string) string {
		v, _ := s.GetSetting(key)
		return v
	}
	return Config{
		AccountID:   get("backup_cf_account_id"),
		AccessKeyID: get("backup_cf_access_key_id"),
		SecretKey:   get("backup_cf_secret_key"),
		BucketName:  get("backup_cf_bucket_name"),
	}
}

// CreatePgDump 通过 pgx COPY TO STDOUT 将所有表数据导出为 zip 压缩包（每表一个 CSV）。
// 纯 Go 实现，不依赖外部命令。
func CreatePgDump(ctx context.Context, databaseURL string) ([]byte, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("连接数据库: %w", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`)
	if err != nil {
		return nil, fmt.Errorf("枚举表: %w", err)
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, fmt.Errorf("扫描表名: %w", err)
		}
		tables = append(tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("枚举表: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, table := range tables {
		fw, err := zw.Create(table + ".csv")
		if err != nil {
			return nil, fmt.Errorf("创建 zip 条目 %s: %w", table, err)
		}
		sql := fmt.Sprintf("COPY %s TO STDOUT (FORMAT CSV, HEADER)", pgx.Identifier{table}.Sanitize())
		if _, err := conn.PgConn().CopyTo(ctx, fw, sql); err != nil {
			return nil, fmt.Errorf("导出表 %s: %w", table, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("关闭 zip: %w", err)
	}
	return buf.Bytes(), nil
}

// RestoreFromDump 将 CreatePgDump 生成的 zip 备份还原到数据库。
// 整个还原在单一事务内执行：失败时自动回滚，避免数据库处于半还原状态。
// SET LOCAL session_replication_role = replica 跳过 FK 约束检查（事务结束后自动恢复）。
// 注意：还原前目标库必须已完成 schema 初始化（表结构由应用启动时 init() 创建）。
func RestoreFromDump(ctx context.Context, databaseURL string, data []byte) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("连接数据库: %w", err)
	}
	defer conn.Close(ctx)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("打开备份: %w", err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启还原事务: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// SET LOCAL 作用于事务级别，事务结束后自动恢复，无需 defer 手动重置
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role = replica"); err != nil {
		return fmt.Errorf("禁用 FK 检查: %w", err)
	}

	for _, f := range zr.File {
		table := strings.TrimSuffix(f.Name, ".csv")
		safe := pgx.Identifier{table}.Sanitize()

		if _, err := tx.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", safe)); err != nil {
			return fmt.Errorf("清空表 %s: %w", table, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("读取备份条目 %s: %w", f.Name, err)
		}
		copySql := fmt.Sprintf("COPY %s FROM STDIN (FORMAT CSV, HEADER)", safe)
		_, copyErr := tx.Conn().PgConn().CopyFrom(ctx, rc, copySql)
		rc.Close()
		if copyErr != nil {
			return fmt.Errorf("还原表 %s: %w", table, copyErr)
		}
	}
	return tx.Commit(ctx)
}

// UploadToR2 使用 S3 兼容 API 将数据上传到 Cloudflare R2。
func UploadToR2(ctx context.Context, cfg Config, objectKey string, data []byte) error {
	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", cfg.AccountID)
	rawURL := fmt.Sprintf("https://%s/%s/%s", host, cfg.BucketName, objectKey)

	t := time.Now().UTC()
	dateStamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")
	bodyHash := hexSHA256(data)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建请求: %w", err)
	}
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", bodyHash)
	req.Header.Set("Content-Type", "application/octet-stream")

	const region = "auto"
	const service = "s3"

	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf(
		"content-type:application/octet-stream\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		host, bodyHash, amzDate,
	)
	canonicalURI := "/" + cfg.BucketName + "/" + objectKey
	canonicalReq := strings.Join([]string{
		"PUT", canonicalURI, "",
		canonicalHeaders, signedHeaders, bodyHash,
	}, "\n")

	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, credScope,
		hexSHA256([]byte(canonicalReq)),
	}, "\n")

	signingKey := hmacSHA256Bytes(
		hmacSHA256Bytes(
			hmacSHA256Bytes(
				hmacSHA256Bytes([]byte("AWS4"+cfg.SecretKey), []byte(dateStamp)),
				[]byte(region),
			),
			[]byte(service),
		),
		[]byte("aws4_request"),
	)
	sig := hex.EncodeToString(hmacSHA256Bytes(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		cfg.AccessKeyID, credScope, signedHeaders, sig,
	))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("R2 返回错误 %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func hexSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256Bytes(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// BackupObject R2 中的备份对象信息。
type BackupObject struct {
	Key          string    `json:"key"`
	LastModified time.Time `json:"last_modified"`
	Size         int64     `json:"size"`
}

// listBucketResult S3 ListObjectsV2 XML 响应结构。
type listBucketResult struct {
	Contents []struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		Size         int64  `xml:"Size"`
	} `xml:"Contents"`
}

// signedGetRequest 创建带 AWS SigV4 签名的 GET 请求。
func signedGetRequest(ctx context.Context, cfg Config, rawURL string, queryParams url.Values) (*http.Request, error) {
	fullURL := rawURL
	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", cfg.AccountID)
	t := time.Now().UTC()
	dateStamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")
	emptyHash := hexSHA256([]byte{})

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", emptyHash)

	const region = "auto"
	const service = "s3"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, emptyHash, amzDate)

	// 提取路径（不含 host）
	parsed, _ := url.Parse(rawURL)
	canonicalURI := parsed.Path

	// 规范化查询字符串（键按字典序排列）
	canonicalQS := ""
	if len(queryParams) > 0 {
		canonicalQS = queryParams.Encode()
	}

	canonicalReq := strings.Join([]string{
		"GET", canonicalURI, canonicalQS,
		canonicalHeaders, signedHeaders, emptyHash,
	}, "\n")

	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, credScope,
		hexSHA256([]byte(canonicalReq)),
	}, "\n")

	signingKey := hmacSHA256Bytes(
		hmacSHA256Bytes(
			hmacSHA256Bytes(
				hmacSHA256Bytes([]byte("AWS4"+cfg.SecretKey), []byte(dateStamp)),
				[]byte(region),
			),
			[]byte(service),
		),
		[]byte("aws4_request"),
	)
	sig := hex.EncodeToString(hmacSHA256Bytes(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		cfg.AccessKeyID, credScope, signedHeaders, sig,
	))
	return req, nil
}

// ListBackups 列出 R2 中所有以 pulse_backup_ 开头的备份文件。
func ListBackups(ctx context.Context, cfg Config) ([]BackupObject, error) {
	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", cfg.AccountID)
	rawURL := fmt.Sprintf("https://%s/%s", host, cfg.BucketName)

	params := url.Values{}
	params.Set("list-type", "2")
	params.Set("prefix", "pulse_backup_")

	req, err := signedGetRequest(ctx, cfg, rawURL, params)
	if err != nil {
		return nil, fmt.Errorf("创建列表请求: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("列表请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("R2 列表错误 %d: %s", resp.StatusCode, string(body))
	}

	var result listBucketResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析列表响应: %w", err)
	}

	objects := make([]BackupObject, 0, len(result.Contents))
	for _, c := range result.Contents {
		t, _ := time.Parse(time.RFC3339, c.LastModified)
		objects = append(objects, BackupObject{Key: c.Key, LastModified: t, Size: c.Size})
	}
	return objects, nil
}

// DownloadFromR2 从 R2 下载指定对象内容。
func DownloadFromR2(ctx context.Context, cfg Config, objectKey string) ([]byte, error) {
	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", cfg.AccountID)
	rawURL := fmt.Sprintf("https://%s/%s/%s", host, cfg.BucketName, objectKey)

	req, err := signedGetRequest(ctx, cfg, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建下载请求: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("R2 下载错误 %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// DeleteFromR2 删除 R2 中的指定对象。
func DeleteFromR2(ctx context.Context, cfg Config, objectKey string) error {
	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", cfg.AccountID)
	rawURL := fmt.Sprintf("https://%s/%s/%s", host, cfg.BucketName, objectKey)

	t := time.Now().UTC()
	dateStamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")
	emptyHash := hexSHA256([]byte{})

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rawURL, nil)
	if err != nil {
		return fmt.Errorf("创建删除请求: %w", err)
	}
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", emptyHash)

	const region = "auto"
	const service = "s3"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, emptyHash, amzDate)

	parsed, _ := url.Parse(rawURL)
	canonicalReq := strings.Join([]string{
		"DELETE", parsed.Path, "",
		canonicalHeaders, signedHeaders, emptyHash,
	}, "\n")

	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, credScope,
		hexSHA256([]byte(canonicalReq)),
	}, "\n")

	signingKey := hmacSHA256Bytes(
		hmacSHA256Bytes(
			hmacSHA256Bytes(
				hmacSHA256Bytes([]byte("AWS4"+cfg.SecretKey), []byte(dateStamp)),
				[]byte(region),
			),
			[]byte(service),
		),
		[]byte("aws4_request"),
	)
	sig := hex.EncodeToString(hmacSHA256Bytes(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		cfg.AccessKeyID, credScope, signedHeaders, sig,
	))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("删除请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("R2 删除错误 %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// PruneOldBackups 保留最新的 keepCount 份备份，删除其余旧备份。
// keepCount <= 0 表示不限制。
func PruneOldBackups(ctx context.Context, cfg Config, keepCount int) error {
	if keepCount <= 0 {
		return nil
	}
	objects, err := ListBackups(ctx, cfg)
	if err != nil {
		return fmt.Errorf("列举备份: %w", err)
	}
	if len(objects) <= keepCount {
		return nil
	}
	// 显式按 LastModified 升序排列，最旧的在前，不依赖 R2 返回顺序或文件名格式
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].LastModified.Before(objects[j].LastModified)
	})
	toDelete := objects[:len(objects)-keepCount]
	for _, obj := range toDelete {
		if err := DeleteFromR2(ctx, cfg, obj.Key); err != nil {
			log.Printf("[backup] 删除旧备份 %s 失败: %v", obj.Key, err)
		} else {
			log.Printf("[backup] 已删除旧备份 %s", obj.Key)
		}
	}
	return nil
}

