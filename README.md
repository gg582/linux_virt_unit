# 🐧 linux\_virt\_unit

`linux_virt_unit` is a Go module for controlling Incus (LXD alternative) containers. It provides backend logic for creating, deleting, and managing containers via REST API, including secure user authentication, port allocation, and state control.

This module is part of the **LVirt Project**, designed for lightweight, secure virtualization management with TLS-secured API access.

---

## 📦 Features

| Feature                      | Description |
|-----------------------------|-------------|
| Container Creation          | Creates a new container using encrypted user credentials and distro/version info |
| Container Deletion          | Deletes container(s) by tag or associated username |
| Container State Management  | Start, stop, pause, resume, and restart containers dynamically |
| User Authentication         | AES-encrypted credentials and bcrypt password verification |
| Port Pooling                | Random port allocation and release using a mutex-protected heap |
| Asynchronous Task Handling  | Goroutine-based worker pool for responsive and parallel container operations |

---

## 📁 Structure

```
linux_virt_unit/
├── crypto
│   └── crypto.go #encryption logics
├── go.mod
├── go.sum
├── http_request
│   └── http_request.go #RestAPI endpoints
├── incus_unit
│   ├── base_images.go #(auto-generated) base image fingerprints
│   ├── change_container_status.go # 
│   ├── create_containers.go 
│   ├── get_info.go # get miscellaneous informations
│   ├── handle_container_state_change.go # Start/Stop/Pause/Resume/Restart logic
│   ├── handle_user_info.go # Secure user registration and verification
│   └── worker_pool.go # multi-processing worker pool
├── linux_virt_unit.go # shared structure definitions
├── mongo_connect
│   └── mongo_connect.go # mongoDB client connection establishment
└── README.md

```

---

## 🧪 Swagger Request Example

```json
POST /create
Content-Type: application/json

{
  "username": "user123",
  "username_iv": "ivValue1",
  "password": "encryptedPassword",
  "password_iv": "ivValue2",
  "key": "aesEncryptionKey",
  "tag": "ubuntu20",
  "serverip": "10.72.1.100",
  "serverport": "27020",
  "vmstatus": "running",
  "distro": "ubuntu",
  "version": "20.04"
}
```

---

## 🔐 Security

- AES-256-GCM encryption for credentials
- Bcrypt hashing for password comparison
- TLS-enabled REST API server
- Port-per-container network allocation

---

## 🧩 Architecture

```
[Client (KivyMD)] ⇄ [REST API (Go)] ⇄ [linux_virt_unit] ⇄ [Incus API]
                                       ⇅
                                   [MongoDB]
```

---

## ⚙️ Requirements

- Go 1.23 or higher
- Incus installed (NOT LXD)
- MongoDB 6.0
- Ubuntu host with container support

---

## 📜 License

MIT License

