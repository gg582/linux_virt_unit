package incus_unit

// Task pool for container creation.
import (
	"github.com/yoonjin67/linux_virt_unit"
	"github.com/lxc/incus/shared/api"
	client "github.com/lxc/incus/client"
    "os"
	"log"
	"sync"
)

type StateChangeTarget struct {
	Tag    string
	Status string
}

type ContainerQueue struct {
	Tasks      chan linux_virt_unit.ContainerInfo
	wg         sync.WaitGroup
	StateTasks chan StateChangeTarget
}

var WorkQueue *ContainerQueue

// InitWorkQueue initializes the container work queue.
func InitWorkQueue() {
	WorkQueue = &ContainerQueue{
		Tasks:      make(chan linux_virt_unit.ContainerInfo, 1024),
		wg:         sync.WaitGroup{},
		StateTasks: make(chan StateChangeTarget, 1024),
	}
	log.Println("InitWorkQueue: Container work queue initialized.")
}


// Start starts the worker goroutines for the container queue.
func (q *ContainerQueue) Start(numWorkers int) {
	log.Printf("Start: Starting %d worker goroutines.", numWorkers)
	for i := 0; i < numWorkers; i++ {
		q.wg.Add(1)
		go q.ContainerCreationWorker()
		q.wg.Add(1)
		go q.StateChangeWorker()
	}
}

// Stop stops the worker goroutines.
func (q *ContainerQueue) Stop() {
	log.Println("Stop: Stopping worker goroutines.")
	close(q.Tasks)
	close(q.StateTasks)

	q.wg.Wait()
	log.Println("Stop: All worker goroutines stopped.")
}

// worker is the worker goroutine that processes container creation tasks.
func (q *ContainerQueue) ContainerCreationWorker() {
	defer q.wg.Done()
	log.Println("worker: Worker goroutine started.")
	for info := range q.Tasks {
		log.Println("worker: Received container creation task.")
		createContainer(info)
		log.Println("worker: Container creation task completed.")
	}
	log.Println("worker: Worker goroutine finished.")
}
func (q *ContainerQueue) StateChangeWorker() {
	defer q.wg.Done()
	log.Println("worker: Worker goroutine started.")
	for target := range q.StateTasks {
        if target.Status == "delete" {
            DeleteContainerByName(target.Tag)
        } else {
            err := ChangeState(target.Tag, target.Status)
            if err != nil {
                log.Printf("Status change failed on task %s, err: %v\n", target.Status, err)
                if target.Status == "start" || target.Status == "restart" || target.Status == "unfreeze" {
                    command := []string{"/usr/bin/manage_ssh"}
                    execArgs := api.InstanceExecPost{
                    	Command: command,
                    	User:    0,
                    	Group:   0,
                    }
       
                    ioDescriptor := client.InstanceExecArgs{
                    	Stdin:  os.Stdin,
                    	Stdout: os.Stdout,
                    	Stderr: os.Stderr,
                    }
                    op, _ := IncusCli.ExecInstance(target.Tag, execArgs, &ioDescriptor)
                    op.Wait()

                }
            }
        }

	}
	log.Println("worker: Worker goroutine finished.")

}
