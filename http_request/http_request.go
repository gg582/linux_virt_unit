package http_request

import (
    "log"
    "net/http"
    "time"

    "github.com/gorilla/mux"
    httpSwagger "github.com/swaggo/http-swagger/v2"
    "github.com/gg582/linux_virt_unit"
    "github.com/gg582/linux_virt_unit/incus_unit"
    "github.com/gg582/linux_virt_unit/file_upload"
    "path/filepath"
)

var certfile = linux_virt_unit.LINUX_VIRT_PATH+"/certs/server.crt"
var keyfile = linux_virt_unit.LINUX_VIRT_PATH+"/certs/server.key"

// InitHttpRequest initializes the HTTP request handler.
func InitHttpRequest() {
    linux_virt_unit.LinuxVirtualizationAPIRouter = mux.NewRouter()

    // Register container related endpoints.
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/register", incus_unit.Register).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/unregister", incus_unit.Unregister).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/create", incus_unit.CreateContainer).Methods("POST")
	linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/upload", file_upload.UploadHandler).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/request", incus_unit.GetContainers).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/delete", incus_unit.DeleteByTag).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/stop", incus_unit.ChangeStateHandler("stop")).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/start", incus_unit.ChangeStateHandler("start")).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/pause", incus_unit.ChangeStateHandler("freeze")).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/resume", incus_unit.ChangeStateHandler("unfreeze")).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/restart", incus_unit.ChangeStateHandler("restart")).Methods("POST")

    // Swagger UI setup.
    swaggerURL := "/docs/swagger.json"
    linux_virt_unit.LinuxVirtualizationAPIRouter.PathPrefix("/swagger/").Handler(httpSwagger.Handler(httpSwagger.URL(swaggerURL)))

    // Redirect root to Swagger UI.

    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/docs/swagger.json", func(w http.ResponseWriter, r *http.Request) {
        filePath := filepath.Join(linux_virt_unit.LINUX_VIRT_PATH, "docs", "swagger.json")
        http.ServeFile(w, r, filePath)
    })

    // Server configuration.
    srv := &http.Server{
        Handler:      linux_virt_unit.LinuxVirtualizationAPIRouter,
        Addr:         ":32000",
        ReadTimeout:  60 * time.Minute,
        WriteTimeout: 60 * time.Minute,
        IdleTimeout:  60 * time.Minute,
    }

    log.Printf("Starting server on port 32000")

    // Start HTTPS server.
    if err := srv.ListenAndServeTLS(certfile, keyfile); err != nil && err != http.ErrServerClosed {
        log.Printf("HTTP server ListenAndServe error: %v", err)
    } else {
        log.Println("HTTP server stopped gracefully.")
    }
}
