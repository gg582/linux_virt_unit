package incus_unit

// Task pool for container creation.
import (
	"fmt"
	client "github.com/lxc/incus/client"
	"github.com/lxc/incus/shared/api"
	"github.com/yoonjin67/linux_virt_unit"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)


type PortTagTarget struct {
	port int
	tag  string
}


var NGINX_LOCATION = "/etc/nginx.conf"

type StateChangeTarget struct {
	Tag    string
	Status string
}

type ContainerQueue struct {
	Tasks      chan linux_virt_unit.ContainerInfo
	wg         sync.WaitGroup
	StateTasks chan StateChangeTarget
    RetrieveTag chan PortTagTarget
    WQReturns chan error
    DeletionQueue chan int
}

var WorkQueue *ContainerQueue

// InitWorkQueue initializes the container work queue.
func InitWorkQueue() {
	WorkQueue = &ContainerQueue{
		Tasks:      make(chan linux_virt_unit.ContainerInfo, 1024),
		wg:         sync.WaitGroup{},
		StateTasks: make(chan StateChangeTarget, 1024),
		RetrieveTag:  make(chan PortTagTarget, 1024),
	    WQReturns: make(chan error, 1024),
	    DeletionQueue: make(chan int, 1024),
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
		q.wg.Add(1)
		go q.NginxSyncWorker() 
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
func syncNginxToAdd(tag string, allocatedPort int) {
	// Delete the last closing brace "}" from the Nginx configuration file
	cmdDelLastLine := exec.Command("bash", "-c", `tac "$0" | sed '0,/}/ s/}//' | tac > /tmp/temp.txt`, NGINX_LOCATION)
	err := cmdDelLastLine.Run()
	if err != nil {
		log.Printf("syncNginxToAdd: (nginx) Cannot delete last line of nginx config. %v", err)
		return
	}
	cmdCopyNginx := exec.Command("mv", "/tmp/temp.txt", NGINX_LOCATION)
	err = cmdCopyNginx.Run()
	if err != nil {
		log.Println("syncNginxToAdd: (nginx) Cannot copy file to nginx config")
		return
	}

	var currentContainerIP, currentContainerIPv6 string
	// Waits for a maximum of 10 seconds
	for repeat := 0; repeat < 1000; repeat++ {
		containerInfo, _, err := IncusCli.GetInstanceState(tag)
		if err != nil {
			log.Printf("syncNginxToAdd: Container Check failed: %v\n", err)
		}

		for _, network := range containerInfo.Network {
			if network.Type == "broadcast" {
				for _, addr := range network.Addresses {
					if addr.Family == "inet" {
						currentContainerIP = addr.Address
					} else if addr.Family == "inet6" {
						currentContainerIPv6 = addr.Address
					}
					// find ipv4 and ipv6
				}
			}
		}
		if currentContainerIPv6 != "" && currentContainerIP != "" {
			nginxConfig := fmt.Sprintf(`
    server {
    	listen 0.0.0.0:%d;
    	proxy_pass %s:22;
    }
    server {
    	listen 0.0.0.0:%d;
    	proxy_pass %s:30001;
    }
    server {
    	listen 0.0.0.0:%d;
    	proxy_pass %s:30002;
    }
    server {
    	listen [::]:%d;
    	proxy_pass [%s]:22;
    }
    server {
    	listen [::]:%d;
    	proxy_pass [%s]:30001;
    }
    server {
    	listen [::]:%d;
    	proxy_pass [%s]:30002;
    }
}
        `, allocatedPort, currentContainerIP, allocatedPort+1, currentContainerIP, allocatedPort+2, currentContainerIP,
        				allocatedPort, currentContainerIPv6, allocatedPort+1, currentContainerIPv6, allocatedPort+2, currentContainerIPv6)

			// Use sed command to append the configuration
            cmdStr := fmt.Sprintf("cat <<EOF >> %s\n%s\nEOF", NGINX_LOCATION, nginxConfig)
            cmd := exec.Command("bash", "-c", cmdStr)
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("syncNginxToAdd: Error adding config to nginx: %v, output: %s", err, string(output))
				return
			}
			log.Printf("syncNginxToAdd: Successfully added config to nginx. Output: %s", string(output))

			nginxRestart := exec.Command("nginx", "-s", "reload")
			err = nginxRestart.Run()
			if err != nil {
				log.Printf("syncNginxToAdd: Error reloading nginx: %v", err)
			} else {
				fmt.Println("Nginx configuration has been successfully updated.")
			}
            break
		}
		time.Sleep(time.Second / 10)
		log.Println("syncNginxToAdd: Waiting for IP response...")
	}
	log.Printf("syncNginxToAdd: Timed out waiting for IP address for tag '%s'.", tag)
}
// worker is the worker goroutine that processes container creation tasks.
func (q *ContainerQueue) ContainerCreationWorker() {
	defer q.wg.Done()
	log.Println("worker: Worker goroutine started.")
	for info := range q.Tasks {
		log.Println("worker: Received container creation task.")
		createContainer(info)
		log.Println("worker: Container creation task completed. Sending Nginx sync request.")
	}
}

func (q *ContainerQueue) StateChangeWorker() {
	defer q.wg.Done()
	log.Println("worker: Worker goroutine started.")
	for target := range q.StateTasks {
		if target.Status == "delete" {
			go DeleteContainerByName(target.Tag)
		} else {
			go ChangeState(target.Tag, target.Status)
		}
		for len(WorkQueue.WQReturns) != 0 {
			err := <-WorkQueue.WQReturns
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

	for len(WorkQueue.DeletionQueue) != 0 {
		port := <- WorkQueue.DeletionQueue
		DeleteNginxConfig(port)
	}
}

// NginxSyncWorker is a dedicated worker to handle Nginx configuration synchronization.
func (q *ContainerQueue) NginxSyncWorker() {
	defer q.wg.Done()
	log.Println("NginxSyncWorker: Nginx sync worker started.")
	for target := range WorkQueue.RetrieveTag {
		log.Printf("NginxSyncWorker: Received Nginx sync request for tag '%s', port %d.", target.tag, target.port)
		syncNginxToAdd(target.tag, target.port)
		log.Printf("NginxSyncWorker: Nginx sync completed for tag '%s', port %d.", target.tag, target.port)
	}
	log.Println("NginxSyncWorker: Nginx sync worker finished.")
}

func DeleteNginxConfig(port int) error {
	nginxConfPath := NGINX_LOCATION
	portStr := strconv.Itoa(port)
	portPlusOneStr := strconv.Itoa(port + 1)
	portPlusTwoStr := strconv.Itoa(port + 2)

	// Construct the sed command to delete the three server blocks related to the port.
	sedCommand := fmt.Sprintf(`sed -i '/listen 0.0.0.0:%s;/ {
N; /proxy_pass .*:%s;/ d;
N; /listen 0.0.0.0:%s;/ {
N; /proxy_pass .*:%s;/ d;
N; /listen 0.0.0.0:%s;/ {
N; /proxy_pass .*:%s;/ d;
}; }; };' %s`,
		portStr, "22",
		portPlusOneStr, "30001",
		portPlusTwoStr, "30002",
		nginxConfPath)

	cmd := exec.Command("bash", "-c", sedCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("DeleteNginxConfig: Failed to execute sed command: %v, output: %s", err, string(output))
		return fmt.Errorf("failed to delete nginx config: %v, output: %s", err, string(output))
	}
	log.Printf("DeleteNginxConfig: Successfully executed sed command. Output: %s", string(output))
	return nil
}
