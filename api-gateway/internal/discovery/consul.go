package discovery

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

func DiscoverService(consulAddr string, serviceName string) (string, error) {
	consulClient, err := api.NewClient(&api.Config{Address: consulAddr})
	if err != nil {
		return "", fmt.Errorf("create consul client failed: %w", err)
	}

	services, err := consulClient.Agent().Services()
	if err != nil {
		return "", fmt.Errorf("query consul service failed: %w", err)
	}
	for _, svc := range services {
		if svc.Service == serviceName {
			return fmt.Sprintf("%s:%d", svc.Address, svc.Port), nil
		}
	}

	if len(services) == 0 {
		return "", fmt.Errorf("no service registered in consul")
	}
	return "", fmt.Errorf("service %s is not registered", serviceName)
}
