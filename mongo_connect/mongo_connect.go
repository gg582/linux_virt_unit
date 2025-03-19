package mongo_connect
import (
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    "net/http"
    "context"
    "log"
    "github.com/yoonjin67/lvirt_applicationUnit"
    "encoding/json"
)

var col *mongo.Collection
var ipCol , UserCol *mongo.Collection

func botCheck(u string, pw string) bool {
    cur, err := UserCol.Find(context.Background(), bson.D{{}})
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
        var i UserInfo
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
    
    var in UserInfo
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
    cur, err := ipCol.Find(ctx, filter)
    if err != nil {
        http.Error(wr, err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)

    var results []ContainerInfo
    for cur.Next(ctx) {
        var info ContainerInfo
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


func initMongoDB() {
    ctx, cancel = context.WithCancel(context.Background())
    defer cancel()

    // MongoDB 연결 설정
    clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
    client, err := mongo.Connect(ctx, clientOptions)
    if err != nil {
        log.Fatal(err)
    }
    defer client.Disconnect(ctx)

    // MongoDB 연결 테스트
    err = client.Ping(ctx, nil)
    if err != nil {
        log.Fatal(err)
    }

    // 컬렉션 초기화
    col = client.Database("MC_Json").Collection("Flag Collections")
    ipCol = client.Database("MC_IP").Collection("IP Collections")
    UserCol = client.Database("MC_USER").Collection("User Collections")
}
