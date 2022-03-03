package equinix

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/andfasano/dianomeas/internal/pkg/hosts"
	"github.com/packethost/packngo"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Client struct {
	client         *packngo.Client
	projectID      string
	requiredPlan   string
	requiredOS     string
	requiredMetros []string
}

func NewClient(projectID string, requiredPlan string, requiredOS string, requiredMetros []string) (*Client, error) {

	c, err := packngo.NewClient()
	if err != nil {
		return nil, err
	}

	return &Client{
		client:         c,
		projectID:      projectID,
		requiredPlan:   requiredPlan,
		requiredOS:     requiredOS,
		requiredMetros: requiredMetros,
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
			if c.requiredMetros == nil {
				return metro, nil
			}

			for _, m := range c.requiredMetros {
				if m == metro {
					return metro, nil
				}
			}
		}
	}

	return "", errors.New("no availability found")
}

func (c *Client) exists(hostname string) (hosts.Host, error) {
	currentDevices, _, err := c.client.Devices.List(c.projectID, nil)
	if err != nil {
		return nil, err
	}
	for _, d := range currentDevices {
		if d.Hostname == hostname {
			return hosts.NewEquinixWrapper(d), nil
		}
	}
	return nil, nil
}

func (c *Client) create(hostname string) (hosts.Host, error) {
	metro, err := c.CheckAvailabilityFor(c.requiredPlan)
	if err != nil {
		return nil, err
	}

	log.Printf("Creating new instance %s (%s, %s) in metro %s\n", hostname, c.requiredPlan, c.requiredOS, metro)
	device, _, err := c.client.Devices.Create(&packngo.DeviceCreateRequest{
		Hostname:  hostname,
		Metro:     metro,
		Plan:      c.requiredPlan,
		OS:        c.requiredOS,
		ProjectID: c.projectID,
	})
	if err != nil {
		return nil, err
	}

	host := hosts.NewEquinixWrapper(*device)
	return host, nil
}

func (c *Client) waitForActive(host hosts.Host) (hosts.Host, error) {

	var newHost hosts.Host

	err := wait.PollImmediate(1*time.Minute, 30*time.Minute, func() (bool, error) {
		h, _, err := c.client.Devices.Get(host.Id(), nil)
		if err != nil {
			return false, err
		}
		log.Printf("Host %s (%s) current state is %s\n", h.Hostname, h.ID, h.State)
		if h.State == "active" {
			newHost = hosts.NewEquinixWrapper(*h)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return newHost, nil
}

func (c *Client) SetupDevice(prefix string) (hosts.Host, error) {

	hostname := fmt.Sprintf("%s-%d", prefix, rand.Intn(1000))

	start := time.Now()
	defer func() {
		end := time.Now()
		log.Printf("%s setup completed in %0.2f minutes\n", hostname, end.Sub(start).Minutes())
	}()

	host, err := c.exists(hostname)
	if err != nil {
		return nil, err
	}

	if host == nil {
		host, err = c.create(hostname)
		if err != nil {
			return nil, err
		}
	}

	host, err = c.waitForActive(host)
	if err != nil {
		return nil, err
	}

	return host, nil
}

func (c *Client) TeardownDevice(hostname string) error {

	host, err := c.exists(hostname)
	if err != nil {
		return err
	}

	if host == nil {
		log.Printf("Host %s already removed\n", hostname)
		return nil
	}

	log.Printf("Deleting host %s (%s)", host.Name(), host.Id())
	_, err = c.client.Devices.Delete(host.Id(), false)
	if err != nil {
		return err
	}

	return nil
}
