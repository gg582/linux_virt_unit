package incus_unit

import (
    "log"
    "context"
    "fmt"

    "go.mongodb.org/mongo-driver/bson"
    "github.com/lxc/incus/shared/api"

    db "github.com/yoonjin67/linux_virt_unit/mongo_connect"
)

// ChangeState changes the state of a container (start, stop, restart, pause).
func ChangeState(tag string, newState string) error {
    log.Printf("ChangeState: Request to change state of container '%s' to '%s'.", tag, newState)
    // Update the container status in MongoDB
    _, err := db.ContainerInfoCollection.UpdateOne(
        context.Background(),
        bson.M{"TAG": tag},
        bson.D{{"$set", bson.M{"vmstatus": newState}}},
    )
    if err != nil {
        log.Printf("ChangeState: MongoDB update failed for tag '%s' to state '%s': %v", tag, newState, err)
        return fmt.Errorf("failed to update container status in DB: %w", err)
    }
    log.Printf("ChangeState: MongoDB status updated for tag '%s' to '%s'.", tag, newState)

    // Get the Incus instance information
    inst, _, err := IncusCli.GetInstance(tag)
    if err != nil {
        log.Printf("ChangeState: Failed to get Incus instance information for tag '%s': %v", tag, err)
        return fmt.Errorf("failed to get Incus instance: %w", err)
    }
    log.Printf("ChangeState: Current status of Incus instance '%s': %s.", tag, inst.Status)

    // Handle 'stop' state change
    if newState == "stop" && inst.Status != "Stopped" {
        req := api.InstanceStatePut{
            Action:  "stop",
            Timeout: 30,
            Force  : true,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus stop request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus stop: %w", err)
        }
        log.Printf("ChangeState: Incus stop request sent for tag '%s'.", tag)

        // Wait for the Incus stop operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus stop operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus stop: %w", err)
        }
        log.Printf("ChangeState: Incus stop operation completed for tag '%s'.", tag)

        // Update DB status to 'stopped' after Incus stop
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"TAG": tag},
            bson.D{{"$set", bson.M{"vmstatus": "stopped"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'stopped' failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'stopped' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'stopped' for tag '%s'.", tag)
    } else if newState == "start" && inst.Status != "Running" {
        // Handle 'start' state change
        req := api.InstanceStatePut{
            Action:  "start",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus start request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus start: %w", err)
        }
        log.Printf("ChangeState: Incus start request sent for tag '%s'.", tag)

        // Wait for the Incus start operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus start operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus start: %w", err)
        }
        log.Printf("ChangeState: Incus start operation completed for tag '%s'.", tag)

        // Update DB status to 'running' after Incus start
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"TAG": tag},
            bson.D{{"$set", bson.M{"vmstatus": "running"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'running' failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'running' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'running' for tag '%s'.", tag)
    } else if newState == "restart" {
        // Handle 'restart' state change
        req := api.InstanceStatePut{
            Action:  "restart",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus restart request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus restart: %w", err)
        }
        log.Printf("ChangeState: Incus restart request sent for tag '%s'.", tag)

        // Wait for the Incus restart operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus restart operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus restart: %w", err)
        }
        log.Printf("ChangeState: Incus restart operation completed for tag '%s'.", tag)

        // Update DB status to 'running' after Incus restart (assuming restart leads to running)
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"TAG": tag},
            bson.D{{"$set", bson.M{"vmstatus": "running"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'running' failed after Incus restart for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'running' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'running' for tag '%s' after restart.", tag)
    } else if newState == "freeze" && inst.Status != "Frozen" {
        // Handle 'freeze' (pause) state change
        req := api.InstanceStatePut{
            Action:  "freeze",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus freeze request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus freeze: %w", err)
        }
        log.Printf("ChangeState: Incus freeze request sent for tag '%s'.", tag)

        // Wait for the Incus freeze operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus freeze operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus freeze: %w", err)
        }
        log.Printf("ChangeState: Incus freeze operation completed for tag '%s'.", tag)

        // Update DB status to 'frozen' after Incus freeze
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"TAG": tag},
            bson.D{{"$set", bson.M{"vmstatus": "frozen"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'frozen' failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'frozen' in DB: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'frozen' for tag '%s'.", tag)
    } else if newState == "unfreeze" && inst.Status == "Frozen" {
        // Handle 'unfreeze' (resume) state change
        req := api.InstanceStatePut{
            Action:  "unfreeze",
            Timeout: 30,
        }
        op, err := IncusCli.UpdateInstanceState(tag, req, "")
        if err != nil {
            log.Printf("ChangeState: Incus unfreeze request failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to request Incus unfreeze: %w", err)
        }
        log.Printf("ChangeState: Incus unfreeze request sent for tag '%s'.", tag)

        // Wait for the Incus unfreeze operation to complete
        err = op.Wait()
        if err != nil {
            log.Printf("ChangeState: Incus unfreeze operation wait failed for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to wait for Incus unfreeze: %w", err)
        }
        log.Printf("ChangeState: Incus unfreeze operation completed for tag '%s'.", tag)

        // Update DB status to 'running' after Incus unfreeze (assuming it goes back to running)
        _, err = db.ContainerInfoCollection.UpdateOne(
            context.Background(),
            bson.M{"TAG": tag},
            bson.D{{"$set", bson.M{"vmstatus": "running"}}},
        )
        if err != nil {
            log.Printf("ChangeState: MongoDB update to 'running' failed after Incus unfreeze for tag '%s': %v", tag, err)
            return fmt.Errorf("failed to update container status to 'running' in DB after unfreeze: %w", err)
        }
        log.Printf("ChangeState: MongoDB status updated to 'running' for tag '%s' after unfreeze.", tag)
    } else {
        log.Printf("ChangeState: No state change needed for container '%s' to '%s' (current status: %s).", tag, newState, inst.Status)
    }

    return nil
}
