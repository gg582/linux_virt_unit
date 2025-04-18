package incus_unit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	db "github.com/yoonjin67/linux_virt_unit/mongo_connect"
	"go.mongodb.org/mongo-driver/bson"
)

func ChangeStateHandler(state string) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		tagBytes, err := io.ReadAll(req.Body)
		if err != nil {
			log.Printf("%s: Failed to read request body: %v", state, err)
			http.Error(wr, err.Error(), http.StatusBadRequest)
			return
		}

		Tag := strings.Trim(string(tagBytes), "\"")
		log.Printf("%s: Received request to stop container with tag '%s'.", state, Tag)
		info := StateChangeTarget{
			Tag:    Tag,
			Status: state,
		}

		select {
		case WorkQueue.StateTasks <- info:
			log.Println("CreateContainer: Added container creation task to the work queue.")
			wr.WriteHeader(http.StatusOK)
			wr.Write([]byte(fmt.Sprintf("%s command sent for container '%s'", state, Tag)))
			return
		default:
			log.Println("CreateContainer: Work queue is full.")
			http.Error(wr, "Server is busy", http.StatusServiceUnavailable)
			return
		}

	}

}

// DeleteContainerByName stops and then deletes an Incus container by its tag.
func DeleteContainerByName(tag string) error {
	log.Printf("DeleteContainerByName: Attempting to delete Incus container with tag '%s'.", tag)
	// Check if the tag is nil
	if tag == "" {
		log.Println("DeleteContainerByName: Error: tag is nil")
		return errors.New("tag is nil")
	}
	// Get the container information
	container, _, err := IncusCli.GetInstance(tag)
	if err != nil {
		return fmt.Errorf("DeleteContainerByName: failed to get container '%s': %w", tag, err)
	}
	log.Printf("DeleteContainerByName: Current status of container '%s': %s.", tag, container.Status)

	// If the container is running, stop it
	if container.Status != "Stopped" {
		log.Printf("DeleteContainerByName: Container '%s' is running, requesting stop.", tag)
		err := ChangeState(tag, "stop")
		if err != nil {
			log.Printf("DeleteContainerByName: ChangeState call failed for tag '%s': %v", tag, err)
			return err
		}

		// Wait for the container to stop (with a timeout)
		stopChan := make(chan bool)
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				currentContainer, _, err := IncusCli.GetInstance(tag)
				if err != nil {
					log.Printf("DeleteContainerByName: Failed to get container '%s' information while waiting for stop: %v", tag, err)
					stopChan <- true // Consider stopped if info cannot be retrieved
					return
				}
				if currentContainer.Status == "Stopped" {
					log.Printf("DeleteContainerByName: Container '%s' is now Stopped.", tag)
					stopChan <- true
					return
				}
				log.Printf("DeleteContainerByName: Container '%s' status: %s, waiting...", tag, currentContainer.Status)
			}
		}()

		select {
		case <-stopChan:
			log.Printf("DeleteContainerByName: Container '%s' stop confirmed, proceeding with deletion.", tag)
		case <-time.After(30 * time.Second): // Set a timeout to prevent indefinite waiting
			return fmt.Errorf("DeleteContainerByName: container '%s' did not stop in time", tag)
		}
	} else {
		log.Printf("DeleteContainerByName: Container '%s' is already Stopped.", tag)
	}

	// Delete the container
	op, err := IncusCli.DeleteInstance(tag)
	if err != nil {
		return fmt.Errorf("DeleteContainerByName: failed to delete container '%s': %w", tag, err)
	}
	log.Printf("DeleteContainerByName: Delete request sent for container '%s'.", tag)

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("DeleteContainerByName: error waiting for deletion of container '%s': %w", tag, err)
	}
	log.Printf("DeleteContainerByName: Container '%s' deleted successfully.", tag)
	return nil
}

func DeleteNginxConfig(port int) error {
	nginxConfPath := NGINX_LOCATION
	portStr := strconv.Itoa(port)
	portPlusOneStr := strconv.Itoa(port + 1)
	portPlusTwoStr := strconv.Itoa(port + 2)

	// Construct the sed command to delete the three server blocks related to the port.
	sedCommand := fmt.Sprintf(`sed -i '/listen 0.0.0.0:%s;/ {
N; /proxy_pass .*:%s;/ d;
N; /listen 0.0.0.0:%s;/ {
N; /proxy_pass .*:%s;/ d;
N; /listen 0.0.0.0:%s;/ {
N; /proxy_pass .*:%s;/ d;
}; }; };' %s`,
		portStr, "22",
		portPlusOneStr, "30001",
		portPlusTwoStr, "30002",
		nginxConfPath)

	cmd := exec.Command("bash", "-c", sedCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("DeleteNginxConfig: Failed to execute sed command: %v, output: %s", err, string(output))
		return fmt.Errorf("failed to delete nginx config: %v, output: %s", err, string(output))
	}
	log.Printf("DeleteNginxConfig: Successfully executed sed command. Output: %s", string(output))
	return nil
}

// DeleteByTag godoc
// @Summary Delete container by tag
// @Description Deletes a container with the specified tag.
// @Accept json
// @Produce json
// @Param request body string true "Tag to delete"
// @Status 200
// @Failure 400
// @Router /delete [post]
func DeleteByTag(wr http.ResponseWriter, req *http.Request) {
	portDeleteMutex.Lock()
	defer portDeleteMutex.Unlock()
	log.Println("DeleteByTag: Start.")
	tagBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("DeleteByTag: Failed to read request Body: %v", err)
		http.Error(wr, err.Error(), http.StatusBadRequest)
		return
	}
	Tag := strings.Trim(string(tagBytes), "\"")
	log.Printf("DeleteByTag: Received request to delete container with tag '%s'.", Tag)

	cur, err := db.ContainerInfoCollection.Find(context.Background(), bson.D{{Key: "TAG", Value: Tag}})
	if err != nil {
		log.Printf("DeleteByTag: MongoDB Find failed: %v", err)
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cur.Close(context.Background())
	log.Println("DeleteByTag: MongoDB Find completed.")

	found, foundTag := db.FindTag(Tag)

	if found {
		found, port := db.FindPortByTag(foundTag)
		if found == false {
			log.Printf("DeleteByTag: Failed to convert ServerPort to integer: %v", err)
			http.Error(wr, "Internal server error", http.StatusInternalServerError)
			return
		}
		log.Printf("DeleteByTag: Port to return: %d", port)

		if DeleteNginxConfig(port) != nil {
			log.Println("DeleteByTag: (nginx) eNginx policy modification failed")
		}
		PORT_LIST = DeleteFromListByValue(PORT_LIST, port)

		filter := bson.D{{"tag", Tag}}
		_, err = db.ContainerInfoCollection.DeleteOne(context.Background(), filter)
		if err != nil {
			log.Printf("DeleteByTag: MongoDB DeleteOne failed: %v", err)
		} else {
			log.Println("DeleteByTag: MongoDB DeleteOne Success.")
		}

		log.Println("DeleteByTag: Calling DeleteContainerByName.")
		err = DeleteContainerByName(Tag)
		if err != nil {
			log.Printf("DeleteByTag: DeleteContainerByName failed: %v", err)
		} else {
			log.Println("DeleteByTag: DeleteContainerByName Success.")
		}
		wr.WriteHeader(http.StatusOK)
		wr.Write([]byte(fmt.Sprintf("Container with tag '%s' deleted", Tag)))
		return
	} else {
		log.Printf("DeleteByTag: Container with tag '%s' not found.", Tag)
		http.Error(wr, fmt.Sprintf("Container with tag '%s' not found", Tag), http.StatusNotFound)
		return
	}
}

// DeleteFromListByValue removes a specific value from an integer slice.
func DeleteFromListByValue(slice []int, value int) []int {
	for i, itm := range slice {
		if itm == value {
			log.Printf("DeleteFromListByValue: Found value '%d' at index '%d', removing.", value, i)
			return append(slice[:i], slice[i+1:]...)
		}
	}
	log.Printf("DeleteFromListByValue: Value '%d' not found in the slice.", value)
	return slice
}
