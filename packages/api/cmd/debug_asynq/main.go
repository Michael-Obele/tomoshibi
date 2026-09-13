package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"

	"github.com/hibiken/asynq"
	"github.com/Michael-Obele/tomoshibi/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	u, err := url.Parse(cfg.Redis.URL)
	if err != nil {
		log.Fatal(err)
	}
	password, _ := u.User.Password()
	redisOpt := asynq.RedisClientOpt{
		Addr:     u.Host,
		Password: password,
	}
	if u.Scheme == "rediss" {
		redisOpt.TLSConfig = &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		}
	}

	inspector := asynq.NewInspector(redisOpt)

	taskID := "8c1cbc9b-a3b9-45dd-a319-e97f75dbd511" // From logs

	fmt.Printf("Checking task %s in 'default' queue...\n", taskID)
	info, err := inspector.GetTaskInfo("default", taskID)
	if err != nil {
		fmt.Printf("Error getting task info: %v\n", err)
	} else {
		fmt.Printf("Success! State: %v\n", info.State)
	}

	// List queues
	fmt.Println("Listing queues:")
	queues, err := inspector.Queues()
	if err != nil {
		fmt.Printf("Error listing queues: %v\n", err)
	} else {
		for _, q := range queues {
			fmt.Printf("- %s\n", q)
		}
	}
}
