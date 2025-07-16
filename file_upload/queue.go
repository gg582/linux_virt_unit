package file_upload

import (
	"log"
	"sync"
)

// UploadTask holds data to push a file from host temp storage to an Incus container.
type UploadTask struct {
	HostTempFilePath         string // Path to the temp file on the host
	ContainerName            string // Target Incus container name
	HostFilename             string // Original filename from host
	ContainerDestinationPath string // Absolute destination path in container
}

var taskQueue chan UploadTask
var once sync.Once

// InitWorkQueue initializes the in-memory task queue. Call once during application startup.
func InitWorkQueue() {
	once.Do(func() {
		taskQueue = make(chan UploadTask, 100) // Queue buffer size
		log.Println("Upload Info: Work queue initialized.")
	})
}

// EnqueueTask adds an UploadTask to the queue. Non-blocking if capacity available.
func EnqueueTask(task UploadTask) {
	if taskQueue == nil {
		log.Fatal("ERROR: Task queue not initialized. Call InitWorkQueue() first.")
	}
	taskQueue <- task
	log.Printf("Upload Info: Queue: Task enqueued. Size: %d", len(taskQueue))
}

// DequeueTask retrieves an UploadTask from the queue, blocking if empty.
func DequeueTask() UploadTask {
	if taskQueue == nil {
		log.Fatal("ERROR: Task queue not initialized.")
	}
	task := <-taskQueue
	log.Printf("Upload Info: Queue: Task dequeued. Size: %d", len(taskQueue))
	return task
}

// GetQueueLength returns current queue size.
func GetQueueLength() int {
	if taskQueue == nil {
		return 0
	}
	return len(taskQueue)
}
