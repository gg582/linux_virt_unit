package http_request

import (
    "log"
    "net/http"
    "time"

    "path/filepath"
    "github.com/gorilla/mux"
    "github.com/yoonjin67/linux_virt_unit"
    "github.com/yoonjin67/linux_virt_unit/incus_unit"
    httpSwagger "github.com/swaggo/http-swagger/v2"
)

const certfile = "/usr/local/bin/linuxVirtualization/certs/server.crt"
const keyfile  = "/usr/local/bin/linuxVirtualization/certs/server.key"
const LVIRT_INSTALL_DIR = "/usr/local/bin/linuxVirtualization"

// InitHttpRequest initializes the HTTP request handler.
func InitHttpRequest() {
    linux_virt_unit.LinuxVirtualizationAPIRouter = mux.NewRouter()

    // Register container related endpoints.
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/register", incus_unit.Register).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/create", incus_unit.CreateContainer).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/request", incus_unit.GetContainers).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/delete", incus_unit.DeleteByTag).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/stop", incus_unit.StopByTag).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/start", incus_unit.StartByTag).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/pause", incus_unit.PauseByTag).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/resume", incus_unit.ResumeByTag).Methods("POST")
    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/restart", incus_unit.RestartByTag).Methods("POST")

    // Swagger UI setup.
    swaggerURL := "/docs/swagger.json"
    linux_virt_unit.LinuxVirtualizationAPIRouter.PathPrefix("/swagger/").Handler(httpSwagger.Handler(httpSwagger.URL(swaggerURL)))

    // Redirect root to Swagger UI.

    linux_virt_unit.LinuxVirtualizationAPIRouter.HandleFunc("/docs/swagger.json", func(w http.ResponseWriter, r *http.Request) {
        filePath := filepath.Join(LVIRT_INSTALL_DIR, "docs", "swagger.json")
        http.ServeFile(w, r, filePath)
    })

    // Server configuration.
    srv := &http.Server{
        Handler:     linux_virt_unit.LinuxVirtualizationAPIRouter,
        Addr:    ":443",
        ReadTimeout: 15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout: 60 * time.Second,
    }

    log.Printf("Starting server on port 443: for gh deploy")

    // Start HTTPS server.
    if err := srv.ListenAndServeTLS(certfile, keyfile); err != nil && err != http.ErrServerClosed {
        log.Printf("HTTP server ListenAndServe error: %v", err)
    } else {
        log.Println("HTTP server stopped gracefully.")
    }
}
