package util

import (
	"context"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Simple in-memory cookie jar keyed by host
type CookieJar struct {
	mu      sync.RWMutex
	cookies map[string]string
}

func NewJar() *CookieJar {
	return &CookieJar{
		cookies: make(map[string]string),
	}
}

func (j *CookieJar) Store(host string, setCookies []string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	for _, sc := range setCookies {
		parts := strings.SplitN(sc, ";", 2)
		if len(parts) > 0 {
			cookie := parts[0]
			if kv := strings.SplitN(cookie, "=", 2); len(kv) == 2 {
				key, val := kv[0], kv[1]
				// Store single cookie per host (can be extended)
				j.cookies[host] = key + "=" + val
			}
		}
	}
}

func (j *CookieJar) Load(host string) string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.cookies[host]
}

func CookieInterceptor(jar *CookieJar, host string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Attach stored cookie (if any)
		if cookie := jar.Load(host); cookie != "" {
			existingMD, ok := metadata.FromOutgoingContext(ctx)
			if !ok {
				existingMD = metadata.New(nil)
			}

			// Clone metadata and append the cookie header
			newMD := existingMD.Copy()
			newMD.Append("cookie", cookie)

			ctx = metadata.NewOutgoingContext(ctx, newMD)
		}

		// Capture response headers
		var header metadata.MD
		opts = append(opts, grpc.Header(&header))

		// Invoke actual RPC
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Store any Set-Cookie headers
		if setCookies := header.Get("set-cookie"); len(setCookies) > 0 {
			jar.Store(host, setCookies)
		}

		return err
	}
}
