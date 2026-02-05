pub struct Response {
    pub StatusCode: i64,
    pub Status: String,
    pub Body: String,
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
