package incusUnit
import (
    "github.com/yoonjin67/lvirt_applicationUnit"
    "github.com/yoonjin67/lvirt_applicationUnit/crypto"
)

func TouchFile(name string) {
    file, _ := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0644)
    file.Close()
}

func get_TAG(mydir string, user string) string {
    var err error
    var file *os.File
    file, err = os.OpenFile(mydir+"/container/latest_access", os.O_RDWR, os.FileMode(0644))
    if err != nil {
        log.Println(tag)
    }
    tagRet := user+"-"+RandStringBytes(20)
    file.Write([]byte(tagRet))
    file.Close()
    return tagRet
}

// 컨테이너 생성을 위한 작업자 풀
type ContainerQueue struct {
    tasks chan ContainerInfo
    wg    sync.WaitGroup
}

var containerQueue = &ContainerQueue{
    tasks: make(chan ContainerInfo, 100), // 버퍼 크기 100으로 설정
}

func (q *ContainerQueue) Start(numWorkers int) {
    for i := 0; i < numWorkers; i++ {
        q.wg.Add(1)
        go q.worker()
    }
}

func (q *ContainerQueue) Stop() {
    close(q.tasks)
    q.wg.Wait()
}

func (q *ContainerQueue) worker() {
    defer q.wg.Done()
    for info := range q.tasks {
        createContainer(info)
    }
}

func getContainerInfo(tag string, info ContainerInfo) ContainerInfo {
     state, _, err := lxdClient.GetInstanceState(tag)
     if err != nil {
         log.Println("failed to get instance state")
     }
    // 결과 문자열 처리
    info.VMStatus = state.Status

    // 결과 출력
    fmt.Println("STATE:", info.VMStatus)
    return info
}


func createContainer(info ContainerInfo) {
    username, err := decrypt_password(info.Username, info.Key, info.UsernameIV)
    password, err := decrypt_password(info.Password, info.Key, info.PasswordIV)
    if err != nil {
        return
    }
    tag := get_TAG(mydir, username)
    info.TAG = tag

    portMutex.Lock()
    if portHeap.Len() == 0 {
        port := strconv.Itoa(portInt + 3)
        log.Println("/container_creation.sh " + tag + " " + port + " " + username +  " " + password)
        portInt += 3
    } else {
        port := strconv.Itoa(heap.Pop(portHeap).(int))
        log.Println("/container_creation.sh " + tag + " " + port + " " + username +  " " + password)
    }
    portMutex.Unlock()

    info.Serverport = port
    portprev = port

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    cmdCreate := exec.CommandContext(ctx, "/bin/bash", "-c", "container_creation.sh "+tag+" "+port+" "+username+" "+password)
    cmdCreate.Stdout = os.Stdout
    cmdCreate.Stderr = os.Stderr
    
    if err := cmdCreate.Run(); err != nil {
        log.Printf("Error creating container: %v", err)
        return
    }

    mcEXEC := exec.CommandContext(ctx, "/bin/bash", "-c",  "init_server.sh " +tag)
    mcEXEC.Stdout = os.Stdout
    mcEXEC.Stderr = os.Stderr
    if err := mcEXEC.Run(); err != nil {
        log.Printf("Error initializing server: %v", err)
        return
    }

    info = getContainerInfo(tag, info)

    ipRes, insertErr := ipCol.InsertOne(ctx, info)
    if insertErr != nil {
        log.Println("Cannot insert container IP into MongoDB")
    } else {
        log.Println("container IP Insert succeed. Result is : ", ipRes)
    }

}

func CreateContainer(wr http.ResponseWriter, req *http.Request) {
    wr.Header().Set("Content-Type", "application/json; charset=utf-8")

    var info ContainerInfo
    if err := json.NewDecoder(req.Body).Decode(&info); err != nil {
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    select {
    case containerQueue.tasks <- info:
        string_Reply, _ := json.Marshal(info)
        wr.Write(string_Reply)
    default:
        http.Error(wr, "Server is busy", http.StatusServiceUnavailable)
    }
}

func DeleteFromListByValue(slice []int64, value int64) []int64 {
    for i, itm := range slice {
        if itm == value {
            return append(slice[:i], slice[i+1:]...)
        }
    }
    return slice
}

func ChangeState(tag string, state string) {
    req := api.InstanceStatePut{
        Action: state,
    }

    _, err := lxdClient.UpdateInstanceState(tag, req, "")
    if err != nil {
        log.Fatalf("Container state change failed: %v", err)
    }
}

func StopByTag(wr http.ResponseWriter, req *http.Request) {
    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    //stringForStopTask := string(forTag)
    //cmdStop := exec.CommandContext(ctx, "/bin/bash", "-c", "stop.sh " +stringForStopTask)
    //cmdStop.Run()
    ChangeState(string(forTag), "stop")
}

func RestartByTag(wr http.ResponseWriter, req *http.Request) {

    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    log.Println("Received TAG:" + string(forTag))
    ChangeState(string(forTag), "restart")

}

func PauseByTag(wr http.ResponseWriter, req *http.Request) {

    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    log.Println("Received TAG:" + string(forTag))
    ChangeState(string(forTag), "freeze")

}

func StartByTag(wr http.ResponseWriter, req *http.Request) {

    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    log.Println("Received TAG:" + string(forTag))
    ChangeState(string(forTag), "start")
    //stringForStartTask := string(forTag)
    //cmdStart := exec.CommandContext(ctx, "/bin/bash", "-c", "start.sh "+stringForStartTask)
    //cmdStart.Run()

}

func DeleteByTag(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    stringForTag := string(forTag)
    cmdDelete := exec.CommandContext(ctx, "/bin/bash", "delete_container.sh "+stringForTag)

    cur, err := ipCol.Find(ctx, bson.D{{}})
    if err != nil {
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)

    for cur.Next(ctx) {
        resp, err := bson.MarshalExtJSON(cur.Current, false, false)
        if err != nil {
            continue
        }
        var INFO ContainerInfo
        if err := json.Unmarshal(resp, &INFO); err != nil {
            continue
        }
        if INFO.TAG == stringForTag {
            p32, _ := strconv.Atoi(INFO.Serverport)
            p := int(p32)
            
            portMutex.Lock()
            PORT_LIST = DeleteFromListByValue(PORT_LIST, int64(p))
            heap.Push(portHeap, int64(p))
            portMutex.Unlock()

            if _, err := ipCol.DeleteOne(ctx, cur.Current); err != nil {
                log.Printf("Error deleting container from database: %v", err)
            }

            cmdDelete.Stdout = os.Stdout
            cmdDelete.Stderr = os.Stderr
            if err := cmdDelete.Run(); err != nil {
                log.Printf("Error deleting container: %v", err)
                http.Error(wr, "Failed to delete container", http.StatusInternalServerError)
                return
            }
            return
        }
    }
}

func GetContainers(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    INFO.Serverip = SERVER_IP
    wr.Header().Set("Content-Type", "application/json; charset=utf-8")

    var in UserInfo
    body, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }

    if err := json.Unmarshal(body, &in); err != nil {
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    decodedUsername, err := decrypt_password(in.Username, in.Key, in.UsernameIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }
    decodedPassword, err := decrypt_password(in.Password, in.Key, in.PasswordIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt password: "+err.Error(), http.StatusBadRequest)
        return
    }

    cur, err := ipCol.Find(ctx, bson.D{{}})
    if err != nil {
        log.Println("Error on finding information: ", err)
        http.Error(wr, "Database error: "+err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)

    jsonList := make([]interface{}, 0, 100000)
    for cur.Next(ctx) {
        var info ContainerInfo
        if err := cur.Decode(&info); err != nil {
            log.Println("Error decoding document: ", err)
            continue
        }
        Username, _ := decrypt_password(info.Username, info.Key, info.UsernameIV)
        Password, _ := decrypt_password(info.Password, info.Key, info.PasswordIV)
        if Username == decodedUsername && Password == decodedPassword {
            jsonList = append(jsonList, info)
        }
    }

    resp, err := json.Marshal(jsonList)
    if err != nil {
        http.Error(wr, "Failed to marshal response: "+err.Error(), http.StatusInternalServerError)
        return
    }

    wr.WriteHeader(http.StatusOK)
    wr.Write(resp)
}
