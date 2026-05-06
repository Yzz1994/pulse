package cloudflarex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const apiBase = "https://api.cloudflare.com/client/v4"

// Zone 代表一个 CF 域名
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`   // example.com
	Status string `json:"status"` // active, pending 等
}

// DNSRecord 代表一条 DNS 记录
type DNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`              // A, AAAA, CNAME
	Name    string `json:"name"`              // 完整域名: sub.example.com
	Content string `json:"content"`           // IP 或目标
	TTL     int    `json:"ttl"`               // 1 = auto
	Proxied bool   `json:"proxied"`           // CF 代理开关
	Comment string `json:"comment,omitempty"` // CF 控制台备注
}

// Client 是 CF API 客户端
type Client struct {
	token string
	http  *http.Client
}

// NewClient 创建 CF API 客户端
func NewClient(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// apiResponse 是 CF API 的通用响应结构
type apiResponse struct {
	Success  bool             `json:"success"`
	Errors   []apiError       `json:"errors"`
	Result   json.RawMessage  `json:"result"`
	Info     *resultInfo      `json:"result_info,omitempty"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type resultInfo struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

// do 执行 HTTP 请求并解析 CF API 响应
func (c *Client) do(req *http.Request) (*apiResponse, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflarex: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 最大 10MB
	if err != nil {
		return nil, fmt.Errorf("cloudflarex: 读取响应失败: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("cloudflarex: 解析响应失败 (HTTP %d): %s", resp.StatusCode, body)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("cloudflarex: API 错误: %v", formatErrors(apiResp.Errors))
	}

	return &apiResp, nil
}

// formatErrors 格式化 CF API 错误列表
func formatErrors(errs []apiError) string {
	if len(errs) == 0 {
		return "未知错误"
	}
	if len(errs) == 1 {
		return fmt.Sprintf("[%d] %s", errs[0].Code, errs[0].Message)
	}
	s := ""
	for i, e := range errs {
		if i > 0 {
			s += "; "
		}
		s += fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return s
}

// ListZones 列出 token 下所有域名，自动处理分页
func (c *Client) ListZones(ctx context.Context) ([]Zone, error) {
	var all []Zone
	page := 1

	for {
		u := fmt.Sprintf("%s/zones?page=%d&per_page=50", apiBase, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}

		apiResp, err := c.do(req)
		if err != nil {
			return nil, err
		}

		var zones []Zone
		if err := json.Unmarshal(apiResp.Result, &zones); err != nil {
			return nil, fmt.Errorf("cloudflarex: 解析 zones 失败: %w", err)
		}
		all = append(all, zones...)

		// 无分页信息或已到最后一页
		if apiResp.Info == nil || page >= apiResp.Info.TotalPages || page >= 100 {
			break
		}
		page++
	}

	return all, nil
}

// ListDNSRecords 列出某 zone 下的 DNS 记录，recordType 为空则不过滤
func (c *Client) ListDNSRecords(ctx context.Context, zoneID string, recordType string) ([]DNSRecord, error) {
	u := fmt.Sprintf("%s/zones/%s/dns_records?per_page=100", apiBase, url.PathEscape(zoneID))
	if recordType != "" {
		u += "&type=" + url.QueryEscape(recordType)
	}

	var all []DNSRecord
	page := 1

	for {
		paged := u + "&page=" + strconv.Itoa(page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, paged, nil)
		if err != nil {
			return nil, err
		}

		apiResp, err := c.do(req)
		if err != nil {
			return nil, err
		}

		var records []DNSRecord
		if err := json.Unmarshal(apiResp.Result, &records); err != nil {
			return nil, fmt.Errorf("cloudflarex: 解析 dns_records 失败: %w", err)
		}
		all = append(all, records...)

		if apiResp.Info == nil || page >= apiResp.Info.TotalPages || page >= 100 {
			break
		}
		page++
	}

	return all, nil
}

// CreateDNSRecord 创建 DNS 记录
func (c *Client) CreateDNSRecord(ctx context.Context, zoneID string, record DNSRecord) (DNSRecord, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return DNSRecord{}, err
	}

	u := fmt.Sprintf("%s/zones/%s/dns_records", apiBase, url.PathEscape(zoneID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return DNSRecord{}, err
	}

	apiResp, err := c.do(req)
	if err != nil {
		return DNSRecord{}, err
	}

	var created DNSRecord
	if err := json.Unmarshal(apiResp.Result, &created); err != nil {
		return DNSRecord{}, fmt.Errorf("cloudflarex: 解析创建结果失败: %w", err)
	}
	return created, nil
}

// UpdateDNSRecord 更新 DNS 记录
func (c *Client) UpdateDNSRecord(ctx context.Context, zoneID string, recordID string, record DNSRecord) (DNSRecord, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return DNSRecord{}, err
	}

	u := fmt.Sprintf("%s/zones/%s/dns_records/%s", apiBase, url.PathEscape(zoneID), url.PathEscape(recordID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return DNSRecord{}, err
	}

	apiResp, err := c.do(req)
	if err != nil {
		return DNSRecord{}, err
	}

	var updated DNSRecord
	if err := json.Unmarshal(apiResp.Result, &updated); err != nil {
		return DNSRecord{}, fmt.Errorf("cloudflarex: 解析更新结果失败: %w", err)
	}
	return updated, nil
}

// DeleteDNSRecord 删除 DNS 记录
func (c *Client) DeleteDNSRecord(ctx context.Context, zoneID string, recordID string) error {
	u := fmt.Sprintf("%s/zones/%s/dns_records/%s", apiBase, url.PathEscape(zoneID), url.PathEscape(recordID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}

	_, err = c.do(req)
	return err
}

// VerifyToken 验证 token 是否有效
func (c *Client) VerifyToken(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/user/tokens/verify", nil)
	if err != nil {
		return err
	}

	_, err = c.do(req)
	return err
}
