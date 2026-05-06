package ipsentinel

import "time"

// Config IP Sentinel 节点配置。
type Config struct {
	RegionCode     string   `json:"region_code"`
	RegionName     string   `json:"region_name"`
	BaseLat        float64  `json:"base_lat"`
	BaseLon        float64  `json:"base_lon"`
	LangParams     string   `json:"lang_params"`      // 例如 "hl=en&gl=US"
	ValidURLSuffix string   `json:"valid_url_suffix"` // 例如 "com"
	EnableGoogle   bool     `json:"enable_google"`
	EnableTrust    bool     `json:"enable_trust"`
	WhiteURLs      []string `json:"white_urls"`
	Keywords       []string `json:"keywords"`
}

// DetectResult IP 检测结果。
type DetectResult struct {
	IP          string    `json:"ip"`
	Country     string    `json:"country"`
	CountryCode string    `json:"country_code"` // ISO 3166-1 alpha-2，如 "US"、"JP"
	RegionName  string    `json:"region_name"`
	City        string    `json:"city"`
	ISP         string    `json:"isp"`
	Org         string    `json:"org"`
	AS          string    `json:"as"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	Timezone    string    `json:"timezone"`
	DetectedAt  time.Time `json:"detected_at"`
}

// RunResult 任务执行结果。
type RunResult struct {
	TaskType   string        `json:"task_type"`            // "detect" | "google" | "trust" | "auto"
	Status     string        `json:"status"`               // "success" | "failed"
	Output     []string      `json:"output"`               // 日志行
	Detect     *DetectResult `json:"detect,omitempty"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
}

// NodeStatus 节点 IP Sentinel 运行状态。
// GoogleDetectResult 表示从节点实际发起 Google 请求后得到的检测结果。
// 反映的是 Google 对该 IP 的地区判断，而非第三方 IP 数据库的归属。
type GoogleDetectResult struct {
	FinalURL      string    `json:"final_url"`      // 最终落地 URL（经重定向）
	GoogleDomain  string    `json:"google_domain"`  // Google 判断后使用的域名，如 google.com / google.cn
	IsChina       bool      `json:"is_china"`       // 是否被定向到 .cn 或 .com.hk（送中）
	MatchExpected bool      `json:"match_expected"` // 是否符合配置的 ValidURLSuffix
	DetectedAt    time.Time `json:"detected_at"`
}

type NodeStatus struct {
	Running bool       `json:"running"`
	Last    *RunResult `json:"last"`
}
