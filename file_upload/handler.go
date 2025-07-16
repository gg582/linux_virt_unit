package file_upload

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"time"
)

const uploadTempDir = "/tmp/incus_uploads" // Host temp directory for uploads

// uploadStreamRequest holds data for background file saving.
type uploadStreamRequest struct {
	Body             io.ReadCloser // Data stream (from io.PipeReader)
	OriginalFilePath string        // Target container path
	OriginalFilename string        // Base filename
	ContainerName    string        // Target container name
	HostTempFilePath string        // Unique temp path on host
	DoneChan         chan error    // Channel to signal completion or error
}

// uploadStreamChannel is a buffered channel for handing off file streams to workers.
var uploadStreamChannel chan *uploadStreamRequest

func init() {
	uploadStreamChannel = make(chan *uploadStreamRequest, 100) // Channel buffer size

	// Start 5 concurrent file saving workers.
	for i := 0; i < 5; i++ {
		go uploadStreamWorker()
	}

	log.Println("Upload Info: Stream workers and channel initialized.")
}

// UploadHandler processes file upload HTTP requests.
func UploadHandler(wr http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(wr, "POST method required.", http.StatusMethodNotAllowed)
		// net/http server will handle req.Body.Close() if not fully read.
		return
	}

	originalFilePath := req.Header.Get("X-File-Path")
	originalFilename := filepath.Base(req.Header.Get("X-Host-Path"))
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

	// Generate a unique temporary file path on the host.
	tempFileName := fmt.Sprintf("%d-%s", time.Now().UnixNano(), originalFilename)
	hostTempFilePath := filepath.Join(uploadTempDir, tempFileName)

	// Create an io.Pipe to safely transfer req.Body content to the worker.
	pr, pw := io.Pipe()

	// Create a channel to receive completion or error from the worker.
	doneChan := make(chan error, 1)

	// Create request for worker, passing the PipeReader and DoneChan.
	streamReq := &uploadStreamRequest{
		Body:             pr,
		OriginalFilePath: cleanContainerDestPath,
		OriginalFilename: originalFilename,
		ContainerName:    containerName,
		HostTempFilePath: hostTempFilePath,
		DoneChan:         doneChan,
	}

	// Start a goroutine to copy req.Body to the pipe writer.
	go func(reqBody io.ReadCloser) {
		defer pw.Close()      // Ensure PipeWriter is closed on completion or error.
		defer reqBody.Close() // Ensure original request body is closed.

		_, err := io.Copy(pw, reqBody)
		if err != nil && err != io.EOF {
			log.Printf("ERROR: Background copy from req.Body to pipe failed: %v", err)
			pw.CloseWithError(err)
		}
	}(req.Body) // Pass req.Body as an explicit argument to the goroutine.

	// Attempt to send request to worker channel.
	select {
	case uploadStreamChannel <- streamReq:
		// Wait for the worker to finish writing the file to temp storage.
		log.Printf("Upload Info: Request for '%s' to '%s' handed off to stream worker queue.", originalFilename, containerName)
		err := <-doneChan
		if err != nil {
			log.Printf("ERROR: Failed to write file '%s' to temp storage: %v", originalFilename, err)
			http.Error(wr, fmt.Sprintf("Failed to process file '%s': %v", originalFilename, err), http.StatusInternalServerError)
			return
		}
		// Add a 1-second delay to ensure server-side processing is complete.
		time.Sleep(1 * time.Second)
		wr.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(wr, "File '%s' queued for processing on container '%s'.\n", originalFilename, containerName)
		log.Printf("Upload Info: File '%s' successfully written to temp storage and queued for Incus push.", originalFilename)
	default:
		// If channel is full, server is busy.
		log.Printf("INFO: Upload stream channel is full, rejecting request for %s.", originalFilename)
		http.Error(wr, "Server is currently busy. Please try again later.", http.StatusServiceUnavailable)
		pr.Close() // Close the PipeReader if not sent to worker, to release resources.
		// req.Body ownership is with the goroutine, no direct close needed here.
	}
}
