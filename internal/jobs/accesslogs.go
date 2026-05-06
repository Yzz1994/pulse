package jobs

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"pulse/internal/accesslogs"
	"pulse/internal/auditrules"
	"pulse/internal/nodes"
)

// SyncAccessLogs 从各节点拉取 access log 缓冲区并批量写入数据库，
// 同时对照审计规则检测异常并触发告警。
func SyncAccessLogs(ctx context.Context, nodeStore nodes.Store, logStore accesslogs.Store, ruleStore auditrules.Store, alerter Alerter, dial NodeDialer) error {
	// 加载当前启用的规则（只查一次，避免每条日志都查 DB）
	var rules []auditrules.Rule
	if ruleStore != nil {
		if rs, err := ruleStore.List(); err == nil {
			for _, r := range rs {
				if r.Enabled {
					rules = append(rules, r)
				}
			}
		}
	}

	allNodes, err := nodeStore.List()
	if err != nil {
		return err
	}

	for _, node := range allNodes {
		// 只对线路机（非落地机）采集 access log
		if node.IsLanding {
			continue
		}
		client, err := dial(node.ID)
		if err != nil {
			continue
		}
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := client.AccessLogs(reqCtx)
		cancel()
		if err != nil {
			continue
		}
		if len(resp.Entries) == 0 {
			continue
		}

		entries := make([]accesslogs.Entry, 0, len(resp.Entries))
		for _, e := range resp.Entries {
			entries = append(entries, accesslogs.Entry{
				NodeID:      node.ID,
				Username:    e.User,
				SourceIP:    e.SourceIP,
				SourcePort:  e.SourcePort,
				Destination: e.Destination,
				RemoteIP:    e.RemoteIP,
				RouteTag:    e.RouteTag,
				Protocol:    e.Protocol,
				InboundTag:  e.InboundTag,
				CreatedAt:   e.Time,
			})
		}
		if err := logStore.Insert(entries); err != nil {
			// 插入失败不中断规则匹配，由调度器层记录错误
			return err
		}

		// 规则匹配告警
		if alerter != nil && len(rules) > 0 {
			for _, e := range entries {
				if hit, desc := matchRules(rules, e); hit {
					title := "⚠️ 审计告警：" + desc
					body := fmt.Sprintf("用户 %s 访问 %s\n节点：%s | %s",
						e.Username, e.Destination, node.Name,
						e.CreatedAt.Format("2006-01-02 15:04:05"),
					)
					_ = alerter.Send(ctx, title, body)
				}
			}
		}
	}
	return nil
}

// matchRules 对单条访问记录逐条匹配规则，返回第一条命中的描述。
func matchRules(rules []auditrules.Rule, e accesslogs.Entry) (bool, string) {
	// 从 destination（host:port）中分离 host 和 port
	host, port, _ := net.SplitHostPort(e.Destination)
	if host == "" {
		host = e.Destination
	}

	for _, r := range rules {
		switch r.Type {
		case auditrules.RuleTypeDomainKeyword:
			if strings.Contains(strings.ToLower(host), strings.ToLower(r.Value)) {
				return true, "域名关键词命中：" + r.Value
			}
		case auditrules.RuleTypePort:
			if port == r.Value {
				return true, "高危端口访问：" + r.Value
			}
		case auditrules.RuleTypeIP:
			if host == r.Value {
				return true, "IP 黑名单命中：" + r.Value
			}
		}
	}
	return false, ""
}

// AccessLogRetention 审计日志保留时长，与 handleAuditAnalysis 默认查询窗口一致。
const AccessLogRetention = 24 * time.Hour

// CleanupAccessLogs 删除超过保留时长的日志。
func CleanupAccessLogs(ctx context.Context, logStore accesslogs.Store) error {
	_, err := logStore.DeleteOlderThan(time.Now().Add(-AccessLogRetention))
	return err
}
