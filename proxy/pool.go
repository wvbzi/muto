package proxy

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type ProxyPool struct {
	Proxies chan url.URL
}

// Initiates and returns a ProxyPool object with 3 proxies pulled from the env.
func InitProxyPool() (*ProxyPool, error) {
	proxy1 := os.Getenv("PROXY_1")
	proxy2 := os.Getenv("PROXY_2")
	proxy3 := os.Getenv("PROXY_3")
	if proxy1 == "" || proxy2 == "" || proxy3 == "" {
		return nil, fmt.Errorf("PROXY_# env variables not set")
	}

	//Buffered channel implementation to limit 3 concurrent downloads at a time.
	proxies := [3]string{proxy1, proxy2, proxy3}
	pool := ProxyPool{
		Proxies: make(chan url.URL, 3),
	}
	for _, proxy := range proxies {
		pSplit := strings.Split(proxy, ":")
		if len(pSplit) != 4 {
			return nil, fmt.Errorf("Improper proxy format, please use host:port:user:password format")
		}
		proxyFormatted := fmt.Sprintf("http://%s:%s@%s:%s", pSplit[2], pSplit[3], pSplit[0], pSplit[1])

		newProxy, err := url.Parse(proxyFormatted)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse proxy: %s", err)
		}
		pool.Proxies <- *newProxy
	}

	return &pool, nil
}

// Checks if a proxy is available in the pool, if so, returns proxy URL and true.
func (p *ProxyPool) ProxyAvailable() (url.URL, bool) {
	select {
	case proxy := <-p.Proxies:
		return proxy, true
	default:
		return url.URL{}, false
	}
}

// Adds the proxy back to the pool.
func (p *ProxyPool) MakeProxyAvailable(proxy url.URL) {
	p.Proxies <- proxy
}