package ipsentinel

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// RunTrust 执行信用净化任务：访问白名单网站注入正常流量。
func RunTrust(ctx context.Context, cfg Config) RunResult {
	startedAt := time.Now()
	result := RunResult{
		TaskType:  "trust",
		StartedAt: startedAt,
	}

	urls := cfg.WhiteURLs
	if len(urls) == 0 {
		// 默认白名单
		urls = []string{
			"https://www.apple.com",
			"https://www.microsoft.com",
			"https://www.cloudflare.com",
			"https://www.wikipedia.org",
			"https://www.github.com",
		}
	}

	// 随机选 3-5 个
	count := 3 + rand.Intn(3)
	if count > len(urls) {
		count = len(urls)
	}
	// 打乱顺序后取前 count 个
	perm := rand.Perm(len(urls))
	selected := make([]string, count)
	for i := 0; i < count; i++ {
		selected[i] = urls[perm[i]]
	}

	successCount := 0
	for i, u := range selected {
		select {
		case <-ctx.Done():
			result.Output = append(result.Output, "任务已取消")
			goto done
		default:
		}

		ua := randUA()
		result.Output = append(result.Output, fmt.Sprintf("[%d/%d] 访问 %s", i+1, count, u))

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			result.Output = append(result.Output, fmt.Sprintf("  构建请求失败: %v", err))
			continue
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err != nil {
			result.Output = append(result.Output, fmt.Sprintf("  请求失败: %v", err))
		} else {
			// 丢弃响应 body（限制 1MB 防止恶意服务端撑爆内存）
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			result.Output = append(result.Output, fmt.Sprintf("  成功 status=%d", resp.StatusCode))
			if resp.StatusCode < 400 {
				successCount++
			}
		}

		// 间隔 3-5 秒
		if i < count-1 {
			sleepDur := time.Duration(3+rand.Intn(3)) * time.Second
			select {
			case <-ctx.Done():
				goto done
			case <-time.After(sleepDur):
			}
		}
	}

done:
	result.Output = append(result.Output, fmt.Sprintf("完成：%d/%d 个站点访问成功", successCount, count))
	if successCount > 0 {
		result.Status = "success"
	} else {
		result.Status = "failed"
	}
	result.FinishedAt = time.Now()
	return result
}
