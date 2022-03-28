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

	from := time.Now().Truncate(24 * time.Hour)
	to := from
	if len(os.Args) == 3 {
		from, err = time.Parse("2006-01-02", os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		to, err = time.Parse("2006-01-02", os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
	}

	if from.After(to) {
		log.Fatal("Invalid date range")
	}

	err = c.ListEvents(from, to)
	if err != nil {
		log.Fatal(err)
	}
}
