package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const (
	listenAddr          = ":2375"
	dockerDialTimeout   = 5 * time.Second
	proxyReadTimeout    = 15 * time.Second
	proxyWriteTimeout   = 30 * time.Second
	proxyIdleTimeout    = 60 * time.Second
	maxRequestBodyBytes = 2 << 20 // 2 MiB is plenty for these endpoints
)

var (
	reContainersJSON = regexp.MustCompile(`^/containers/json$`)
	reImagesJSON     = regexp.MustCompile(`^/images/json$`)
	reInfo           = regexp.MustCompile(`^/info$`)
	reVersion        = regexp.MustCompile(`^/version$`)
	reContainerLogs  = regexp.MustCompile(`^/containers/([^/]+)/logs$`)

	// Container/Image inspection (read-only details)
	reContainerInspect = regexp.MustCompile(`^/containers/([^/]+)/json$`)
	reImageInspect     = regexp.MustCompile(`^/images/([^/]+)/json$`)

	// Container lifecycle actions
	reContainerAction = regexp.MustCompile(`^/containers/([^/]+)/(start|stop|restart)$`)
	reContainerCreate = regexp.MustCompile(`^/containers/create$`)
	reContainerRemove = regexp.MustCompile(`^/containers/([^/]+)$`)

	// Image operations
	reImageCreate = regexp.MustCompile(`^/images/create$`)
	reImageRemove = regexp.MustCompile(`^/images/([^/]+)$`)

	// Network operations
	reNetworkConnect    = regexp.MustCompile(`^/networks/([^/]+)/connect$`)
	reNetworkDisconnect = regexp.MustCompile(`^/networks/([^/]+)/disconnect$`)

	// Cleanup operations
	reContainersPrune = regexp.MustCompile(`^/containers/prune$`)
	reImagesPrune     = regexp.MustCompile(`^/images/prune$`)
	reNetworksPrune   = regexp.MustCompile(`^/networks/prune$`)
	reBuildPrune      = regexp.MustCompile(`^/build/prune$`)
)

func allowed(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return false
	}

	// Strip Docker API version prefix (e.g., /v1.41/containers/json -> /containers/json)
	if strings.HasPrefix(path, "/v") {
		if idx := strings.Index(path[1:], "/"); idx > 0 {
			path = path[idx+1:]
		}
	}

	// Read-only
	if method == http.MethodGet {
		switch {
		case reContainersJSON.MatchString(path):
			return true
		case reImagesJSON.MatchString(path):
			return true
		case reInfo.MatchString(path):
			return true
		case reVersion.MatchString(path):
			return true
		case reContainerLogs.MatchString(path):
			return true
		case reContainerInspect.MatchString(path):
			return true
		case reImageInspect.MatchString(path):
			return true
		default:
			return false
		}
	}

	// Mutations (keep operation ability)
	if method == http.MethodPost {
		switch {
		case reContainerAction.MatchString(path):
			return true
		case reContainerCreate.MatchString(path):
			return true
		case reImageCreate.MatchString(path):
			return true
		case reNetworkConnect.MatchString(path):
			return true
		case reNetworkDisconnect.MatchString(path):
			return true
		case reContainersPrune.MatchString(path):
			return true
		case reImagesPrune.MatchString(path):
			return true
		case reNetworksPrune.MatchString(path):
			return true
		case reBuildPrune.MatchString(path):
			return true
		default:
			return false
		}
	}
	if method == http.MethodDelete {
		switch {
		case reContainerRemove.MatchString(path):
			return true
		case reImageRemove.MatchString(path):
			return true
		default:
			return false
		}
	}

	return false
}

func resolveSockPath() string {
	if v := strings.TrimSpace(os.Getenv("DOCKER_SOCK")); v != "" {
		return v
	}
	return "/var/run/docker.sock"
}

func parseBytes(s string) (int64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	// Accept plain integer bytes.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return n, true
	}

	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "kib"):
		mult = 1024
		s = strings.TrimSuffix(s, "kib")
	case strings.HasSuffix(s, "kb"):
		mult = 1000
		s = strings.TrimSuffix(s, "kb")
	case strings.HasSuffix(s, "mib"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "mib")
	case strings.HasSuffix(s, "mb"):
		mult = 1000 * 1000
		s = strings.TrimSuffix(s, "mb")
	case strings.HasSuffix(s, "gib"):
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gib")
	case strings.HasSuffix(s, "gb"):
		mult = 1000 * 1000 * 1000
		s = strings.TrimSuffix(s, "gb")
	}
	val := strings.TrimSpace(s)
	if val == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return int64(f * float64(mult)), true
}

func applyMemoryLimitFromEnv() {
	// Prefer GOMEMLIMIT (Go runtime supports it), but also accept DOCKER_PROXY_MEM.
	if v := strings.TrimSpace(os.Getenv("GOMEMLIMIT")); v != "" {
		log.Printf("dockerproxy: GOMEMLIMIT=%s", v)
		return
	}
	if v := strings.TrimSpace(os.Getenv("DOCKER_PROXY_MEM")); v != "" {
		if b, ok := parseBytes(v); ok {
			debug.SetMemoryLimit(b)
			log.Printf("dockerproxy: set memory limit=%d bytes (from DOCKER_PROXY_MEM=%s)", b, v)
			return
		}
		log.Printf("dockerproxy: invalid DOCKER_PROXY_MEM=%q (examples: 8MiB, 12MB)", v)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	applyMemoryLimitFromEnv()

	sock := resolveSockPath()
	log.Printf("dockerproxy: listening on %s, docker sock=%s", listenAddr, sock)

	target, _ := url.Parse("http://docker")
	dialer := &net.Dialer{Timeout: dockerDialTimeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", sock)
		},
		ResponseHeaderTimeout: proxyReadTimeout,
		DisableKeepAlives:     true,
		MaxIdleConns:          0,
		IdleConnTimeout:       30 * time.Second,
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// Keep response JSON-ish to match upstream usage.
		msg := err.Error()
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"docker proxy error: ` + jsonEscape(msg) + `"}`))
	}

	// Preserve original Director but force unix target.
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		// Don't let client-supplied Host propagate.
		r.Host = "docker"
		// Drop hop-by-hop headers are handled by ReverseProxy.
	}

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}

		if !allowed(r.Method, r.URL.Path) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"docker proxy: endpoint not allowed"}`))
			return
		}

		// Hardening: avoid large bodies.
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}

		proxy.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       proxyReadTimeout,
		WriteTimeout:      proxyWriteTimeout,
		IdleTimeout:       proxyIdleTimeout,
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func jsonEscape(s string) string {
	// minimal escape for embedding into a JSON string
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
