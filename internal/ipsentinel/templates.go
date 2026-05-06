package ipsentinel

import "strings"

// RegionTemplate 按国家代码存储的配置建议模板。
type RegionTemplate struct {
	Name           string   // 国家/地区中文名
	DefaultLat     float64  // 典型城市纬度（手动选择国家时使用）
	DefaultLon     float64  // 典型城市经度
	LangParams     string
	ValidURLSuffix string
	Keywords       []string
	WhiteURLs      []string
}

// regionTemplates 按 ISO 3166-1 alpha-2 国家代码索引的配置模板。
var regionTemplates = map[string]RegionTemplate{
	"US": {
		Name: "美国", DefaultLat: 37.7749, DefaultLon: -122.4194,
		LangParams:     "hl=en&gl=US",
		ValidURLSuffix: "com",
		Keywords: []string{
			"weather forecast", "breaking news", "stock market", "sports scores",
			"restaurant near me", "movie showtimes", "flight status", "recipe ideas",
			"local events", "traffic update",
		},
		WhiteURLs: []string{
			"https://en.wikipedia.org",
			"https://www.nytimes.com",
			"https://www.cnn.com",
			"https://www.reddit.com",
			"https://www.yelp.com",
		},
	},
	"JP": {
		Name: "日本", DefaultLat: 35.6762, DefaultLon: 139.6503,
		LangParams:     "hl=ja&gl=JP",
		ValidURLSuffix: "co.jp",
		Keywords: []string{
			"天気予報", "ニュース速報", "株価", "電車時刻表",
			"レストラン", "映画上映", "スポーツ結果", "旅行情報",
			"求人情報", "レシピ",
		},
		WhiteURLs: []string{
			"https://ja.wikipedia.org",
			"https://www.yahoo.co.jp",
			"https://www.nhk.or.jp",
			"https://www.asahi.com",
			"https://tabelog.com",
		},
	},
	"SG": {
		Name: "新加坡", DefaultLat: 1.3521, DefaultLon: 103.8198,
		LangParams:     "hl=en&gl=SG",
		ValidURLSuffix: "com.sg",
		Keywords: []string{
			"weather singapore", "sg news today", "hawker centre", "mrt status",
			"property for sale", "job vacancy", "restaurant review", "flight deals",
			"school holiday", "local events",
		},
		WhiteURLs: []string{
			"https://en.wikipedia.org",
			"https://www.straitstimes.com",
			"https://www.channelnewsasia.com",
			"https://www.gov.sg",
			"https://www.sgcarmart.com",
		},
	},
	"GB": {
		Name: "英国", DefaultLat: 51.5074, DefaultLon: -0.1278,
		LangParams:     "hl=en&gl=GB",
		ValidURLSuffix: "co.uk",
		Keywords: []string{
			"weather forecast uk", "uk news", "premier league scores", "train times",
			"restaurant near me", "cinema listings", "job vacancies", "property for sale",
			"local council", "nhs appointment",
		},
		WhiteURLs: []string{
			"https://en.wikipedia.org",
			"https://www.bbc.co.uk",
			"https://www.theguardian.com",
			"https://www.gov.uk",
			"https://www.rightmove.co.uk",
		},
	},
	"DE": {
		Name: "德国", DefaultLat: 52.5200, DefaultLon: 13.4050,
		LangParams:     "hl=de&gl=DE",
		ValidURLSuffix: "de",
		Keywords: []string{
			"Wettervorhersage", "Nachrichten aktuell", "Bundesliga Ergebnisse", "Zugfahrplan",
			"Restaurant in meiner Nähe", "Kinoprogramm", "Stellenangebote", "Immobilien kaufen",
			"Rezepte", "Veranstaltungen heute",
		},
		WhiteURLs: []string{
			"https://de.wikipedia.org",
			"https://www.spiegel.de",
			"https://www.sueddeutsche.de",
			"https://www.t-online.de",
			"https://www.immobilienscout24.de",
		},
	},
	"FR": {
		Name: "法国", DefaultLat: 48.8566, DefaultLon: 2.3522,
		LangParams:     "hl=fr&gl=FR",
		ValidURLSuffix: "fr",
		Keywords: []string{
			"météo aujourd'hui", "actualités france", "résultats ligue 1", "horaires train",
			"restaurant proche", "programme cinéma", "offres d'emploi", "immobilier",
			"recettes cuisine", "événements locaux",
		},
		WhiteURLs: []string{
			"https://fr.wikipedia.org",
			"https://www.lemonde.fr",
			"https://www.lefigaro.fr",
			"https://www.lequipe.fr",
			"https://www.seloger.com",
		},
	},
	"HK": {
		Name: "香港", DefaultLat: 22.3193, DefaultLon: 114.1694,
		LangParams:     "hl=zh-TW&gl=HK",
		ValidURLSuffix: "com.hk",
		Keywords: []string{
			"天氣預報", "香港新聞", "港股行情", "交通消息",
			"餐廳推介", "電影時間表", "求職招聘", "樓盤買賣",
			"旅遊資訊", "本地活動",
		},
		WhiteURLs: []string{
			"https://zh.wikipedia.org",
			"https://www.hk01.com",
			"https://www.mingpao.com",
			"https://www.gov.hk",
			"https://www.28hse.com",
		},
	},
	"KR": {
		Name: "韩国", DefaultLat: 37.5665, DefaultLon: 126.9780,
		LangParams:     "hl=ko&gl=KR",
		ValidURLSuffix: "co.kr",
		Keywords: []string{
			"날씨 예보", "오늘 뉴스", "주가 현황", "지하철 시간표",
			"맛집 추천", "영화 상영시간", "구인구직", "부동산 매물",
			"요리 레시피", "지역 행사",
		},
		WhiteURLs: []string{
			"https://ko.wikipedia.org",
			"https://www.naver.com",
			"https://news.naver.com",
			"https://www.chosun.com",
			"https://www.daum.net",
		},
	},
	"AU": {
		Name: "澳大利亚", DefaultLat: -33.8688, DefaultLon: 151.2093,
		LangParams:     "hl=en&gl=AU",
		ValidURLSuffix: "com.au",
		Keywords: []string{
			"weather forecast australia", "aussie news", "afl scores", "train timetable",
			"restaurant near me", "cinema sessions", "job vacancies", "property for sale",
			"local events", "beach conditions",
		},
		WhiteURLs: []string{
			"https://en.wikipedia.org",
			"https://www.abc.net.au",
			"https://www.smh.com.au",
			"https://www.realestate.com.au",
			"https://www.seek.com.au",
		},
	},
	"CA": {
		Name: "加拿大", DefaultLat: 43.6532, DefaultLon: -79.3832,
		LangParams:     "hl=en&gl=CA",
		ValidURLSuffix: "ca",
		Keywords: []string{
			"weather canada", "canadian news", "nhl scores", "transit schedule",
			"restaurant near me", "movie times", "jobs hiring", "homes for sale",
			"local events", "road conditions",
		},
		WhiteURLs: []string{
			"https://en.wikipedia.org",
			"https://www.cbc.ca",
			"https://globalnews.ca",
			"https://www.kijiji.ca",
			"https://www.realtor.ca",
		},
	},
	"TW": {
		Name: "台湾", DefaultLat: 25.0330, DefaultLon: 121.5654,
		LangParams:     "hl=zh-TW&gl=TW",
		ValidURLSuffix: "com.tw",
		Keywords: []string{
			"天氣預報", "台灣新聞", "台股行情", "捷運路線",
			"餐廳推薦", "電影時刻表", "求職招募", "房屋買賣",
			"旅遊景點", "活動資訊",
		},
		WhiteURLs: []string{
			"https://zh.wikipedia.org",
			"https://www.udn.com",
			"https://www.chinatimes.com",
			"https://www.gov.tw",
			"https://www.591.com.tw",
		},
	},
	"NL": {
		Name: "荷兰", DefaultLat: 52.3676, DefaultLon: 4.9041,
		LangParams:     "hl=nl&gl=NL",
		ValidURLSuffix: "nl",
		Keywords: []string{
			"weersvoorspelling", "nieuws vandaag", "eredivisie uitslagen", "treinrooster",
			"restaurant buurt", "bioscoop programma", "vacatures", "huizen te koop",
			"recepten", "lokale evenementen",
		},
		WhiteURLs: []string{
			"https://nl.wikipedia.org",
			"https://www.nos.nl",
			"https://www.nu.nl",
			"https://www.funda.nl",
			"https://www.marktplaats.nl",
		},
	},
}

// defaultTemplate 兜底模板，适用于未列出国家。
var defaultTemplate = RegionTemplate{
	LangParams:     "hl=en&gl=US",
	ValidURLSuffix: "com",
	Keywords: []string{
		"weather forecast", "latest news", "local restaurants", "job opportunities",
		"movie times", "sports results", "travel guide", "online shopping",
		"health tips", "technology news",
	},
	WhiteURLs: []string{
		"https://en.wikipedia.org",
		"https://www.google.com",
		"https://www.bbc.com",
		"https://www.reuters.com",
		"https://www.tripadvisor.com",
	},
}

// SuggestConfigByCountry 直接按国家代码生成配置，使用模板默认坐标。
// 用于用户手动选择国家时，不需要先做 IP 检测。
func SuggestConfigByCountry(countryCode string) Config {
	code := strings.ToUpper(countryCode)
	tmpl, ok := regionTemplates[code]
	if !ok {
		tmpl = defaultTemplate
		code = ""
	}
	return Config{
		RegionCode:     code,
		RegionName:     tmpl.Name,
		BaseLat:        tmpl.DefaultLat,
		BaseLon:        tmpl.DefaultLon,
		LangParams:     tmpl.LangParams,
		ValidURLSuffix: tmpl.ValidURLSuffix,
		EnableGoogle:   false,
		EnableTrust:    false,
		Keywords:       tmpl.Keywords,
		WhiteURLs:      tmpl.WhiteURLs,
	}
}

