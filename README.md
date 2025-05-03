
# linux\_virt\_unit

## Purpose

`linux\_virt\_unit` is a Go module designed to control Incus (LXD alternative) containers. It provides backend logic for creating, deleting, and managing containers via a REST API, including secure user authentication, port allocation, and state control. This module is part of the LVirt Project, designed for lightweight and secure virtualization management with TLS-secured API access.

## Features

| Feature                         | Description                                                                                   |
|---------------------------------|-----------------------------------------------------------------------------------------------|
| **Container Creation**          | Creates a new container using encrypted user credentials and distro/version info. *(POST /create)*  |
| **Container Deletion**          | Deletes container(s) by tag or associated username. *(POST /delete)*                          |
| **Container State Management**  | Dynamically start, stop, pause, resume, and restart containers. *(POST /start, /stop, /pause, /resume, /restart)* |
| **User Authentication**         | AES-encrypted credentials and bcrypt password verification. *(POST /authenticate)*            |
| **Port Pooling**                | Sequential port allocation and release using a mutex-protected heap. *(GET /port-pool)*       |
| **Asynchronous Task Handling**  | Goroutine-based worker pool for responsive and parallel container operations. *(POST /tasks)* |

## Structure

```
linux\_virt\_unit/
├── crypto
│   └── crypto.go                  # Encryption logic
├── go.mod
├── go.sum
├── http\_request
│   └── http\_request.go            # RestAPI endpoints
├── incus\_unit
│   ├── base\_images.go             # Auto-generated base image fingerprints
│   ├── change\_container\_status.go # Logic for changing container status
│   ├── create\_containers.go       # Logic for creating containers
│   ├── get\_info.go                # Fetches miscellaneous information
│   ├── handle\_container\_state\_change.go # Logic for start/stop/pause/resume/restart
│   ├── handle\_user\_info.go        # User registration and verification
│   └── worker\_pool.go             # Multi-processing worker pool
├── linux\_virt\_unit.go            # Shared structure definitions
├── mongo\_connect
│   └── mongo\_connect.go          # MongoDB client connection setup
└── README.md
```

## Swagger Request Example

**POST /create**  
**Content-Type:** `application/json`

```json
{
  "username": "user123",
  "username\_iv": "ivValue1",
  "password": "encryptedPassword",
  "password\_iv": "ivValue2",
  "key": "aesEncryptionKey",
  "tag": "ubuntu20",
  "serverip": "10.72.1.100",
  "serverport": "27020",
  "vmstatus": "running",
  "distro": "ubuntu",
  "version": "20.04"
}
```

## Security

- AES-256-GCM encryption for credentials
- bcrypt hashing for password comparison
- TLS-enabled REST API server
- Port-per-container network allocation

## Architecture

```
[Client (KivyMD)] ⇄ [REST API (Go)] ⇄ [linux\_virt\_unit] ⇄ [Incus API]
                                       ⇅
                                   [MongoDB]
```

## Requirements

- Go 1.23 or higher
- Incus installed (NOT LXD)
- MongoDB 6.0
- Ubuntu host with container support

## License

MIT License

