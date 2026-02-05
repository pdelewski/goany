package http

import gohttp "runtime/http/go"

type Response struct {
	StatusCode int
	Status     string
	Body       string
}

func Get(url string) (Response, string) {
	resp, errStr := gohttp.Get(url)
	return Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       resp.Body,
	}, errStr
}

func Post(url string, contentType string, body string) (Response, string) {
	resp, errStr := gohttp.Post(url, contentType, body)
	return Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       resp.Body,
	}, errStr
}
