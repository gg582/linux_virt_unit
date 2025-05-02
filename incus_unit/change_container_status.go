package incus_unit

import (
	"context"
	"fmt"
	"log"

	"github.com/lxc/incus/shared/api"
	"go.mongodb.org/mongo-driver/bson"

	db "github.com/yoonjin67/linux_virt_unit/mongo_connect"
)

// ChangeState changes the state of a container (start, stop, restart, pause).
func ChangeState(tag string, newState string) {
	log.Printf("ChangeState: Request to change state of container '%s' to '%s'.", tag, newState)
	// Update the container status in MongoDB
	_, err := db.ContainerInfoCollection.UpdateOne(
		context.Background(),
		bson.M{"TAG": tag},
		bson.D{{"$set", bson.M{"vmstatus": newState}}},
	)
	if err != nil {
		log.Printf("ChangeState: MongoDB update failed for tag '%s' to state '%s': %v", tag, newState, err)
		 WQReturns <- fmt.Errorf("failed to update container status in DB: %w", err)
	}
	log.Printf("ChangeState: MongoDB status updated for tag '%s' to '%s'.", tag, newState)

	// Get the Incus instance information
	inst, _, err := IncusCli.GetInstance(tag)
	if err != nil {
		log.Printf("ChangeState: Failed to get Incus instance information for tag '%s': %v", tag, err)
		WQReturns <-  fmt.Errorf("failed to get Incus instance: %w", err)
	}
	log.Printf("ChangeState: Current status of Incus instance '%s': %s.", tag, inst.Status)

	// Handle state change
	req := api.InstanceStatePut{
		Action:  newState,
		Timeout: 30,
		Force:   true,
	}
	op, err := IncusCli.UpdateInstanceState(tag, req, "")
	if err != nil {
		log.Printf("ChangeState: Incus %s request failed for tag '%s': %v", newState, tag, err)
		WQReturns <- fmt.Errorf("failed to request Incus %s: %w", newState, err)
	}
	log.Printf("ChangeState: Incus %s request sent for tag '%s'.", newState, tag)

	// Wait for the Incus operation to complete
	err = op.Wait()
	if err != nil {
		log.Printf("ChangeState: Incus operation %s wait failed for tag '%s': %v", newState, tag, err)
		WQReturns <- fmt.Errorf("failed to wait for Incus %s: %w", newState, err)
	}
	log.Printf("ChangeState: Incus operation %s completed for tag '%s'.", newState, tag)

	// Update DB status after Incus operation
	_, err = db.ContainerInfoCollection.UpdateOne(
		context.Background(),
		bson.M{"TAG": tag},
		bson.D{{"$set", bson.M{"vmstatus": inst.Status}}},
	)
	if err != nil {
		log.Printf("ChangeState: MongoDB update to %s failed for tag '%s': %v", newState, tag, err)
		WQReturns <- fmt.Errorf("failed to update container status to %s in DB: %w", newState, err)
	}
	log.Printf("ChangeState: MongoDB status updated to %s for tag '%s'.", newState, tag)
    WQReturns <- error(nil)
}
