// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var robotTests = []string{
	"Mozilla/5.0 (compatible; TweetedTimes Bot/1.0; +http://tweetedtimes.com)",
	"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)",
	"Mozilla/5.0 (compatible; MJ12bot/v1.4.3; http://www.majestic12.co.uk/bot.php?+)",
	"Go 1.1 package http",
	"Java/1.7.0_25	0.003	0.003",
	"Python-urllib/2.6",
	"Mozilla/5.0 (compatible; archive.org_bot +http://www.archive.org/details/archive.org_bot)",
	"Mozilla/5.0 (compatible; Ezooms/1.0; ezooms.bot@gmail.com)",
	"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
}

func TestRobotPat(t *testing.T) {
	// TODO(light): isRobot checks for more than just the User-Agent.
	// Extract out the database interaction to an interface to test the
	// full analysis.

	for _, tt := range robotTests {
		if !robotPat.MatchString(tt) {
			t.Errorf("%s not a robot", tt)
		}
	}
}

func TestHandlePkgGoDevRedirect(t *testing.T) {
	handler := pkgGoDevRedirectHandler(func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	for _, test := range []struct {
		name, url, wantLocationHeader, wantSetCookieHeader string
		wantStatusCode                                     int
		cookie                                             *http.Cookie
	}{
		{
			name:                "test pkggodev-redirect param is on",
			url:                 "http://godoc.org/net/http?redirect=on",
			wantLocationHeader:  "https://pkg.go.dev/net/http?tab=doc&utm_source=godoc",
			wantSetCookieHeader: "pkggodev-redirect=on; Path=/",
			wantStatusCode:      http.StatusFound,
		},
		{
			name:                "test pkggodev-redirect param is off",
			url:                 "http://godoc.org/net/http?redirect=off",
			wantLocationHeader:  "",
			wantSetCookieHeader: "pkggodev-redirect=; Path=/; Max-Age=0",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "test pkggodev-redirect param is unset",
			url:                 "http://godoc.org/net/http",
			wantLocationHeader:  "",
			wantSetCookieHeader: "",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "toggle enabled pkggodev-redirect cookie",
			url:                 "http://godoc.org/net/http?redirect=off",
			cookie:              &http.Cookie{Name: "pkggodev-redirect", Value: "true"},
			wantLocationHeader:  "",
			wantSetCookieHeader: "pkggodev-redirect=; Path=/; Max-Age=0",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "pkggodev-redirect enabled cookie should redirect",
			url:                 "http://godoc.org/net/http",
			cookie:              &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			wantLocationHeader:  "https://pkg.go.dev/net/http?tab=doc&utm_source=godoc",
			wantSetCookieHeader: "",
			wantStatusCode:      http.StatusFound,
		},
		{
			name:           "do not redirect if user is returning from pkg.go.dev",
			url:            "http://godoc.org/net/http?utm_source=backtogodoc",
			cookie:         &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			wantStatusCode: http.StatusOK,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.url, nil)
			if test.cookie != nil {
				req.AddCookie(test.cookie)
			}

			w := httptest.NewRecorder()
			err := handler(w, req)
			if err != nil {
				t.Fatal(err)
			}
			resp := w.Result()

			if got, want := resp.Header.Get("Location"), test.wantLocationHeader; got != want {
				t.Errorf("Location header mismatch: got %q; want %q", got, want)
			}

			if got, want := resp.Header.Get("Set-Cookie"), test.wantSetCookieHeader; got != want {
				t.Errorf("Set-Cookie header mismatch: got %q; want %q", got, want)
			}

			if got, want := resp.StatusCode, test.wantStatusCode; got != want {
				t.Errorf("Status code mismatch: got %d; want %d", got, want)
			}
		})
	}
}

func TestGodoc(t *testing.T) {
	testCases := []struct {
		from, to string
	}{
		{
			from: "https://godoc.org/-/about",
			to:   "https://pkg.go.dev/about?utm_source=godoc",
		},
		{
			from: "https://godoc.org/-/go",
			to:   "https://pkg.go.dev/std?tab=packages&utm_source=godoc",
		},
		{
			from: "https://godoc.org/?q=foo",
			to:   "https://pkg.go.dev/search?q=foo&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=doc&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?imports",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=imports&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?importers",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=importedby&utm_source=godoc",
		},
		{
			from: "https://godoc.org/golang.org/x/vgo/vendor/cmd/go/internal/modfile",
			to:   "https://pkg.go.dev/?utm_source=godoc",
		},
		{
			from: "https://godoc.org/golang.org/x/vgo/vendor",
			to:   "https://pkg.go.dev/?utm_source=godoc",
		},
	}

	for _, tc := range testCases {
		u, err := url.Parse(tc.from)
		if err != nil {
			t.Errorf("url.Parse(%q): %v", tc.from, err)
			continue
		}
		to := pkgGoDevURL(u)
		if got, want := to.String(), tc.to; got != want {
			t.Errorf("pkgGoDevURL(%q) = %q; want %q", u, got, want)
		}
	}
}

func TestNewGDDOEvent(t *testing.T) {
	for _, test := range []struct {
		name   string
		url    string
		cookie *http.Cookie
		want   *gddoEvent
	}{
		{
			name: "home page request",
			url:  "https://godoc.org",
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "",
				UsePkgGoDev: false,
			},
		},
		{
			name:   "home page request with cookie on should redirect",
			url:    "https://godoc.org",
			cookie: &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "",
				UsePkgGoDev: true,
			},
		},
		{
			name: "about page request",
			url:  "https://godoc.org/-/about",
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "/-/about",
				UsePkgGoDev: false,
			},
		},
		{
			name: "request with search query parameter",
			url:  "https://godoc.org/?q=test",
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "/",
				UsePkgGoDev: false,
			},
		},
		{
			name: "package page request",
			url:  "https://godoc.org/net/http",
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "/net/http",
				UsePkgGoDev: false,
			},
		},
		{
			name:   "package page request with wrong cookie on should not redirect",
			url:    "https://godoc.org/net/http",
			cookie: &http.Cookie{Name: "bogus-cookie", Value: "on"},
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "/net/http",
				UsePkgGoDev: false,
			},
		},
		{
			name:   "package page request with query parameter off should not redirect",
			url:    "https://godoc.org/net/http?redirect=off",
			cookie: &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "/net/http",
				UsePkgGoDev: false,
			},
		},
		{
			name:   "package page request with cookie on should redirect",
			url:    "https://godoc.org/net/http",
			cookie: &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "/net/http",
				UsePkgGoDev: true,
			},
		},
		{
			name:   "package page request with query parameter on should redirect",
			url:    "https://godoc.org/net/http?redirect=on",
			cookie: &http.Cookie{Name: "pkggodev-redirect", Value: ""},
			want: &gddoEvent{
				Host:        "godoc.org",
				Path:        "/net/http",
				UsePkgGoDev: true,
			},
		},
		{
			name: "api request",
			url:  "https://api.godoc.org/imports/net/http",
			want: &gddoEvent{
				Host:        "api.godoc.org",
				Path:        "/imports/net/http",
				UsePkgGoDev: false,
			},
		},
		{
			name:   "api requests should never redirect, even with cookie on",
			url:    "https://api.godoc.org/imports/net/http",
			cookie: &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			want: &gddoEvent{
				Host:        "api.godoc.org",
				Path:        "/imports/net/http",
				UsePkgGoDev: false,
			},
		},
		{
			name:   "api requests should never redirect, even with query parameter on",
			url:    "https://api.godoc.org/imports/net/http?redirect=on",
			cookie: &http.Cookie{Name: "pkggodev-redirect", Value: ""},
			want: &gddoEvent{
				Host:        "api.godoc.org",
				Path:        "/imports/net/http",
				UsePkgGoDev: false,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := test.want
			want.Latency = 100
			want.URL = test.url
			want.Header = http.Header{}
			want.IsRobot = true
			r := httptest.NewRequest("GET", test.url, nil)
			if test.cookie != nil {
				r.AddCookie(test.cookie)
				want.Header.Add("Cookie", test.cookie.String())
			}
			got := newGDDOEvent(r, want.Latency, want.IsRobot, http.StatusOK)
			want.Status = http.StatusOK
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewGDDOEventFromRequests(t *testing.T) {
	for _, test := range []struct {
		name       string
		requestURI string
		host       string
		want       *gddoEvent
	}{
		{
			name:       "absolute index path",
			requestURI: "https://godoc.org",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "",
				URL:  "https://godoc.org",
			},
		},
		{
			name:       "absolute index path with trailing slash",
			requestURI: "https://godoc.org/",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/",
				URL:  "https://godoc.org/",
			},
		},
		{
			name:       "relative index path",
			requestURI: "/",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/",
				URL:  "https://godoc.org/",
			},
		},
		{
			name:       "absolute about path",
			requestURI: "https://godoc.org/-/about",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/-/about",
				URL:  "https://godoc.org/-/about",
			},
		},
		{
			name:       "relative about path",
			requestURI: "/-/about",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/-/about",
				URL:  "https://godoc.org/-/about",
			},
		},
		{
			name:       "absolute package path",
			requestURI: "https://godoc.org/net/http",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/net/http",
				URL:  "https://godoc.org/net/http",
			},
		},
		{
			name:       "relative package path",
			requestURI: "/net/http",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/net/http",
				URL:  "https://godoc.org/net/http",
			},
		},
		{
			name:       "absolute path with query parameters",
			requestURI: "https://godoc.org/net/http?q=test",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/net/http",
				URL:  "https://godoc.org/net/http?q=test",
			},
		},
		{
			name:       "relative path with query parameters",
			requestURI: "/net/http?q=test",
			host:       "godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/net/http",
				URL:  "https://godoc.org/net/http?q=test",
			},
		},
		{
			name:       "absolute api path",
			requestURI: "https://api.godoc.org/imports/net/http",
			host:       "api.godoc.org",
			want: &gddoEvent{
				Host: "api.godoc.org",
				Path: "/imports/net/http",
				URL:  "https://api.godoc.org/imports/net/http",
			},
		},
		{
			name:       "relative api path",
			requestURI: "/imports/net/http",
			host:       "api.godoc.org",
			want: &gddoEvent{
				Host: "api.godoc.org",
				Path: "/imports/net/http",
				URL:  "https://api.godoc.org/imports/net/http",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := test.want
			want.Latency = 100
			want.Header = http.Header{}
			want.IsRobot = false
			requestLine := "GET " + test.requestURI + " HTTP/1.1\r\nHost: " + test.host + "\r\n\r\n"
			req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(requestLine)))
			if err != nil {
				t.Fatal("invalid NewRequest arguments; " + err.Error())
			}
			got := newGDDOEvent(req, want.Latency, want.IsRobot, http.StatusOK)
			want.Status = http.StatusOK
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
