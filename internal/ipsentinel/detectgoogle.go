package ipsentinel

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DetectGoogle 从节点直接向 Google 发起请求，检测 Google 实际将该 IP 判断为哪个地区。
// 通过观察重定向目标域名来判断是否"送中"。
func DetectGoogle(ctx context.Context, cfg Config) (*GoogleDetectResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 用普通英文 UA，不带地区暗示，让 Google 纯粹按 IP 判断
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.google.com/", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	// 跟随最多 5 次重定向，记录最终落地 URL
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		// 如果是重定向截止，err 仍会返回但 resp 有值
		if resp == nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	// 只读取少量响应体，不关心内容
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	finalURL := resp.Request.URL.String()

	// 优先取 Location 头（未跟随重定向时）
	if loc := resp.Header.Get("Location"); loc != "" {
		if abs, err := url.Parse(loc); err == nil {
			if !abs.IsAbs() {
				abs = resp.Request.URL.ResolveReference(abs)
			}
			finalURL = abs.String()
		}
	}

	domain := extractGoogleDomain(finalURL)
	isChina := isChineseDomain(domain)

	expectedSuffix := cfg.ValidURLSuffix
	if expectedSuffix == "" {
		expectedSuffix = "com"
	}
	// google.com 是国际通用域名，无论配置何种后缀均视为正常
	// （Google 已不强制跳转国家域名，多数国家 IP 都返回 google.com）
	matchExpected := domain == "google.com" ||
		strings.HasSuffix(domain, "."+expectedSuffix) ||
		domain == "google."+expectedSuffix

	return &GoogleDetectResult{
		FinalURL:      finalURL,
		GoogleDomain:  domain,
		IsChina:       isChina,
		MatchExpected: matchExpected,
		DetectedAt:    time.Now().UTC(),
	}, nil
}

// extractGoogleDomain 从 URL 中提取 google.xxx 部分。
func extractGoogleDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := u.Hostname()
	// 去掉 www. 前缀
	host = strings.TrimPrefix(host, "www.")
	return host
}

// isChineseDomain 判断是否被定向到中国大陆的 Google 域名（不含香港）。
// google.com.hk 是合法的香港区域域名，不属于"送中"。
func isChineseDomain(domain string) bool {
	return domain == "google.cn" ||
		strings.HasSuffix(domain, ".google.cn")
}
