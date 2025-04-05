package linux_virt_unit

import (
    "github.com/gorilla/mux"
)

type UserInfo struct {
    Username     string `json:"username"`
    UsernameIV   string `json:"username_iv"`
    Password     string `json:"password"`
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


var LinuxVirtualizationAPIRouter *mux.Router
