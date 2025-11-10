package incus_unit

import (
    "encoding/json"
    "net/http"
    //about http requests

    "log"
    //logging

    "context"
    "io"
    "os"
    "time"
    //file io, and contexts

    "github.com/lxc/incus/shared/api"
    //incus api
    "go.mongodb.org/mongo-driver/bson"
    //mongodb bson string

    "github.com/gg582/linux_virt_unit"
    linux_virt_unit_crypto "github.com/gg582/linux_virt_unit/crypto"
    db "github.com/gg582/linux_virt_unit/mongo_connect"
    //custom modules
)

// getTAG generates a unique tag for a container.
func getTAG(tag string) string {
    var err error
    var file *os.File
    filePath := linux_virt_unit.LINUX_VIRT_PATH + "/container/latest_access"
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


// @Summary Get containers
// @Description GetContainers retrieves a list of containers for a specific user by manually scanning the collection.
// @Accept json
// @Produce json
// @Param request body linux_virt_unit.UserInfo true "User information"
//
//    {
//       "username": "user123",
//       "username_iv": "someIV1",
//       "password": "passwordHash",
//       "key": "encryptionKey",
//    }
//
// @Success 200 {array} linux_virt_unit.ContainerInfo "Created containers list"
// [
// {
//
//       "username": "user123",
//       "username_iv": "someIV1",
//       "password": "encryptedPassword",
//       "password_iv": "someIV2",
//       "key": "encryptionKey",
//       "tag": "ubuntu20",
//       "serverip": "10.72.1.100",
//       "serverport": "27020",
//       "vmstatus": "running",
//       "distro": "ubuntu",
//       "version": "20.04"
//    },
//
//    {
//       "username": "user122",
//       "username_iv": "someIV1",
//       "password": "encryptedPassword",
//       "password_iv": "someIV2",
//       "key": "encryptionKey",
//       "tag": "ubuntu24",
//       "serverip": "10.72.1.101",
//       "serverport": "27023",
//       "vmstatus": "running",
//       "distro": "ubuntu",
//       "virtual": "24.04",
//    },
//
// ]
// @Failure 400
// @Router /request [post]
func GetContainers(wr http.ResponseWriter, req *http.Request) {
    if req.Method != http.MethodPost {
        http.Error(wr, "This endpoint allows only POST methods. aborting", http.StatusMethodNotAllowed)
        return
    }
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
    in.Username, err = linux_virt_unit_crypto.DecryptString(in.Username, in.Key, in.UsernameIV)
    if err != nil {
        log.Printf("GetContainers: Failed to decrypt username: %v", err)
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }
    log.Printf("GetContainers: Decrypted username: '%s'.", in.Username)

    if CheckUserExists(in.Username, in.Password, ctx) == false {
        wr.WriteHeader(404)
        wr.Write([]byte("No user found\n"))
        return
    }
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
        if usernameFromDB == in.Username {
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
    log.Printf("GetContainers: Returned %d containers for user '%s'.", len(jsonList), in.Username)
}

func GetImages(wr http.ResponseWriter, req *http.Request) {
    if req.Method != http.MethodGet {
        http.Error(wr, "This endpoint allows only GET methods. aborting", http.StatusMethodNotAllowed)
        return
    }
    wr.Header().Set("Content-Type", "application/json; charset=utf-8")

    images := make([]string, 0, len(BaseImages))
    for k := range BaseImages {
        images = append(images, k)
    }

    resp, err := json.Marshal(images)
    if err != nil {
        log.Printf("GetImages: Failed to marshal image list to JSON: %v", err)
        http.Error(wr, "Failed to marshal response: "+err.Error(), http.StatusInternalServerError)
        return
    }

    wr.WriteHeader(http.StatusOK)
    wr.Write(resp)
    log.Printf("GetImages: Returned %d images.", len(images))
}

