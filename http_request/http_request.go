package http_request


import (
    "net/http"
    i "github.com/yoonjin67/lvirt_applicationUnit/incusUnit"
    lvirt "github.com/yoonjin67/lvirt_applicationUnit"
    db "github.com/yoonjin67/lvirt_applicationUnit/mongo_connect"
    "log"
    "time"
    "sync"
    "github.com/gorilla/mux"
)

// 포트 관리를 위한 뮤텍스 추가
var portMutex sync.Mutex

func InitHttpRequest(WorkQueue *i.ContainerQueue) {

    db.InitMongoDB()
    defer db.CloseMongoDB() // 프로그램 종료 시 MongoDB 연결 해제
    WorkQueue.Start(5) // 5개의 작업자 시작
    defer WorkQueue.Stop()

    // 라우터 설정
    lvirt.Route = mux.NewRouter()
    lvirt.Route.HandleFunc("/register", i.Register).Methods("POST")
    lvirt.Route.HandleFunc("/create", i.CreateContainer).Methods("POST")
    lvirt.Route.HandleFunc("/request", i.GetContainers).Methods("POST")
    lvirt.Route.HandleFunc("/delete", i.DeleteByTag).Methods("POST")
    lvirt.Route.HandleFunc("/stop", i.StopByTag).Methods("POST")
    lvirt.Route.HandleFunc("/start", i.StartByTag).Methods("POST")
    lvirt.Route.HandleFunc("/pause", i.PauseByTag).Methods("POST")
    lvirt.Route.HandleFunc("/restart", i.RestartByTag).Methods("POST")


    // HTTP 서버 설정
    srv := &http.Server{
        Handler:      lvirt.Route,
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


