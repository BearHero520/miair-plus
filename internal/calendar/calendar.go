// Package calendar applies published mainland China holiday adjustments.
package calendar

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

//go:embed 2026.json
var builtin []byte

type Day struct {
	Name string `json:"name"`
	Date string `json:"date"`
	Off  bool   `json:"isOffDay"`
}
type Year struct {
	Year   int      `json:"year"`
	Papers []string `json:"papers"`
	Days   []Day    `json:"days"`
}
type Calendar struct {
	mu    sync.RWMutex
	years map[int]Year
	dir   string
}

func parse(b []byte, year int) (Year, error) {
	var y Year
	if json.Unmarshal(b, &y) != nil || y.Year != year || len(y.Days) < 20 || len(y.Days) > 100 || len(y.Papers) == 0 {
		return y, errors.New("该年度尚无完整节假日公告，继续使用已有数据")
	}
	for _, paper := range y.Papers {
		u, err := url.Parse(paper)
		if err != nil || u.Scheme != "https" || (u.Hostname() != "www.gov.cn" && u.Hostname() != "gov.cn") {
			return y, errors.New("节假日数据缺少国务院公告来源")
		}
	}
	seen := map[string]bool{}
	for _, d := range y.Days {
		date, err := time.Parse("2006-01-02", d.Date)
		if err != nil || date.Year() != year || seen[d.Date] {
			return y, errors.New("节假日日期格式错误")
		}
		seen[d.Date] = true
	}
	return y, nil
}
func New(dir string) *Calendar {
	c := &Calendar{years: map[int]Year{}, dir: dir}
	y, _ := parse(builtin, 2026)
	c.years[2026] = y
	for year := 2025; year <= time.Now().Year()+1; year++ {
		b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("calendar-%d.json", year)))
		if err == nil {
			if y, err = parse(b, year); err == nil {
				c.years[year] = y
			}
		}
	}
	return c
}

// Unknown years deliberately do not silently fall back to weekday-only rules.
func (c *Calendar) Match(rule string, date time.Time) (bool, bool) {
	c.mu.RLock()
	y, ok := c.years[date.Year()]
	c.mu.RUnlock()
	if !ok {
		return false, false
	}
	off := date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
	holiday := false
	for _, day := range y.Days {
		if day.Date == date.Format("2006-01-02") {
			off = day.Off
			holiday = day.Off
			break
		}
	}
	switch rule {
	case "workday":
		return !off, true
	case "restday":
		return off, true
	case "holiday":
		return holiday, true
	}
	return false, false
}
func (c *Calendar) Info() any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := map[int][]string{}
	for year, y := range c.years {
		out[year] = append([]string(nil), y.Papers...)
	}
	return out
}
func (c *Calendar) Refresh(ctx context.Context, year int) error {
	if year < time.Now().Year() || year > time.Now().Year()+1 {
		return errors.New("仅更新本年度和下一年度")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/%d.json", year), nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("无法下载节假日数据，已有离线数据仍可用")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("节假日数据尚未发布或暂时不可用")
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if err != nil {
		return err
	}
	y, err := parse(b, year)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(c.dir, ".calendar-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, filepath.Join(c.dir, fmt.Sprintf("calendar-%d.json", year))); err != nil {
		return err
	}
	c.mu.Lock()
	c.years[year] = y
	c.mu.Unlock()
	return nil
}
