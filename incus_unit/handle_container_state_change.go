package incus_unit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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
		log.Printf("%s: Received request with tag '%s'.", state, Tag)
		info := StateChangeTarget{
			Tag:    Tag,
			Status: state,
		}

		select {
		case WorkQueue.StateTasks <- info:
			log.Printf("ChangeContainer: Added container %s task to the work queue.\n", state)
			wr.WriteHeader(http.StatusOK)
			wr.Write([]byte(fmt.Sprintf("%s command sent for container '%s'", state, Tag)))
			return
		default:
			log.Println("ChangeContainer: Work queue is full.")
			http.Error(wr, "Server is busy", http.StatusServiceUnavailable)
			return
		}

	}

}

// DeleteContainerByName stops and then deletes an Incus container by its tag.
func DeleteContainerByName(tag string) {
	log.Printf("DeleteContainerByName: Attempting to delete Incus container with tag '%s'.", tag)
	// Check if the tag is nil
	if tag == "" {
		log.Println("DeleteContainerByName: Error: tag is nil")
		WorkQueue.WQReturns <- errors.New("tag is nil")
	}
	// Get the container information
	container, _, err := IncusCli.GetInstance(tag)
	if err != nil {
		WorkQueue.WQReturns <- fmt.Errorf("DeleteContainerByName: failed to get container '%s': %w", tag, err)
	}
	log.Printf("DeleteContainerByName: Current status of container '%s': %s.", tag, container.Status)

	// If the container is running, stop it
	if container.Status != "Stopped" {
		log.Printf("DeleteContainerByName: Container '%s' is running, requesting stop.", tag)
		ChangeState(tag, "stop")
        err := <- WorkQueue.WQReturns
		if err != nil {
			log.Printf("DeleteContainerByName: ChangeState call failed for tag '%s': %v", tag, err)
			WorkQueue.WQReturns <- err
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
                    WorkQueue.WQReturns <- error(nil)
				}
				if currentContainer.Status == "Stopped" {
					log.Printf("DeleteContainerByName: Container '%s' is now Stopped.", tag)
					stopChan <- true
                    WorkQueue.WQReturns <- error(nil)
				}
				log.Printf("DeleteContainerByName: Container '%s' status: %s, waiting...", tag, currentContainer.Status)
			}
		}()

		select {
		case <-stopChan:
			log.Printf("DeleteContainerByName: Container '%s' stop confirmed, proceeding with deletion.", tag)
		case <-time.After(30 * time.Second): // Set a timeout to prevent indefinite waiting
			WorkQueue.WQReturns <- fmt.Errorf("DeleteContainerByName: container '%s' did not stop in time", tag)
		}
	} else {
		log.Printf("DeleteContainerByName: Container '%s' is already Stopped.", tag)
	}

	// Delete the container
	op, err := IncusCli.DeleteInstance(tag)
	if err != nil {
		WorkQueue.WQReturns <- fmt.Errorf("DeleteContainerByName: failed to delete container '%s': %w", tag, err)
	}
	log.Printf("DeleteContainerByName: Delete request sent for container '%s'.", tag)

	err = op.Wait()
	if err != nil {
		WorkQueue.WQReturns <- fmt.Errorf("DeleteContainerByName: error waiting for deletion of container '%s': %w", tag, err)
	}
	log.Printf("DeleteContainerByName: Container '%s' deleted successfully.", tag)
	WorkQueue.WQReturns <- nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
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

	cur, err := db.ContainerInfoCollection.Find(ctx, bson.D{{Key: "TAG", Value: Tag}})
	if err != nil {
		log.Printf("DeleteByTag: MongoDB Find failed: %v", err)
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)
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

		filter := bson.D{{"tag", Tag}}
		_, err = db.ContainerInfoCollection.DeleteOne(ctx, filter)
		if err != nil {
			log.Printf("DeleteByTag: MongoDB DeleteOne failed: %v", err)
		} else {
			log.Println("DeleteByTag: MongoDB DeleteOne Success.")
		}

        info := StateChangeTarget {
            Tag: Tag,
            Status: "delete",
        }
        WorkQueue.DeletionQueue <- port
		select {
		case WorkQueue.StateTasks <- info:
			log.Println("CreateContainer: Added container creation task to the work queue.")
			wr.WriteHeader(http.StatusOK)
			wr.Write([]byte(fmt.Sprintf("%s command sent for container '%s'", "delete", Tag)))
			return
		default:
			log.Println("CreateContainer: Work queue is full.")
			http.Error(wr, "Server is busy", http.StatusServiceUnavailable)
			return
		}
    }
}

