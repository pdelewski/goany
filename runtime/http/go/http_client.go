package gohttp

import (
	"io"
	gohttp "net/http"
	"strings"
)

type Response struct {
	StatusCode int
	Status     string
	Body       string
}

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
