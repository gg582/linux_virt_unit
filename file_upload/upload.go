package file_upload

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	client "github.com/lxc/incus/client"         // For client.InstanceFileArgs
	incusapi "github.com/lxc/incus/shared/api" // For incusapi.InstanceExecPost and api.Frozen
	"github.com/yoonjin67/linux_virt_unit/incus_unit" // Assuming this provides IncusCli
)

const uploadDir = "/tmp" // Temporary directory for received files on host

// UploadHandler handles file uploads to Incus containers.
// It expects X-File-Path (destination path in container) and X-Container-Name headers.
func UploadHandler(wr http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(wr, "POST only. Aborting.", http.StatusMethodNotAllowed)
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

	// Clean the path (e.g., /a/./b becomes /a/b, /a/../b becomes /b)
	cleanFilePath := filepath.Clean(originalFilePath)

	// Validate: The path must be absolute (start with /).
	// filepath.Clean resolves ".." components, so if it's absolute after Clean, it's generally safe from traversal.
	if !filepath.IsAbs(cleanFilePath) {
		http.Error(wr, "File path must be absolute (e.g., /home/user/file.txt)", http.StatusBadRequest)
		return
	}
	
	// Use only the filename for the temporary path on the host to avoid deep nesting in /tmp.
	// The original directory structure will be recreated inside the container.
	tmpFilePath := filepath.Join(uploadDir, filepath.Base(cleanFilePath))

	// Ensure the temporary directory on the host exists
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
	defer os.Remove(tmpFilePath) // Clean up temp file

	bytesWritten, err := io.Copy(outFile, req.Body)
	if err != nil {
		log.Printf("Failed to write temp file '%s': %v", tmpFilePath, err)
		http.Error(wr, "Server error", http.StatusInternalServerError)
		return
	}
	log.Printf("Temp file saved on host: %s (%d bytes)", tmpFilePath, bytesWritten)

	// --- Incus Integration ---
	// Get the Incus container instance
	container, _, err := incus_unit.IncusCli.GetInstance(containerName)
	if err != nil {
		log.Printf("Incus: Failed to get container '%s': %v", containerName, err)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' not found.", containerName), http.StatusInternalServerError)
		return
	}

	// Check container state
	if container.Status != "Running" {
		log.Printf("Container '%s' not running: %s", containerName, container.Status)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' not running.", containerName), http.StatusBadRequest)
		return
	}
	if container.StatusCode == incusapi.Frozen {
		log.Printf("Container '%s' is frozen.", containerName)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' is frozen.", containerName), http.StatusBadRequest)
		return
	}

	// Read file content for Incus push
	fileBytes, err := os.ReadFile(tmpFilePath)
	if err != nil {
		log.Printf("Failed to read temp file for push to container: %v", err)
		http.Error(wr, "Server error: failed to read file for push", http.StatusInternalServerError)
		return
	}
	fileReader := bytes.NewReader(fileBytes)

	// Use the cleanFilePath directly as the destination path inside the container.
	// It's already validated to be an absolute path by the above logic.
	containerDestPath := cleanFilePath 
	
	// Get the directory part of the target path inside the container.
	// This directory will be created if it doesn't exist.
	containerDestDir := filepath.Dir(containerDestPath)
	
	// Execute mkdir -p inside the Incus container to ensure the destination directory exists.
	// ExecInstance returns an Operation, which needs to be waited upon for completion.
	op, err := incus_unit.IncusCli.ExecInstance(containerName, incusapi.InstanceExecPost{
		Command: []string{"mkdir", "-p", containerDestDir},
		WaitForWS: true, // Wait for the command to complete
	}, nil)

	if err != nil {
		log.Printf("Incus: Failed to prepare mkdir command for container '%s': %v", containerName, err)
		http.Error(wr, fmt.Sprintf("Incus: Failed to prepare directory creation command. Error: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for the mkdir operation to complete and check its result.
	err = op.Wait()
	if err != nil {
		// op.Wait() returns an error if the command itself fails (e.g., permission denied)
		log.Printf("Incus: mkdir command failed in container '%s' for dir '%s': %v", containerName, containerDestDir, err)
		http.Error(wr, fmt.Sprintf("Incus: Failed to create container directory '%s'. Error: %v", containerDestDir, err), http.StatusInternalServerError)
		return
	}
	
	log.Printf("Container directory created/ensured: %s", containerDestDir)

    destInfo, err := os.Stat(containerDestPath)
    if destInfo.IsDir() {
        containerDestPath = filepath.Join(containerDestPath,filepath.Base(originalFilePath))
    }
	// Push file into container with 777 permissions
	err = incus_unit.IncusCli.CreateInstanceFile(containerName, containerDestPath, client.InstanceFileArgs{
		Content: fileReader,
		Mode:    0777, // Permissions: rwxrwxrwx
		UID:     0,    // Owner: root
		GID:     0,    // Group: root
	})

	if err != nil {
		log.Printf("Incus push failed for container '%s' to '%s': %v", containerName, containerDestPath, err)
		http.Error(wr, fmt.Sprintf("Incus: File push failed. Error: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("File pushed to Incus container '%s' at '%s'", containerName, containerDestPath)
	fmt.Fprintf(wr, "File '%s' uploaded and pushed to container '%s' at '%s'.", originalFilePath, containerName, containerDestPath)
}
