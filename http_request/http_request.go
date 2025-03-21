package http_request


import (
    client "github.com/lxc/incus/client"
    "net/http"
    "context"
    "bytes"
    i "github.com/yoonjin67/lvirt_applicationUnit/incusUnit"
    "log"
    "os"
    "sync"
    "time"

    "github.com/gorilla/mux"
    "go.mongodb.org/mongo-driver/mongo"
)

var ePlace int64
var IncusCli client.InstanceServer
var mydir string = "/usr/local/bin/linuxVirtualization/"
var SERVER_IP = os.Args[1]
var PORT_LIST = make([]int64,0,100000)
var flag   bool
var authFlag bool = false
var port   string
var portprev string = "60001"
var cursor interface{}
var route *mux.Router
var route_MC *mux.Router
var current []byte
var current_Config []byte
var buf bytes.Buffer
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890"
var col *mongo.Collection
var ipCol , UserCol *mongo.Collection
var portInt int = 27020
var portIntonePlace int = 27020
var ctx context.Context
var cancel context.CancelFunc
var tag string
var ADMIN    string = "yjlee"
var password string = "asdfasdf"
var ADDR string = "http://hobbies.yoonjin2.kr"

// 포트 관리를 위한 뮤텍스 추가
var portMutex sync.Mutex

type UserInfo struct {
    Username     string `json:"username"`
    UsernameIV   string `json:"username_iv"`
    Password     string `json:"password"`
    PasswordIV   string `json:"password_iv"`
    Key          string `json:"key"`
}

type ContainerInfo struct {
    Username string `json:"username"`
    UsernameIV string `json:"username_iv"`
    Password string `json:"password"`
    PasswordIV       string `json:"password_iv"`
    Key      string `json:"key"`
    TAG      string `json:"tag"`
    Serverip string `json:"serverip"`
    Serverport string `json:"serverport"`
    VMStatus     string `json:"vmstatus"`
}

var INFO ContainerInfo

func InitHttpRequest(WorkQueue *i.ContainerQueue) {
    WorkQueue.Start(5) // 5개의 작업자 시작
    defer WorkQueue.Stop()

    // 라우터 설정
    route = mux.NewRouter()
    route.HandleFunc("/register", i.Register).Methods("POST")
    route.HandleFunc("/create", i.CreateContainer).Methods("POST")
    route.HandleFunc("/request", i.GetContainers).Methods("POST")
    route.HandleFunc("/delete", i.DeleteByTag).Methods("POST")
    route.HandleFunc("/stop", i.StopByTag).Methods("POST")
    route.HandleFunc("/start", i.StartByTag).Methods("POST")
    route.HandleFunc("/pause", i.PauseByTag).Methods("POST")
    route.HandleFunc("/restart", i.RestartByTag).Methods("POST")


    // HTTP 서버 설정
    srv := &http.Server{
        Handler:      route,
        Addr:         ":32000",
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    // 서버 시작
    log.Printf("Starting server on port 32000")
    if err := srv.ListenAndServe(); err != nil {
        log.Fatal(err)
    }
}


