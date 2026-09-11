package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const releasesURL = "https://github.com/BearHero520/miair-plus/releases"
const latestReleaseAPI = "https://api.github.com/repos/BearHero520/miair-plus/releases/latest"

type updateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"update_available"`
	URL       string `json:"release_url"`
	Name      string `json:"release_name,omitempty"`
	Notes     string `json:"notes,omitempty"`
	Published string `json:"published_at,omitempty"`
	Checked   string `json:"checked_at"`
	Error     string `json:"error,omitempty"`
}

type updateChecker struct {
	mu      sync.Mutex
	client  *http.Client
	checked time.Time
	result  updateInfo
}

var stableVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\+[0-9A-Za-z.-]+)?$`)

func releaseVersion(value string) ([3]uint64, bool) {
	var result [3]uint64
	match := stableVersion.FindStringSubmatch(value)
	if match == nil {
		return result, false
	}
	for i := range result {
		n, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return result, false
		}
		result[i] = n
	}
	return result, true
}

func newerRelease(latest, current string) bool {
	l, lok := releaseVersion(latest)
	c, cok := releaseVersion(current)
	if !lok || !cok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func (u *updateChecker) check(ctx context.Context, force bool) updateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	ttl := time.Hour
	if force || u.result.Error != "" {
		ttl = time.Minute
	}
	if !u.checked.IsZero() && time.Since(u.checked) < ttl {
		return u.result
	}
	result := updateInfo{Current: Version, URL: releasesURL, Checked: time.Now().UTC().Format(time.RFC3339)}
	defer func() { u.checked = time.Now(); u.result = result }()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MiAir-Plus/"+Version)
	client := u.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = "无法连接 GitHub，请检查网络后重试"
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case 404:
			result.Error = "未找到公开的正式版本，请稍后重试或查看版本发布页"
		case 403, 429:
			result.Error = "GitHub 请求受限，请稍后重试"
		default:
			result.Error = "更新服务暂时不可用，请稍后重试"
		}
		return result
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Name       string `json:"name"`
		Body       string `json:"body"`
		Published  string `json:"published_at"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		result.Error = "更新服务返回了无效数据"
		return result
	}
	if _, ok := releaseVersion(release.Tag); !ok || release.Draft || release.Prerelease {
		result.Error = "未找到可识别的正式版本"
		return result
	}
	result.Latest = strings.TrimPrefix(release.Tag, "v")
	result.Available = newerRelease(release.Tag, Version)
	result.URL = releasesURL + "/tag/" + url.PathEscape(release.Tag)
	result.Name, result.Notes, result.Published = release.Name, release.Body, release.Published
	return result
}
