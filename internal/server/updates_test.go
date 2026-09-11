package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReleaseComparison(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"v2.0.10", "2.0.9", true}, {"3.0.0", "2.99.99", true},
		{"v2.0.5", "2.0.5", false}, {"2.0.4", "2.0.5", false},
		{"2.0.6-beta.1", "2.0.5", false}, {"invalid", "2.0.5", false},
		{"2.0.5+build.7", "2.0.5", false}, {"2.01.0", "2.0.5", false},
	} {
		if got := newerRelease(tc.latest, tc.current); got != tc.want {
			t.Errorf("%s > %s = %v", tc.latest, tc.current, got)
		}
	}
}

func TestUpdateDetectionAndCache(t *testing.T) {
	calls := 0
	checker := updateChecker{client: &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != latestReleaseAPI || r.Header.Get("User-Agent") == "" {
			t.Fatal("unexpected request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"v2.0.10","name":"New release","body":"notes","html_url":"javascript:bad"}`))}, nil
	})}}
	result := checker.check(context.Background(), false)
	if !result.Available || result.Latest != "2.0.10" || result.Error != "" || result.URL != releasesURL+"/tag/v2.0.10" {
		t.Fatal(result)
	}
	checker.check(context.Background(), false)
	checker.check(context.Background(), true)
	if calls != 1 {
		t.Fatal("cache not used", calls)
	}
	checker.checked = time.Now().Add(-2 * time.Minute)
	checker.check(context.Background(), true)
	if calls != 2 {
		t.Fatal("manual refresh not performed")
	}
}

func TestUpdateFailuresNeverReportLatest(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{403, `{}`}, {429, `{}`}, {404, `{}`}, {500, `{}`}, {200, `not json`},
		{200, `{"tag_name":"v9.0.0","prerelease":true}`}, {200, `{"tag_name":"bad"}`},
	} {
		checker := updateChecker{client: &http.Client{Transport: updateTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}}
		result := checker.check(context.Background(), false)
		if result.Error == "" || result.Available || result.Latest != "" {
			t.Fatal(result)
		}
	}
}
