package mongo_connect

import (
    "net/http"
    "context"
    "encoding/json"
    "io/ioutil"
    "log"
    "time"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    linux_virt_unit "github.com/yoonjin67/linux_virt_unit"
)

var ContainerInfoCollection, UserInfoCollection *mongo.Collection
var MongoClient *mongo.Client // 전역 변수로 선언
var INFO linux_virt_unit.ContainerInfo


func botCheck(u string, pw string) bool {
    cur, err := UserInfoCollection.Find(context.Background(), bson.D{{}})
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
        var i linux_virt_unit.UserInfo
        if err := json.Unmarshal(current, &i); err != nil {
            continue
        }
        if i.Password == pw && i.Username == u {
            return false
        }
    }
    return true
}



func UseContainer(wr http.ResponseWriter, req *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    wr.Header().Set("Content-Type", "application/json; charset=utf-8")
    
    var in linux_virt_unit.UserInfo
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
    cur, err := ContainerInfoCollection.Find(ctx, filter)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)

    var results []linux_virt_unit.ContainerInfo
    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
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



func InitMongoDB() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    var err error
    MongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
    if err != nil {
        log.Fatal(err)
    }

    // 연결 테스트
    err = MongoClient.Ping(context.Background(), nil)
    if err != nil {
        log.Fatal(err)
    }

    // 컬렉션 초기화
    ContainerInfoCollection = MongoClient.Database("MC_IP").Collection("Container Info Collections")
    UserInfoCollection = MongoClient.Database("MC_USER").Collection("User Collections")

    log.Println("MongoDB Connected")
}

func CloseMongoDB() {
    if MongoClient != nil {
        MongoClient.Disconnect(context.Background())
        log.Println("MongoDB Disconnected")
    }
}

