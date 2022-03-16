package equinix

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strings"
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

func (c *Client) getDeviceId(e packngo.Event) string {
	re := regexp.MustCompile("\\\"(.[^\\\"]*)\\\"")

	deviceId := ""
	match := re.FindStringSubmatch(e.Interpolated)
	if match != nil {
		deviceId = match[1]
	}

	return deviceId

}

func (c *Client) ListEvents() error {

	maxPages := 30
	lastNdays := 8
	instanceCostPerHour := 2.0
	leakHoursThreshold := 4.0

	numCreations := make(map[string]int)
	now := time.Now()
	continueSearch := true

	instancesCreatedAt := make(map[string]time.Time)
	instancesDeletedAt := make(map[string]time.Time)

	prevNumDays := -1

	log.Printf("Fetching events for the last %d days\n", lastNdays)
	for i := 1; continueSearch && i <= maxPages; i++ {

		events, _, err := c.client.Projects.ListEvents(c.projectID, &packngo.GetOptions{
			PerPage: 500,
			Page:    i,
		})
		if err != nil {
			return err
		}

		for _, event := range events {

			t2 := now.Truncate(24 * time.Hour)
			t1 := event.CreatedAt.Time.Truncate(24 * time.Hour)

			numDays := int(t2.Sub(t1).Hours()) / 24
			if numDays > lastNdays {
				continueSearch = false
				break
			}

			if numDays == 0 {
				continue
			}

			if numDays != prevNumDays {
				log.Printf("Scanning events for %s (T-%d)\n", event.CreatedAt.Format("2006-01-02"), numDays)
				prevNumDays = numDays
			}

			deviceId := c.getDeviceId(event)
			if !strings.HasPrefix(deviceId, "ipi") {
				continue
			}

			switch event.Type {
			case "instance.created":
				instancesCreatedAt[deviceId] = event.CreatedAt.Time
				key := fmt.Sprintf("%d-%02d-%02d", event.CreatedAt.Year(), event.CreatedAt.Month(), event.CreatedAt.Day())
				numCreations[key]++
			case "instance.deleted":
				instancesDeletedAt[deviceId] = event.CreatedAt.Time
			}
		}
	}
	log.Println()

	totalInstances := 0
	var totalTime, maxTime time.Duration
	var maxId string
	numLeaks := 0
	var totalCost, maxCost float64

	// Instances per day / total
	var keys []string
	for k := range numCreations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		log.Println(k, "num instances:", numCreations[k])
		totalInstances += numCreations[k]
	}
	log.Println("Total instances:", totalInstances)
	log.Println()

	// Max and average usage
	for deviceId, creationTime := range instancesCreatedAt {
		if deletionTime, ok := instancesDeletedAt[deviceId]; ok {

			curr := deletionTime.Sub(creationTime)
			currCost := math.Ceil(curr.Hours()) * instanceCostPerHour

			totalTime += curr
			if curr >= maxTime {
				maxTime = curr
				maxId = deviceId
				maxCost = currCost
			}
			totalCost += currCost
			if currCost >= maxCost {
				maxCost = currCost
			}

			if curr.Hours() > leakHoursThreshold {
				numLeaks++
			}

		}
	}

	avgSecs := int(totalTime.Seconds()) / len(instancesCreatedAt)
	avgTime := time.Duration(avgSecs) * time.Second
	log.Printf("Average instance uptime: %s\n", avgTime)
	log.Printf("Num leaks (uptime > 4h): %d\n", numLeaks)
	log.Println()
	log.Printf("Total uptime:            %s\n", totalTime)
	log.Printf("Max instance uptime:     %s (%s)\n", maxTime, maxId)
	log.Println()
	log.Printf("Total cost:              $ %.f", totalCost)
	log.Printf("Most expensive instance: %s (%s, $ %.f)", maxId, maxTime, maxCost)

	return nil
}
