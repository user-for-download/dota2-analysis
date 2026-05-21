package transport

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

func For(pu *url.URL, timeout time.Duration) (*http.Transport, error) {
	if pu == nil {
		return &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		}, nil
	}

	switch strings.ToLower(pu.Scheme) {
	case "http", "https", "":
		return &http.Transport{
			Proxy: http.ProxyURL(pu),
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		}, nil

	case "socks5", "socks5h":
		dialURL := *pu
		if strings.EqualFold(dialURL.Scheme, "socks5h") {
			dialURL.Scheme = "socks5"
		}

		baseDialer := &net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}
		dialer, err := xproxy.FromURL(&dialURL, baseDialer)
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer: %w", err)
		}

		ctxDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("socks5 dialer does not support context")
		}

		return &http.Transport{
			DisableKeepAlives: false,
			DialContext:       ctxDialer.DialContext,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %q", pu.Scheme)
	}
}