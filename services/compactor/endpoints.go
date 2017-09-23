package compactor

import (
	"context"
	"net/url"
	"strings"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
)

// Endpoints collects all of the endpoints that compose a compactor service.
// It's meant to be used as a helper struct, to collect all of the endpoints
// into a single parameter.
//
// In a server, it's useful for functions that need to operate on a per-endpoint
// basis. For example, you might pass an Endpoints to a function that produces
// an http.Handler, with each method (endpoint) wired up to a specific path. (It
// is probably a mistake in design to invoke the Service methods on the
// Endpoints struct in a server.)
//
// In a client, it's useful to collect individually constructed endpoints into a
// single type that implements the Service interface. For example, you might
// construct individual endpoints using transport/http.NewClient, combine them
// into an Endpoints, and return it to the caller as a Service.
type Endpoints struct {
	ProcessEndpoint endpoint.Endpoint
}

// MakeServerEndpoints returns an Endpoints struct where each endpoint invokes
// the corresponding method on the provided service. Useful in a compactor
// server.
func MakeServerEndpoints(s Service) Endpoints {
	return Endpoints{
		ProcessEndpoint: MakeProcessEndpoint(s),
	}
}

// MakeClientEndpoints returns an Endpoints struct where each endpoint invokes
// the corresponding method on the remote instance, via a transport/http.Client.
// Useful in a compactor client.
func MakeClientEndpoints(instance string) (Endpoints, error) {
	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	tgt, err := url.Parse(instance)
	if err != nil {
		return Endpoints{}, err
	}
	tgt.Path = ""

	options := []httptransport.ClientOption{}

	// Note that the request encoders need to modify the request URL, changing
	// the path and method. That's fine: we simply need to provide specific
	// encoders for each endpoint.

	return Endpoints{
		ProcessEndpoint: httptransport.NewClient("POST", tgt, encodeProcessRequest, decodeProcessResponse, options...).Endpoint(),
	}, nil
}

// Process implements Service. Primarily useful in a client.
func (e Endpoints) Process(ctx context.Context, p Profile) error {
	request := ProcessRequest{Profile: p}
	response, err := e.ProcessEndpoint(ctx, request)
	if err != nil {
		return err
	}
	resp := response.(ProcessResponse)
	return resp.Err
}

// MakeProcessEndpoint returns an endpoint via the passed service.
// Primarily useful in a server.
func MakeProcessEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(ProcessRequest)
		e := s.Process(ctx, req.Profile)
		return ProcessResponse{Err: e}, nil
	}
}

// We have two options to return errors from the business logic.
//
// We could return the error via the endpoint itself. That makes certain things
// a little bit easier, like providing non-200 HTTP responses to the client. But
// Go kit assumes that endpoint errors are (or may be treated as)
// transport-domain errors. For example, an endpoint error will count against a
// circuit breaker error count.
//
// Therefore, it's often better to return service (business logic) errors in the
// response object. This means we have to do a bit more work in the HTTP
// response encoder to detect e.g. a not-found error and provide a proper HTTP
// status code. That work is done with the errorer interface, in transport.go.
// Response types that may contain business-logic errors implement that
// interface.

// ProcessRequest holds the request data for the Process handler
type ProcessRequest struct {
	Profile Profile
}

// ProcessResponse holds the response data for the Process handler
type ProcessResponse struct {
	Err error `json:"err,omitempty"`
}

func (r ProcessResponse) error() error { return r.Err }