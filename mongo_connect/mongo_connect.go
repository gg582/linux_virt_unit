package mongo_connect

import (
    "context"
    "encoding/json"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    "io"
    "log"
    "net/http"
    "strconv"
    "time"

    linux_virt_unit "github.com/gg582/linux_virt_unit"
)

const MAX_PORT = 65535

var ContainerInfoCollection, UserInfoCollection *mongo.Collection

var MongoClient *mongo.Client // Declared globally for MongoDB operations

// GetTopPort manually scans the collection to find the highest Serverport value
func GetTopPort() (int, error) {
    ctx := context.Background()
    cur, err := ContainerInfoCollection.Find(ctx, bson.M{})
    if err != nil {
        log.Println("GetTopPort: Failed to fetch container list:", err)
        return -1, err
    }
    defer cur.Close(ctx)

    topPort := -1
    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            continue
        }
        port, err := strconv.Atoi(info.Serverport)
        if err != nil {
            continue
        }
        if port > topPort {
            topPort = port
        }
    }

    if topPort == -1 {
        log.Println("GetTopPort: Port list is empty")
    }
    return topPort, nil
}

// FindTag manually scans the collection to find if a tag exists
func FindTag(tag string) (bool, string) {
    ctx := context.Background()
    cur, err := ContainerInfoCollection.Find(ctx, bson.M{})
    if err != nil {
        log.Println("FindTag: Failed to fetch container list:", err)
        return false, ""
    }
    defer cur.Close(ctx)

    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            continue
        }
        if info.TAG == tag {
            return true, info.TAG
        }
    }
    return false, ""
}

func FindPortByTag(tag string) (bool, int) {
    ctx := context.Background()
    cur, err := ContainerInfoCollection.Find(ctx, bson.M{})
    if err != nil {
        log.Println("FindPortByTag: Failed to fetch container list:", err)
        return false, -1
    }
    defer cur.Close(ctx)

    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            continue
        }
        if info.TAG == tag {
            port, err := strconv.Atoi(info.Serverport)
            if err == nil {
                return true, port
            }
        }
    }
    return false, -1
}

// FindPort manually scans the collection to find if a port exists
func FindPort(port int) (bool, int) {
    ctx := context.Background()
    cur, err := ContainerInfoCollection.Find(ctx, bson.M{})
    if err != nil {
        log.Println("FindPort: Failed to fetch container list:", err)
        return false, -1
    }
    defer cur.Close(ctx)

    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            continue
        }
        p, err := strconv.Atoi(info.Serverport)
        if err != nil {
            continue
        }
        if p == port {
            return true, p
        }
    }
    log.Println("FindPort: Port not found in collection")
    return false, -1
}

// SetupSort creates a descending index on the Serverport field
func SetupSort(c *mongo.Collection) bool {
    ctx := context.Background()
    indexModel := mongo.IndexModel{
        Keys: bson.D{{Key: "Serverport", Value: -1}},
        Options: &options.IndexOptions{
            Unique: new(bool), // not strictly unique but required by driver
        },
    }
    _, err := c.Indexes().CreateOne(ctx, indexModel)
    if err != nil {
        log.Println("SetupSort: Failed to create Serverport index:", err)
        return false
    }
    return true
}

// UseContainer returns all containers matching a manually matched username and password
func UseContainer(wr http.ResponseWriter, req *http.Request) {
    if req.Method != http.MethodPost {
        http.Error(wr, "This endpoint allows only POST methods. aborting",
        http.StatusMethodNotAllowed)
        return
    }
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    wr.Header().Set("Content-Type", "application/json; charset=utf-8")

    var in linux_virt_unit.UserInfo
    body, err := io.ReadAll(req.Body)
    if err != nil {
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }

    if err := json.Unmarshal(body, &in); err != nil {
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    // Manually scan the collection and compare credentials
    cur, err := ContainerInfoCollection.Find(ctx, bson.M{})
    if err != nil {
        http.Error(wr, "Database error: "+err.Error(), http.StatusInternalServerError)
        return
    }
    defer cur.Close(ctx)

    var results []linux_virt_unit.ContainerInfo
    for cur.Next(ctx) {
        var info linux_virt_unit.ContainerInfo
        if err := cur.Decode(&info); err != nil {
            continue
        }
        if info.Username == in.Username && info.Password == in.Password {
            results = append(results, info)
        }
    }

    resp, err := json.Marshal(results)
    if err != nil {
        http.Error(wr, "Failed to encode response: "+err.Error(), http.StatusInternalServerError)
        return
    }

    wr.WriteHeader(http.StatusOK)
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

    //Ping Test
    err = MongoClient.Ping(context.Background(), nil)
    if err != nil {
        log.Fatal(err)
    }

    //obtain collections

    ContainerInfoCollection = MongoClient.Database("INCUSPEED_CONTAINER").Collection("Container Info Collections")
    if SetupSort(ContainerInfoCollection) == false {
        log.Println("Error setting up sort")
    }
    UserInfoCollection = MongoClient.Database("INCUSPEED_USER").Collection("User Collections")

    log.Println("MongoDB Connected")
}

func CloseMongoDB() {
    if MongoClient != nil {
        MongoClient.Disconnect(context.Background())
        log.Println("MongoDB Disconnected")
    }
}
