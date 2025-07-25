package incus_unit

// Task pool for container creation.
import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gg582/linux_virt_unit"
    "go.mongodb.org/mongo-driver/bson"
    db "github.com/gg582/linux_virt_unit/mongo_connect"
	linux_virt_unit_crypto "github.com/gg582/linux_virt_unit/crypto"
	client "github.com/lxc/incus/client"
	"github.com/lxc/incus/shared/api"
)


type PortTagTarget struct {
    port int
    tag  string
}

var nginxMutex sync.Mutex
var nginxDeletionMutex sync.Mutex

type StateChangeTarget struct {
    Tag    string
    Status string
}

type ContainerQueue struct {
    Tasks      chan linux_virt_unit.ContainerInfo
    Unreg chan linux_virt_unit.UserInfo
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
        Unreg: make(chan linux_virt_unit.UserInfo, 1024),
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
        q.wg.Add(1)
        go q.UnregisterWorker()
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
    cmdDelLastLine := exec.Command("bash", "-c", `tac "$0" | sed '0,/}/ s/}//' | tac > /tmp/temp.txt`, linux_virt_unit.NGINX_LOCATION)
    err := cmdDelLastLine.Run()
    if err != nil {
        log.Printf("syncNginxToAdd: (nginx) Cannot delete last line of nginx config. %v", err)
        return
    }
    cmdCopyNginx := exec.Command("mv", "/tmp/temp.txt", linux_virt_unit.NGINX_LOCATION)
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
            cmdStr := fmt.Sprintf("cat <<EOF >> %s\n%s\nEOF", linux_virt_unit.NGINX_LOCATION, nginxConfig)
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
        for i := 0; i < 1000; i++ {
            instance, _, err := IncusCli.GetInstance(info.TAG)
            if err != nil || instance.Status != "Running" {
                fmt.Fprintf(os.Stderr, "Waiting for container bootup: %v\n", err)
                time.Sleep(time.Second / 10)
                continue
            }
            username, err := linux_virt_unit_crypto.DecryptString(info.Username, info.Key, info.UsernameIV)
            if err != nil {
                log.Printf("createContainer: Error decrypting username: %v", err)
                return
            }
            password, err := linux_virt_unit_crypto.DecryptString(info.Password, info.Key, info.PasswordIV)
            if err != nil {
                log.Printf("createContainer: Error decrypting password: %v", err)
                return
            }

            log.Println("Trying to setup ssh...")
            command := []string{"/bin/bash", "/conSSH.sh", username, password, info.TAG}
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

            op, err := IncusCli.ExecInstance(info.TAG, execArgs, &ioDescriptor)
            err = op.Wait()
            if err != nil {
                log.Printf("createContainer: (ssh) Failed to get SSH setup result: %v\n", err)
                break
            } else {
                log.Println("Successfully get SSH setup result: ok")
                break
            }
        }
        log.Println("worker: Container creation task completed. Sending Nginx sync request.")
    }
}

func (q *ContainerQueue) StateChangeWorker() {
    defer q.wg.Done()
    for target := range q.StateTasks {
        if target.Status == "delete" {
            nginxDeletionMutex.Lock()
            DeleteContainerByName(target.Tag)
            nginxDeletionMutex.Unlock()
        } else {
            go ChangeState(target.Tag, target.Status)
        }
    }
}

func (q *ContainerQueue) UnregisterWorker() {
    defer q.wg.Done()
    log.Println("Unregister worker started.")
    for info := range q.Unreg {
        log.Printf("Received username %s to unregister.", info.Username)
        UnregisterUser(info)
    }
}

// NginxSyncWorker is a dedicated worker to handle Nginx configuration synchronization.
func (q *ContainerQueue) NginxSyncWorker() {
    defer q.wg.Done()
    log.Println("NginxSyncWorker: Nginx sync worker started.")
    for target := range WorkQueue.RetrieveTag {
        log.Printf("NginxSyncWorker: Received Nginx sync request for tag '%s', port %d.", target.tag, target.port)
        nginxMutex.Lock()
        syncNginxToAdd(target.tag, target.port)
        nginxMutex.Unlock()
        log.Printf("NginxSyncWorker: Nginx sync completed for tag '%s', port %d.", target.tag, target.port)
    }
    log.Println("NginxSyncWorker: Nginx sync worker finished.")
}


func UnregisterUser(in linux_virt_unit.UserInfo) {
    ctx, cancel := context.WithTimeout(context.Background(), time.Minute * 5)
    defer cancel()
    cur, err := db.ContainerInfoCollection.Find(ctx, bson.M{})
    if err != nil {
        log.Printf("Unregister: Error finding container information in MongoDB: %v", err)
        return
    }
    defer cur.Close(ctx)

    // Collect matching containers
    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            log.Printf("Unregister: Error decoding container document from MongoDB: %v", err)
            continue
        }

        // Decrypt stored username
        usernameFromDB, err := linux_virt_unit_crypto.DecryptString(info.Username, info.Key, info.UsernameIV)
        if err != nil {
            log.Printf("Unregister: Error decrypting username from DB for tag '%s': %v", info.TAG, err)
            continue
        }

        // Check if the decrypted username matches the requester
        if usernameFromDB == in.Username {
            // Retrieve container status
            ChangeState(info.TAG, "stop")
            DeleteContainerByName(info.TAG)

            found, port := db.FindPortByTag(info.TAG)
            if found == false {
                log.Println("No port found on DB! Failed to delete")
            } else {
                log.Println("Nginx Port Deletion triggered")
                nginxMutexToDelete.Lock()
                DeleteNginxConfig(port)
                ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
                defer cancel()
                filter := bson.D{{"tag", info.TAG}}
                _, err = db.ContainerInfoCollection.DeleteOne(ctx, filter)
                if err != nil {
                    log.Println("Port Deletion from MongoDB Failed!")
                }
                nginxMutexToDelete.Unlock()
                defer cur.Close(ctx)
            }
    
            if err != nil {
                log.Printf("DeleteByTag: MongoDB DeleteOne failed: %v", err)
            } else {
                log.Println("DeleteByTag: MongoDB DeleteOne Success.")
            }
        }
    }
    DeleteExistingUser(in.Username, in.Password, ctx)
}
