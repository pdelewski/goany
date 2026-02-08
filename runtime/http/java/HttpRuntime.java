import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.HashMap;
import java.util.Map;
import java.util.function.Function;

class http {
    public static class Response {
        public int StatusCode;
        public String Status;
        public String Body;

        public Response(int statusCode, String status, String body) {
            this.StatusCode = statusCode;
            this.Status = status;
            this.Body = body;
        }
    }

    public static class Request {
        public String Method;
        public String Path;
        public String Body;

        public Request(String method, String path, String body) {
            this.Method = method;
            this.Path = path;
            this.Body = body;
        }
    }

    private static final HttpClient client = HttpClient.newHttpClient();
    private static final Map<String, Function<Request, Response>> handlers = new HashMap<>();

    public static Object[] Get(String url) {
        try {
            HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .GET()
                .build();

            HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());

            return new Object[]{
                new Response(response.statusCode(), String.valueOf(response.statusCode()), response.body()),
                ""
            };
        } catch (Exception e) {
            return new Object[]{new Response(0, "", ""), e.getMessage()};
        }
    }

    public static Object[] Post(String url, String contentType, String body) {
        try {
            HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .header("Content-Type", contentType)
                .POST(HttpRequest.BodyPublishers.ofString(body))
                .build();

            HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());

            return new Object[]{
                new Response(response.statusCode(), String.valueOf(response.statusCode()), response.body()),
                ""
            };
        } catch (Exception e) {
            return new Object[]{new Response(0, "", ""), e.getMessage()};
        }
    }

    public static void HandleFunc(String pattern, Function<Request, Response> handler) {
        handlers.put(pattern, handler);
    }

    public static String ListenAndServe(String addr) {
        try {
            // Parse address
            String host = "0.0.0.0";
            int port = 8080;
            int colonIdx = addr.lastIndexOf(':');
            if (colonIdx >= 0) {
                String hostPart = addr.substring(0, colonIdx);
                port = Integer.parseInt(addr.substring(colonIdx + 1));
                if (!hostPart.isEmpty()) {
                    host = hostPart;
                }
            }

            var serverSocket = new java.net.ServerSocket(port, 50, java.net.InetAddress.getByName(host));

            while (true) {
                var clientSocket = serverSocket.accept();
                handleClient(clientSocket);
            }
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    private static void handleClient(java.net.Socket clientSocket) {
        try (clientSocket;
             var reader = new java.io.BufferedReader(new java.io.InputStreamReader(clientSocket.getInputStream()));
             var writer = new java.io.PrintWriter(clientSocket.getOutputStream(), true)) {

            // Read request line
            String requestLine = reader.readLine();
            if (requestLine == null) return;

            String[] parts = requestLine.split(" ");
            if (parts.length < 2) return;

            String method = parts[0];
            String path = parts[1];

            // Read headers (skip for simplicity)
            String line;
            int contentLength = 0;
            while ((line = reader.readLine()) != null && !line.isEmpty()) {
                if (line.toLowerCase().startsWith("content-length:")) {
                    contentLength = Integer.parseInt(line.substring(15).trim());
                }
            }

            // Read body if present
            String requestBody = "";
            if (contentLength > 0) {
                char[] bodyChars = new char[contentLength];
                reader.read(bodyChars, 0, contentLength);
                requestBody = new String(bodyChars);
            }

            // Find handler
            Response httpResp = new Response(404, "Not Found", "Not Found");
            for (var entry : handlers.entrySet()) {
                String pattern = entry.getKey();
                if (path.equals(pattern) || path.startsWith(pattern.replaceAll("/$", "") + "/") || pattern.equals("/")) {
                    var httpReq = new Request(method, path, requestBody);
                    httpResp = entry.getValue().apply(httpReq);
                    break;
                }
            }

            // Send response
            writer.print("HTTP/1.1 " + httpResp.StatusCode + " " + httpResp.Status + "\r\n");
            writer.print("Content-Length: " + httpResp.Body.length() + "\r\n");
            writer.print("Content-Type: text/plain\r\n");
            writer.print("\r\n");
            writer.print(httpResp.Body);
            writer.flush();

        } catch (Exception e) {
            // Ignore client errors
        }
    }
}
