package main

import (
	"log"

	"github.com/andfasano/dianomeas/internal/pkg/equinix"
)

func main() {

	c, err := equinix.NewClient([]string{"dc", "ch", "sv"})
	if err != nil {
		log.Fatal(err)
	}

	metroCode, err := c.CheckAvailabilityFor("n2.xlarge.x86")
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Found availability in metro:", metroCode)
}
