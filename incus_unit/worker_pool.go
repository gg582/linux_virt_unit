package incus_unit

// Task pool for container creation.
import (
    "sync"
    "github.com/yoonjin67/linux_virt_unit"
    "log"
)

type ContainerQueue struct {
    Tasks chan linux_virt_unit.ContainerInfo
    wg    sync.WaitGroup
}

var WorkQueue *ContainerQueue

// InitWorkQueue initializes the container work queue.
func InitWorkQueue() {
    WorkQueue = &ContainerQueue{
        Tasks: make(chan linux_virt_unit.ContainerInfo, 1024),
        wg:    sync.WaitGroup{},
    }
    log.Println("InitWorkQueue: Container work queue initialized.")
}

// Stop stops the worker goroutines.
func (q *ContainerQueue) Stop() {
    log.Println("Stop: Stopping worker goroutines.")
    close(q.Tasks)
    q.wg.Wait()
    log.Println("Stop: All worker goroutines stopped.")
}

// worker is the worker goroutine that processes container creation tasks.
func (q *ContainerQueue) worker() {
    defer q.wg.Done()
    log.Println("worker: Worker goroutine started.")
    for info := range q.Tasks {
        log.Println("worker: Received container creation task.")
        createContainer(info)
        log.Println("worker: Container creation task completed.")
    }
    log.Println("worker: Worker goroutine finished.")
}
