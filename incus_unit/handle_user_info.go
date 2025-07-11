package incus_unit

import (
    "context"
    "encoding/json"
    "io"
    "net/http"
    "time"
    //for file io, net  connection

    "log"
    //for logging

    "go.mongodb.org/mongo-driver/bson"
    "golang.org/x/crypto/bcrypt"
    //for mongodb

    "github.com/gg582/linux_virt_unit"
    linux_virt_unit_crypto "github.com/gg582/linux_virt_unit/crypto"
    db "github.com/gg582/linux_virt_unit/mongo_connect"
    //custom packages
)

// CheckUserExists checks if a user exists in the database.
func CheckUserExists(username string, password string, ctx context.Context) bool {
    const maxWait = time.Second

    cursor, err := db.UserInfoCollection.Find(ctx, bson.D{})
    if err != nil {
        log.Printf("CheckUserExists: Failed to query users: %v", err)
        return false
    }
    defer cursor.Close(ctx)

    start := time.Now()
    for cursor.Next(ctx) {
        var user linux_virt_unit.UserInfo
        if err := cursor.Decode(&user); err != nil {
            continue
        }
        if user.Username == username {
            err := bcrypt.CompareHashAndPassword([]byte(password), []byte(user.Password))
            if err != nil {
                log.Printf("CheckUserExists: Password mismatch for user '%s'", username)
                log.Printf("%s:%s", user.Password, password)
                return false
            }
            return true
        }
        if time.Since(start) > maxWait {
            log.Printf("CheckUserExists: Timed out while scanning for user '%s'", username)
            return false
        }
    }

    log.Printf("CheckUserExists: User '%s' not found", username)
    return false
}
// DeleteExistingUser deletes a user if it exists
func DeleteExistingUser(username string, password string, ctx context.Context) bool {
    const maxWait = time.Second

    cursor, err := db.UserInfoCollection.Find(ctx, bson.D{})
    if err != nil {
        log.Printf("DeleteExistingUser: Failed to query users: %v", err)
        return false
    }
    defer cursor.Close(ctx)

    start := time.Now()
    for cursor.Next(ctx) {
        var user linux_virt_unit.UserInfo
        if err := cursor.Decode(&user); err != nil {
            continue
        }
        if user.Username == username {
            err := bcrypt.CompareHashAndPassword([]byte(password), []byte(user.Password))
            if err != nil {
                log.Printf("DeleteExistingUser: Password mismatch for user '%s'", username)
                return false
            }
            db.UserInfoCollection.DeleteOne(ctx, bson.D{{"username", user.Username}})
            return true

        }
        if time.Since(start) > maxWait {
            log.Printf("CheckUserExists: Timed out while scanning for user '%s'", username)
            return false
        }
    }

    log.Printf("CheckUserExists: User '%s' not found", username)
    return false
}

// Register godoc
// @Summary Register a new user
// @Description Registers a new user
// @Accept json
// @Produce json
// @Param request body linux_virt_unit.UserInfo true "User registration request"
// @Success 200 body string true "User Registration Done."
// @Failure 400
// @Router /register [post]
func Register(wr http.ResponseWriter, req *http.Request) {
    if req.Method != http.MethodPost {
        http.Error(wr, "This endpoint allows only POST methods. aborting", http.StatusMethodNotAllowed)
        return
    }
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    var u linux_virt_unit.UserInfo
    body, err := io.ReadAll(req.Body)
    if err != nil {
        log.Printf("Register: Failed to read request body: %v", err)
        http.Error(wr, "Failed to read request body", http.StatusBadRequest)
        return
    }

    // Unmarshal the JSON into UserInfo struct
    if err := json.Unmarshal(body, &u); err != nil {
        log.Printf("Register: Invalid JSON: %v", err)
        http.Error(wr, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Decrypt the username from client
    u.Username, err = linux_virt_unit_crypto.DecryptString(u.Username, u.Key, u.UsernameIV)
    if err != nil {
        log.Printf("Register: Failed to decrypt username: %v", err)
        http.Error(wr, "Username decryption failed", http.StatusBadRequest)
        return
    }
    u.Password, err = linux_virt_unit_crypto.DecryptString(u.Password, u.Key, u.PasswordIV)
    if err != nil {
        log.Printf("Register: Failed to decrypt username: %v", err)
        http.Error(wr, "Password decryption failed", http.StatusBadRequest)
        return
    }


    // Check if the username already exists
    if CheckUserExists(u.Username, u.Password, ctx) {
        log.Printf("Register: Username '%s' already exists", u.Username)
        http.Error(wr, "User already exists", http.StatusConflict)
        return
    }

    // Insert the new user into MongoDB
    if _, err := db.UserInfoCollection.InsertOne(ctx, u); err != nil {
        log.Printf("Register: Failed to insert user into DB: %v", err)
        http.Error(wr, "Failed to register user", http.StatusInternalServerError)
        return
    }

    log.Printf("Register: User '%s' registered successfully", u.Username)
    wr.Write([]byte("User Registration Done"))
}
// @Summary Delete User
// @Description DeleteUser retrieves a list of containers for a specific user by manually scanning the collection, and deletes user.
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
// @Success 200 {array} linux_virt_unit.ContainerInfo "Deleted containers list for debugging"
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
// @Router /unregister [post]
func Unregister(wr http.ResponseWriter, req *http.Request) {
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
        log.Printf("Unregister: Failed to read request body: %v", err)
        http.Error(wr, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
        return
    }
    if err := json.Unmarshal(body, &in); err != nil {
        log.Printf("Unregister: Failed to parse JSON request body: %v", err)
        http.Error(wr, "Failed to parse JSON: "+err.Error(), http.StatusBadRequest)
        return
    }

    log.Printf("Unregister: Received request for containers of user '%s' (encrypted).", in.Username)
    //Received request (encrypted)

    // Decrypt the username
    in.Username, err = linux_virt_unit_crypto.DecryptString(in.Username, in.Key, in.UsernameIV)
    if err != nil {
        log.Printf("Unregister: Failed to decrypt username: %v", err)
        http.Error(wr, "Failed to decrypt username: "+err.Error(), http.StatusBadRequest)
        return
    }
    log.Printf("Unregister: Decrypted username: '%s'.", in.Username)

    if CheckUserExists(in.Username, in.Password, ctx) == false {
        wr.WriteHeader(404)
        wr.Write([]byte("No user found\n"))
        return
    }
    WorkQueue.Unreg <- in
    // Manually scan the entire collection
}
