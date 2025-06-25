package file_upload

import (
	"fmt"
	"io"
    "bytes"
    "github.com/lxc/incus/shared/api"
    "github.com/lxc/incus/client"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const uploadDir = "/tmp"


func uploadHandler(wr http.ResponseWriter, req *http.Request) {

    if req.Method != http.MethodPost {
        http.Error(wr, "This endpoint allows only POST methods. aborting", http.StatusMethodNotAllowed)
        return
    }

	originalFilePath := req.Header.Get("X-File-Path")
	if originalFilePath == "" {
		http.Error(wr, "X-File-Path header missing", http.StatusBadRequest)
		return
	}

	containerName := req.Header.Get("X-Container-Name")
	if containerName == "" {
		http.Error(wr, "X-Container-Name header missing", http.StatusBadRequest)
		return
	}

	cleanFilePath := filepath.Clean(originalFilePath)
	if filepath.IsAbs(cleanFilePath) || strings.HasPrefix(cleanFilePath, "..") {
		http.Error(wr, "Invalid file path", http.StatusBadRequest)
		return
	}

	tmpFilePath := filepath.Join(uploadDir, cleanFilePath)

	err := os.MkdirAll(filepath.Dir(tmpFilePath), 0755)
	if err != nil {
		log.Printf("Failed to create temp dir: %v", err)
		http.Error(wr, "Server error", http.StatusInternalServerError)
		return
	}

	outFile, err := os.Create(tmpFilePath)
	if err != nil {
		log.Printf("Failed to create temp file '%s': %v", tmpFilePath, err)
		http.Error(wr, "Server error", http.StatusInternalServerError)
		return
	}
	defer outFile.Close()
	defer os.Remove(tmpFilePath)

	bytesWritten, err := io.Copy(outFile, r.Body)
	if err != nil {
		log.Printf("Failed to write temp file '%s': %v", tmpFilePath, err)
		http.Error(wr, "Server error", http.StatusInternalServerError)
		return
	}
	log.Printf("Temp saved: %s (%d bytes)", tmpFilePath, bytesWritten)

	client, err := incus.ConnectInCusUnix("", nil)
	if err != nil {
		log.Printf("Incus connect fail: %v", err)
		http.Error(wr, "Server error: Incus connect", http.StatusInternalServerError)
		return
	}

	container, _, err := client.GetContainer(containerName)
	if err != nil {
		log.Printf("Incus get container '%s' fail: %v", containerName, err)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' not found.", containerName), http.StatusInternalServerError)
		return
	}

	if container.RunningState != "Running" {
		log.Printf("Container '%s' not running: %s", containerName, container.RunningState)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' not running.", containerName), http.StatusBadRequest)
		return
	}
	if container.State.FreezeState == "frozen" {
		log.Printf("Container '%s' is frozen.", containerName)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' is frozen.", containerName), http.StatusBadRequest)
		return
	}

	fileBytes, err := os.ReadFile(tmpFilePath)
	if err != nil {
		log.Printf("Failed to read temp file for push: %v", err)
		http.Error(wr, "Server error: read file for push", http.StatusInternalServerError)
		return
	}
	fileReader := bytes.NewReader(fileBytes)

	containerDestPath := filepath.Join("/root", cleanFilePath)
	
	_, _, stderr, err := client.ExecContainer(containerName, api.ContainerExecPost{
		Command: []string{"mkdir", "-p", filepath.Dir(containerDestPath)},
	}, nil)

	if err != nil || stderr.String() != "" {
		log.Printf("Container mkdir fail: %v, Stderr: %s", err, stderr.String())
		http.Error(wr, fmt.Sprintf("Incus: Container mkdir fail. Error: %v, Stderr: %s", err, stderr.String()), http.StatusInternalServerError)
		return
	}
	log.Printf("Container dir created: %s", filepath.Dir(containerDestPath))

	err = client.CreateContainerFile(containerName, containerDestPath, api.ContainerFileArgs{
		Content: fileReader,
		Mode:    0644,
		UID:     0,
		GID:     0,
	})

	if err != nil {
		log.Printf("Incus push fail: %v", err)
		http.Error(wr, fmt.Sprintf("Incus: File push fail. Error: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Pushed to Incus '%s' at '%s'", containerName, containerDestPath)
	fmt.Fprintf(wr, "File '%s' uploaded and pushed to container '%s' at '%s'.", originalFilePath, containerName, containerDestPath)
}

