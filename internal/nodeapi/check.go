package nodeapi

import (
"context"
"encoding/json"
"fmt"
"io"
"net/http"
"net/url"
"regexp"
"strings"
"sync"
"time"
)

// serviceCheckResult 单个服务的检测结果。
type serviceCheckResult struct {
Service  string `json:"service"`
Unlocked bool   `json:"unlocked"`
Region   string `json:"region,omitempty"`
Note     string `json:"note,omitempty"`
}

// CheckResponse 解锁检测结果，与 nodes.CheckUnlockResponse 同形。
type CheckResponse struct {
Direct         []serviceCheckResult `json:"direct"`
Proxied        []serviceCheckResult `json:"proxied,omitempty"`
ProxyAvailable bool                 `json:"proxy_available"`
}

// DoCheckUnlock 直连 + 代理双轨并发检测各流媒体服务。
func (a *API) DoCheckUnlock(ctx context.Context) CheckResponse {
var (
direct         []serviceCheckResult
proxied        []serviceCheckResult
proxyAvailable bool
wg             sync.WaitGroup
)

wg.Add(1)
go func() {
defer wg.Done()
direct = runChecks(ctx, checkHTTPClient)
}()

if proxyURL := findLocalProxyPort(a.activeManager().Config()); proxyURL != "" {
proxyAvailable = true
wg.Add(1)
go func() {
defer wg.Done()
proxied = runChecks(ctx, newProxiedClient(proxyURL))
}()
}

wg.Wait()

resp := CheckResponse{Direct: direct, ProxyAvailable: proxyAvailable}
if proxyAvailable {
resp.Proxied = proxied
}
return resp
}

const checkUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var checkHTTPClient = &http.Client{
Timeout: 9 * time.Second,
CheckRedirect: func(req *http.Request, via []*http.Request) error {
if len(via) >= 5 {
return http.ErrUseLastResponse
}
return nil
},
}

func newProxiedClient(proxyURL string) *http.Client {
u, _ := url.Parse(proxyURL)
return &http.Client{
Timeout: 9 * time.Second,
Transport: &http.Transport{
Proxy: http.ProxyURL(u),
},
CheckRedirect: func(req *http.Request, via []*http.Request) error {
if len(via) >= 5 {
return http.ErrUseLastResponse
}
return nil
},
}
}

func findLocalProxyPort(configJSON string) string {
if configJSON == "" {
return ""
}
var cfg struct {
Inbounds []struct {
Protocol string `json:"protocol"`
Port     int    `json:"port"`
} `json:"inbounds"`
}
if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
return ""
}
for _, ib := range cfg.Inbounds {
switch ib.Protocol {
case "http":
return fmt.Sprintf("http://127.0.0.1:%d", ib.Port)
case "socks":
return fmt.Sprintf("socks5://127.0.0.1:%d", ib.Port)
}
}
return ""
}

type streamServiceDef struct {
name string
// 简单模式：GET url + checkFn 解析。
url     string
checkFn func(status int, finalURL, body string) (unlocked bool, region, note string)
// 高级模式：customCheck 不为空时优先使用，框架仅传入 ctx/client，由实现自行发请求。
customCheck func(ctx context.Context, client *http.Client) (unlocked bool, region, note string)
}

func isoToName(code string) string {
if code == "" {
return ""
}
names := map[string]string{
"US": "美国", "GB": "英国", "CA": "加拿大", "AU": "澳大利亚",
"JP": "日本", "KR": "韩国", "SG": "新加坡", "HK": "香港",
"TW": "台湾", "DE": "德国", "FR": "法国", "NL": "荷兰",
"IT": "意大利", "ES": "西班牙", "PT": "葡萄牙", "SE": "瑞典",
"NO": "挪威", "FI": "芬兰", "DK": "丹麦", "CH": "瑞士",
"AT": "奥地利", "BE": "比利时", "PL": "波兰", "CZ": "捷克",
"TR": "土耳其", "RU": "俄罗斯", "IN": "印度", "BR": "巴西",
"MX": "墨西哥", "AR": "阿根廷", "CL": "智利", "CO": "哥伦比亚",
"ZA": "南非", "TH": "泰国", "MY": "马来西亚", "ID": "印度尼西亚",
"PH": "菲律宾", "VN": "越南", "NZ": "新西兰",
"SA": "沙特阿拉伯", "AE": "阿联酋", "IL": "以色列",
}
if name, ok := names[strings.ToUpper(code)]; ok {
return name
}
return strings.ToUpper(code)
}

func parseCountry(body string, patterns []string) string {
for _, p := range patterns {
re := regexp.MustCompile(p)
if m := re.FindStringSubmatch(body); len(m) > 1 {
return strings.ToUpper(m[1])
}
}
return ""
}

// claudeSupportedCountries 来自 HsukqiLee/MediaUnlockTest，对应 Anthropic 公开支持的国家列表。
var claudeSupportedCountries = map[string]struct{}{
"AL": {}, "DZ": {}, "AD": {}, "AO": {}, "AG": {}, "AR": {}, "AM": {}, "AU": {}, "AT": {}, "AZ": {},
"BS": {}, "BH": {}, "BD": {}, "BB": {}, "BE": {}, "BZ": {}, "BJ": {}, "BT": {}, "BO": {}, "BA": {},
"BW": {}, "BR": {}, "BN": {}, "BG": {}, "BF": {}, "BI": {}, "CV": {}, "KH": {}, "CM": {}, "CA": {},
"TD": {}, "CL": {}, "CO": {}, "KM": {}, "CG": {}, "CR": {}, "CI": {}, "HR": {}, "CY": {}, "CZ": {},
"DK": {}, "DJ": {}, "DM": {}, "DO": {}, "EC": {}, "EG": {}, "SV": {}, "GQ": {}, "EE": {}, "SZ": {},
"FJ": {}, "FI": {}, "FR": {}, "GA": {}, "GM": {}, "GE": {}, "DE": {}, "GH": {}, "GR": {}, "GD": {},
"GT": {}, "GN": {}, "GW": {}, "GY": {}, "HT": {}, "HN": {}, "HU": {}, "IS": {}, "IN": {}, "ID": {},
"IQ": {}, "IE": {}, "IL": {}, "IT": {}, "JM": {}, "JP": {}, "JO": {}, "KZ": {}, "KE": {}, "KI": {},
"KW": {}, "KG": {}, "LA": {}, "LV": {}, "LB": {}, "LS": {}, "LR": {}, "LI": {}, "LT": {}, "LU": {},
"MG": {}, "MW": {}, "MY": {}, "MV": {}, "MT": {}, "MH": {}, "MR": {}, "MU": {}, "MX": {}, "FM": {},
"MD": {}, "MC": {}, "MN": {}, "ME": {}, "MA": {}, "MZ": {}, "NA": {}, "NR": {}, "NP": {}, "NL": {},
"NZ": {}, "NE": {}, "NG": {}, "MK": {}, "NO": {}, "OM": {}, "PK": {}, "PW": {}, "PS": {}, "PA": {},
"PG": {}, "PY": {}, "PE": {}, "PH": {}, "PL": {}, "PT": {}, "QA": {}, "RO": {}, "RW": {}, "KN": {},
"LC": {}, "VC": {}, "WS": {}, "SM": {}, "ST": {}, "SA": {}, "SN": {}, "RS": {}, "SC": {}, "SL": {},
"SG": {}, "SK": {}, "SI": {}, "SB": {}, "ZA": {}, "KR": {}, "ES": {}, "LK": {}, "SR": {}, "SE": {},
"CH": {}, "TW": {}, "TJ": {}, "TZ": {}, "TH": {}, "TL": {}, "TG": {}, "TO": {}, "TT": {}, "TN": {},
"TR": {}, "TM": {}, "TV": {}, "UG": {}, "UA": {}, "AE": {}, "GB": {}, "US": {}, "UY": {}, "UZ": {},
"VU": {}, "VA": {}, "VN": {}, "ZM": {}, "ZW": {},
}

func claudeSupports(loc string) bool {
_, ok := claudeSupportedCountries[strings.ToUpper(loc)]
return ok
}

// extractCFLoc 从 Cloudflare /cdn-cgi/trace 响应中提取 loc=XX 国码。
func extractCFLoc(body string) string {
i := strings.Index(body, "loc=")
if i == -1 {
return ""
}
rest := body[i+4:]
end := strings.IndexAny(rest, "\r\n")
if end == -1 {
end = len(rest)
}
return strings.TrimSpace(rest[:end])
}

// openAISupportedCountries 来自 HsukqiLee/MediaUnlockTest checks/ChatGPT.go。
var openAISupportedCountries = map[string]struct{}{
"AL": {}, "DZ": {}, "AD": {}, "AO": {}, "AG": {}, "AR": {}, "AM": {}, "AU": {}, "AT": {}, "AZ": {},
"BS": {}, "BD": {}, "BB": {}, "BE": {}, "BZ": {}, "BJ": {}, "BT": {}, "BA": {}, "BW": {}, "BR": {},
"BG": {}, "BF": {}, "CV": {}, "CA": {}, "CL": {}, "CO": {}, "KM": {}, "CR": {}, "HR": {}, "CY": {},
"DK": {}, "DJ": {}, "DM": {}, "DO": {}, "EC": {}, "SV": {}, "EE": {}, "FJ": {}, "FI": {}, "FR": {},
"GA": {}, "GM": {}, "GE": {}, "DE": {}, "GH": {}, "GR": {}, "GD": {}, "GT": {}, "GN": {}, "GW": {},
"GY": {}, "HT": {}, "HN": {}, "HU": {}, "IS": {}, "IN": {}, "ID": {}, "IQ": {}, "IE": {}, "IL": {},
"IT": {}, "JM": {}, "JP": {}, "JO": {}, "KZ": {}, "KE": {}, "KI": {}, "KW": {}, "KG": {}, "LV": {},
"LB": {}, "LS": {}, "LR": {}, "LI": {}, "LT": {}, "LU": {}, "MG": {}, "MW": {}, "MY": {}, "MV": {},
"ML": {}, "MT": {}, "MH": {}, "MR": {}, "MU": {}, "MX": {}, "MC": {}, "MN": {}, "ME": {}, "MA": {},
"MZ": {}, "MM": {}, "NA": {}, "NR": {}, "NP": {}, "NL": {}, "NZ": {}, "NI": {}, "NE": {}, "NG": {},
"MK": {}, "NO": {}, "OM": {}, "PK": {}, "PW": {}, "PA": {}, "PG": {}, "PE": {}, "PH": {}, "PL": {},
"PT": {}, "QA": {}, "RO": {}, "RW": {}, "KN": {}, "LC": {}, "VC": {}, "WS": {}, "SM": {}, "ST": {},
"SN": {}, "RS": {}, "SC": {}, "SL": {}, "SG": {}, "SK": {}, "SI": {}, "SB": {}, "ZA": {}, "ES": {},
"LK": {}, "SR": {}, "SE": {}, "CH": {}, "TH": {}, "TG": {}, "TO": {}, "TT": {}, "TN": {}, "TR": {},
"TV": {}, "UG": {}, "AE": {}, "US": {}, "UY": {}, "VU": {}, "ZM": {}, "BO": {}, "BN": {}, "CG": {},
"CZ": {}, "VA": {}, "FM": {}, "MD": {}, "PS": {}, "KR": {}, "TW": {}, "TZ": {}, "TL": {}, "GB": {},
"AQ": {},
}

func openAISupports(loc string) bool {
_, ok := openAISupportedCountries[strings.ToUpper(loc)]
return ok
}

// geminiSupportedCountries 来自 HsukqiLee/MediaUnlockTest checks/Gemini.go。
var geminiSupportedCountries = map[string]struct{}{
"AX": {}, "AL": {}, "DZ": {}, "AS": {}, "AD": {}, "AO": {}, "AI": {}, "AQ": {}, "AG": {}, "AR": {},
"AM": {}, "AW": {}, "AU": {}, "AT": {}, "AZ": {}, "BH": {}, "BD": {}, "BB": {}, "BE": {}, "BZ": {},
"BJ": {}, "BM": {}, "BT": {}, "BO": {}, "BA": {}, "BW": {}, "BR": {}, "IO": {}, "VG": {}, "BN": {},
"BG": {}, "BF": {}, "BI": {}, "CV": {}, "KH": {}, "CM": {}, "CA": {}, "BQ": {}, "KY": {}, "CF": {},
"TD": {}, "CL": {}, "CX": {}, "CC": {}, "CO": {}, "KM": {}, "CK": {}, "CR": {}, "CI": {}, "HR": {},
"CW": {}, "CZ": {}, "CD": {}, "DK": {}, "DJ": {}, "DM": {}, "DO": {}, "EC": {}, "EG": {}, "SV": {},
"GQ": {}, "ER": {}, "EE": {}, "SZ": {}, "ET": {}, "FK": {}, "FO": {}, "FJ": {}, "FI": {}, "FR": {},
"GF": {}, "PF": {}, "TF": {}, "GA": {}, "GE": {}, "DE": {}, "GH": {}, "GI": {}, "GR": {}, "GL": {},
"GD": {}, "GP": {}, "GU": {}, "GT": {}, "GG": {}, "GN": {}, "GW": {}, "GY": {}, "HT": {}, "HM": {},
"HN": {}, "HU": {}, "IS": {}, "IN": {}, "ID": {}, "IQ": {}, "IE": {}, "IM": {}, "IL": {}, "IT": {},
"JM": {}, "JP": {}, "JE": {}, "JO": {}, "KZ": {}, "KE": {}, "KI": {}, "XK": {}, "KW": {}, "KG": {},
"LA": {}, "LV": {}, "LB": {}, "LS": {}, "LR": {}, "LY": {}, "LI": {}, "LT": {}, "LU": {}, "MG": {},
"MW": {}, "MY": {}, "MV": {}, "ML": {}, "MT": {}, "MH": {}, "MQ": {}, "MR": {}, "MU": {}, "YT": {},
"MX": {}, "FM": {}, "MD": {}, "MC": {}, "MN": {}, "ME": {}, "MS": {}, "MA": {}, "MZ": {}, "MM": {},
"NA": {}, "NR": {}, "NP": {}, "NL": {}, "NC": {}, "NZ": {}, "NI": {}, "NE": {}, "NG": {}, "NU": {},
"NF": {}, "MK": {}, "MP": {}, "NO": {}, "OM": {}, "PK": {}, "PW": {}, "PS": {}, "PA": {}, "PG": {},
"PY": {}, "PE": {}, "PH": {}, "PN": {}, "PL": {}, "PT": {}, "PR": {}, "QA": {}, "CY": {}, "CG": {},
"RE": {}, "RO": {}, "RW": {}, "BL": {}, "SH": {}, "KN": {}, "LC": {}, "MF": {}, "PM": {}, "VC": {},
"WS": {}, "SM": {}, "ST": {}, "SA": {}, "SN": {}, "RS": {}, "SC": {}, "SL": {}, "SG": {}, "SX": {},
"SK": {}, "SI": {}, "SB": {}, "SO": {}, "ZA": {}, "GS": {}, "KR": {}, "SS": {}, "ES": {}, "LK": {},
"SD": {}, "SR": {}, "SJ": {}, "SE": {}, "CH": {}, "TW": {}, "TJ": {}, "TZ": {}, "TH": {}, "BS": {},
"GM": {}, "TL": {}, "TG": {}, "TK": {}, "TO": {}, "TT": {}, "TN": {}, "TR": {}, "TM": {}, "TC": {},
"TV": {}, "VI": {}, "UG": {}, "UA": {}, "AE": {}, "GB": {}, "US": {}, "UM": {}, "UY": {}, "UZ": {},
"VU": {}, "VA": {}, "VE": {}, "VN": {}, "WF": {}, "EH": {}, "YE": {}, "ZM": {}, "ZW": {},
}

func geminiSupports(loc string) bool {
_, ok := geminiSupportedCountries[strings.ToUpper(loc)]
return ok
}

// iso3to2 ISO 3166-1 alpha-3 → alpha-2 映射，仅含 Gemini 白名单内的国码。
var iso3to2 = map[string]string{
"ALB": "AL", "DZA": "DZ", "ASM": "AS", "AND": "AD", "AGO": "AO", "AIA": "AI", "ATA": "AQ",
"ATG": "AG", "ARG": "AR", "ARM": "AM", "ABW": "AW", "AUS": "AU", "AUT": "AT", "AZE": "AZ",
"BHR": "BH", "BGD": "BD", "BRB": "BB", "BEL": "BE", "BLZ": "BZ", "BEN": "BJ", "BMU": "BM",
"BTN": "BT", "BOL": "BO", "BIH": "BA", "BWA": "BW", "BRA": "BR", "IOT": "IO", "VGB": "VG",
"BRN": "BN", "BGR": "BG", "BFA": "BF", "BDI": "BI", "CPV": "CV", "KHM": "KH", "CMR": "CM",
"CAN": "CA", "BES": "BQ", "CYM": "KY", "CAF": "CF", "TCD": "TD", "CHL": "CL", "CXR": "CX",
"CCK": "CC", "COL": "CO", "COM": "KM", "COK": "CK", "CRI": "CR", "CIV": "CI", "HRV": "HR",
"CUW": "CW", "CYP": "CY", "CZE": "CZ", "COD": "CD", "DNK": "DK", "DJI": "DJ", "DMA": "DM",
"DOM": "DO", "ECU": "EC", "EGY": "EG", "SLV": "SV", "GNQ": "GQ", "ERI": "ER", "EST": "EE",
"SWZ": "SZ", "ETH": "ET", "FLK": "FK", "FRO": "FO", "FJI": "FJ", "FIN": "FI", "FRA": "FR",
"GUF": "GF", "PYF": "PF", "ATF": "TF", "GAB": "GA", "GMB": "GM", "GEO": "GE", "DEU": "DE",
"GHA": "GH", "GIB": "GI", "GRC": "GR", "GRL": "GL", "GRD": "GD", "GLP": "GP", "GUM": "GU",
"GTM": "GT", "GGY": "GG", "GIN": "GN", "GNB": "GW", "GUY": "GY", "HTI": "HT", "HMD": "HM",
"HND": "HN", "HUN": "HU", "ISL": "IS", "IND": "IN", "IDN": "ID", "IRQ": "IQ", "IRL": "IE",
"IMN": "IM", "ISR": "IL", "ITA": "IT", "JAM": "JM", "JPN": "JP", "JEY": "JE", "JOR": "JO",
"KAZ": "KZ", "KEN": "KE", "KIR": "KI", "XKX": "XK", "KWT": "KW", "KGZ": "KG", "LAO": "LA",
"LVA": "LV", "LBN": "LB", "LSO": "LS", "LBR": "LR", "LBY": "LY", "LIE": "LI", "LTU": "LT",
"LUX": "LU", "MDG": "MG", "MWI": "MW", "MYS": "MY", "MDV": "MV", "MLI": "ML", "MLT": "MT",
"MHL": "MH", "MTQ": "MQ", "MRT": "MR", "MUS": "MU", "MYT": "YT", "MEX": "MX", "FSM": "FM",
"MDA": "MD", "MCO": "MC", "MNG": "MN", "MNE": "ME", "MSR": "MS", "MAR": "MA", "MOZ": "MZ",
"MMR": "MM", "NAM": "NA", "NRU": "NR", "NPL": "NP", "NLD": "NL", "NCL": "NC", "NZL": "NZ",
"NIC": "NI", "NER": "NE", "NGA": "NG", "NIU": "NU", "NFK": "NF", "MKD": "MK", "MNP": "MP",
"NOR": "NO", "OMN": "OM", "PAK": "PK", "PLW": "PW", "PSE": "PS", "PAN": "PA", "PNG": "PG",
"PRY": "PY", "PER": "PE", "PHL": "PH", "PCN": "PN", "POL": "PL", "PRT": "PT", "PRI": "PR",
"QAT": "QA", "COG": "CG", "REU": "RE", "ROU": "RO", "RWA": "RW", "BLM": "BL", "SHN": "SH",
"KNA": "KN", "LCA": "LC", "MAF": "MF", "SPM": "PM", "VCT": "VC", "WSM": "WS", "SMR": "SM",
"STP": "ST", "SAU": "SA", "SEN": "SN", "SRB": "RS", "SYC": "SC", "SLE": "SL", "SGP": "SG",
"SXM": "SX", "SVK": "SK", "SVN": "SI", "SLB": "SB", "SOM": "SO", "ZAF": "ZA", "SGS": "GS",
"KOR": "KR", "SSD": "SS", "ESP": "ES", "LKA": "LK", "SDN": "SD", "SUR": "SR", "SJM": "SJ",
"SWE": "SE", "CHE": "CH", "TWN": "TW", "TJK": "TJ", "TZA": "TZ", "THA": "TH", "BHS": "BS",
"TLS": "TL", "TGO": "TG", "TKL": "TK", "TON": "TO", "TTO": "TT", "TUN": "TN", "TUR": "TR",
"TKM": "TM", "TCA": "TC", "TUV": "TV", "VIR": "VI", "UGA": "UG", "UKR": "UA", "ARE": "AE",
"GBR": "GB", "USA": "US", "UMI": "UM", "URY": "UY", "UZB": "UZ", "VUT": "VU", "VAT": "VA",
"VEN": "VE", "VNM": "VN", "WLF": "WF", "ESH": "EH", "YEM": "YE", "ZMB": "ZM", "ZWE": "ZW",
"ALA": "AX",
}

func iso3ToIso2(code string) string {
return iso3to2[strings.ToUpper(code)]
}

var streamServices = []streamServiceDef{
{
name: "Netflix",
url:  "https://www.netflix.com/title/81280792",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
// 参考 HsukqiLee/MediaUnlockTest：访问 LEGO 标题页（非 Netflix Originals）
// 200/301 → 完整解锁；404 → 仅原创；403 → IP 被封。
switch status {
case 200, 301:
// finalURL 形如 https://www.netflix.com/<region>-en/title/...
if strings.Contains(finalURL, "/title/") {
parts := strings.Split(finalURL, "/")
for _, p := range parts {
if i := strings.Index(p, "-"); i == 2 {
return true, isoToName(strings.ToUpper(p[:i])), ""
}
}
return true, "美国", ""
}
return true, "", ""
case 404:
return false, "", "仅 Netflix Originals"
case 403:
return false, "", "IP 被封"
default:
return false, "", "HTTP " + http.StatusText(status)
}
},
},
{
name: "YouTube",
url:  "https://www.youtube.com/",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
if status != 200 {
return false, "", "HTTP " + http.StatusText(status)
}
code := parseCountry(body, []string{
`"GL"\s*:\s*"([A-Z]{2})"`,
`"gl"\s*:\s*"([a-zA-Z]{2})"`,
`INNERTUBE_CONTEXT_GL[" ]*:\s*"([A-Z]{2})"`,
})
return true, isoToName(code), ""
},
},
{
name:        "Disney+",
customCheck: checkDisneyPlus,
},
{
name: "Claude",
url:  "https://claude.ai/cdn-cgi/trace",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
// 参考 HsukqiLee/MediaUnlockTest：从 Cloudflare trace 取 loc=XX 国码，对照支持国家白名单。
if status != 200 {
return false, "", "HTTP " + http.StatusText(status)
}
loc := extractCFLoc(body)
if loc == "" {
return false, "", "无法获取地区"
}
if loc == "T1" {
return true, "Tor", ""
}
if claudeSupports(loc) {
return true, isoToName(loc), ""
}
return false, isoToName(loc), "地区不可用"
},
},
{
name: "OpenAI",
url:  "https://chatgpt.com/cdn-cgi/trace",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
// 参考 HsukqiLee/MediaUnlockTest：从 chatgpt.com 的 CF trace 取 loc，对照 OpenAI 支持国家白名单。
if status != 200 {
return false, "", "HTTP " + http.StatusText(status)
}
loc := extractCFLoc(body)
if loc == "" {
return false, "", "无法获取地区"
}
if loc == "T1" {
return true, "Tor", ""
}
if openAISupports(loc) {
return true, isoToName(loc), ""
}
return false, isoToName(loc), "地区不可用"
},
},
{
name: "Gemini",
url:  "https://gemini.google.com/?hl=en",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
// 参考 HsukqiLee/MediaUnlockTest：解析 body 中 `,2,1,200,"XXX"` 取 3 字母国码，转 2 字母后对照白名单。
if status == 403 || status == 451 {
return false, "", "地区封锁"
}
if status != 200 {
return false, "", "HTTP " + http.StatusText(status)
}
re := regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`)
m := re.FindStringSubmatch(body)
if len(m) < 2 {
return false, "", "无法获取地区"
}
two := iso3ToIso2(m[1])
if two == "" {
return false, m[1], "未知地区"
}
if geminiSupports(two) {
return true, isoToName(two), ""
}
return false, isoToName(two), "地区不可用"
},
},
{
name:        "Spotify",
customCheck: checkSpotify,
},
{
name: "TikTok",
url:  "https://www.tiktok.com/explore",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
// 参考 HsukqiLee/MediaUnlockTest：从 explore 页 body 中匹配 `"region":"XX"` 取地区。
if strings.Contains(body, "https://www.tiktok.com/hk/notfound") {
return false, "香港", "地区不可用"
}
re := regexp.MustCompile(`"region":"(\w+)"`)
if m := re.FindStringSubmatch(body); len(m) > 1 {
return true, isoToName(m[1]), ""
}
if status != 200 {
return false, "", "HTTP " + http.StatusText(status)
}
return false, "", "无法获取地区"
},
},
{
name: "Twitter/X",
url:  "https://x.com/",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
return status == 200, "", ""
},
},
{
name: "GitHub",
url:  "https://github.com/",
checkFn: func(status int, finalURL, body string) (bool, string, string) {
return status == 200, "", ""
},
},
}

func runChecks(ctx context.Context, client *http.Client) []serviceCheckResult {
results := make([]serviceCheckResult, len(streamServices))
var wg sync.WaitGroup

for i, svc := range streamServices {
wg.Add(1)
go func(idx int, s streamServiceDef) {
defer wg.Done()
result := serviceCheckResult{Service: s.name}

if s.customCheck != nil {
result.Unlocked, result.Region, result.Note = s.customCheck(ctx, client)
results[idx] = result
return
}

req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
if err != nil {
result.Note = "请求构建失败"
results[idx] = result
return
}
req.Header.Set("User-Agent", checkUserAgent)
req.Header.Set("Accept-Language", "en-US,en;q=0.9")
req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

resp, err := client.Do(req)
if err != nil {
result.Note = "连接超时"
results[idx] = result
return
}
defer resp.Body.Close()

body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
result.Unlocked, result.Region, result.Note = s.checkFn(
resp.StatusCode, resp.Request.URL.String(), string(body),
)
results[idx] = result
}(i, svc)
}

wg.Wait()
return results
}

// checkSpotify 参考 HsukqiLee/MediaUnlockTest checks/Spotify.go：
// 通过 signup API 触发账号创建预检，返回 status/Country/is_country_launched 判定。
func checkSpotify(ctx context.Context, client *http.Client) (bool, string, string) {
form := `birth_day=11&birth_month=11&birth_year=2000&collect_personal_info=undefined&creation_flow=&creation_point=https%3A%2F%2Fwww.spotify.com%2Fhk-en%2F&displayname=Test&gender=male&iagree=1&key=a1e486e2729f46d6bb368d6b2bcda326&platform=www&referrer=&send-email=0&thirdpartyemail=0&identifier_token=AgE6YTvEzkReHNfJpO114514`
req, err := http.NewRequestWithContext(ctx, http.MethodPost,
"https://spclient.wg.spotify.com/signup/public/v1/account",
strings.NewReader(form))
if err != nil {
return false, "", "请求构建失败"
}
req.Header.Set("User-Agent", checkUserAgent)
req.Header.Set("Accept-Language", "en")
req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
req.Header.Set("Cache-Control", "no-cache")
resp, err := client.Do(req)
if err != nil {
return false, "", "连接超时"
}
defer resp.Body.Close()
if resp.StatusCode == 403 {
return false, "", "IP 被封"
}
body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
var res struct {
Status            int
Country           string
IsCountryLaunched bool `json:"is_country_launched"`
}
if err := json.Unmarshal(body, &res); err != nil {
return false, "", "解析失败"
}
if res.Status == 320 {
return false, "", "地区不可用"
}
if res.Status == 311 && res.IsCountryLaunched {
return true, isoToName(strings.ToUpper(res.Country)), ""
}
return false, "", "地区不可用"
}

// checkDisneyPlus 参考 HsukqiLee/MediaUnlockTest checks/DisneyPlus.go 的简化版：
// 链式 4 个 API 中前两步即可判定大多数封锁情况；完整 GraphQL region 提取需 4 步，此处只做前 2 步快检。
func checkDisneyPlus(ctx context.Context, client *http.Client) (bool, string, string) {
const bearer = "ZGlzbmV5JmJyb3dzZXImMS4wLjA.Cu56AgSfBTDag5NiRA81oLHkDZfu5L3CKadnefEAY84"
// Step 1: 取 device assertion
devReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
"https://disney.api.edge.bamgrid.com/devices",
strings.NewReader(`{"deviceFamily":"browser","applicationRuntime":"chrome","deviceProfile":"windows","attributes":{}}`))
if err != nil {
return false, "", "请求构建失败"
}
devReq.Header.Set("User-Agent", checkUserAgent)
devReq.Header.Set("Authorization", "Bearer "+bearer)
devReq.Header.Set("Content-Type", "application/json")
devResp, err := client.Do(devReq)
if err != nil {
return false, "", "连接超时"
}
devBody, _ := io.ReadAll(io.LimitReader(devResp.Body, 65536))
devResp.Body.Close()
if strings.Contains(string(devBody), "403 ERROR") {
return false, "", "地区不可用"
}
var dev struct {
Assertion string `json:"assertion"`
}
if err := json.Unmarshal(devBody, &dev); err != nil || dev.Assertion == "" {
return false, "", "解析失败"
}
// Step 2: token exchange，看是否 forbidden-location
tokenForm := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange&latitude=0&longitude=0&platform=browser&subject_token=" + dev.Assertion + "&subject_token_type=urn%3Abamtech%3Aparams%3Aoauth%3Atoken-type%3Adevice"
tokReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
"https://disney.api.edge.bamgrid.com/token",
strings.NewReader(tokenForm))
if err != nil {
return false, "", "请求构建失败"
}
tokReq.Header.Set("User-Agent", checkUserAgent)
tokReq.Header.Set("Authorization", bearer)
tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
tokResp, err := client.Do(tokReq)
if err != nil {
return false, "", "连接超时"
}
tokBody, _ := io.ReadAll(io.LimitReader(tokResp.Body, 65536))
tokResp.Body.Close()
if tokResp.StatusCode == 403 || strings.Contains(string(tokBody), "forbidden-location") {
return false, "", "地区不可用"
}
// Step 3: 主页 GET 检查最终 URL（preview/unavailable 即不可用）。
homeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.disneyplus.com/", nil)
if err != nil {
return false, "", "请求构建失败"
}
homeReq.Header.Set("User-Agent", checkUserAgent)
homeResp, err := client.Do(homeReq)
if err != nil {
return false, "", "连接超时"
}
finalURL := homeResp.Request.URL.String()
homeResp.Body.Close()
if strings.Contains(finalURL, "preview") || strings.Contains(finalURL, "unavailable") {
return false, "", "地区不可用"
}
// 通过 token+主页双校验判定为解锁；省略 GraphQL region 提取，region 留空。
return true, "", ""
}
