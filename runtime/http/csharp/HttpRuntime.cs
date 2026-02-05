public struct http_Response {
    public int StatusCode;
    public string Status;
    public string Body;

    public http_Response(int statusCode, string status, string body) {
        StatusCode = statusCode;
        Status = status;
        Body = body;
    }
}

public static class http {
    private static readonly System.Net.Http.HttpClient client = new System.Net.Http.HttpClient();

    public static (http_Response, string) Get(string url) {
        try {
            var response = client.GetAsync(url).Result;
            var body = response.Content.ReadAsStringAsync().Result;
            return (new http_Response(
                (int)response.StatusCode,
                ((int)response.StatusCode).ToString(),
                body
            ), "");
        } catch (System.Exception e) {
            return (new http_Response(0, "", ""), e.Message);
        }
    }

    public static (http_Response, string) Post(string url, string contentType, string body) {
        try {
            var content = new System.Net.Http.StringContent(body, System.Text.Encoding.UTF8, contentType);
            var response = client.PostAsync(url, content).Result;
            var responseBody = response.Content.ReadAsStringAsync().Result;
            return (new http_Response(
                (int)response.StatusCode,
                ((int)response.StatusCode).ToString(),
                responseBody
            ), "");
        } catch (System.Exception e) {
            return (new http_Response(0, "", ""), e.Message);
        }
    }
}
