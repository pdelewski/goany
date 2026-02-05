package http

import gohttp "runtime/http/go"

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Status     string
	Body       string
}

// Request represents an incoming HTTP request (for server)
type Request struct {
	Method string
	Path   string
	Body   string
}

// Get performs an HTTP GET request
func Get(url string) (Response, string) {
	resp, errStr := gohttp.Get(url)
	return Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       resp.Body,
	}, errStr
}

// Post performs an HTTP POST request
func Post(url string, contentType string, body string) (Response, string) {
	resp, errStr := gohttp.Post(url, contentType, body)
	return Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       resp.Body,
	}, errStr
}

// HandleFunc registers a handler function for the given pattern
func HandleFunc(pattern string, handler func(Request) Response) {
	gohttp.HandleFunc(pattern, func(req gohttp.Request) gohttp.Response {
		httpReq := Request{
			Method: req.Method,
			Path:   req.Path,
			Body:   req.Body,
		}
		httpResp := handler(httpReq)
		return gohttp.Response{
			StatusCode: httpResp.StatusCode,
			Status:     httpResp.Status,
			Body:       httpResp.Body,
		}
	})
}

// ListenAndServe starts the HTTP server on the given address
// Returns error string ("" = no error)
func ListenAndServe(addr string) string {
	return gohttp.ListenAndServe(addr)
}
