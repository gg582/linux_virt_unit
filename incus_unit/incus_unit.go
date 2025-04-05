package incus_unit

import (
    "fmt"
    "errors"
    "container/heap"
    "strings"
    client "github.com/lxc/incus/client"
    "github.com/lxc/incus/shared/api"
    "net/http"
    linux_virt_unit_crypto "github.com/yoonjin67/linux_virt_unit/crypto"
    linux_virt_unit "github.com/yoonjin67/linux_virt_unit"
    db "github.com/yoonjin67/linux_virt_unit/mongo_connect"
    "context"
    "bytes"
    "encoding/json"
    "io/ioutil"
    "log"
    "os"
    "os/exec"
    "strconv"
    "sync"
    "time"

    "go.mongodb.org/mongo-driver/bson"
)


var INFO linux_virt_unit.ContainerInfo


// IntHeap은 int64 값을 저장하는 최소 힙입니다.
type IntHeap []int

// Len은 요소 개수를 반환합니다.
func (h *IntHeap) Len() int { return len(*h) }

// Less는 작은 값이 먼저 나오도록 정렬합니다.
func (h *IntHeap) Less(i, j int) bool {
    var heap_nopointer = *h
    return heap_nopointer[i] < heap_nopointer[j] 
}

// Swap은 두 요소를 교환합니다.
func (h *IntHeap) Swap(i, j int) { 
    var heap_nopointer = *h
    heap_nopointer[i], heap_nopointer[j] = heap_nopointer[j], heap_nopointer[i] 
}

// Push는 새로운 요소를 추가합니다.
func (h *IntHeap) Push(x interface{}) {
	*h = append(*h, x.(int))
}

// Pop은 최솟값을 제거하고 반환합니다.
func (h *IntHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

var PortHeap *IntHeap

var ePlace int64
var IncusCli client.InstanceServer
var mydir string = "/usr/local/bin/linuxVirtualization/"
var SERVER_IP = "127.0.0.1"
var PORT_LIST = make([]int64,0,100000)
var flag   bool
var authFlag bool = false
var port   string
var portprev string = "60001"
var cursor interface{}
var current []byte
var current_Config []byte
var buf bytes.Buffer
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890"
var portInt int = 27020
var portIntonePlace int = 27020
var ctx context.Context
var cancel context.CancelFunc
var tag string
var ADDR string = "http://hobbies.yoonjin2.kr"

// 포트 관리를 위한 뮤텍스 추가
var portMutex sync.Mutex


// 컨테이너 생성을 위한 작업자 풀
type ContainerQueue struct {
    Tasks chan linux_virt_unit.ContainerInfo
    wg    sync.WaitGroup
}

var WorkQueue *ContainerQueue

func InitWorkQueue() {
    WorkQueue = &ContainerQueue{
        Tasks: make(chan linux_virt_unit.ContainerInfo, 1024),
        wg:    sync.WaitGroup{},
    }
}


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
    tagRet := user+"-"+linux_virt_unit_crypto.RandStringBytes(20)
    file.Write([]byte(tagRet))
    file.Close()
    return tagRet
}

func (q *ContainerQueue) Start(numWorkers int) {
    for i := 0; i < numWorkers; i++ {
        q.wg.Add(1)
        go q.worker()
    }
}

func (q *ContainerQueue) Stop() {
    close(q.Tasks)
    q.wg.Wait()
}

func (q *ContainerQueue) worker() {
    defer q.wg.Done()
    for info := range q.Tasks {
        createContainer(info)
    }
}

func GetContainerInfo(tag string, info linux_virt_unit.ContainerInfo) linux_virt_unit.ContainerInfo {
     state, _, err := IncusCli.GetInstanceState(tag)
     if err != nil {
         log.Println("failed to get instance state")
     }
    // 결과 문자열 처리
    info.VMStatus = state.Status

    // 결과 출력
    fmt.Println("STATE:", info.VMStatus)
    return info
}


func createContainer(info linux_virt_unit.ContainerInfo) {
    username, err := linux_virt_unit_crypto.DecryptString(info.Username, info.Key, info.UsernameIV)
    password, err := linux_virt_unit_crypto.DecryptString(info.Password, info.Key, info.PasswordIV)
    if err != nil {
        return
    }
    tag := get_TAG(mydir, username)
    info.TAG = tag

    portMutex.Lock()
    if PortHeap.Len() == 0 {
        port = strconv.Itoa(portInt + 3)
        log.Println("/container_creation.sh " + tag + " " + port + " " + username +  " " + password)
        portInt += 3
    } else {
        port = strconv.Itoa(heap.Pop(PortHeap).(int))
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
    cmdCreate.Wait()

    info = GetContainerInfo(tag, info)

    ipRes, insertErr := db.ContainerInfoCollection.InsertOne(context.Background(), info)
    if insertErr != nil {
        log.Println("Cannot insert container IP into MongoDB")
    } else {
        log.Println("container IP Insert succeed. Result is : ", ipRes)
    }

}

func CreateContainer(wr http.ResponseWriter, req *http.Request) {
    wr.Header().Set("Content-Type", "application/json; charset=utf-8")

    var info linux_virt_unit.ContainerInfo
    if err := json.NewDecoder(req.Body).Decode(&info); err != nil {
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    select {
    case WorkQueue.Tasks <- info:
        string_Reply, _ := json.Marshal(info)
        wr.Write(string_Reply)
    default:
        http.Error(wr, "Server is busy", http.StatusServiceUnavailable)
    }
}

func DeleteContainerByName(tag string) error {
    // local Incus 서버에 연결 (유닉스 소켓)
    // 컨테이너 존재 확인
    if tag == "" {
        log.Println("Error: tag is nil")
        return errors.New("tag is nil")
    }
    container, _, err := IncusCli.GetInstance(tag)
    if err != nil {
        return fmt.Errorf("failed to get container: %w", err)
    }

    // 컨테이너가 실행 중이면 멈춤
    if container.Status != "Stopped" {
        ChangeState(tag, "stop")
    }

    // 컨테이너 삭제
    op, err := IncusCli.DeleteInstance(tag)
    if err != nil {
        return fmt.Errorf("failed to delete container: %w", err)
    }

    return op.Wait()
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
    log.Println("ChangeState: Tag is ", tag)
    req := api.InstanceStatePut{
        Action: state,
    }

    _, err := IncusCli.UpdateInstanceState(tag, req, "")
    if err != nil {
        log.Printf("Container state change failed: %v\n", err)
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
    stringForTag := string(forTag)
    stringForTag = strings.Trim(stringForTag, "\"")
    ChangeState(stringForTag, "stop")
}

func RestartByTag(wr http.ResponseWriter, req *http.Request) {

    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    log.Println("Received TAG:" + string(forTag))
    stringForTag := string(forTag)
    stringForTag = strings.Trim(stringForTag, "\"")
    ChangeState(stringForTag, "restart")

}

func PauseByTag(wr http.ResponseWriter, req *http.Request) {

    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    log.Println("Received TAG:" + string(forTag))
    stringForTag := string(forTag)
    stringForTag = strings.Trim(stringForTag, "\"")
    ChangeState(stringForTag, "freeze")

}

func StartByTag(wr http.ResponseWriter, req *http.Request) {

    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    log.Println("Received TAG:" + string(forTag))
    stringForTag := string(forTag)
    stringForTag = strings.Trim(stringForTag, "\"")
    ChangeState(stringForTag, "start")

}

func DeleteByTag(wr http.ResponseWriter, req *http.Request) {
    log.Println("DeleteByTag: 시작")
    forTag, err := ioutil.ReadAll(req.Body)
    if err != nil {
        log.Printf("DeleteByTag: 요청 Body 읽기 실패: %v", err)
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }
    stringForTag := string(forTag)
    stringForTag = strings.Trim(stringForTag, "\"")
    log.Printf("DeleteByTag: 삭제할 태그: %s", stringForTag)

    cur, err := db.ContainerInfoCollection.Find(context.Background(), bson.D{{}})
    if err != nil {
        log.Printf("DeleteByTag: MongoDB 조회 실패: %v", err)
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(context.Background())
    log.Println("DeleteByTag: MongoDB 조회 완료")

    found := false
    var foundInfo linux_virt_unit.ContainerInfo

    for cur.Next(context.Background()) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            log.Printf("DeleteByTag: MongoDB 문서 디코딩 실패: %v", err)
            continue
        }
        if info.TAG == stringForTag {
            found = true
            foundInfo = info
            log.Printf("DeleteByTag: 찾은 컨테이너 정보: %+v", foundInfo)
            break
        }
    }

    if found {
        p32, err := strconv.Atoi(foundInfo.Serverport)
        if err != nil {
            log.Printf("DeleteByTag: ServerPort 변환 실패: %v", err)
            http.Error(wr, "Internal server error", http.StatusInternalServerError)
            return
        }
        p := int(p32)
        log.Printf("DeleteByTag: 변환된 포트: %d", p)

        portMutex.Lock()
        log.Printf("DeleteByTag: PortHeap 상태 (Before Pop): %+v", *PortHeap)
        PORT_LIST = DeleteFromListByValue(PORT_LIST, int64(p))
        heap.Push(PortHeap, int64(p))
        log.Printf("DeleteByTag: PortHeap 상태 (After Pop/Push): %+v", *PortHeap)
        portMutex.Unlock()

        filter := bson.M{"tag": stringForTag}
        _, err = db.ContainerInfoCollection.DeleteOne(context.Background(), filter)
        if err != nil {
            log.Printf("DeleteByTag: MongoDB 삭제 실패: %v", err)
        } else {
            log.Println("DeleteByTag: MongoDB 삭제 성공")
        }

        log.Println("DeleteByTag: DeleteContainerByName 호출")
        err = DeleteContainerByName(stringForTag)
        if err != nil {
            log.Printf("DeleteByTag: DeleteContainerByName 실패: %v", err)
        } else {
            log.Println("DeleteByTag: DeleteContainerByName 성공")
        }
        return
    } else {
        log.Printf("DeleteByTag: 태그 '%s'에 해당하는 컨테이너 정보 없음", stringForTag)
        http.Error(wr, fmt.Sprintf("Container with tag '%s' not found", stringForTag), http.StatusNotFound)
        return
    }
}

func GetContainers(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    wr.Header().Set("Content-Type", "application/json; charset=utf-8")

    var in linux_virt_unit.UserInfo
    body, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }

    if err := json.Unmarshal(body, &in); err != nil {
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    decodedUsername, err := linux_virt_unit_crypto.DecryptString(in.Username, in.Key, in.UsernameIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }
    decodedPassword, err := linux_virt_unit_crypto.DecryptString(in.Password, in.Key, in.PasswordIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt password: "+err.Error(), http.StatusBadRequest)
        return
    }

    cur, err := db.ContainerInfoCollection.Find(ctx, bson.D{{}})
    if err != nil {
        log.Println("Error on finding information: ", err)
        http.Error(wr, "Database error: "+err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)

    jsonList := make([]interface{}, 0, 100000)
    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            log.Println("Error decoding document: ", err)
            continue
        }
        Username, _ := linux_virt_unit_crypto.DecryptString(info.Username, info.Key, info.UsernameIV)
        Password, _ := linux_virt_unit_crypto.DecryptString(info.Password, info.Key, info.PasswordIV)
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

func Register(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    var u linux_virt_unit.UserInfo
    body, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }

    if err := json.Unmarshal(body, &u); err != nil {
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    u.Password, err = linux_virt_unit_crypto.DecryptString(u.Password, u.Key, u.PasswordIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt password: "+err.Error(), http.StatusBadRequest)
        return
    }
    u.Username, err = linux_virt_unit_crypto.DecryptString(u.Username, u.Key, u.UsernameIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }

    if _, err := db.UserInfoCollection.InsertOne(ctx, u); err != nil {
        http.Error(wr, "Failed to register user: "+err.Error(), http.StatusInternalServerError)
        return
    }

    wr.Write([]byte("User Registration Done"))
}


