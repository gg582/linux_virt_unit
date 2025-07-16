package file_upload

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	client "github.com/lxc/incus/client"
	"github.com/lxc/incus/shared/api"
	"github.com/gg582/linux_virt_unit/incus_unit"
)

const (
	MaxRetries = 3          // Max retries for Incus push operations
	RetryDelay = 5 * time.Second // Delay between retries
)

// uploadStreamWorker saves incoming file streams to temporary host files.
func uploadStreamWorker() {
	// Ensure temp upload directory exists.
	if err := os.MkdirAll(uploadTempDir, 0755); err != nil {
		log.Fatalf("FATAL: Worker: Failed to create temp upload directory: %v", err)
	}

	for streamReq := range uploadStreamChannel {
		log.Printf("Worker: Saving stream for %s to %s", streamReq.ContainerName, streamReq.HostTempFilePath)

		// Create temporary file on host.
		outFile, err := os.Create(streamReq.HostTempFilePath)
		if err != nil {
			log.Printf("ERROR: Worker: Failed to create temp file %s: %v", streamReq.HostTempFilePath, err)
			streamReq.DoneChan <- err
			streamReq.Body.Close() // Explicitly close PipeReader on file creation error.
			continue
		}

		// Copy data from the PipeReader to the temp file.
		bytesWritten, err := io.Copy(outFile, streamReq.Body)
		if err != nil {
			log.Printf("ERROR: Worker: Failed to copy body to temp file %s: %v", streamReq.HostTempFilePath, err)
			outFile.Close()
			os.Remove(streamReq.HostTempFilePath) // Clean up partially written file
			streamReq.DoneChan <- err
			streamReq.Body.Close()
			continue
		}

		// Ensure the file is fully written and synced to disk.
		if err := outFile.Sync(); err != nil {
			log.Printf("ERROR: Worker: Failed to sync temp file %s: %v", streamReq.HostTempFilePath, err)
			outFile.Close()
			os.Remove(streamReq.HostTempFilePath) // Clean up partially written file
		 streamReq.DoneChan <- err
			streamReq.Body.Close()
			continue
		}

		// Close the file and signal completion.
		outFile.Close()
		streamReq.Body.Close()
		streamReq.DoneChan <- nil
		log.Printf("Worker: Saved %d bytes for %s to %s.", bytesWritten, streamReq.OriginalFilename, streamReq.HostTempFilePath)

		// Create and enqueue task for Incus push.
		task := UploadTask{
			HostTempFilePath:         streamReq.HostTempFilePath,
			ContainerName:            streamReq.ContainerName,
			HostFilename:             streamReq.OriginalFilename,
			ContainerDestinationPath: streamReq.OriginalFilePath,
		}
		EnqueueTask(task)
		log.Printf("Worker: UploadTask enqueued for Incus push: %s to %s.", streamReq.OriginalFilename, streamReq.ContainerName)
	}
}

// StartFilePushWorker processes upload tasks from the queue and pushes files to Incus.
func StartFilePushWorker() {
	for {
		task := DequeueTask() // Blocks until a task is available
		log.Printf("Upload Info: Incus push worker processing task for %s from %s.", task.ContainerName, task.HostFilename)

		// Defer cleanup of the temp file after Incus push.
		defer func(filePath string) {
			if err := os.Remove(filePath); err != nil {
				log.Printf("ERROR: Incus push worker: Failed to remove temp file '%s': %v", filePath, err)
			} else {
				log.Printf("Upload Info: Incus push worker: Cleaned up temp file: %s", filePath)
			}
		}(task.HostTempFilePath)

		// Process task with retries.
		for i := 0; i <= MaxRetries; i++ {
			err := processUploadTask(task)
			if err == nil {
				log.Printf("SUCCESS: Incus push worker: Task completed for %s.", task.ContainerName)
				break
			}

			// Check for transient errors to retry.
			isTransient := true
			if err.Error() == "incus: container not found" {
				isTransient = false
			}

			if isTransient && i < MaxRetries {
				log.Printf("WARNING: Incus push worker: Task failed for %s (attempt %d/%d): %v. Retrying.", task.ContainerName, i+1, MaxRetries, err)
				time.Sleep(RetryDelay)
			} else {
				log.Printf("ERROR: Incus push worker: Task permanently failed for %s after %d attempts: %v.", task.ContainerName, i+1, err)
				break
			}
		}
	}
}

// processUploadTask performs the actual Incus file push.
func processUploadTask(task UploadTask) error {
	// Open temp file on host.
	file, err := os.Open(task.HostTempFilePath)
	if err != nil {
		return fmt.Errorf("failed to open temp file '%s': %w", task.HostTempFilePath, err)
	}
	defer file.Close()

	// Get container and check its state.
	container, _, err := incus_unit.IncusCli.GetInstance(task.ContainerName)
	if err != nil {
		return fmt.Errorf("incus: failed to get container '%s': %w", task.ContainerName, err)
	}
	if container.Status != "Running" || container.StatusCode == api.Frozen {
		return fmt.Errorf("incus: container '%s' not ready: status %s", task.ContainerName, container.Status)
	}

	// Ensure destination directory exists in container.
	containerDestDir := filepath.Dir(task.ContainerDestinationPath)
	op, err := incus_unit.IncusCli.ExecInstance(task.ContainerName, api.InstanceExecPost{
		Command:   []string{"mkdir", "-p", containerDestDir},
		WaitForWS: true,
	}, nil)
	if err != nil {
		return fmt.Errorf("incus: failed to prepare mkdir for '%s': %w", task.ContainerName, err)
	}
	if err = op.Wait(); err != nil {
		return fmt.Errorf("incus: mkdir failed in '%s' for '%s': %w", task.ContainerName, containerDestDir, err)
	}
	log.Printf("Upload Info: Directory '%s' ensured in container '%s'.", containerDestDir, task.ContainerName)

	// Determine final destination path in container.
	finalContainerDestPath := task.ContainerDestinationPath
	// Check if the target path has an extension, indicating it's a file.
	if strings.Contains(filepath.Base(task.ContainerDestinationPath), ".") {
		log.Printf("Upload Info: Target path '%s' treated as a file path.", task.ContainerDestinationPath)
	} else if IsDirectoryInContainer(task.ContainerName, task.ContainerDestinationPath) {
		log.Printf("Upload Info: Target path '%s' is a directory, using original filename '%s'.", task.ContainerDestinationPath, task.HostFilename)
		finalContainerDestPath = filepath.Join(task.ContainerDestinationPath, task.HostFilename)
	} else {
		log.Printf("Upload Info: Target path '%s' treated as a file path.", task.ContainerDestinationPath)
	}

	// Push file to container.
	err = incus_unit.IncusCli.CreateInstanceFile(task.ContainerName, finalContainerDestPath, client.InstanceFileArgs{
		Content: file,
		Mode:    0777,
		UID:     0,
		GID:     0,
	})
	if err != nil {
		return fmt.Errorf("incus: file push failed for '%s' to '%s': %w", task.ContainerName, finalContainerDestPath, err)
	}

	log.Printf("Upload Info: File pushed to Incus container '%s' at '%s'.", task.ContainerName, finalContainerDestPath)
	return nil
}

// IsDirectoryInContainer checks if the given path is a directory in the container.
func IsDirectoryInContainer(containerName, path string) bool {
	op, err := incus_unit.IncusCli.ExecInstance(containerName, api.InstanceExecPost{
		Command:   []string{"test", "-d", path},
		WaitForWS: true,
	}, nil)
	if err != nil {
		log.Printf("ERROR: Failed to check if '%s' is a directory in '%s': %v", path, containerName, err)
		return false
	}
	err = op.Wait()
	if err != nil {
		log.Printf("Upload Info: Path '%s' is not a directory in '%s'.", path, containerName)
		return false
	}
	log.Printf("Upload Info: Path '%s' is a directory in '%s'.", path, containerName)
	return true
}
