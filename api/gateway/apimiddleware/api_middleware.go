package apimiddleware

import (
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/mux"
)

// APIProxyMiddleware is a proxy between an consensus API HTTP client and grpc-gateway.
// The purpose of the proxy is to handle HTTP requests and gRPC responses in such a way that:
//   - consensus API requests can be handled by grpc-gateway correctly
//   - gRPC responses can be returned as spec-compliant consensus API responses
type APIProxyMiddleware struct {
	GatewayAddress  string
	EndpointCreator EndpointFactory
	Timeout         time.Duration
	router          *mux.Router
}

// EndpointFactory is responsible for creating new instances of Endpoint values.
type EndpointFactory interface {
	Create(path string) (*Endpoint, error)
	Paths() []string
	IsNil() bool
}

// Endpoint is a representation of an API HTTP endpoint that should be proxied by the middleware.
type Endpoint struct {
	Path               string          // The path of the HTTP endpoint.
	GetResponse        interface{}     // The struct corresponding to the JSON structure used in a GET response.
	PostRequest        interface{}     // The struct corresponding to the JSON structure used in a POST request.
	PostResponse       interface{}     // The struct corresponding to the JSON structure used in a POST response.
	DeleteRequest      interface{}     // The struct corresponding to the JSON structure used in a DELETE request.
	DeleteResponse     interface{}     // The struct corresponding to the JSON structure used in a DELETE response.
	RequestURLLiterals []string        // Names of URL parameters that should not be base64-encoded.
	RequestQueryParams []QueryParam    // Query parameters of the request.
	Err                ErrorJSON       // The struct corresponding to the error that should be returned in case of a request failure.
	Hooks              HookCollection  // A collection of functions that can be invoked at various stages of the request/response cycle.
	CustomHandlers     []CustomHandler // Functions that will be executed instead of the default request/response behavior.
}

// RunDefault expresses whether the default processing logic should be carried out after running a pre hook.
type RunDefault bool

// DefaultEndpoint returns an Endpoint with default configuration, e.g. DefaultErrorJSON for error handling.
func DefaultEndpoint() Endpoint {
	return Endpoint{
		Err: &DefaultErrorJSON{},
	}
}

// QueryParam represents a single query parameter's metadata.
type QueryParam struct {
	Name string
	Hex  bool
	Enum bool
}

// CustomHandler is a function that can be invoked at the very beginning of the request,
// essentially replacing the whole default request/response logic with custom logic for a specific endpoint.
type CustomHandler = func(m *APIProxyMiddleware, endpoint Endpoint, w http.ResponseWriter, req *http.Request) (handled bool)

// HookCollection contains hooks that can be used to amend the default request/response cycle with custom logic for a specific endpoint.
type HookCollection struct {
	OnPreDeserializeRequestBodyIntoContainer      func(endpoint *Endpoint, w http.ResponseWriter, req *http.Request) (RunDefault, ErrorJSON)
	OnPostDeserializeRequestBodyIntoContainer     func(endpoint *Endpoint, w http.ResponseWriter, req *http.Request) ErrorJSON
	OnPreDeserializeGrpcResponseBodyIntoContainer func([]byte, interface{}) (RunDefault, ErrorJSON)
	OnPreSerializeMiddlewareResponseIntoJSON      func(interface{}) (RunDefault, []byte, ErrorJSON)
}

// fieldProcessor applies the processing function f to a value when the tag is present on the field.
type fieldProcessor struct {
	tag string
	f   func(value reflect.Value) error
}

// Run starts the proxy, registering all proxy endpoints.
func (m *APIProxyMiddleware) Run(gatewayRouter *mux.Router) {
	for _, path := range m.EndpointCreator.Paths() {
		gatewayRouter.HandleFunc(path, m.WithMiddleware(path))
	}
	m.router = gatewayRouter
}

// ServeHTTP for the proxy middleware.
func (m *APIProxyMiddleware) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	m.router.ServeHTTP(w, req)
}

// WithMiddleware wraps the given endpoint handler with the middleware logic.
func (m *APIProxyMiddleware) WithMiddleware(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		endpoint, err := m.EndpointCreator.Create(path)
		if err != nil {
			log.WithError(err).Errorf("Could not create endpoint for path: %s", path)
			return
		}

		for _, handler := range endpoint.CustomHandlers {
			if handler(m, *endpoint, w, req) {
				return
			}
		}

		if req.Method == http.MethodPost {
			if errJSON := handlePostRequestForEndpoint(endpoint, w, req); errJSON != nil {
				WriteError(w, errJSON, nil)
				return
			}
		}

		if req.Method == http.MethodDelete {
			if errJSON := handleDeleteRequestForEndpoint(endpoint, req); errJSON != nil {
				WriteError(w, errJSON, nil)
				return
			}
		}

		if errJSON := m.PrepareRequestForProxying(*endpoint, req); errJSON != nil {
			WriteError(w, errJSON, nil)
			return
		}
		grpcResp, errJSON := m.ProxyRequest(req)
		if errJSON != nil {
			WriteError(w, errJSON, nil)
			return
		}
		grpcRespBody, errJSON := ReadGrpcResponseBody(grpcResp.Body)
		if errJSON != nil {
			WriteError(w, errJSON, nil)
			return
		}

		var respJSON []byte
		if !GrpcResponseIsEmpty(grpcRespBody) {
			respHasError, errJSON := HandleGrpcResponseError(endpoint.Err, grpcResp, grpcRespBody, w)
			if errJSON != nil {
				WriteError(w, errJSON, nil)
				return
			}
			if respHasError {
				return
			}

			var resp interface{}
			if req.Method == http.MethodGet {
				resp = endpoint.GetResponse
			} else if req.Method == http.MethodDelete {
				resp = endpoint.DeleteResponse
			} else {
				resp = endpoint.PostResponse
			}
			if errJSON = deserializeGrpcResponseBodyIntoContainerWrapped(endpoint, grpcRespBody, resp); errJSON != nil {
				WriteError(w, errJSON, nil)
				return
			}
			if errJSON = ProcessMiddlewareResponseFields(resp); errJSON != nil {
				WriteError(w, errJSON, nil)
				return
			}

			respJSON, errJSON = serializeMiddlewareResponseIntoJSONWrapped(endpoint, respJSON, resp)
			if errJSON != nil {
				WriteError(w, errJSON, nil)
				return
			}
		}

		if errJSON := WriteMiddlewareResponseHeadersAndBody(grpcResp, respJSON, w); errJSON != nil {
			WriteError(w, errJSON, nil)
			return
		}
		if errJSON := Cleanup(grpcResp.Body); errJSON != nil {
			WriteError(w, errJSON, nil)
			return
		}
	}
}

func handlePostRequestForEndpoint(endpoint *Endpoint, w http.ResponseWriter, req *http.Request) ErrorJSON {
	if errJSON := deserializeRequestBodyIntoContainerWrapped(endpoint, req, w); errJSON != nil {
		return errJSON
	}
	if errJSON := ProcessRequestContainerFields(endpoint.PostRequest); errJSON != nil {
		return errJSON
	}
	return SetRequestBodyToRequestContainer(endpoint.PostRequest, req)
}

func handleDeleteRequestForEndpoint(endpoint *Endpoint, req *http.Request) ErrorJSON {
	if errJSON := DeserializeRequestBodyIntoContainer(req.Body, endpoint.DeleteRequest); errJSON != nil {
		return errJSON
	}
	if errJSON := ProcessRequestContainerFields(endpoint.DeleteRequest); errJSON != nil {
		return errJSON
	}
	return SetRequestBodyToRequestContainer(endpoint.DeleteRequest, req)
}

func deserializeRequestBodyIntoContainerWrapped(endpoint *Endpoint, req *http.Request, w http.ResponseWriter) ErrorJSON {
	runDefault := true
	if endpoint.Hooks.OnPreDeserializeRequestBodyIntoContainer != nil {
		run, errJSON := endpoint.Hooks.OnPreDeserializeRequestBodyIntoContainer(endpoint, w, req)
		if errJSON != nil {
			return errJSON
		}
		if !run {
			runDefault = false
		}
	}
	if runDefault {
		if errJSON := DeserializeRequestBodyIntoContainer(req.Body, endpoint.PostRequest); errJSON != nil {
			return errJSON
		}
	}
	if endpoint.Hooks.OnPostDeserializeRequestBodyIntoContainer != nil {
		if errJSON := endpoint.Hooks.OnPostDeserializeRequestBodyIntoContainer(endpoint, w, req); errJSON != nil {
			return errJSON
		}
	}
	return nil
}

func deserializeGrpcResponseBodyIntoContainerWrapped(endpoint *Endpoint, grpcResponseBody []byte, resp interface{}) ErrorJSON {
	runDefault := true
	if endpoint.Hooks.OnPreDeserializeGrpcResponseBodyIntoContainer != nil {
		run, errJSON := endpoint.Hooks.OnPreDeserializeGrpcResponseBodyIntoContainer(grpcResponseBody, resp)
		if errJSON != nil {
			return errJSON
		}
		if !run {
			runDefault = false
		}
	}
	if runDefault {
		if errJSON := DeserializeGrpcResponseBodyIntoContainer(grpcResponseBody, resp); errJSON != nil {
			return errJSON
		}
	}
	return nil
}

func serializeMiddlewareResponseIntoJSONWrapped(endpoint *Endpoint, respJSON []byte, resp interface{}) ([]byte, ErrorJSON) {
	runDefault := true
	var errJSON ErrorJSON
	if endpoint.Hooks.OnPreSerializeMiddlewareResponseIntoJSON != nil {
		var run RunDefault
		run, respJSON, errJSON = endpoint.Hooks.OnPreSerializeMiddlewareResponseIntoJSON(resp)
		if errJSON != nil {
			return nil, errJSON
		}
		if !run {
			runDefault = false
		}
	}
	if runDefault {
		respJSON, errJSON = SerializeMiddlewareResponseIntoJSON(resp)
		if errJSON != nil {
			return nil, errJSON
		}
	}
	return respJSON, nil
}
