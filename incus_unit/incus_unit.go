package incus_unit

import (
    "bytes"
    "container/heap"
    "context"
    client "github.com/lxc/incus/client"
    "encoding/json"
    "errors"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/exec"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/lxc/incus/shared/api"
    "github.com/yoonjin67/linux_virt_unit"
    linux_virt_unit_crypto "github.com/yoonjin67/linux_virt_unit/crypto"
    db "github.com/yoonjin67/linux_virt_unit/mongo_connect"
    "go.mongodb.org/mongo-driver/bson"
    "golang.org/x/crypto/bcrypt"
)

var INFO linux_virt_unit.ContainerInfo

/*
var baseImages = map[string]string{
        "almalinux-9":       "8c24928fdceb",
        "archlinux-current": "334438793c27",
        "centos-9-Stream":   "8492a0ddfebf",
        "debian-10":         "162b38789406",
        "debian-11":         "31acfd1829a4",
        "debian-12":         "d15f94c5ebd9",
        "devuan-beowulf":    "60d9b884187d",
        "devuan-chimaera":   "524ba12fa420",
        "devuan-daedalus":   "1111b83930c4",
        "opensuse-tumbleweed": "9e7888975b4c",
        "rockylinux-9":      "76d79e10105b",
        "slackware-15.0":    "310a91673ab4",
        "slackware-current": "9cce9f6846e6",
        "ubuntu-20.04":      "bd1d1f6746fa",
        "ubuntu-22.04":      "7f02ba5e448b",
        "ubuntu-24.04":      "37d2616360cb",
    }

    */
// IntHeap is a min-heap of ints.

type PortHeap []int

func (h *PortHeap) Len() int { return len(*h) }

// Less reports whether the element with index i should sort before the element with index j.
func (h *PortHeap) Less(i, j int) bool {
    var heap_nopointer = *h
    return heap_nopointer[i] > heap_nopointer[j]
}

// Swap swaps the elements with indices i and j.
func (h *PortHeap) Swap(i, j int) {
    var heap_nopointer = *h
    heap_nopointer[i], heap_nopointer[j] = heap_nopointer[j], heap_nopointer[i]
}

// Push adds a new element to the heap.
func (h *PortHeap) Push(x interface{}) {
    *h = append(*h, x.(int))
}

// Pop removes and returns the smallest element from the heap.
func (h *PortHeap) Pop() interface{} {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}

func (h *PortHeap) Top() interface {} {
    n:= len(*h)
    tmp := *h
    return tmp[n-1]
}


var AvailablePorts *PortHeap
var UnavailablePorts *PortHeap

var IncusCli client.InstanceServer
var mydir string = "/usr/local/bin/linuxVirtualization/"
var SERVER_IP = "127.0.0.1"
var PORT_LIST = make([]int, 0, 100000)
var flag bool
var authFlag bool = false
var port string
var portprev string = "60001"
var cursor interface{}
var current []byte
var current_Config []byte
var buf bytes.Buffer
const INITIAL_PORT = 27020
var ctx context.Context
var cancel context.CancelFunc
var ADDR string = "http://hobbies.yoonjin2.kr"

// Mutex to manage port allocation.
var portMutex sync.Mutex
var portDeleteMutex sync.Mutex

// Task pool for container creation.
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

// TouchFile creates an empty file if it doesn't exist.
func TouchFile(name string) {
    file, _ := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0644)
    file.Close()
    log.Printf("TouchFile: File '%s' touched.", name)
}

// getTAG generates a unique tag for a container.
func getTAG(mydir string, tag string) string {
    var err error
    var file *os.File
    filePath := mydir + "/container/latest_access"
    file, err = os.OpenFile(filePath, os.O_RDWR, os.FileMode(0644))
    if err != nil {
        log.Printf("getTAG: Error opening latest_access file '%s': %v", filePath, err)
    }
    defer file.Close()
    tagRet := tag + "-" + linux_virt_unit_crypto.RandStringBytes(20)
    _, err = file.Write([]byte(tagRet))
    if err != nil {
        log.Printf("getTAG: Error writing tag to file '%s': %v", filePath, err)
    }
    log.Printf("getTAG: Generated tag '%s' for user '%s'.", tagRet, tag)
    return tagRet
}

// Start starts the worker goroutines for the container queue.
func (q *ContainerQueue) Start(numWorkers int) {
    log.Printf("Start: Starting %d worker goroutines.", numWorkers)
    for i := 0; i < numWorkers; i++ {
        q.wg.Add(1)
        go q.worker()
    }
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

// GetContainerInfo retrieves the state of a container.
func GetContainerInfo(tag string, info linux_virt_unit.ContainerInfo) linux_virt_unit.ContainerInfo {
    state, _, err := IncusCli.GetInstanceState(tag)
    if err != nil {
        log.Printf("GetContainerInfo: Failed to get instance state for tag '%s': %v", tag, err)
    }
    // Process the resulting string
    info.VMStatus = state.Status

    // Output the result
    log.Printf("GetContainerInfo: State of container '%s': %s", tag, info.VMStatus)
    return info
}

// CreateContainer handles the HTTP request to create a container.
func CreateContainer(wr http.ResponseWriter, req *http.Request) {
    wr.Header().Set("Content-Type", "application/json; charset=utf-8")
    log.Println("CreateContainer: Received request to create a container.")

    var info linux_virt_unit.ContainerInfo
    if err := json.NewDecoder(req.Body).Decode(&info); err != nil {
        log.Printf("CreateContainer: Failed to parse JSON request body: %v", err)
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }
    log.Printf("CreateContainer: Parsed container info: %+v", info)
    if info.Distro == "" || info.DistroVersion == "" || info.TAG == "" || info.Username == "" || info.Password == "" {
        http.Error(wr, "Invalid field", http.StatusBadRequest);
        return
    }
    info.Distro = info.Distro + "-" + info.DistroVersion 
    log.Println("Distro Alias is " + info.Distro)
    if len(baseImages[info.Distro]) == 0{
        log.Println("image is not available. check https://images.linuxcontainers.org")
        log.Println("You requested " + info.Distro)
        http.Error(wr, "Invalid Distro" , http.StatusBadRequest);
        return
    }

    select {
    case WorkQueue.Tasks <- info:
        log.Println("CreateContainer: Added container creation task to the work queue.")
        string_Reply, _ := json.Marshal(info)
        wr.Write(string_Reply)
        return
    default:
        log.Println("CreateContainer: Work queue is full.")
        http.Error(wr, "Server is busy", http.StatusServiceUnavailable)
        return
    }
}

// ChangeState changes the state of a container (start, stop, restart, pause).
func ChangeState(tag string, newState string) error {
    log.Printf("ChangeState: Request to change state of container '%s' to '%s'.", tag, newState)
    // Update the container status in MongoDB
    _, err := db.ContainerInfoCollection.UpdateOne(
        context.Background(),
        bson.M{"tag": tag},
        bson.D{{"$set", bson.M{"vmstatus": newState}}},
    )
    if err != nil {
        log.Printf("ChangeState: MongoDB update failed for tag '%s' to state '%s': %v", tag, newState, err)
        return fmt.Errorf("failed to update container status in DB: %w", err)
    }
    log.Printf("ChangeState: MongoDB status updated for tag '%s' to '%s'.", tag, newState)

    // Get the Incus instance information
    inst, _, err := IncusCli.GetInstance(tag)
    if err != nil {
        log.Printf("ChangeState: Failed to get Incus instance information for tag '%s': %v", tag, err)
        return fmt.Errorf("failed to get Incus instance: %w", err)
    }
    log.Printf("ChangeState: Current status of Incus instance '%s': %s.", tag, inst.Status)

    // Handle 'stop' state change
    if newState == "stop" && inst.Status != "Stopped" {
        req := api.InstanceStatePut{
            Action:  "stop",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus stop request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus stop: %w", err)
        }
        log.Printf("ChangeState: Incus stop request sent for tag '%s'.", tag)

        // Wait for the Incus stop operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus stop operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus stop: %w", err)
        }
        log.Printf("ChangeState: Incus stop operation completed for tag '%s'.", tag)

        // Update DB status to 'stopped' after Incus stop
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"tag": tag},
            bson.D{{"$set", bson.M{"vmstatus": "stopped"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'stopped' failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'stopped' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'stopped' for tag '%s'.", tag)
    } else if newState == "start" && inst.Status != "Running" {
        // Handle 'start' state change
        req := api.InstanceStatePut{
            Action:  "start",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus start request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus start: %w", err)
        }
        log.Printf("ChangeState: Incus start request sent for tag '%s'.", tag)

        // Wait for the Incus start operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus start operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus start: %w", err)
        }
        log.Printf("ChangeState: Incus start operation completed for tag '%s'.", tag)

        // Update DB status to 'running' after Incus start
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"tag": tag},
            bson.D{{"$set", bson.M{"vmstatus": "running"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'running' failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'running' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'running' for tag '%s'.", tag)
    } else if newState == "restart" {
        // Handle 'restart' state change
        req := api.InstanceStatePut{
            Action:  "restart",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus restart request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus restart: %w", err)
        }
        log.Printf("ChangeState: Incus restart request sent for tag '%s'.", tag)

        // Wait for the Incus restart operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus restart operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus restart: %w", err)
        }
        log.Printf("ChangeState: Incus restart operation completed for tag '%s'.", tag)

        // Update DB status to 'running' after Incus restart (assuming restart leads to running)
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"tag": tag},
            bson.D{{"$set", bson.M{"vmstatus": "running"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'running' failed after Incus restart for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'running' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'running' for tag '%s' after restart.", tag)
    } else if newState == "freeze" && inst.Status != "Frozen" {
        // Handle 'freeze' (pause) state change
        req := api.InstanceStatePut{
            Action:  "freeze",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus freeze request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus freeze: %w", err)
        }
        log.Printf("ChangeState: Incus freeze request sent for tag '%s'.", tag)

        // Wait for the Incus freeze operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus freeze operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus freeze: %w", err)
        }
        log.Printf("ChangeState: Incus freeze operation completed for tag '%s'.", tag)

        // Update DB status to 'frozen' after Incus freeze
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"tag": tag},
            bson.D{{"$set", bson.M{"vmstatus": "frozen"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'frozen' failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'frozen' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'frozen' for tag '%s'.", tag)
    } else if newState == "unfreeze" && inst.Status == "Frozen" {
        // Handle 'unfreeze' (resume) state change
        req := api.InstanceStatePut{
            Action:  "unfreeze",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus unfreeze request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus unfreeze: %w", err)
        }
        log.Printf("ChangeState: Incus unfreeze request sent for tag '%s'.", tag)

        // Wait for the Incus unfreeze operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus unfreeze operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus unfreeze: %w", err)
        }
        log.Printf("ChangeState: Incus unfreeze operation completed for tag '%s'.", tag)

        // Update DB status to 'running' after Incus unfreeze (assuming it goes back to running)
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"tag": tag},
            bson.D{{"$set", bson.M{"vmstatus": "running"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'running' failed after Incus unfreeze for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'running' in DB after unfreeze: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'running' for tag '%s' after unfreeze.", tag)
    } else {
        log.Printf("ChangeState: No state change needed for container '%s' to '%s' (current status: %s).", tag, newState, inst.Status)
    }

    return nil
}

// createContainer handles the actual creation of a container.
func createContainer(info linux_virt_unit.ContainerInfo) {
    // Decrypt username and password
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

    // Check if user exists
    if !CheckUserExists(username) {
        log.Printf("createContainer: User '%s' does not exist.", username)
        return
    }
    log.Printf("createContainer: User '%s' exists.", username)

    // Generate a unique tag for the container
    tag := getTAG(mydir, info.TAG)
    info.TAG = tag
    log.Printf("createContainer: Generated tag '%s' for user '%s'.", tag, username)

    // Allocate a port for the container
    portMutex.Lock()
    var allocatedPort int
    if AvailablePorts.Len() == 0 {
        if UnavailablePorts.Len() != 0 {
            allocatedPort = UnavailablePorts.Top().(int) + 3
        } else {
            allocatedPort = INITIAL_PORT
        }
        UnavailablePorts.Push(allocatedPort)
        log.Printf("createContainer: Allocated new port '%d' for tag '%s'.", allocatedPort, tag)
    } else {
        allocatedPort = heap.Pop(AvailablePorts).(int)
        log.Printf("createContainer: Reused port '%d' for tag '%s'. AvailablePorts size: %d", allocatedPort, tag, AvailablePorts.Len())
    }
    portMutex.Unlock()

    port = strconv.Itoa(allocatedPort)
    info.Serverport = port
    portprev = port

    log.Printf("createContainer: Attempting to create container with tag '%s', port '%s', user '%s' password '%s'.", tag, port, username, password)

    // Set a timeout for container creation

    containerConfig := api.InstancesPost {
        Name: tag,
        Source: api.InstanceSource {
            Type: "image",
            Fingerprint: baseImages[info.Distro],
        },
    }
    op, err := IncusCli.CreateInstance(containerConfig)
    // Execute the container creation script
    if err != nil {
        log.Printf("Failed to create container. Error: %v\n", err)
    } else {
        // If creation fails, return the port to the heap
        op.Wait()
    }
    containerInfo, _, err := IncusCli.GetInstanceState(tag)
    if err != nil {
        log.Printf("Container Check failed: %v\n", err)
    }
    log.Printf("createContainer: Container creation script finished for tag '%s'.", tag)
    ChangeState(tag, "start")

    var currentContainerIP, currentContainerIPv6 string

    for _, network := range containerInfo.Network {

        if network.Type == "broadcast" {
            for _, addr := range network.Addresses {
                if addr.Family == "inet" {
                    currentContainerIP = addr.Address
                } else if addr.Family == "inet6" {
                    currentContainerIPv6 = addr.Address
                }
            }
        }
    }
    var file *os.File
    if currentContainerIP != "" {
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
        `, allocatedPort, currentContainerIP, allocatedPort+1, currentContainerIP, allocatedPort+2, currentContainerIP)
    
        // **append** setting into nginx
        file, err = os.OpenFile("/etc/nginx/nginx.conf", os.O_APPEND|os.O_WRONLY, 0644)
        if err != nil {
            log.Fatal("Error opening nginx.conf file: ", err)
        }
        defer file.Close()
    
        //so this should act as "append" mode
        _, err = file.WriteString(nginxConfig)
        if err != nil {
            log.Fatal("Error writing to nginx.conf: ", err)
        }
    }
    if currentContainerIPv6 != "" {
        nginxConfig := fmt.Sprintf(`
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
    `, allocatedPort, currentContainerIPv6, allocatedPort+1, currentContainerIPv6, allocatedPort+2, currentContainerIPv6)
    
        // /etc/nginx/nginx.conf에 설정 덧붙이기
        // nginx 설정을 파일에 덧붙이기
        _, err = file.WriteString(nginxConfig)
        if err != nil {
            log.Fatal("Error writing to nginx.conf: ", err)
        }
}
    exec.Command("/usr/sbin/nginx", "-s", "reload")
    command := []string{"/bin/bash", "/conSSH.sh", username, password, tag}
    execArgs := api.InstanceExecPost{
            Command: command,
            Environment: map[string]string{
            }, 
             User: 0, // root user
             Group: 0, 
    }

    ioDescriptor := client.InstanceExecArgs{
      Stdin: os.Stdin,
      Stdout: os.Stdout,
      Stderr: os.Stderr,
    }
    // 명령 실행
    op, err = IncusCli.ExecInstance(tag, execArgs, &ioDescriptor)
    if err != nil {
            fmt.Fprintf(os.Stderr, "Failed to setup SSH: %v\n", err)
            os.Exit(1)
    }

    // 작업 완료 대기 및 결과 가져오기
    err = op.Wait()
    if err != nil {
            log.Printf("Failed to get SSH setup result: %v\n", err)
            os.Exit(1)
    }
    fmt.Println("Nginx configuration has been successfully updated.")

    // Set the initial VMStatus to "running" after successful creation



    // Insert container information into MongoDB
    _, insertErr := db.ContainerInfoCollection.InsertOne(context.Background(), info)
    if insertErr != nil {
        log.Printf("createContainer: Cannot insert container info into MongoDB for tag '%s': %v", tag, insertErr)
        // If DB insertion fails, attempt to delete the created (potentially failed) container
        err = DeleteContainerByName(tag)
        if err != nil {
            log.Printf("createContainer: Failed to delete potentially failed Incus container '%s': %v", tag, err)
        } else {
            log.Printf("createContainer: Attempted to delete Incus container '%s' after MongoDB insertion failure.", tag)
        }
    } else {
        log.Printf("createContainer: Container info inserted into MongoDB for tag '%s'.", tag)
    }
}

// DeleteFromListByValue removes a specific value from an integer slice.
func DeleteFromListByValue(slice []int, value int) []int {
    for i, itm := range slice {
        if itm == value {
            log.Printf("DeleteFromListByValue: Found value '%d' at index '%d', removing.", value, i)
            return append(slice[:i], slice[i+1:]...)
        }
    }
    log.Printf("DeleteFromListByValue: Value '%d' not found in the slice.", value)
    return slice
}

// StopByTag handles the HTTP request to stop a container.
func StopByTag(wr http.ResponseWriter, req *http.Request) {
    forTagBytes, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("StopByTag: Failed to read request body: %v", err)
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    stringForTag := strings.Trim(string(forTagBytes), "\"")
    log.Printf("StopByTag: Received request to stop container with tag '%s'.", stringForTag)
    err = ChangeState(stringForTag, "stop")
    if err != nil {
        log.Printf("StopByTag: ChangeState failed for tag '%s': %v", stringForTag, err)
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    wr.WriteHeader(http.StatusOK)
    wr.Write([]byte(fmt.Sprintf("Stop command sent for container '%s'", stringForTag)))
}

// RestartByTag handles the HTTP request to restart a container.
func RestartByTag(wr http.ResponseWriter, req *http.Request) {
    forTagBytes, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("RestartByTag: Failed to read request body: %v", err)
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    stringForTag := strings.Trim(string(forTagBytes), "\"")
    log.Printf("RestartByTag: Received request to restart container with tag '%s'.", stringForTag)
    err = ChangeState(stringForTag, "restart")
    if err != nil {
        log.Printf("RestartByTag: ChangeState failed for tag '%s': %v", stringForTag, err)
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    wr.WriteHeader(http.StatusOK)
    wr.Write([]byte(fmt.Sprintf("Restart command sent for container '%s'", stringForTag)))
}

// PauseByTag handles the HTTP request to pause a container.
func PauseByTag(wr http.ResponseWriter, req *http.Request) {
    forTagBytes, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("PauseByTag: Failed to read request body: %v", err)
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    stringForTag := strings.Trim(string(forTagBytes), "\"")
    log.Printf("PauseByTag: Received request to pause container with tag '%s'.", stringForTag)
    err = ChangeState(stringForTag, "freeze")
    if err != nil {
        log.Printf("PauseByTag: ChangeState failed for tag '%s': %v", stringForTag, err)
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    wr.WriteHeader(http.StatusOK)
    wr.Write([]byte(fmt.Sprintf("Pause command sent for container '%s'", stringForTag)))
}

// ResumeByTag handles the HTTP request to resume a container.
func ResumeByTag(wr http.ResponseWriter, req *http.Request) {
    forTagBytes, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("PauseByTag: Failed to read request body: %v", err)
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    stringForTag := strings.Trim(string(forTagBytes), "\"")
    log.Printf("PauseByTag: Received request to pause container with tag '%s'.", stringForTag)
    err = ChangeState(stringForTag, "unfreeze")
    if err != nil {
        log.Printf("PauseByTag: ChangeState failed for tag '%s': %v", stringForTag, err)
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    wr.WriteHeader(http.StatusOK)
    wr.Write([]byte(fmt.Sprintf("Pause command sent for container '%s'", stringForTag)))
}

// StartByTag handles the HTTP request to start a container.
func StartByTag(wr http.ResponseWriter, req *http.Request) {
    forTagBytes, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("StartByTag: Failed to read request body: %v", err)
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    stringForTag := strings.Trim(string(forTagBytes), "\"")
    log.Printf("StartByTag: Received request to start container with tag '%s'.", stringForTag)
    err = ChangeState(stringForTag, "start")
    if err != nil {
        log.Printf("StartByTag: ChangeState failed for tag '%s': %v", stringForTag, err)
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    wr.WriteHeader(http.StatusOK)
    wr.Write([]byte(fmt.Sprintf("Start command sent for container '%s'", stringForTag)))
}

// DeleteContainerByName stops and then deletes an Incus container by its tag.
func DeleteContainerByName(tag string) error {
    log.Printf("DeleteContainerByName: Attempting to delete Incus container with tag '%s'.", tag)
    // Check if the tag is nil
    if tag == "" {
        log.Println("DeleteContainerByName: Error: tag is nil")
        return errors.New("tag is nil")
    }
    // Get the container information
    container, _, err := IncusCli.GetInstance(tag)
    if err != nil {
        return fmt.Errorf("DeleteContainerByName: failed to get container '%s': %w", tag, err)
    }
    log.Printf("DeleteContainerByName: Current status of container '%s': %s.", tag, container.Status)

    // If the container is running, stop it
    if container.Status != "Stopped" {
        log.Printf("DeleteContainerByName: Container '%s' is running, requesting stop.", tag)
        err := ChangeState(tag, "stop")
        if err != nil {
            log.Printf("DeleteContainerByName: ChangeState call failed for tag '%s': %v", tag, err)
            return err
        }

        // Wait for the container to stop (with a timeout)
        stopChan := make(chan bool)
        go func() {
            ticker := time.NewTicker(1 * time.Second)
            defer ticker.Stop()
            for range ticker.C {
                currentContainer, _, err := IncusCli.GetInstance(tag)
                if err != nil {
                    log.Printf("DeleteContainerByName: Failed to get container '%s' information while waiting for stop: %v", tag, err)
                    stopChan <- true // Consider stopped if info cannot be retrieved
                    return
                }
                if currentContainer.Status == "Stopped" {
                    log.Printf("DeleteContainerByName: Container '%s' is now Stopped.", tag)
                    stopChan <- true
                    return
                }
                log.Printf("DeleteContainerByName: Container '%s' status: %s, waiting...", tag, currentContainer.Status)
            }
        }()

        select {
        case <-stopChan:
            log.Printf("DeleteContainerByName: Container '%s' stop confirmed, proceeding with deletion.", tag)
        case <-time.After(30 * time.Second): // Set a timeout to prevent indefinite waiting
            return fmt.Errorf("DeleteContainerByName: container '%s' did not stop in time", tag)
        }
    } else {
        log.Printf("DeleteContainerByName: Container '%s' is already Stopped.", tag)
    }

    // Delete the container
    op, err := IncusCli.DeleteInstance(tag)
    if err != nil {
        return fmt.Errorf("DeleteContainerByName: failed to delete container '%s': %w", tag, err)
    }
    log.Printf("DeleteContainerByName: Delete request sent for container '%s'.", tag)

    err = op.Wait()
    if err != nil {
        return fmt.Errorf("DeleteContainerByName: error waiting for deletion of container '%s': %w", tag, err)
    }
    log.Printf("DeleteContainerByName: Container '%s' deleted successfully.", tag)
    return nil
}

func DeleteNginxConfig(port int) error {
    nginxConfPath := "/etc/nginx/nginx.conf"
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

// DeleteByTag handles the HTTP request to delete a container by its tag.
func DeleteByTag(wr http.ResponseWriter, req *http.Request) {
    log.Println("DeleteByTag: Start.")
    forTagBytes, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("DeleteByTag: Failed to read request Body: %v", err)
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }
    stringForTag := strings.Trim(string(forTagBytes), "\"")
    log.Printf("DeleteByTag: Received request to delete container with tag '%s'.", stringForTag)

    cur, err := db.ContainerInfoCollection.Find(context.Background(), bson.D{{}})
    if err != nil {
        log.Printf("DeleteByTag: MongoDB Find failed: %v", err)
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(context.Background())
    log.Println("DeleteByTag: MongoDB Find completed.")

    found := false
    var foundInfo linux_virt_unit.ContainerInfo

    for cur.Next(context.Background()) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            log.Printf("DeleteByTag: MongoDB document Decode failed: %v", err)
            continue
        }
        if info.TAG == stringForTag {
            found = true
            foundInfo = info
            log.Printf("DeleteByTag: Found Container info: %+v", foundInfo)
            break
        }
    }

    if found {
        p32, err := strconv.Atoi(foundInfo.Serverport)
        if err != nil {
            log.Printf("DeleteByTag: Failed to convert ServerPort to integer: %v", err)
            http.Error(wr, "Internal server error", http.StatusInternalServerError)
            return
        }
        p := int(p32)
        log.Printf("DeleteByTag: Port to return: %d", p)

        if DeleteNginxConfig(p) != nil {
            log.Println("Nginx policy modification failed")
        }
        portDeleteMutex.Lock()
        log.Printf("DeleteByTag: AvailablePorts state (Before Pop/Push): %+v", *AvailablePorts)
        PORT_LIST = DeleteFromListByValue(PORT_LIST, int(p))
        heap.Push(AvailablePorts, int(p))
        log.Printf("DeleteByTag: AvailablePorts state (After Pop/Push): %+v", *AvailablePorts)
        portDeleteMutex.Unlock()

        filter := bson.M{"tag": stringForTag}
        _, err = db.ContainerInfoCollection.DeleteOne(context.Background(), filter)
        if err != nil {
            log.Printf("DeleteByTag: MongoDB DeleteOne failed: %v", err)
        } else {
            log.Println("DeleteByTag: MongoDB DeleteOne Success.")
        }

        log.Println("DeleteByTag: Calling DeleteContainerByName.")
        err = DeleteContainerByName(stringForTag)
        if err != nil {
            log.Printf("DeleteByTag: DeleteContainerByName failed: %v", err)
        } else {
            log.Println("DeleteByTag: DeleteContainerByName Success.")
        }
        wr.WriteHeader(http.StatusOK)
        wr.Write([]byte(fmt.Sprintf("Container with tag '%s' deleted", stringForTag)))
        return
    } else {
        log.Printf("DeleteByTag: Container with tag '%s' not found.", stringForTag)
        http.Error(wr, fmt.Sprintf("Container with tag '%s' not found", stringForTag), http.StatusNotFound)
        return
    }
}

// GetContainers retrieves a list of containers for a specific user.
func GetContainers(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    wr.Header().Set("Content-Type", "application/json; charset=utf-8")

    var in linux_virt_unit.UserInfo
    body, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("GetContainers: Failed to read request body: %v", err)
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }
    if err := json.Unmarshal(body, &in); err != nil {
    log.Printf("GetContainers: Failed to parse JSON request body: %v", err)
    http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
    return
    }

    log.Printf("GetContainers: Received request for containers of user '%s' (encrypted).", in.Username)
    
    decodedUsername, err := linux_virt_unit_crypto.DecryptString(in.Username, in.Key, in.UsernameIV)
    if err != nil {
        log.Printf("GetContainers: Failed to decrypt username: %v", err)
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }
    log.Printf("GetContainers: Decrypted username: '%s'.", decodedUsername)
    
    cur, err := db.ContainerInfoCollection.Find(ctx, bson.D{{}})
    if err != nil {
        log.Printf("GetContainers: Error finding container information in MongoDB: %v", err)
        http.Error(wr, "Database error: "+err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)
    
    jsonList := make([]interface{}, 0, 100000)
    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            log.Printf("GetContainers: Error decoding container document from MongoDB: %v", err)
            continue
        }
        usernameFromDB, err := linux_virt_unit_crypto.DecryptString(info.Username, info.Key, info.UsernameIV)
        if err != nil {
            log.Printf("GetContainers: Error decrypting username from DB for tag '%s': %v", info.TAG, err)
            continue
        }
        // In GetContainers, we should compare the decrypted username with the requested username.
        // We are not verifying the password here, as this endpoint is for listing containers
        // of an already authenticated user (authentication should happen elsewhere).
        if usernameFromDB == decodedUsername {
            inst, _, err := IncusCli.GetInstance(info.TAG)
            if err == nil {
                info.VMStatus = inst.Status
                if inst.StatusCode == api.Frozen {
                    info.VMStatus = "Frozen"
                }
            } else {
                log.Printf("GetContainers: Failed to get Incus instance state for tag '%s': %v", info.TAG, err)
                info.VMStatus = "unknown" // Or some other appropriate status
            }
            jsonList = append(jsonList, info)
        }
    }
    
    resp, err := json.Marshal(jsonList)
    if err != nil {
        log.Printf("GetContainers: Failed to marshal container list to JSON: %v", err)
        http.Error(wr, "Failed to marshal response: "+err.Error(), http.StatusInternalServerError)
        return
    }
    
    wr.WriteHeader(http.StatusOK)
    wr.Write(resp)
    log.Printf("GetContainers: Returned %d containers for user '%s'.", len(jsonList), decodedUsername)
}
// CheckUserExists checks if a user exists in the database.
func CheckUserExists(username string) bool {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    count, err := db.UserInfoCollection.CountDocuments(ctx, bson.M{"username": username})
    if err != nil {
        log.Printf("CheckUserExists: Failed to count documents for username '%s': %v", username, err)
        return false
    }
    log.Printf("CheckUserExists: Found %d users with username '%s'.", count, username)
    return count > 0
}
// Register handles the HTTP request to register a new user.
func Register(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
    defer cancel()
    var u linux_virt_unit.UserInfo
    body, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("Register: Failed to read request body: %v", err)
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }
    
    if err := json.Unmarshal(body, &u); err != nil {
        log.Printf("Register: Failed to parse JSON request body: %v", err)
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }
    log.Printf("Register: Received registration request for user '%s' (encrypted).", u.Username)
    
    u.Username, err = linux_virt_unit_crypto.DecryptString(u.Username, u.Key, u.UsernameIV)
    if err != nil {
        log.Printf("Register: Failed to decrypt username: %v", err)
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }
    log.Printf("Register: Decrypted username: '%s'.", u.Username)
    
    if CheckUserExists(u.Username) {
        log.Printf("Register: User '%s' already registered.", u.Username)
        http.Error(wr, "User already registered", http.StatusConflict)
        return
    }
    
    // Hash the password
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
    if err != nil {
        log.Printf("Register: Failed to hash password for user '%s': %v", u.Username, err)
        http.Error(wr, "Failed to hash password: "+err.Error(), http.StatusInternalServerError)
        return
    }
    u.Password = string(hashedPassword)
    log.Printf("Register: Password hashed for user '%s'.", u.Username)
    
    if _, err := db.UserInfoCollection.InsertOne(ctx, u); err != nil {
        log.Printf("Register: Failed to register user '%s' in MongoDB: %v", u.Username, err)
        http.Error(wr, "Failed to register user: "+err.Error(), http.StatusInternalServerError)
        return
    }
    
    wr.Write([]byte("User Registration Done"))
    log.Printf("Register: User '%s' registered successfully.", u.Username)
}
