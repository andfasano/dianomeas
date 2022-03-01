package equinix

import (
	"errors"

	"github.com/packethost/packngo"
)

type Client struct {
	client        *packngo.Client
	desiredMetros []string
}

func NewClient(desiredMetros []string) (*Client, error) {

	c, err := packngo.NewClient()
	if err != nil {
		return nil, err
	}

	return &Client{
		client:        c,
		desiredMetros: desiredMetros,
	}, nil
}

func (c *Client) CheckAvailabilityFor(requiredInstanceType string) (string, error) {
	cr, _, err := c.client.CapacityService.ListMetros()
	if err != nil {
		return "", err
	}

	for metro, cpb := range *cr {

		for instanceType, capacity := range cpb {
			if instanceType != requiredInstanceType {
				continue
			}

			if capacity.Level == "unavailable" {
				continue
			}

			// If desiredMetros is empty, then the first
			// available metro will be picked up
			if c.desiredMetros == nil {
				return metro, nil
			}

			for _, m := range c.desiredMetros {
				if m == metro {
					return metro, nil
				}
			}
		}
	}

	return "", errors.New("no availability found")
}
