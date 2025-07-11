package linux_virt_unit

import (
    "github.com/gorilla/mux"
)

type UserInfo struct {
    Username   string `json:"username" example:"encryptedUser"`
    UsernameIV string `json:"username_iv" example:"someUsernameIV"`
    Password   string `json:"password" example:"passwordhash"`
    PasswordIV string `json:"password_iv" example:"somePasswordIVForRegistratgion"`
    Key        string `json:"key" example:"encryptionKey"`
}

type ContainerInfo struct {
    Username      string `json:"username" example:"encryptedUser"`
    UsernameIV    string `json:"username_iv" example:"someUsernameIV"`
    Password      string `json:"password" example:"encryptedPW"`
    PasswordIV    string `json:"password_iv" example:"somePasswordIV"`
    Key           string `json:"key" example:"encryptionKey"`
    TAG           string `json:"tag" example:"sometag"`
    Serverip      string `json:"serverip" example:"10.72.1.100"`
    Serverport    string `json:"serverport" example:"27020"`
    VMStatus      string `json:"vmstatus" example:"running"`
    Distro        string `json:"distro" example:"ubuntu"`
    DistroVersion string `json:"version" example:"24.04"`
}

var LinuxVirtualizationAPIRouter *mux.Router
var LINUX_VIRT_PATH = "/usr/local/bin/incuspeed"
var NGINX_LOCATION = "/etc/nginx/nginx.conf"
