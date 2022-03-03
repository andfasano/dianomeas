package main

import (
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/andfasano/dianomeas/internal/pkg/equinix"
)

func main() {

	rand.Seed(time.Now().UnixNano())

	equinixProjectID := os.Getenv("EQUINIX_PROJECT")
	if equinixProjectID == "" {
		log.Fatal("EQUINIX_PROJECT env var not found")
	}

	c, err := equinix.NewClient(equinixProjectID, "n2.xlarge.x86", "rocky_8", []string{"dc", "ch", "sv"})
	if err != nil {
		log.Fatal(err)
	}

	host, err := c.SetupDevice("eom")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("New host: %s (%s)\n", host.Name(), host.IPv4())

	time.Sleep(1 * time.Minute)

	err = c.TeardownDevice(host.Name())
	if err != nil {
		log.Fatal(err)
	}
}
