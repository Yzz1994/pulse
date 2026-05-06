package ipsentinel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Detect 查询节点出口 IP 的地理位置信息。
// 依次尝试 ip-api.com → ipinfo.io → ip.sb，任一成功即返回。
func Detect(ctx context.Context) (*DetectResult, error) {
	if r, err := detectViaIPAPI(ctx); err == nil {
		return r, nil
	}
	if r, err := detectViaIPInfo(ctx); err == nil {
		return r, nil
	}
	return detectViaIPSB(ctx)
}

// ── ip-api.com ───────────────────────────────────────────────────────────────

type ipAPIResponse struct {
	Status      string  `json:"status"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	AS          string  `json:"as"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	Query       string  `json:"query"`
}

func detectViaIPAPI(ctx context.Context) (*DetectResult, error) {
	const apiURL = "https://ip-api.com/json?fields=status,country,countryCode,regionName,city,isp,org,as,lat,lon,timezone,query"
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "curl/7.88.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ip-api.com 返回 %d", resp.StatusCode)
	}

	var r ipAPIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, err
	}
	if r.Status != "success" {
		return nil, fmt.Errorf("ip-api.com status=%s", r.Status)
	}

	return &DetectResult{
		IP:          r.Query,
		Country:     r.Country,
		CountryCode: r.CountryCode,
		RegionName:  r.RegionName,
		City:        r.City,
		ISP:         r.ISP,
		Org:         r.Org,
		AS:          r.AS,
		Lat:         r.Lat,
		Lon:         r.Lon,
		Timezone:    r.Timezone,
		DetectedAt:  time.Now().UTC(),
	}, nil
}

// ── ipinfo.io ────────────────────────────────────────────────────────────────

type ipInfoResponse struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	City     string `json:"city"`
	Org      string `json:"org"`
	Timezone string `json:"timezone"`
	Loc      string `json:"loc"` // "lat,lon"
}

func detectViaIPInfo(ctx context.Context) (*DetectResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://ipinfo.io/json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "curl/7.88.0")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipinfo.io 返回 %d", resp.StatusCode)
	}

	var r ipInfoResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, err
	}

	var lat, lon float64
	fmt.Sscanf(r.Loc, "%f,%f", &lat, &lon)

	return &DetectResult{
		IP:         r.IP,
		Country:    r.Country,
		RegionName: r.Region,
		City:       r.City,
		Org:        r.Org,
		AS:         r.Org,
		Lat:        lat,
		Lon:        lon,
		Timezone:   r.Timezone,
		DetectedAt: time.Now().UTC(),
	}, nil
}

// ── api.ip.sb ────────────────────────────────────────────────────────────────

type ipSBResponse struct {
	IP          string  `json:"ip"`
	CountryCode string  `json:"country_code"`
	Country     string  `json:"country"`
	RegionName  string  `json:"region"`
	City        string  `json:"city"`
	ISP         string  `json:"isp"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
}

func detectViaIPSB(ctx context.Context) (*DetectResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://api.ip.sb/geoip", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "curl/7.88.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api.ip.sb 返回 %d", resp.StatusCode)
	}

	var r ipSBResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, err
	}
	if r.IP == "" {
		return nil, fmt.Errorf("api.ip.sb 返回空 IP")
	}

	return &DetectResult{
		IP:          r.IP,
		Country:     r.Country,
		CountryCode: r.CountryCode,
		RegionName:  r.RegionName,
		City:        r.City,
		ISP:         r.ISP,
		Lat:         r.Latitude,
		Lon:         r.Longitude,
		Timezone:    r.Timezone,
		DetectedAt:  time.Now().UTC(),
	}, nil
}
