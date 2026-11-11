package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// aiProxyHandler proxies /ai/* to the AI gateway, mirroring vite's dev proxy
// (vite.config.js aiProxy): the /ai prefix is stripped, the Host is rewritten
// (changeOrigin), and Authorization is injected server-side from
// AI_PROXY_KEY — any client-sent Authorization is replaced, so the key never
// ships to the browser. Stdlib ReverseProxy passes SSE streams through.
func aiProxyHandler(target, key string) (http.Handler, error) {
	parsed, err := url.Parse(strings.TrimSuffix(target, "/"))
	if err != nil {
		return nil, err
	}
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = parsed.Scheme
			req.URL.Host = parsed.Host
			req.URL.Path = strings.TrimSuffix(parsed.Path, "/") + strings.TrimPrefix(req.URL.Path, "/ai")
			req.Host = parsed.Host
			req.Header.Set("Authorization", "Bearer "+key)
		},
	}
	return proxy, nil
}
