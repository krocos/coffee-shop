package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func CreateServer(port string) *http.Server {
	apiServerURL, err := url.Parse("http://localhost:8888")
	if err != nil {
		panic(err)
	}

	sseServerURL, err := url.Parse("http://localhost:7995")
	if err != nil {
		panic(err)
	}

	apiServer := httputil.NewSingleHostReverseProxy(apiServerURL)
	sseServer := httputil.NewSingleHostReverseProxy(sseServerURL)

	return &http.Server{
		Addr: port,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			switch {
			case strings.HasPrefix(r.URL.Path, "/user-api") ||
				strings.HasPrefix(r.URL.Path, "/payment-gateway-api") ||
				strings.HasPrefix(r.URL.Path, "/kitchen-api") ||
				strings.HasPrefix(r.URL.Path, "/cache-api"):

				apiServer.ServeHTTP(w, r)
			case strings.HasPrefix(r.URL.Path, "/user") ||
				strings.HasPrefix(r.URL.Path, "/kitchen") ||
				strings.HasPrefix(r.URL.Path, "/cache"):

				sseServer.ServeHTTP(w, r)
			default:
				http.DefaultServeMux.ServeHTTP(w, r)
			}
		}),
	}
}
