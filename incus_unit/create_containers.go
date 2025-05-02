package incus_unit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	client "github.com/lxc/incus/client"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/lxc/incus/shared/api"
	"github.com/yoonjin67/linux_virt_unit"
	linux_virt_unit_crypto "github.com/yoonjin67/linux_virt_unit/crypto"
	db "github.com/yoonjin67/linux_virt_unit/mongo_connect"
)

const MAX_PORT = 60001

var IncusCli client.InstanceServer
var mydir string = "/usr/local/bin/linuxVirtualization/"
var SERVER_IP = "127.0.0.1"
var PORT_LIST = make([]int, 0, 100000)
var flag bool
var authFlag bool = false
var port string
var cursor interface{}
var current []byte
var current_Config []byte
var buf bytes.Buffer

const INITIAL_PORT = 27020

var ADDR string = "http://hobbies.yoonjin2.kr"

// Mutex to manage port allocation.
var containerManageMutex sync.Mutex
var nginxMutex sync.Mutex
var portDeleteMutex sync.Mutex

// TouchFile creates an empty file if it doesn't exist.
func TouchFile(name string) {
	file, _ := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0644)
	file.Close()
	log.Printf("TouchFile: File '%s' touched.", name)
}

// @Summary Create a new container
// @Description Creates a new container with the provided information.
// @Accept json
// @Produce json
//
//	@Param request body linux_virt_unit.ContainerInfo true "Container creation request" example(application/json)={ \
//	   "username": "user123", \
//	   "username_iv": "someIV1", \
//	   "password": "encryptedPassword", \
//	   "password_iv": "someIV2", \
//	   "key": "encryptionKey", \
//	   "tag": "uvuntu", \
//	   "serverip": "10.72.1.100", \
//	   "serverport": "27020", \
//	   "vmstatus": "running", \
//	   "distro": "ubuntu", \
//	   "version": "20.04" \
//	}
//
// @Success 200 {object} linux_virt_unit.ContainerInfo "Container Info"
//
//	{
//	   "username": "user123",
//	   "username_iv": "someIV1",
//	   "password": "encryptedPassword",
//	   "password_iv": "someIV2",
//	   "key": "encryptionKey",
//	   "tag": "ubuntu-randtag",
//	   "serverip": "10.72.1.100",
//	   "serverport": "27023",
//	   "vmstatus": "running",
//	   "distro": "ubuntu",
//	   "version": "20.04"
//	}
//
// @Router /create [post]
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
		http.Error(wr, "Invalid field", http.StatusBadRequest)
		return
	}
	//parse distro info
	info.Distro = info.Distro + "-" + info.DistroVersion
	log.Println("Distro Alias is " + info.Distro)
	if len(baseImages[info.Distro]) == 0 {
		log.Println("image is not available. check https://images.linuxcontainers.org")
		log.Println("You requested " + info.Distro)
		http.Error(wr, "Invalid Distro", http.StatusBadRequest)
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

// createContainer handles the actual creation of a container.

func createContainer(info linux_virt_unit.ContainerInfo) {
	// Decrypt username and password
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()
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
	log.Printf("createContainer: Decrypted Username: '%s'", username)
	log.Printf("createContainer: Decrypted Password: '%s'", password)
	if !CheckUserExists(username, password, ctx) {
		log.Printf("createContainer: User '%s' does not exist.", username)
		return
	}
	log.Printf("createContainer: User '%s' exists.", username)

	// Generate a unique tag for the container
	tag := getTAG(mydir, info.TAG)
	info.TAG = tag
	log.Printf("createContainer: Generated tag '%s' for user '%s'.", tag, username)

	// Set a timeout for container creation

	containerConfig := api.InstancesPost{
		Name: tag,
		Source: api.InstanceSource{
			Type:        "image",
			Fingerprint: baseImages[info.Distro],
		},
	}
	op, err := IncusCli.CreateInstance(containerConfig)
	if err != nil {
		log.Printf("cretateContainer: Failed to create container. Error: %v\n", err)
		return
	}

	// Wait for the container to be created successfully
	err = op.Wait()
	if err != nil {
		log.Printf("createContainer: Failed to wait for container creation for tag '%s': %v", tag, err)
		return
	}

	ChangeState(tag, "start")

	// Allocate a unique port for the container
    nginxMutex.Lock()
    defer nginxMutex.Unlock()
	allocatedPort, err := allocateUniquePort()
	if err != nil {
		log.Printf("createContainer: Failed to allocate a unique port for tag '%s': %v", tag, err)
		return
	}
	port := strconv.Itoa(allocatedPort)
	info.Serverport = port
    target := PortTagTarget {
        tag: tag,
        port: allocatedPort,
    }

    WorkQueue.RetrieveTag <- target

	// LOCK THE MUTEX HERE
	// Port should not be duplicated
	
	log.Printf("createContainer: Allocated new port '%d' for tag '%s'.", allocatedPort, tag)
	log.Printf("createContainer: Attempting to create container with tag '%s', port '%s', user '%s' password '%s'.", tag, port, username, password)

	fmt.Println("Nginx configuration has been successfully updated.")
	command := []string{"/bin/bash", "/conSSH.sh", username, password, tag}
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
	op, err = IncusCli.ExecInstance(tag, execArgs, &ioDescriptor)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup SSH: %v\n", err)
		os.Exit(1)
	}

	err = op.Wait()
	if err != nil {
		log.Printf("createContainer: (ssh) Failed to get SSH setup result: %v\n", err)
		os.Exit(1)
	}



    _, insertErr := db.ContainerInfoCollection.InsertOne(ctx, info)
    if insertErr != nil {
    	log.Printf("createContainer: Cannot insert container info into MongoDB for tag '%s': %v", tag, insertErr)
    	go DeleteContainerByName(tag)
        err := <- WorkQueue.WQReturns
    	if err != nil {
    		log.Printf("createContainer: Failed to delete potentially failed Incus container '%s': %v", tag, err)
    	} else {
    		log.Printf("createContainer: Attempted to delete Incus container '%s' after MongoDB insertion failure.", tag)
    	}
    } else {
    	log.Printf("createContainer: Container info inserted into MongoDB for tag '%s'.", tag)
    }

}

func allocateUniquePort() (int, error) {

	for port := INITIAL_PORT; port <= MAX_PORT; port += 3 {
		found, _ := db.FindPort(port)
		if !found {
			return port, nil
		}
	}
	return -1, fmt.Errorf("no available port in the range %d-%d", INITIAL_PORT, MAX_PORT)
}
