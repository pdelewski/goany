use std::sync::Mutex;
use std::collections::HashMap;

#[derive(Clone, Default)]
pub struct Response {
    pub StatusCode: i64,
    pub Status: String,
    pub Body: String,
}

#[derive(Clone, Default)]
pub struct Request {
    pub Method: String,
    pub Path: String,
    pub Body: String,
}

// Handler storage for server
lazy_static::lazy_static! {
    static ref HANDLERS: Mutex<HashMap<String, Box<dyn Fn(Request) -> Response + Send + Sync>>> =
        Mutex::new(HashMap::new());
}

pub fn Get(url: String) -> (Response, String) {
    match ureq::get(&url).call() {
        Ok(resp) => {
            let status = resp.status() as i64;
            let status_text = status.to_string();
            let body = resp.into_string().unwrap_or_default();
            (Response {
                StatusCode: status,
                Status: status_text,
                Body: body,
            }, String::new())
        }
        Err(e) => {
            (Response {
                StatusCode: 0,
                Status: String::new(),
                Body: String::new(),
            }, e.to_string())
        }
    }
}

pub fn Post(url: String, content_type: String, body: String) -> (Response, String) {
    match ureq::post(&url)
        .set("Content-Type", &content_type)
        .send_string(&body)
    {
        Ok(resp) => {
            let status = resp.status() as i64;
            let status_text = status.to_string();
            let resp_body = resp.into_string().unwrap_or_default();
            (Response {
                StatusCode: status,
                Status: status_text,
                Body: resp_body,
            }, String::new())
        }
        Err(e) => {
            (Response {
                StatusCode: 0,
                Status: String::new(),
                Body: String::new(),
            }, e.to_string())
        }
    }
}

// HandleFunc registers a handler function for the given pattern
pub fn HandleFunc<F>(pattern: String, handler: F)
where
    F: Fn(Request) -> Response + Send + Sync + 'static,
{
    let mut handlers = HANDLERS.lock().unwrap();
    handlers.insert(pattern, Box::new(handler));
}

// ListenAndServe starts the HTTP server on the given address
pub fn ListenAndServe(addr: String) -> String {
    // Parse address (e.g., ":8080" or "localhost:8080")
    let bind_addr = if addr.starts_with(':') {
        format!("0.0.0.0{}", addr)
    } else {
        addr
    };

    let server = match tiny_http::Server::http(&bind_addr) {
        Ok(s) => s,
        Err(e) => return e.to_string(),
    };

    for mut request in server.incoming_requests() {
        let method = request.method().to_string();
        let path = request.url().to_string();

        // Read request body
        let mut body = String::new();
        let _ = std::io::Read::read_to_string(request.as_reader(), &mut body);

        // Find matching handler
        let handlers = HANDLERS.lock().unwrap();
        let mut response = Response {
            StatusCode: 404,
            Status: "Not Found".to_string(),
            Body: "Not Found".to_string(),
        };

        for (pattern, handler) in handlers.iter() {
            // Simple pattern matching: exact match or prefix match
            if path == *pattern || path.starts_with(&format!("{}/", pattern.trim_end_matches('/'))) || pattern == "/" {
                let req = Request {
                    Method: method.clone(),
                    Path: path.clone(),
                    Body: body.clone(),
                };
                response = handler(req);
                break;
            }
        }
        drop(handlers);

        // Send response
        let http_response = tiny_http::Response::from_string(response.Body)
            .with_status_code(response.StatusCode as u16);
        let _ = request.respond(http_response);
    }

    String::new()
}
