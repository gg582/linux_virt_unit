package file_upload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	client "github.com/lxc/incus/client"
	incusapi "github.com/lxc/incus/shared/api"
	"github.com/gg582/linux_virt_unit/incus_unit"
)

const (
	MaxRetries = 3
	RetryDelay = 5 * time.Second
)

// StartFilePushWorker processes upload tasks from the queue.
func StartFilePushWorker() {
	for {
		task := DequeueTask()
		log.Printf("INFO: Worker processing task for %s from %s.", task.ContainerName, task.HostTempFilePath)

		// Defer cleanup of the temporary file
		defer func(filePath string) {
			if err := os.Remove(filePath); err != nil {
				log.Printf("ERROR: Worker: Failed to remove temp file '%s': %v", filePath, err)
			} else {
				log.Printf("INFO: Worker: Cleaned up temp file: %s", filePath)
			}
		}(task.HostTempFilePath)

		// Process task with retries for transient Incus errors
		for i := 0; i <= MaxRetries; i++ {
			err := processUploadTask(task)
			if err == nil {
				log.Printf("SUCCESS: Worker: Task completed for %s.", task.ContainerName)
				break
			}

			isTransient := true
			if err.Error() == "incus: container not found" { // Example permanent error
				isTransient = false
			}

			if isTransient && i < MaxRetries {
				log.Printf("WARNING: Worker: Task failed for %s (attempt %d/%d): %v. Retrying.",
					task.ContainerName, i+1, MaxRetries, err)
				time.Sleep(RetryDelay)
			} else {
				log.Printf("ERROR: Worker: Task permanently failed for %s after %d attempts: %v.",
					task.ContainerName, i+1, err)
				break
			}
		}
	}
}

// processUploadTask performs the actual Incus file push.
func processUploadTask(task UploadTask) error {
	// Open temporary file
	file, err := os.Open(task.HostTempFilePath)
	if err != nil {
		return fmt.Errorf("failed to open temp file '%s': %w", task.HostTempFilePath, err)
	}
	defer file.Close()

	// Get container and check state
	container, _, err := incus_unit.IncusCli.GetInstance(task.ContainerName)
	if err != nil {
		return fmt.Errorf("incus: failed to get container '%s': %w", task.ContainerName, err)
	}
	if container.Status != "Running" || container.StatusCode == incusapi.Frozen {
		return fmt.Errorf("incus: container '%s' not ready: status %s", task.ContainerName, container.Status)
	}

	// Ensure destination directory exists in container
	containerDestDir := filepath.Dir(task.ContainerDestinationPath)
	if containerDestDir == "." {
		containerDestDir = "/"
	}
	op, err := incus_unit.IncusCli.ExecInstance(task.ContainerName, incusapi.InstanceExecPost{
		Command:   []string{"mkdir", "-p", containerDestDir},
		WaitForWS: true,
	}, nil)
	if err != nil {
		return fmt.Errorf("incus: failed to prepare mkdir for '%s': %w", task.ContainerName, err)
	}
	if err = op.Wait(); err != nil {
		return fmt.Errorf("incus: mkdir failed in '%s' for '%s': %w", task.ContainerName, containerDestDir, err)
	}
	log.Printf("INFO: Directory '%s' ensured in container '%s'.", containerDestDir, task.ContainerName)

    hostFilename := filepath.Base(task.HostFilename)
    containerDestDir = filepath.Join(containerDestDir, hostFilename)
	// Push file to container (os.File implements io.ReadSeeker)
	err = incus_unit.IncusCli.CreateInstanceFile(task.ContainerName, containerDestDir, client.InstanceFileArgs{
		Content: file,
		Mode:    0777,
		UID:     0,
		GID:     0,
	})
	if err != nil {
		return fmt.Errorf("incus: file push failed for '%s' to '%s': %w", task.ContainerName, task.ContainerDestinationPath, err)
	}

	log.Printf("INFO: File pushed to Incus container '%s' at '%s'.", task.ContainerName, task.ContainerDestinationPath)
	return nil
}

// IsDirectoryInContainer is a placeholder for actual directory check.
func IsDirectoryInContainer(containerName, path string) bool {
	return false
}
