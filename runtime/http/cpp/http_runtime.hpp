#ifndef HTTP_RUNTIME_HPP
#define HTTP_RUNTIME_HPP

#include <string>
#include <tuple>
#include <map>
#include <functional>
#include "../httplib.h"

namespace http {

struct Response {
    int StatusCode;
    std::string Status;
    std::string Body;
};

struct Request {
    std::string Method;
    std::string Path;
    std::string Body;
};

// Handler storage for server
static std::map<std::string, std::function<Response(Request)>> handlers;
static httplib::Server* serverPtr = nullptr;

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

// HandleFunc registers a handler function for the given pattern
static void HandleFunc(const std::string& pattern, std::function<Response(Request)> handler) {
    handlers[pattern] = handler;
}

// ListenAndServe starts the HTTP server on the given address
static std::string ListenAndServe(const std::string& addr) {
    httplib::Server svr;
    serverPtr = &svr;

    // Register all handlers
    for (const auto& pair : handlers) {
        const std::string& pattern = pair.first;
        const auto& handler = pair.second;

        // Register for all HTTP methods
        svr.Get(pattern, [handler](const httplib::Request& req, httplib::Response& res) {
            Request r;
            r.Method = req.method;
            r.Path = req.path;
            r.Body = req.body;
            Response resp = handler(r);
            res.status = resp.StatusCode;
            res.set_content(resp.Body, "text/plain");
        });
        svr.Post(pattern, [handler](const httplib::Request& req, httplib::Response& res) {
            Request r;
            r.Method = req.method;
            r.Path = req.path;
            r.Body = req.body;
            Response resp = handler(r);
            res.status = resp.StatusCode;
            res.set_content(resp.Body, "text/plain");
        });
    }

    // Parse address (e.g., ":8080" or "localhost:8080")
    std::string host = "0.0.0.0";
    int port = 8080;
    size_t colonPos = addr.rfind(':');
    if (colonPos != std::string::npos) {
        std::string hostPart = addr.substr(0, colonPos);
        std::string portPart = addr.substr(colonPos + 1);
        if (!hostPart.empty()) {
            host = hostPart;
        }
        port = std::stoi(portPart);
    }

    // Start server (blocks until error)
    if (!svr.listen(host, port)) {
        return "Failed to start server";
    }
    return "";
}

} // namespace http

#endif // HTTP_RUNTIME_HPP
