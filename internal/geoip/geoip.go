package geoip

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

const (
	downloadBase = "https://download.maxmind.com/app/geoip_download"
	CityFile     = "GeoLite2-City.mmdb"
	ASNFile      = "GeoLite2-ASN.mmdb"
)

// Info 是单个 IP 的地理和 ASN 信息。
type Info struct {
	IP          string  `json:"ip"`
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	ASN         uint    `json:"asn"`
	ASNOrg      string  `json:"asn_org"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
}

// DB 持有已打开的 MaxMind 数据库句柄，支持并发读。
type DB struct {
	mu    sync.RWMutex
	city  *geoip2.Reader
	asn   *geoip2.Reader
	dbDir string
}

// NewDB 打开 dbDir 中的 mmdb 文件；文件不存在时返回空 DB（可后续下载后 Reload）。
func NewDB(dbDir string) *DB {
	db := &DB{dbDir: dbDir}
	_ = db.reload() // 忽略首次打开失败（文件尚未下载）
	return db
}

func (db *DB) reload() error {
	cityPath := filepath.Join(db.dbDir, CityFile)
	asnPath := filepath.Join(db.dbDir, ASNFile)

	city, err := geoip2.Open(cityPath)
	if err != nil {
		return fmt.Errorf("open city db: %w", err)
	}
	asn, err := geoip2.Open(asnPath)
	if err != nil {
		city.Close()
		return fmt.Errorf("open asn db: %w", err)
	}

	db.mu.Lock()
	defer db.mu.Unlock()
	if db.city != nil {
		db.city.Close()
	}
	if db.asn != nil {
		db.asn.Close()
	}
	db.city = city
	db.asn = asn
	return nil
}

// Ready 返回数据库是否已加载。
func (db *DB) Ready() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.city != nil && db.asn != nil
}

// Lookup 查询 IP 的地理和 ASN 信息。
func (db *DB) Lookup(ip net.IP) (Info, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.city == nil || db.asn == nil {
		return Info{}, fmt.Errorf("数据库未加载，请先下载")
	}

	info := Info{IP: ip.String()}

	if cr, err := db.city.City(ip); err == nil {
		info.CountryCode = cr.Country.IsoCode
		if n, ok := cr.Country.Names["zh-CN"]; ok {
			info.CountryName = n
		} else {
			info.CountryName = cr.Country.Names["en"]
		}
		if len(cr.Subdivisions) > 0 {
			if n, ok := cr.Subdivisions[0].Names["zh-CN"]; ok {
				info.Region = n
			} else {
				info.Region = cr.Subdivisions[0].Names["en"]
			}
		}
		if n, ok := cr.City.Names["zh-CN"]; ok {
			info.City = n
		} else {
			info.City = cr.City.Names["en"]
		}
		info.Lat = cr.Location.Latitude
		info.Lon = cr.Location.Longitude
		info.Timezone = cr.Location.TimeZone
	}

	if ar, err := db.asn.ASN(ip); err == nil {
		info.ASN = ar.AutonomousSystemNumber
		info.ASNOrg = ar.AutonomousSystemOrganization
	}

	return info, nil
}

// LookupHost 解析 host（可以是 IP 或域名）并查询 GeoIP 信息。
func (db *DB) LookupHost(host string) (Info, error) {
	ip := net.ParseIP(host)
	if ip == nil {
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return Info{}, fmt.Errorf("DNS 解析 %q 失败: %w", host, err)
		}
		ip = net.ParseIP(addrs[0])
		if ip == nil {
			return Info{}, fmt.Errorf("无法解析地址 %q", addrs[0])
		}
	}
	return db.Lookup(ip)
}

// Download 下载 GeoLite2-City 和 GeoLite2-ASN 数据库到 dbDir。
func (db *DB) Download(licenseKey string) error {
	if err := os.MkdirAll(db.dbDir, 0o755); err != nil {
		return fmt.Errorf("创建目录: %w", err)
	}
	for _, edition := range []string{"GeoLite2-City", "GeoLite2-ASN"} {
		url := fmt.Sprintf("%s?edition_id=%s&license_key=%s&suffix=tar.gz", downloadBase, edition, licenseKey)
		if err := downloadAndExtract(url, edition+".mmdb", db.dbDir); err != nil {
			return fmt.Errorf("下载 %s: %w", edition, err)
		}
	}
	return db.reload()
}

// downloadAndExtract 下载 tar.gz 并提取 targetFile 到 destDir。
func downloadAndExtract(url, targetFile, destDir string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("解压 gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar: %w", err)
		}
		// 找到 .mmdb 文件（在子目录内）
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !strings.HasSuffix(hdr.Name, targetFile) {
			continue
		}
		dest := filepath.Join(destDir, targetFile)
		f, err := os.Create(dest)
		if err != nil {
			return fmt.Errorf("创建文件: %w", err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return fmt.Errorf("写入文件: %w", err)
		}
		f.Close()
		return nil
	}
	return fmt.Errorf("tar 中未找到 %s", targetFile)
}
