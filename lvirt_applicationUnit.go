package lvirt_applicationUnit

import (
    "github.com/gorilla/mux"
    "go.mongodb.org/mongo-driver/mongo"
)

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


var Route *mux.Router
var Colct *mongo.Collection
var AddrCol , UserCol *mongo.Collection
