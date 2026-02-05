const http = {
    Response: function(statusCode, status, body) {
        return { StatusCode: statusCode, Status: status, Body: body };
    },

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
    }
};
