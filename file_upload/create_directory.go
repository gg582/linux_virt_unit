package file_upload
import (
    "log"
    "os"
    "strconv"
    "fmt"
    "os/exec"
    "path/filepath"
    "time"
    . "github.com/yoonjin67/linux_virt_unit"
)

func createFSDirectory(username, password, TAG string) (string, error) {
    
    //Create a FS directory for storing data
    homeDir := FS(username, password)
    currentTime := time.Now()
    date := currentTime.Format("2006-01-02")
    uploadDir := filepath.Join(homeDir, TAG, date)
    err := os.MkdirAll(uploadDir, 0744)
    if err != nil {
        return "", fmt.Errorf("Failed to create user directory: %v", err)
    }
    //Add Linux User for assigning a directory to desired user
    err = exec.Command("useradd", "-m", "-d", homeDir, username).Run()
    if err != nil {
        os.RemoveAll(uploadDir)
        return "", fmt.Errorf("Failed to create user with directory %v", err)
    }
    //Now, I need to setup password for newly created user
    changePassword := exec.Command("chpasswd")
    inputPipe, err := changePassword.StdinPipe()
    if err != nil {
        exec.Command("userdel", "-r", homeDir).Run()
        return "", fmt.Errorf("Failed to Setup Input pipe")
    }
    _, err = fmt.Fprintln(inputPipe, fmt.Sprintf("%s:%s", username, password))
    if err != nil {
        exec.Command("userdel", "-r", homeDir).Run()
        return "", err
    }
    err = inputPipe.Close()
    if err != nil {
        log.Println("[FATAL]: Failed to close input pipe")
        exec.Command("userdel", "-r", homeDir).Run()
        log.Fatal(err)
    }
    err = changePassword.Run()

    if err != nil {
        log.Printf("Failed to setup user PW. user %s cannot use FS\n", err)
        exec.Command("userdel", "-r", homeDir).Run()
        return "", err
    }

    uid, gid, err := getUIDAndGID(username)
    if err != nil {
        log.Printf("Failed to get uid and gid: %v\n", err)
        exec.Command("userdel", "-r", homeDir).Run()
        return "", err
    }

    err = os.Chown(homeDir, uid, gid)
    if err != nil {
        exec.Command("userdel", "-r", homeDir).Run()
        log.Println("Failed to change ownership of home directory")
        return "", err
    }

    err = os.Chown(uploadDir, uid, gid) 

    if err != nil {
        log.Println("Failed to change ownership of container subdir")
        return "", err
    }
    
    return uploadDir, err
}

func getUIDAndGID(username string) (int, int, error) {
    cmd := exec.Command("id", "-u", username)
    r, err := cmd.CombinedOutput()
    if err != nil {
        return -1, -1, fmt.Errorf("Failed to get UID for user %s: %v, output: %s",
        username, err, r)
    }
    uid, err := strconv.Atoi(string(r[:len(r)-1]))
    if err != nil {
        log.Println("Failed to get UID")
        return -1, -1, fmt.Errorf("Failed to get UID for user %s: %v, output: %s",
        username, err, r)
    }

    cmd = exec.Command("id", "-g", username)
    r, err = cmd.CombinedOutput()
    if err != nil {
        return -1, -1, fmt.Errorf("Failed to get GID for user %s: %v, output: %s",
        username, err, r)
    }

    gid, err := strconv.Atoi(string(r[:len(r)-1]))

    if err != nil {
        return -1, -1, fmt.Errorf("Failed to get GID for user %s: %v, output: %s",
        username, err, r)
    }

    return uid, gid, err
}
