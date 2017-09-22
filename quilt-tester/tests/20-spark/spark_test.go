package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/quilt/quilt/api"
	"github.com/quilt/quilt/api/client"
	"github.com/quilt/quilt/connection/credentials"
	"github.com/quilt/quilt/util"
)

func TestCalculatesPI(t *testing.T) {
	clnt, err := client.New(api.DefaultSocket, credentials.Insecure{})
	if err != nil {
		t.Fatalf("couldn't get api client: %s", err.Error())
	}
	defer clnt.Close()

	containers, err := clnt.QueryContainers()
	if err != nil {
		t.Fatalf("couldn't query containers: %s", err.Error())
	}

	containersPretty, _ := exec.Command("quilt", "ps").Output()
	fmt.Println("`quilt ps` output:")
	fmt.Println(string(containersPretty))

	var id string
	for _, dbc := range containers {
		if strings.Join(dbc.Command, " ") == "run master" {
			id = dbc.BlueprintID
			break
		}
	}
	if id == "" {
		t.Fatal("unable to find BlueprintID of Spark master")
	}

	// The Spark job takes some time to complete, so we wait for the appropriate
	// result for up to a minute.
	err = util.BackoffWaitFor(func() bool {
		logs, err := exec.Command("quilt", "logs", id).CombinedOutput()
		if err != nil {
			t.Errorf("unable to get Spark master logs: %s", err)
			return false
		}

		fmt.Printf("`quilt logs %s` output:\n", id)
		fmt.Println(string(logs))
		return strings.Contains(string(logs), "Pi is roughly")
	}, 15*time.Second, time.Minute)

	if err != nil {
		t.Fatalf("unable to get Spark master logs: %s", err.Error())
	}
}
