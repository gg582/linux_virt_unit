package incus_unit

import (
    "net/http"
    "encoding/json"
    //about http requests

    "log"
    //logging 

    "context"
    "os"
    "io"
    "time"
    //file io, and contexts
    
    "github.com/lxc/incus/shared/api"
    //incus api
    "go.mongodb.org/mongo-driver/bson"
    //mongodb bson string

    "github.com/yoonjin67/linux_virt_unit"
    linux_virt_unit_crypto "github.com/yoonjin67/linux_virt_unit/crypto"
    db "github.com/yoonjin67/linux_virt_unit/mongo_connect"
    //custom modules
)

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



// GetContainers retrieves a list of containers for a specific user by manually scanning the collection.
func GetContainers(wr http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wr.Header().Set("Content-Type", "application/json; charset=utf-8")

	var in linux_virt_unit.UserInfo
	body, err := io.ReadAll(req.Body)
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
    //Received request (encrypted)

	// Decrypt the username
	decodedUsername, err := linux_virt_unit_crypto.DecryptString(in.Username, in.Key, in.UsernameIV)
	if err != nil {
		log.Printf("GetContainers: Failed to decrypt username: %v", err)
		http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("GetContainers: Decrypted username: '%s'.", decodedUsername)

	// Manually scan the entire collection
	cur, err := db.ContainerInfoCollection.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("GetContainers: Error finding container information in MongoDB: %v", err)
		http.Error(wr, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	// Collect matching containers
	jsonList := make([]interface{}, 0, 100000)
	for cur.Next(ctx) {
		var info linux_virt_unit.ContainerInfo
		if err := cur.Decode(&info); err != nil {
			log.Printf("GetContainers: Error decoding container document from MongoDB: %v", err)
			continue
		}

		// Decrypt stored username
		usernameFromDB, err := linux_virt_unit_crypto.DecryptString(info.Username, info.Key, info.UsernameIV)
		if err != nil {
			log.Printf("GetContainers: Error decrypting username from DB for tag '%s': %v", info.TAG, err)
			continue
		}

		// Check if the decrypted username matches the requester
		if usernameFromDB == decodedUsername {
			// Retrieve container status
			inst, _, err := IncusCli.GetInstance(info.TAG)
			if err == nil {
				info.VMStatus = inst.Status
				if inst.StatusCode == api.Frozen {
					info.VMStatus = "Frozen"
				}
			} else {
				log.Printf("GetContainers: Failed to get Incus instance state for tag '%s': %v", info.TAG, err)
				info.VMStatus = "unknown"
			}
			jsonList = append(jsonList, info)
		}
	}

	// Encode response
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


