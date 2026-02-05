#ifndef HTTP_RUNTIME_HPP
#define HTTP_RUNTIME_HPP

#include <string>
#include <tuple>
#include "../httplib.h"

namespace http {

struct Response {
    int StatusCode;
    std::string Status;
    std::string Body;
};

// Parse a URL into scheme, host (with port), and path
// e.g. "http://example.com:8080/api/data" -> ("http", "example.com:8080", "/api/data")
static void parseURL(const std::string& url, std::string& scheme, std::string& host, std::string& path) {
    scheme = "http";
    host = "";
    path = "/";

    size_t pos = 0;
    // Extract scheme
    size_t schemeEnd = url.find("://");
    if (schemeEnd != std::string::npos) {
        scheme = url.substr(0, schemeEnd);
        pos = schemeEnd + 3;
    }

    // Extract host and path
    size_t pathStart = url.find('/', pos);
    if (pathStart != std::string::npos) {
        host = url.substr(pos, pathStart - pos);
        path = url.substr(pathStart);
    } else {
        host = url.substr(pos);
        path = "/";
    }
}

static std::tuple<Response, std::string> Get(const std::string& url) {
    std::string scheme, host, path;
    parseURL(url, scheme, host, path);

    std::string baseURL = scheme + "://" + host;
    httplib::Client cli(baseURL);
    cli.set_follow_location(true);
    cli.set_connection_timeout(10, 0);
    cli.set_read_timeout(10, 0);

    auto result = cli.Get(path);
    if (!result) {
        return std::make_tuple(Response{0, "", ""}, std::string("HTTP request failed"));
    }

    Response resp;
    resp.StatusCode = result->status;
    resp.Status = std::to_string(result->status);
    resp.Body = result->body;
    return std::make_tuple(resp, std::string(""));
}

static std::tuple<Response, std::string> Post(const std::string& url, const std::string& contentType, const std::string& body) {
    std::string scheme, host, path;
    parseURL(url, scheme, host, path);

    std::string baseURL = scheme + "://" + host;
    httplib::Client cli(baseURL);
    cli.set_follow_location(true);
    cli.set_connection_timeout(10, 0);
    cli.set_read_timeout(10, 0);

    auto result = cli.Post(path, body, contentType);
    if (!result) {
        return std::make_tuple(Response{0, "", ""}, std::string("HTTP request failed"));
    }

    Response resp;
    resp.StatusCode = result->status;
    resp.Status = std::to_string(result->status);
    resp.Body = result->body;
    return std::make_tuple(resp, std::string(""));
}

} // namespace http

#endif // HTTP_RUNTIME_HPP
