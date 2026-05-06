package ipsentinel

import (
	"context"
	"sync"
	"time"
)

// Runner 管理 IP Sentinel 任务的执行状态。
type Runner struct {
	mu      sync.Mutex
	last    *RunResult // 最后一次执行结果（内存缓存）
	running bool
}

// NewRunner 创建新的 Runner 实例。
func NewRunner() *Runner {
	return &Runner{}
}

// IsRunning 返回当前是否有任务正在运行。
func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// GetLastResult 返回最近一次执行结果，若未执行过则返回 nil。
func (r *Runner) GetLastResult() *RunResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

// Run 同步执行任务，taskType 可为 "google"、"trust" 或 "auto"。
// 若已有任务运行中，则直接返回 running 状态结果。
func (r *Runner) Run(ctx context.Context, cfg Config, taskType string) RunResult {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return RunResult{TaskType: taskType, Status: "running", Output: []string{"已有任务运行中"}}
	}
	r.running = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	var result RunResult

	switch taskType {
	case "auto":
		// auto：启用的任务全部串行执行
		if !cfg.EnableGoogle && !cfg.EnableTrust {
			result = RunResult{TaskType: taskType, Status: "failed", Output: []string{"未启用任何任务类型"}, FinishedAt: time.Now()}
			break
		}
		var output []string
		status := "success"
		if cfg.EnableGoogle {
			gr := RunGoogle(ctx, cfg)
			output = append(output, gr.Output...)
			if gr.Status != "success" {
				status = gr.Status
			}
		}
		if cfg.EnableTrust {
			tr := RunTrust(ctx, cfg)
			output = append(output, tr.Output...)
			if tr.Status != "success" && status == "success" {
				status = tr.Status
			}
		}
		result = RunResult{TaskType: taskType, Status: status, Output: output, FinishedAt: time.Now()}
	case "google":
		if !cfg.EnableGoogle {
			result = RunResult{TaskType: taskType, Status: "failed", Output: []string{"Google 纠偏未启用"}, FinishedAt: time.Now()}
		} else {
			result = RunGoogle(ctx, cfg)
		}
	case "trust":
		if !cfg.EnableTrust {
			result = RunResult{TaskType: taskType, Status: "failed", Output: []string{"Trust 净化未启用"}, FinishedAt: time.Now()}
		} else {
			result = RunTrust(ctx, cfg)
		}
	default:
		result = RunResult{TaskType: taskType, Status: "failed", Output: []string{"未知任务类型"}, FinishedAt: time.Now()}
	}

	result.TaskType = taskType

	r.mu.Lock()
	r.last = &result
	r.mu.Unlock()

	return result
}

