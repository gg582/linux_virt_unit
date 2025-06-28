package file_upload

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const uploadTempDir = "/tmp/incus_uploads" // Host temporary directory

// UploadTask holds data for asynchronous file push.
type UploadTask struct {
	HostTempFilePath         string
	ContainerName            string
	ContainerDestinationPath string
}

// UploadHandler processes file uploads. Saves to temp file, then queues for Incus push.
func UploadHandler(wr http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(wr, "POST method required.", http.StatusMethodNotAllowed)
		return
	}
	originalFilePath := req.Header.Get("X-File-Path")
	containerName := req.Header.Get("X-Container-Name")
	if originalFilePath == "" || containerName == "" {
		http.Error(wr, "Missing X-File-Path or X-Container-Name header.", http.StatusBadRequest)
		return
	}

	cleanContainerDestPath := filepath.Clean(originalFilePath)
	if !filepath.IsAbs(cleanContainerDestPath) {
		http.Error(wr, "File path must be absolute.", http.StatusBadRequest)
		return
	}

	// Create unique temporary file path on host
	tempFileName := fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(originalFilePath))
	hostTempFilePath := filepath.Join(uploadTempDir, tempFileName)

	if err := os.MkdirAll(uploadTempDir, 0755); err != nil {
		log.Printf("ERROR: Failed to create temp upload directory: %v", err)
		http.Error(wr, "Server error.", http.StatusInternalServerError)
		return
	}

	// Create and copy request body to temporary file (synchronous)
	outFile, err := os.Create(hostTempFilePath)
	if err != nil {
		log.Printf("ERROR: Failed to create temporary file: %v", err)
		http.Error(wr, "Server error.", http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	bytesWritten, err := io.Copy(outFile, req.Body)
	if err != nil {
		outFile.Close()
		os.Remove(hostTempFilePath)
		log.Printf("ERROR: Failed to copy request body to temp file: %v", err)
		http.Error(wr, "File transfer failed.", http.StatusInternalServerError)
		return
	}
	log.Printf("INFO: Received %d bytes, saved to %s.", bytesWritten, hostTempFilePath)

	// Enqueue task for asynchronous Incus push
	task := UploadTask{
		HostTempFilePath:         hostTempFilePath,
		ContainerName:            containerName,
		ContainerDestinationPath: cleanContainerDestPath,
	}
	EnqueueTask(task)
	log.Printf("INFO: Task enqueued for %s to %s.", originalFilePath, containerName)

	// Send immediate 202 Accepted response
	wr.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(wr, "File '%s' queued for processing on container '%s'.\n", originalFilePath, containerName)
}
