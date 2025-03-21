package mongo_connect

import (
    lvirt "github.com/yoonjin67/lvirt_applicationUnit"
    "net/http"
    "context"
    "encoding/json"
    "io/ioutil"
    "log"
    "time"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

var ADMIN    string = "yjlee"
var password string = "asdfasdf"


func botCheck(u string, pw string) bool {
    cur, err := lvirt.UserCol.Find(context.Background(), bson.D{{}})
    if err != nil {
        log.Printf("Database query error: %v", err)
        return true
    }
    defer cur.Close(context.Background())

    for cur.Next(context.TODO()) {
        current, err := bson.MarshalExtJSON(cur.Current, false, false)
        if err != nil {
            continue
        }
        var i lvirt.UserInfo
        if err := json.Unmarshal(current, &i); err != nil {
            continue
        }
        if i.Password == pw && i.Username == u {
            return false
        }
    }
    return true
}

func check(u string, pw string) bool {
    if (u == ADMIN) && !botCheck(u, pw) {
        return true
    }
    return false
}


func UseContainer(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    wr.Header().Set("Content-Type", "application/json; charset=utf-8")
    
    var in lvirt.UserInfo
    body, err := ioutil.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }
    
    if err := json.Unmarshal(body, &in); err != nil {
        http.Error(wr, err.Error(), http.StatusBadRequest)
        return
    }

    filter := bson.M{"username": in.Username, "password": in.Password}
    cur, err := lvirt.AddrCol.Find(ctx, filter)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)

    var results []lvirt.ContainerInfo
    for cur.Next(ctx) {
        var info lvirt.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            continue
        }
        results = append(results, info)
    }

    resp, err := json.Marshal(results)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }

    wr.Write(resp)
}


var MongoClient *mongo.Client // 전역 변수로 선언

func InitMongoDB() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    var err error
    MongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
    if err != nil {
        log.Fatal(err)
    }

    // 연결 테스트
    err = MongoClient.Ping(ctx, nil)
    if err != nil {
        log.Fatal(err)
    }

    // 컬렉션 초기화
    lvirt.Colct = MongoClient.Database("MC_Json").Collection("Flag Collections")
    lvirt.AddrCol = MongoClient.Database("MC_IP").Collection("IP Collections")
    lvirt.UserCol = MongoClient.Database("MC_USER").Collection("User Collections")

    log.Println("MongoDB 연결 성공")
}

func CloseMongoDB() {
    if MongoClient != nil {
        MongoClient.Disconnect(context.Background())
        log.Println("MongoDB 연결 종료")
    }
}

