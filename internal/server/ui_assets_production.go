//go:build !development

package server

import (
	"bytes"
	"compress/gzip"
	"embed"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

const uiAssetsDevelopment = false

const (
	immutableUIAssetCacheControl = "public, max-age=31536000, immutable"
	shortUIAssetCacheControl     = "public, max-age=3600"
	minimumGzipUIAssetSize       = 1024
)

//go:embed ui/*
var uiAssets embed.FS

type cachedGzipUIAsset struct {
	once sync.Once
	data []byte
}

var gzipUIAssetCache sync.Map

func (s *Server) serveUIAsset(w http.ResponseWriter, r *http.Request, name string) bool {
	data, err := uiAssets.ReadFile("ui" + name)
	if err != nil {
		if path.Ext(name) != "" {
			return false
		}
		data, err = uiAssets.ReadFile("ui/index.html")
		if err != nil {
			return false
		}
		name = "/index.html"
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if w.Header().Get("Content-Type") == "" {
		if contentType := fallbackContentType(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
	}
	w.Header().Set("Cache-Control", uiAssetCacheControl(name))
	if shouldCompressUIAsset(name, data) {
		w.Header().Add("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(gzipUIAsset(name, data))
			return true
		}
	}
	_, _ = w.Write(data)
	return true
}

func gzipUIAsset(name string, data []byte) []byte {
	value, _ := gzipUIAssetCache.LoadOrStore(name, &cachedGzipUIAsset{})
	cached := value.(*cachedGzipUIAsset)
	cached.once.Do(func() {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		_, _ = writer.Write(data)
		_ = writer.Close()
		cached.data = compressed.Bytes()
	})
	return cached.data
}

func uiAssetCacheControl(name string) string {
	if name == "/index.html" {
		return "no-store"
	}
	if strings.HasPrefix(name, "/assets/") {
		return immutableUIAssetCacheControl
	}
	return shortUIAssetCacheControl
}

func shouldCompressUIAsset(name string, data []byte) bool {
	if len(data) < minimumGzipUIAssetSize {
		return false
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".css", ".html", ".js", ".json", ".mjs", ".svg":
		return true
	default:
		return false
	}
}

func acceptsGzip(header string) bool {
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "gzip") {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			keyValue := strings.SplitN(parameter, "=", 2)
			if len(keyValue) != 2 || !strings.EqualFold(strings.TrimSpace(keyValue[0]), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(keyValue[1]), 64)
			if err != nil {
				return false
			}
			quality = parsed
		}
		return quality > 0
	}
	return false
}

func fallbackContentType(ext string) string {
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".json":
		return "application/json"
	default:
		return ""
	}
}
