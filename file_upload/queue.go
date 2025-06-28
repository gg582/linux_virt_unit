package file_upload

import (
	"log"
	"sync"
)

var taskQueue chan UploadTask
var once sync.Once

// InitWorkQueue initializes the in-memory task queue.
func InitWorkQueue() {
	once.Do(func() {
		taskQueue = make(chan UploadTask, 100)
		log.Println("INFO: Work queue initialized.")
	})
}

// EnqueueTask adds an UploadTask to the queue.
func EnqueueTask(task UploadTask) {
	if taskQueue == nil {
		log.Fatal("ERROR: Task queue not initialized.")
	}
	taskQueue <- task
	log.Printf("INFO: Queue: Task enqueued. Size: %d", len(taskQueue))
}

// DequeueTask retrieves an UploadTask from the queue, blocking if empty.
func DequeueTask() UploadTask {
	if taskQueue == nil {
		log.Fatal("ERROR: Task queue not initialized.")
	}
	task := <-taskQueue
	log.Printf("INFO: Queue: Task dequeued. Size: %d", len(taskQueue))
	return task
}

// GetQueueLength returns current queue size.
func GetQueueLength() int {
	if taskQueue == nil {
		return 0
	}
	return len(taskQueue)
}
