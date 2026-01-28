package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"golang.org/x/net/proxy"
)

var (
	systemDirectClient *http.Client
	systemProxyClient  *http.Client
	systemProxyURL     string
	clientLock         sync.RWMutex
)

// GetHTTPClientSystemProxy returns a cached http.Client.
// - useProxy=false: bypass proxy
// - useProxy=true: use proxy settings from system/app settings (setting key: proxy_url)
func GetHTTPClientSystemProxy(useProxy bool) (*http.Client, error) {
	if useProxy {
		currentProxyURL, err := op.SettingGetString(model.SettingKeyProxyURL)
		if err != nil {
			return nil, err
		}
		if currentProxyURL == "" {
			return nil, fmt.Errorf("proxy url is empty")
		}

		clientLock.RLock()
		if systemProxyClient != nil && systemProxyURL == currentProxyURL {
			clientLock.RUnlock()
			return systemProxyClient, nil
		}
		clientLock.RUnlock()

		clientLock.Lock()
		defer clientLock.Unlock()

		// Re-check after acquiring write lock.
		if systemProxyClient != nil && systemProxyURL == currentProxyURL {
			return systemProxyClient, nil
		}

		client, err := newHTTPClientCustomProxy(currentProxyURL)
		if err != nil {
			return nil, err
		}
		systemProxyClient = client
		systemProxyURL = currentProxyURL
		return systemProxyClient, nil
	}

	clientLock.RLock()
	if !useProxy && systemDirectClient != nil {
		clientLock.RUnlock()
		return systemDirectClient, nil
	}
	clientLock.RUnlock()

	clientLock.Lock()
	defer clientLock.Unlock()

	if systemDirectClient != nil {
		return systemDirectClient, nil
	}
	client, err := newHTTPClientNoProxy()
	if err != nil {
		return nil, err
	}
	systemDirectClient = client
	return systemDirectClient, nil
}

// GetHTTPClientCustomProxy returns a NEW http.Client every time (no reuse).
// proxyURL supports: http, https, socks, socks5
func GetHTTPClientCustomProxy(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return nil, fmt.Errorf("proxy url is empty")
	}
	return newHTTPClientCustomProxy(proxyURL)
}

func clonedDefaultTransport() (*http.Transport, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is not *http.Transport")
	}
	cloned := transport.Clone()

	// 精细化超时设置，防止请求无限挂起
	cloned.ResponseHeaderTimeout = 60 * time.Second // 等待响应头超时
	cloned.TLSHandshakeTimeout = 15 * time.Second   // TLS 握手超时
	cloned.ExpectContinueTimeout = 5 * time.Second
	cloned.IdleConnTimeout = 90 * time.Second // 空闲连接超时

	return cloned, nil
}

func applyUpstreamTransportTimeouts(tr *http.Transport) {
	if tr == nil {
		return
	}
	cfg := conf.AppConfig.UpstreamHTTP

	if cfg.DialTimeoutSec > 0 {
		d := &net.Dialer{
			Timeout:   time.Duration(cfg.DialTimeoutSec) * time.Second,
			KeepAlive: 30 * time.Second,
		}
		tr.DialContext = d.DialContext
	}
	if cfg.TLSHandshakeTimeoutSec > 0 {
		tr.TLSHandshakeTimeout = time.Duration(cfg.TLSHandshakeTimeoutSec) * time.Second
	}
	if cfg.ResponseHeaderTimeoutSec > 0 {
		tr.ResponseHeaderTimeout = time.Duration(cfg.ResponseHeaderTimeoutSec) * time.Second
	}
	if cfg.ExpectContinueTimeoutSec > 0 {
		tr.ExpectContinueTimeout = time.Duration(cfg.ExpectContinueTimeoutSec) * time.Second
	}
	if cfg.IdleConnTimeoutSec > 0 {
		tr.IdleConnTimeout = time.Duration(cfg.IdleConnTimeoutSec) * time.Second
	}
}

func socksDialContext(socksDialer proxy.Dialer, dialTimeout time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	// Prefer a real context-aware dialer if available.
	if cd, ok := socksDialer.(proxy.ContextDialer); ok {
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			if dialTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, dialTimeout)
				defer cancel()
			}
			return cd.DialContext(ctx, network, addr)
		}
	}

	// Fallback: run Dial() in a goroutine and respect ctx cancellation.
	// Note: the underlying Dial() can't be forcibly canceled, but we prevent the caller from hanging
	// and close the conn if it completes after ctx cancellation.
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dialTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, dialTimeout)
			defer cancel()
		}
		type dialRes struct {
			conn net.Conn
			err  error
		}
		ch := make(chan dialRes, 1)
		go func() {
			c, err := socksDialer.Dial(network, addr)
			ch <- dialRes{conn: c, err: err}
		}()

		select {
		case <-ctx.Done():
			go func() {
				r := <-ch
				if r.conn != nil {
					_ = r.conn.Close()
				}
			}()
			return nil, ctx.Err()
		case r := <-ch:
			return r.conn, r.err
		}
	}
}

func newHTTPClientNoProxy() (*http.Client, error) {
	cloned, err := clonedDefaultTransport()
	if err != nil {
		return nil, err
	}
	cloned.Proxy = nil
	applyUpstreamTransportTimeouts(cloned)
	return &http.Client{Transport: cloned}, nil
}

func newHTTPClientCustomProxy(proxyURLStr string) (*http.Client, error) {
	cloned, err := clonedDefaultTransport()
	if err != nil {
		return nil, err
	}
	applyUpstreamTransportTimeouts(cloned)

	proxyURL, err := url.Parse(proxyURLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url: %w", err)
	}

	switch proxyURL.Scheme {
	case "http", "https":
		cloned.Proxy = http.ProxyURL(proxyURL)
	case "socks", "socks5":
		socksDialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("invalid socks proxy: %w", err)
		}
		cloned.Proxy = nil
		var dialTimeout time.Duration
		if conf.AppConfig.UpstreamHTTP.DialTimeoutSec > 0 {
			dialTimeout = time.Duration(conf.AppConfig.UpstreamHTTP.DialTimeoutSec) * time.Second
		}
		cloned.DialContext = socksDialContext(socksDialer, dialTimeout)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}

	return &http.Client{Transport: cloned}, nil
}
