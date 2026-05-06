package ipsentinel

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// commonUserAgents 内嵌常用 UA，覆盖 Windows/macOS/iOS/Android。
var commonUserAgents = []string{
	// Windows Chrome
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	// Windows Firefox
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:123.0) Gecko/20100101 Firefox/123.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:122.0) Gecko/20100101 Firefox/122.0",
	// Windows Edge
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 Edg/122.0.0.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0",
	// macOS Chrome
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	// macOS Safari
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Safari/605.1.15",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	// macOS Firefox
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:123.0) Gecko/20100101 Firefox/123.0",
	// iOS Safari
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 16_7_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1",
	// iOS Chrome
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/122.0.6261.89 Mobile/15E148 Safari/604.1",
	// Android Chrome
	"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.6261.90 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 14; Pixel 7a) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.6261.90 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 13; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.6261.90 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 13; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.6261.90 Mobile Safari/537.36",
	// Android Firefox
	"Mozilla/5.0 (Android 14; Mobile; rv:123.0) Gecko/123.0 Firefox/123.0",
	// iPad
	"Mozilla/5.0 (iPad; CPU OS 17_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Mobile/15E148 Safari/604.1",
	// Android Samsung
	"Mozilla/5.0 (Linux; Android 14; SAMSUNG SM-S928B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/24.0 Chrome/117.0.0.0 Mobile Safari/537.36",
	// Windows Opera
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 OPR/108.0.0.0",
	// Android generic
	"Mozilla/5.0 (Linux; Android 12; moto g(60)) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.6167.178 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 11; Redmi Note 9 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.210 Mobile Safari/537.36",
	// macOS Opera
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_3_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 OPR/108.0.0.0",
}

// randUA 随机选择一个 UA。
func randUA() string {
	return commonUserAgents[rand.Intn(len(commonUserAgents))]
}

// jitter 返回 [0, max) 内的随机 float64。
func jitter(max float64) float64 {
	return rand.Float64() * max
}

// RunGoogle 执行 Google 区域纠偏任务。
func RunGoogle(ctx context.Context, cfg Config) RunResult {
	startedAt := time.Now()
	result := RunResult{
		TaskType:  "google",
		StartedAt: startedAt,
	}

	// 无关键词时返回失败
	keywords := cfg.Keywords
	if len(keywords) == 0 {
		keywords = []string{"weather today", "local news", "time now", "directions", "restaurant near me"}
	}

	// 总超时 3 分钟
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	const rounds = 3
	successCount := 0

	for i := 0; i < rounds; i++ {
		// 检查 context 是否已取消
		select {
		case <-runCtx.Done():
			result.Output = append(result.Output, "任务超时或已取消")
			goto done
		default:
		}

		ua := randUA()
		keyword := keywords[rand.Intn(len(keywords))]
		lat := cfg.BaseLat + jitter(0.03) - 0.015
		lon := cfg.BaseLon + jitter(0.03) - 0.015

		searchURL := fmt.Sprintf("https://www.google.com/search?q=%s&%s",
			url.QueryEscape(keyword), cfg.LangParams)

		logLine := fmt.Sprintf("[round %d/%d] keyword=%q lat=%.4f lon=%.4f", i+1, rounds, keyword, lat, lon)
		result.Output = append(result.Output, logLine)

		reqCtx, reqCancel := context.WithTimeout(runCtx, 15*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, searchURL, nil)
		if err != nil {
			reqCancel()
			result.Output = append(result.Output, fmt.Sprintf("  构建请求失败: %v", err))
			continue
		}

		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept-Language", acceptLangFromLangParams(cfg.LangParams))
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		// 带坐标 hint（如 Google 支持）
		req.Header.Set("X-Geo-Hint", fmt.Sprintf("%.4f,%.4f", lat, lon))

		// 不跟随重定向，只看 Location
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		reqCancel()
		if err != nil {
			result.Output = append(result.Output, fmt.Sprintf("  请求失败: %v", err))
		} else {
			resp.Body.Close()
			finalURL := resp.Header.Get("Location")
			if finalURL == "" {
				finalURL = resp.Request.URL.String()
			}
			suffix := "." + cfg.ValidURLSuffix
			if strings.Contains(finalURL, suffix) || strings.Contains(resp.Header.Get("Location"), suffix) || resp.StatusCode == 200 {
				successCount++
				result.Output = append(result.Output, fmt.Sprintf("  成功 status=%d", resp.StatusCode))
			} else {
				result.Output = append(result.Output, fmt.Sprintf("  区域不匹配 status=%d location=%s", resp.StatusCode, finalURL))
			}
		}

		// 间隔 3-5 秒
		if i < rounds-1 {
			sleepDur := time.Duration(3+rand.Intn(3)) * time.Second
			select {
			case <-runCtx.Done():
				goto done
			case <-time.After(sleepDur):
			}
		}
	}

done:
	result.Output = append(result.Output, fmt.Sprintf("完成：%d/%d 次成功", successCount, rounds))
	if successCount > 0 {
		result.Status = "success"
	} else {
		result.Status = "failed"
	}
	result.FinishedAt = time.Now()
	return result
}

// acceptLangFromLangParams 从 LangParams（如 "hl=en&gl=US"）提取 Accept-Language。
func acceptLangFromLangParams(langParams string) string {
	for _, part := range strings.Split(langParams, "&") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && kv[0] == "hl" {
			return kv[1] + ",en;q=0.9"
		}
	}
	return "en-US,en;q=0.9"
}
