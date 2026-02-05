public static class http {
    public struct Response {
        public int StatusCode;
        public string Status;
        public string Body;

        public Response(int statusCode, string status, string body) {
            StatusCode = statusCode;
            Status = status;
            Body = body;
        }
    }

    public struct Request {
        public string Method;
        public string Path;
        public string Body;

        public Request(string method, string path, string body) {
            Method = method;
            Path = path;
            Body = body;
        }
    }

    private static readonly System.Net.Http.HttpClient client = new System.Net.Http.HttpClient();
    private static readonly System.Collections.Generic.Dictionary<string, System.Func<Request, Response>> handlers =
        new System.Collections.Generic.Dictionary<string, System.Func<Request, Response>>();

    public static (Response, string) Get(string url) {
        try {
            var response = client.GetAsync(url).Result;
            var body = response.Content.ReadAsStringAsync().Result;
            return (new Response(
                (int)response.StatusCode,
                ((int)response.StatusCode).ToString(),
                body
            ), "");
        } catch (System.Exception e) {
            return (new Response(0, "", ""), e.Message);
        }
    }

    public static (Response, string) Post(string url, string contentType, string body) {
        try {
            var content = new System.Net.Http.StringContent(body, System.Text.Encoding.UTF8, contentType);
            var response = client.PostAsync(url, content).Result;
            var responseBody = response.Content.ReadAsStringAsync().Result;
            return (new Response(
                (int)response.StatusCode,
                ((int)response.StatusCode).ToString(),
                responseBody
            ), "");
        } catch (System.Exception e) {
            return (new Response(0, "", ""), e.Message);
        }
    }

    public static void HandleFunc(string pattern, System.Func<Request, Response> handler) {
        handlers[pattern] = handler;
    }

    public static string ListenAndServe(string addr) {
        try {
            // Parse address (e.g., ":8080" or "localhost:8080")
            string host = "+";  // Listen on all interfaces
            string port = "8080";
            int colonIdx = addr.LastIndexOf(':');
            if (colonIdx >= 0) {
                string hostPart = addr.Substring(0, colonIdx);
                port = addr.Substring(colonIdx + 1);
                if (!string.IsNullOrEmpty(hostPart)) {
                    host = hostPart;
                }
            }

            var listener = new System.Net.HttpListener();
            listener.Prefixes.Add($"http://{host}:{port}/");
            listener.Start();

            while (true) {
                var context = listener.GetContext();
                var request = context.Request;
                var response = context.Response;

                // Read request body
                string requestBody = "";
                using (var reader = new System.IO.StreamReader(request.InputStream, request.ContentEncoding)) {
                    requestBody = reader.ReadToEnd();
                }

                // Find matching handler
                string path = request.Url.AbsolutePath;
                Response httpResp = new Response(404, "Not Found", "Not Found");

                foreach (var kvp in handlers) {
                    // Simple pattern matching: exact match or prefix match with /
                    if (path == kvp.Key || path.StartsWith(kvp.Key.TrimEnd('/') + "/") || kvp.Key == "/") {
                        var httpReq = new Request(request.HttpMethod, path, requestBody);
                        httpResp = kvp.Value(httpReq);
                        break;
                    }
                }

                // Write response
                response.StatusCode = httpResp.StatusCode;
                byte[] buffer = System.Text.Encoding.UTF8.GetBytes(httpResp.Body);
                response.ContentLength64 = buffer.Length;
                response.OutputStream.Write(buffer, 0, buffer.Length);
                response.OutputStream.Close();
            }
        } catch (System.Exception e) {
            return e.Message;
        }
    }
}
