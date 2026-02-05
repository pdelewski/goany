const http = {
    Response: function(statusCode, status, body) {
        return { StatusCode: statusCode, Status: status, Body: body };
    },

    Request: function(method, path, body) {
        return { Method: method, Path: path, Body: body };
    },

    _handlers: {},

    Get: function(url) {
        try {
            var result = require('child_process').execSync(
                'curl -s -w "\\n%{http_code}" "' + url + '"',
                { encoding: 'utf8', timeout: 10000 }
            );
            var lines = result.split('\n');
            var statusCode = parseInt(lines[lines.length - 1]) || 0;
            var body = lines.slice(0, -1).join('\n');
            return [{ StatusCode: statusCode, Status: String(statusCode), Body: body }, ""];
        } catch(e) {
            return [{ StatusCode: 0, Status: "", Body: "" }, e.message];
        }
    },

    Post: function(url, contentType, body) {
        try {
            var escaped = body.replace(/'/g, "'\\''");
            var result = require('child_process').execSync(
                "curl -s -w '\\n%{http_code}' -X POST -H 'Content-Type: " + contentType + "' -d '" + escaped + "' '" + url + "'",
                { encoding: 'utf8', timeout: 10000 }
            );
            var lines = result.split('\n');
            var statusCode = parseInt(lines[lines.length - 1]) || 0;
            var responseBody = lines.slice(0, -1).join('\n');
            return [{ StatusCode: statusCode, Status: String(statusCode), Body: responseBody }, ""];
        } catch(e) {
            return [{ StatusCode: 0, Status: "", Body: "" }, e.message];
        }
    },

    HandleFunc: function(pattern, handler) {
        http._handlers[pattern] = handler;
    },

    ListenAndServe: function(addr) {
        try {
            var nodeHttp = require('http');

            // Parse address (e.g., ":8080" or "localhost:8080")
            var host = '0.0.0.0';
            var port = 8080;
            var colonIdx = addr.lastIndexOf(':');
            if (colonIdx >= 0) {
                var hostPart = addr.substring(0, colonIdx);
                var portPart = addr.substring(colonIdx + 1);
                if (hostPart) {
                    host = hostPart;
                }
                port = parseInt(portPart);
            }

            var server = nodeHttp.createServer(function(req, res) {
                var body = '';
                req.on('data', function(chunk) {
                    body += chunk;
                });
                req.on('end', function() {
                    var path = req.url;
                    var response = { StatusCode: 404, Status: 'Not Found', Body: 'Not Found' };

                    // Find matching handler
                    for (var pattern in http._handlers) {
                        // Simple pattern matching: exact match or prefix match
                        if (path === pattern || path.startsWith(pattern.replace(/\/$/, '') + '/') || pattern === '/') {
                            var request = { Method: req.method, Path: path, Body: body };
                            response = http._handlers[pattern](request);
                            break;
                        }
                    }

                    res.writeHead(response.StatusCode, { 'Content-Type': 'text/plain' });
                    res.end(response.Body);
                });
            });

            server.listen(port, host);
            // Block forever (server runs in background)
            // In Node.js, the event loop keeps running while server is listening
            return "";
        } catch(e) {
            return e.message;
        }
    }
};
