package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"github.com/buildertools/svctools-go/clients"
	rancher "github.com/rancher/go-rancher/client"
)

const imageUUIDPrefix = `docker:`
var (
	interval = time.Duration(1)*time.Second
	jitter = time.Duration(500)*time.Millisecond
)
type Config struct {
	Endpoint string
	AccessKey string
	SecretKey string
	Service string
	Stack string
	Image string
	StartFirst bool
	Confirm bool
	BatchSize int64
	BatchInterval time.Duration
	Timeout time.Duration
	NotifyHook string
	SuccessChannel string
	BlockedChannel string
	SuccessEmoji string
	BlockedEmoji string
}

func main() {
	c := getConfig()
	rc, err := rancher.NewRancherClient(&rancher.ClientOpts{
		Url:       c.Endpoint,
		AccessKey: c.AccessKey,
		SecretKey: c.SecretKey,
	})
	if err != nil {
		panic(fmt.Sprintf(`Failed to create rancher client: %s`, err))
	}

	var stacks *rancher.EnvironmentCollection
	te, err := clients.RetryLinear(
		func() (interface{}, clients.ClientError) {
			s, err := rc.Environment.List(&rancher.ListOpts{})
			if err != nil {
				return nil, clients.RetriableError{ E: err }
			}
			return s, nil
		},
		c.Timeout,
		interval,
		jitter,
	)
	stacks = te.(*rancher.EnvironmentCollection)

	if err != nil {
		panic(fmt.Sprintf(`Unable to retrieve the list of stacks from rancher: %v`, err))
	}

	var services *rancher.ServiceCollection
	ts, err := clients.RetryLinear(
		func() (interface{}, clients.ClientError) {
			s, err := rc.Service.List(&rancher.ListOpts{})
			if err != nil {
				return nil, clients.RetriableError{ E: err }
			}
			return s, nil
		},
		c.Timeout,
		interval,
		jitter,
	)
	services = ts.(*rancher.ServiceCollection)

	if err != nil {
		panic(fmt.Sprintf(`Unable to retrieve the list of services from rancher: %v`, err))
	}

	sCache := buildStackNameToServiceNameToServiceMap(stacks, services)
	stack, ok := sCache[c.Stack]
	if !ok {
		panic(`No stack exists with the specified name.`)
	}
	service, ok := stack[c.Service]
	if !ok {
		panic(`No service exists with the specified name.`)
	}

	// Verify that upgrade can occur
	if _, ok = service.Actions["upgrade"]; !ok {
		notify(fmt.Sprintf(`CD pipeline blocked on deployment to %s/%s`, c.Stack, c.Service), c.BlockedChannel, c.BlockedEmoji, c.NotifyHook)
		panic(fmt.Sprintf(`Upgrade not available. Current status: %v`, service.State))
	}

	service.LaunchConfig.ImageUuid = c.Image
	cmd := &rancher.ServiceUpgrade{}
	cmd.InServiceStrategy = &rancher.InServiceUpgradeStrategy{
		BatchSize:              c.BatchSize,
		IntervalMillis:         int64(c.BatchInterval/time.Millisecond),
		LaunchConfig:           service.LaunchConfig,
		SecondaryLaunchConfigs: service.SecondaryLaunchConfigs,
		StartFirst:             c.StartFirst,
	}
	cmd.ToServiceStrategy = &rancher.ToServiceUpgradeStrategy{}

	_, err = rc.Service.ActionUpgrade(&service, cmd)
	if err != nil {
		notify(fmt.Sprintf(`CD pipeline blocked on deployment to %s/%s`, c.Stack, c.Service), c.BlockedChannel, c.BlockedEmoji, c.NotifyHook)
		panic(fmt.Sprintf(`Upgrade command failed for service %s/%s: %s`, c.Stack, c.Service, err))
	}

	if !c.Confirm {
		notify(fmt.Sprintf(`Unfinished deployment to %s/%s initialized`, c.Stack, c.Service), c.SuccessChannel, c.SuccessEmoji, c.NotifyHook)
		fmt.Println(`Upgrade issued but not confirmed`)
		return
	}

	upgraded, err := clients.RetryPeriodic(
		func() (interface{}, clients.ClientError) {
			s, err := rc.Service.ById(service.Id)
			if err != nil {
				return nil, clients.RetriableError{ E: err }
			}
			if s.State != `upgraded` {
				return nil, clients.RetriableError{ E: errors.New(`Not upgraded yet.`) }
			}
			return s, nil
		},
		c.Timeout,
		interval,
		jitter,
	)

	if err != nil {
		panic(fmt.Sprintf(`Timeout while waiting for the upgrade to complete: %v`, err))
	}

	_, err = clients.RetryLinear(
		func() (interface{}, clients.ClientError) {
			_, err := rc.Service.ActionFinishupgrade(upgraded.(*rancher.Service))
			if err != nil {
				return nil, clients.RetriableError{ E: err }
			}
			return nil, nil
		},
		c.Timeout, 
		interval,
		jitter,
	)

	notify(fmt.Sprintf(`Deployment to %s/%s completed`, c.Stack, c.Service), c.SuccessChannel, c.SuccessEmoji, c.NotifyHook)
	fmt.Printf("Finished %s/%s deployment\n", c.Stack, c.Service)

}

func buildStackNameToServiceNameToServiceMap(es *rancher.EnvironmentCollection, ss *rancher.ServiceCollection) map[string]map[string]rancher.Service {
	ec := map[string]rancher.Environment{}
	r := map[string]map[string]rancher.Service{}
	for _, e := range es.Data {
		ec[e.Id] = e
	}
	for _, s := range ss.Data {
		if len(s.EnvironmentId) == 0 {
			continue
		}
		e, ok := ec[s.EnvironmentId]
		if !ok {
			panic(`Service references non-existant stack ID.`)
		}
		var sm map[string]rancher.Service
		if sm, ok = r[e.Name]; !ok {
			sm = map[string]rancher.Service{}
			r[e.Name] = sm
		}
		sm[s.Name] = s
	}
	return r
}

func notify(m string, c string, e string, h string) {
	if len(h) != 0 {
		msg := strings.NewReader(fmt.Sprintf(
			`{"text": "%s", "channel": "#%s", "username": "drone-rancher-plugin", "icon_emoji": ":%s:"}`,
			m,
			c,
			e))
		http.Post(h, `application/json`, msg)
	}
}

func getConfig() Config {
	c := Config{
		Endpoint:       os.Getenv(`PLUGIN_URL`),
		AccessKey:      os.Getenv(`PLUGIN_ACCESS_KEY`),
		SecretKey:      os.Getenv(`PLUGIN_SECRET_KEY`),
		Service:        os.Getenv(`PLUGIN_SERVICE`),
		Stack:          os.Getenv(`PLUGIN_STACK`),
		Image:          os.Getenv(`PLUGIN_DOCKER_IMAGE`),
		NotifyHook:     os.Getenv(`PLUGIN_NOTIFY_WEBHOOK`),
		SuccessChannel: os.Getenv(`PLUGIN_SUCCESS_CHANNEL`),
		BlockedChannel: os.Getenv(`PLUGIN_BLOCKED_CHANNEL`),
		SuccessEmoji:   os.Getenv(`PLUGIN_SUCCESS_EMOJI`),
		BlockedEmoji:   os.Getenv(`PLUGIN_BLOCKED_EMOJI`),
	}
	var err error

	// Parse and validate
	c.Timeout, err = time.ParseDuration(os.Getenv(`PLUGIN_TIMEOUT`))
	if err != nil {
		panic(fmt.Sprintf(`Invalid timeout specification: %v`, err))
	}
	c.BatchSize, err = strconv.ParseInt(os.Getenv(`PLUGIN_BATCH_SIZE`), 10, 64)
	if err != nil {
		panic(fmt.Sprintf(`Invalid batch size specification: %v`, err))
	}
	c.BatchInterval, err = time.ParseDuration(os.Getenv(`PLUGIN_BATCH_INTERVAL`))
	if err != nil {
		panic(fmt.Sprintf(`Invalid batch interval specification: %v`, err))
	}
	c.Confirm, err = strconv.ParseBool(os.Getenv(`PLUGIN_CONFIRM`))
	if err != nil {
		panic(fmt.Sprintf(`Invalid confirm specification: %v`, err))
	}
	c.StartFirst, err = strconv.ParseBool(os.Getenv(`PLUGIN_START_FIRST`))
	if err != nil {
		panic(fmt.Sprintf(`Invalid startfirst specification: %v`, err))
	}

	// Validate strings
	if len(c.Endpoint) == 0 {
		panic(`Missing required parameter: endpoint`)
	}
	if len(c.AccessKey) == 0 {
		panic(`Missing required parameter: accesskey`)
	}
	if len(c.SecretKey) == 0 {
		panic(`Missing required parameter: secretkey`)
	}
	if len(c.Service) == 0 {
		panic(`Missing required parameter: service`)
	}
	if len(c.Image) == 0 {
		panic(`Missing required parameter: image`)
	}

	// Scrub
	if !strings.HasPrefix(c.Image, imageUUIDPrefix) {
		c.Image = fmt.Sprintf("%s%s", imageUUIDPrefix, c.Image)
	}
	if strings.Contains(c.Service, "/") {
		if len(c.Stack) != 0 {
			panic(`Cannot specify stack by both field and prefix`)
		}
		p := strings.SplitN(c.Service, "/", 2)
		c.Stack = p[0]
		c.Service = p[1]
	}

	return c
}

