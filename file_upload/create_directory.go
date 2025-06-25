package file_upload

import (
	"fmt"
	"io"
    "bytes"
    "github.com/lxc/incus/shared/api"
<<<<<<< HEAD
    "github.com/lxc/incus/client"
=======
    client "github.com/lxc/incus/client"
    "github.com/yoonjin67/linux_virt_unit/incus_unit"
>>>>>>> 33f3d43 (added file push function;not ready yet)
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const uploadDir = "/tmp"


<<<<<<< HEAD
func uploadHandler(wr http.ResponseWriter, req *http.Request) {
=======
func UploadHandler(wr http.ResponseWriter, req *http.Request) {
>>>>>>> 33f3d43 (added file push function;not ready yet)

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

<<<<<<< HEAD
	bytesWritten, err := io.Copy(outFile, r.Body)
=======
	bytesWritten, err := io.Copy(outFile, req.Body)
>>>>>>> 33f3d43 (added file push function;not ready yet)
	if err != nil {
		log.Printf("Failed to write temp file '%s': %v", tmpFilePath, err)
		http.Error(wr, "Server error", http.StatusInternalServerError)
		return
	}
	log.Printf("Temp saved: %s (%d bytes)", tmpFilePath, bytesWritten)

<<<<<<< HEAD
	client, err := incus.ConnectInCusUnix("", nil)
	if err != nil {
		log.Printf("Incus connect fail: %v", err)
		http.Error(wr, "Server error: Incus connect", http.StatusInternalServerError)
		return
	}

	container, _, err := client.GetContainer(containerName)
=======
	container, _, err := incus_unit.IncusCli.GetInstance(containerName)
>>>>>>> 33f3d43 (added file push function;not ready yet)
	if err != nil {
		log.Printf("Incus get container '%s' fail: %v", containerName, err)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' not found.", containerName), http.StatusInternalServerError)
		return
	}

<<<<<<< HEAD
	if container.RunningState != "Running" {
		log.Printf("Container '%s' not running: %s", containerName, container.RunningState)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' not running.", containerName), http.StatusBadRequest)
		return
	}
	if container.State.FreezeState == "frozen" {
=======
	if container.Status != "Running" {
		log.Printf("Container '%s' not running: %s", containerName, container.Status)
		http.Error(wr, fmt.Sprintf("Incus: Container '%s' not running.", containerName), http.StatusBadRequest)
		return
	}
	if container.StatusCode == api.Frozen {
>>>>>>> 33f3d43 (added file push function;not ready yet)
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
	
<<<<<<< HEAD
	_, _, stderr, err := client.ExecContainer(containerName, api.ContainerExecPost{
		Command: []string{"mkdir", "-p", filepath.Dir(containerDestPath)},
	}, nil)

	if err != nil || stderr.String() != "" {
		log.Printf("Container mkdir fail: %v, Stderr: %s", err, stderr.String())
		http.Error(wr, fmt.Sprintf("Incus: Container mkdir fail. Error: %v, Stderr: %s", err, stderr.String()), http.StatusInternalServerError)
=======
	//stderr, err := incus_unit.IncusCli.ExecInstance(containerName, api.InstanceExecPost{
	//Command: []string{"mkdir", "-p", filepath.Dir(containerDestPath)},
    //Interactive: false,
    //Width: 0,
    //Height: 0}, nil)
    dir := filepath.Join(containerDestPath, containerName)
    err = os.MkdirAll(dir, os.FileMode(os.O_CREATE) | os.FileMode(os.O_RDWR))

	if err != nil {
        log.Printf("Container mkdir fail: %v, Error: ", err)
        http.Error(wr, fmt.Sprintf("Incus: Container mkdir fail. Error: %v", err), http.StatusInternalServerError)
>>>>>>> 33f3d43 (added file push function;not ready yet)
		return
	}
	log.Printf("Container dir created: %s", filepath.Dir(containerDestPath))

<<<<<<< HEAD
	err = client.CreateContainerFile(containerName, containerDestPath, api.ContainerFileArgs{
=======
	err = incus_unit.IncusCli.CreateInstanceFile(containerName, containerDestPath, client.InstanceFileArgs{
>>>>>>> 33f3d43 (added file push function;not ready yet)
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

