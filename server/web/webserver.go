package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/gorilla/mux"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

// RoutedService defines the interface required for any service to register itself
// in the web server to provide new endpoints.
type RoutedService interface {
	RegisterRoutes(*mux.Router)
}

// Server is the structure responsible for managing the HTTP web server.
type Server struct {
	http.Server

	baseURL    string
	rootPath   string
	basePrefix string
	port       int
	ssl        bool
	logger     mlog.LoggerIFace
}

// NewServer creates a new instance of the web server.
func NewServer(rootPath string, serverRoot string, port int, ssl, localOnly bool, logger mlog.LoggerIFace) *Server {
	r := mux.NewRouter()

	basePrefix := os.Getenv("FOCALBOARD_HTTP_SERVER_BASEPATH")
	if basePrefix != "" {
		r = r.PathPrefix(basePrefix).Subrouter()
	}

	var addr string
	if localOnly {
		addr = fmt.Sprintf("localhost:%d", port)
	} else {
		addr = fmt.Sprintf(":%d", port)
	}

	baseURL := ""
	parsedURL, err := url.Parse(serverRoot)
	if err != nil {
		logger.Error("Invalid ServerRoot setting", mlog.Err(err))
	}
	baseURL = parsedURL.Path

	ws := &Server{
		// (TODO: Add ReadHeaderTimeout)
		Server: http.Server{ //nolint:gosec
			Addr:    addr,
			Handler: r,
		},
		baseURL:    baseURL,
		rootPath:   rootPath,
		port:       port,
		ssl:        ssl,
		logger:     logger,
		basePrefix: basePrefix,
	}

	return ws
}

// Router returns the current router.
func (ws *Server) Router() *mux.Router {
	return ws.Server.Handler.(*mux.Router)
}

// AddRoutes allows services to register themselves in the web server router
// and provide new endpoints.
func (ws *Server) AddRoutes(rs RoutedService) {
	rs.RegisterRoutes(ws.Router())
}

// serveFileWithCompression serves the requested file with compression support.
// It checks the 'Accept-Encoding' header and serves the appropriate compressed file
// (Brotli, Gzip, or Zstd) if available; otherwise, it falls back to the original file.
func (ws *Server) serveFileWithCompression(w http.ResponseWriter, r *http.Request, filePath string) {
	encodings := r.Header.Get("Accept-Encoding")

	// Check for Zstd encoding support (if applicable).
	if strings.Contains(encodings, "zstd") {
		zstFile := filePath + ".zst"
		if fileExists(zstFile) {
			w.Header().Set("Content-Encoding", "zstd")
			http.ServeFile(w, r, zstFile)
			return
		}
	}

	// Check for Brotli encoding support.
	if strings.Contains(encodings, "br") {
		brFile := filePath + ".br"
		if fileExists(brFile) {
			w.Header().Set("Content-Encoding", "br")
			http.ServeFile(w, r, brFile)
			return
		}
	}

	// Check for Gzip encoding support.
	if strings.Contains(encodings, "gzip") {
		gzFile := filePath + ".gz"
		if fileExists(gzFile) {
			w.Header().Set("Content-Encoding", "gzip")
			http.ServeFile(w, r, gzFile)
			return
		}
	}

	// Fallback: serve the uncompressed file.
	http.ServeFile(w, r, filePath)
}

// registerRoutes sets up the routes for serving static files, index page, WebSocket, and robots.txt.
func (ws *Server) registerRoutes() {
	// Route for /robots.txt with no cache.
	ws.Router().HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("User-agent: *\nDisallow: /"))
	})

	// Static files with compression support and 30-day cache control.
	ws.Router().PathPrefix("/static/").Handler(http.StripPrefix(ws.basePrefix+"/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set Cache-Control header for 30 days (2592000 seconds)
		w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
		// Derive the relative path from the URL.
		relativePath := strings.TrimPrefix(r.URL.Path, ws.basePrefix+"/static/")
		filePath := filepath.Join(ws.rootPath, "static", relativePath)
		ws.serveFileWithCompression(w, r, filePath)
	})))

	// Default route: serve the index.html file with no cache.
	ws.Router().PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set headers to prevent caching of index.html.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		indexTemplate, err := template.New("index").ParseFiles(path.Join(ws.rootPath, "index.html"))
		if err != nil {
			ws.logger.Log(errorOrWarn(), "Unable to serve the index.html file", mlog.Err(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = indexTemplate.ExecuteTemplate(w, "index.html", map[string]string{"BaseURL": ws.baseURL})
		if err != nil {
			ws.logger.Log(errorOrWarn(), "Unable to serve the index.html file", mlog.Err(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

// Start runs the web server and starts listening for connections.
func (ws *Server) Start() {
	ws.registerRoutes()
	if ws.port == -1 {
		ws.logger.Debug("Server not bound to any port")
		return
	}

	isSSL := ws.ssl && fileExists("./cert/cert.pem") && fileExists("./cert/key.pem")
	if isSSL {
		ws.logger.Info("HTTPS server started", mlog.Int("port", ws.port))
		go func() {
			if err := ws.ListenAndServeTLS("./cert/cert.pem", "./cert/key.pem"); err != nil {
				ws.logger.Fatal("ListenAndServeTLS", mlog.Err(err))
			}
		}()
		return
	}

	ws.logger.Info("HTTP server started", mlog.Int("port", ws.port))
	go func() {
		if err := ws.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			ws.logger.Fatal("ListenAndServe", mlog.Err(err))
		}
		ws.logger.Info("HTTP server stopped")
	}()
}

// Shutdown gracefully shuts down the server.
func (ws *Server) Shutdown() error {
	return ws.Close()
}

// fileExists returns true if a file exists at the given path.
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

// errorOrWarn returns a 'warn' level if the server instance is running unit tests,
// otherwise it returns 'error'.
func errorOrWarn() mlog.Level {
	unitTesting := strings.ToLower(strings.TrimSpace(os.Getenv("FOCALBOARD_UNIT_TESTING")))
	if unitTesting == "1" || unitTesting == "y" || unitTesting == "t" {
		return mlog.LvlWarn
	}
	return mlog.LvlError
}
