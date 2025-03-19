package http_request

import (
    "net/http"
    "github.com/gorilla/mux"
    "encoding/base64"
    "github.com/yoonjin67/lvirt_applicationUnit/crypto"
    "github.com/yoonjin67/lvirt_applicationUnit"
    "github.com/yoonjin67/lvirt_applicationUnit/incusUnit"
    "encoding/json"
    "context"
)


func initHttpRequest(containerQueue *ContainerQueue) {
    containerQueue.Start(5) // 5개의 작업자 시작
    defer containerQueue.Stop()

    // 라우터 설정
    route = mux.NewRouter()
    route.HandleFunc("/register", Register).Methods("POST")
    route.HandleFunc("/create", CreateContainer).Methods("POST")
    route.HandleFunc("/request", GetContainers).Methods("POST")
    route.HandleFunc("/delete", DeleteByTag).Methods("POST")
    route.HandleFunc("/stop", StopByTag).Methods("POST")
    route.HandleFunc("/start", StartByTag).Methods("POST")
    route.HandleFunc("/pause", PauseByTag).Methods("POST")
    route.HandleFunc("/restart", RestartByTag).Methods("POST")


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

func Register(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    var u UserInfo
    body, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }

    if err := json.Unmarshal(body, &u); err != nil {
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    u.Password, err = decrypt_password(u.Password, u.Key, u.PasswordIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt password: "+err.Error(), http.StatusBadRequest)
        return
    }
    u.Username, err = decrypt_password(u.Username, u.Key, u.UsernameIV)
    if err != nil {
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }

    if _, err := UserCol.InsertOne(ctx, u); err != nil {
        http.Error(wr, "Failed to register user: "+err.Error(), http.StatusInternalServerError)
        return
    }

    wr.Write([]byte("User Registration Done"))
}


