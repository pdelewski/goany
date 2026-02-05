package gohttp

import (
	"io"
	gohttp "net/http"
	"strings"
	"sync"
)

type Response struct {
	StatusCode int
	Status     string
	Body       string
}

type Request struct {
	Method string
	Path   string
	Body   string
}

// Handler storage for server
var (
	handlers     = make(map[string]func(Request) Response)
	handlersLock sync.Mutex
)

func Get(url string) (Response, string) {
	resp, err := gohttp.Get(url)
	if err != nil {
		return Response{}, err.Error()
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{StatusCode: resp.StatusCode, Status: resp.Status}, err.Error()
	}

	return Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       string(body),
	}, ""
}

func Post(url string, contentType string, body string) (Response, string) {
	resp, err := gohttp.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		return Response{}, err.Error()
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{StatusCode: resp.StatusCode, Status: resp.Status}, err.Error()
	}

	return Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       string(respBody),
	}, ""
}

// HandleFunc registers a handler function for the given pattern
func HandleFunc(pattern string, handler func(Request) Response) {
	handlersLock.Lock()
	handlers[pattern] = handler
	handlersLock.Unlock()
}

// ListenAndServe starts the HTTP server on the given address
func ListenAndServe(addr string) string {
	// Create a mux and register all handlers
	mux := gohttp.NewServeMux()

	handlersLock.Lock()
	for pattern, handler := range handlers {
		h := handler // capture for closure
		mux.HandleFunc(pattern, func(w gohttp.ResponseWriter, r *gohttp.Request) {
			// Read request body
			body, _ := io.ReadAll(r.Body)
			r.Body.Close()

			// Create our Request
			req := Request{
				Method: r.Method,
				Path:   r.URL.Path,
				Body:   string(body),
			}

			// Call handler
			resp := h(req)

			// Write response
			w.WriteHeader(resp.StatusCode)
			w.Write([]byte(resp.Body))
		})
	}
	handlersLock.Unlock()

	// Start the server (blocks until error)
	err := gohttp.ListenAndServe(addr, mux)
	if err != nil {
		return err.Error()
	}
	return ""
}
