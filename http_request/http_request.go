package http_request

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/gg582/linux_virt_unit"
	"github.com/gg582/linux_virt_unit/file_upload"
	"github.com/gg582/linux_virt_unit/incus_unit"
)

const (
	defaultCertFile = "certs/server.crt"
	defaultKeyFile  = "certs/server.key"
	listenAddr      = ":32000"
)

func resolveTLSFiles() (string, string, error) {
	certPath := filepath.Join(linux_virt_unit.LINUX_VIRT_PATH, defaultCertFile)
	keyPath := filepath.Join(linux_virt_unit.LINUX_VIRT_PATH, defaultKeyFile)

	if envCert, ok := os.LookupEnv("LINUX_VIRT_TLS_CERT"); ok && envCert != "" {
		certPath = envCert
	}
	if envKey, ok := os.LookupEnv("LINUX_VIRT_TLS_KEY"); ok && envKey != "" {
		keyPath = envKey
	}

	if err := ensureReadableFile(certPath); err != nil {
		return "", "", fmt.Errorf("certificate file: %w", err)
	}
	if err := ensureReadableFile(keyPath); err != nil {
		return "", "", fmt.Errorf("key file: %w", err)
	}

	return certPath, keyPath, nil
}

func ensureReadableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}

// InitHttpRequest initializes the HTTP request handler.
func InitHttpRequest() {
	router := mux.NewRouter()
	linux_virt_unit.LinuxVirtualizationAPIRouter = router

	// Register container related endpoints.
	router.HandleFunc("/register", incus_unit.Register).Methods(http.MethodPost)
	router.HandleFunc("/unregister", incus_unit.Unregister).Methods(http.MethodPost)
	router.HandleFunc("/create", incus_unit.CreateContainer).Methods(http.MethodPost)
	router.HandleFunc("/upload", file_upload.UploadHandler).Methods(http.MethodPost)
	router.HandleFunc("/request", incus_unit.GetContainers).Methods(http.MethodPost)
	router.HandleFunc("/delete", incus_unit.DeleteByTag).Methods(http.MethodPost)
	router.HandleFunc("/stop", incus_unit.ChangeStateHandler("stop")).Methods(http.MethodPost)
	router.HandleFunc("/start", incus_unit.ChangeStateHandler("start")).Methods(http.MethodPost)
	router.HandleFunc("/pause", incus_unit.ChangeStateHandler("freeze")).Methods(http.MethodPost)
	router.HandleFunc("/resume", incus_unit.ChangeStateHandler("unfreeze")).Methods(http.MethodPost)
	router.HandleFunc("/restart", incus_unit.ChangeStateHandler("restart")).Methods(http.MethodPost)
	router.HandleFunc("/images", incus_unit.GetImages).Methods(http.MethodGet)

	// Swagger UI setup.
	swaggerURL := "/docs/swagger.json"
	router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(httpSwagger.URL(swaggerURL)))

	router.HandleFunc("/docs/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		filePath := filepath.Join(linux_virt_unit.LINUX_VIRT_PATH, "docs", "swagger.json")
		http.ServeFile(w, r, filePath)
	})

	certFile, keyFile, err := resolveTLSFiles()
	if err != nil {
		log.Printf("InitHttpRequest: TLS configuration error: %v", err)
		return
	}

	srv := &http.Server{
		Handler:           router,
		Addr:              listenAddr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	log.Printf("Starting server on port %s", listenAddr)

	if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("HTTP server ListenAndServe error: %v", err)
	} else {
		log.Println("HTTP server stopped gracefully.")
	}
}
