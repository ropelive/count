package clients

import (
	"io"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/sd"
	"github.com/go-kit/kit/sd/consul"
	"github.com/go-kit/kit/sd/lb"
	"github.com/ropelive/count/services/compactor"
)

// NewCompactor returns a service that's load-balanced over instances of
// compactor found in the provided Consul server. The mechanism of looking up
// compactor instances in Consul is hard-coded into the client.
func NewCompactor(consulAddr string, logger log.Logger) (compactor.Service, error) {
	apiclient, err := consulapi.NewClient(&consulapi.Config{
		Address: consulAddr,
	})
	if err != nil {
		return nil, err
	}

	// As the implementer of compactor, we declare and enforce these
	// parameters for all of the compactor consumers.
	var (
		consulService = "compactor"
		consulTags    = []string{"prod"}
		passingOnly   = true
		retryMax      = 3
		retryTimeout  = 500 * time.Millisecond
	)

	var (
		sdclient  = consul.NewClient(apiclient)
		instancer = consul.NewInstancer(sdclient, logger, consulService, consulTags, passingOnly)
		endpoints compactor.Endpoints
	)
	{
		factory := factoryForCompactor(compactor.MakeProcessEndpoint)
		endpointer := sd.NewEndpointer(instancer, factory, logger)
		balancer := lb.NewRoundRobin(endpointer)
		retry := lb.Retry(retryMax, retryTimeout, balancer)
		endpoints.ProcessEndpoint = retry
	}

	return endpoints, nil
}

func factoryForCompactor(makeEndpoint func(compactor.Service) endpoint.Endpoint) sd.Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		service, err := compactor.MakeHTTPClientEndpoints(instance)
		if err != nil {
			return nil, nil, err
		}
		return makeEndpoint(service), nil, nil
	}
}
