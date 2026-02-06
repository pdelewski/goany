use std::collections::HashMap;
use std::net::{TcpListener, TcpStream, ToSocketAddrs};
use std::sync::Mutex;
use std::time::Duration;

// Connection and listener handle management
lazy_static::lazy_static! {
    static ref CONNECTIONS: Mutex<HashMap<i32, TcpStream>> = Mutex::new(HashMap::new());
    static ref LISTENERS: Mutex<HashMap<i32, TcpListener>> = Mutex::new(HashMap::new());
    static ref NEXT_CONN_HANDLE: Mutex<i32> = Mutex::new(1);
    static ref NEXT_LISTEN_HANDLE: Mutex<i32> = Mutex::new(1);
}

// Dial connects to the address on the named network.
pub fn Dial(network: String, address: String) -> (i32, String) {
    if network != "tcp" && network != "tcp4" && network != "tcp6" {
        return (-1, format!("unsupported network: {}", network));
    }

    // Resolve address
    let addrs: Vec<_> = match address.to_socket_addrs() {
        Ok(addrs) => addrs.collect(),
        Err(e) => return (-1, e.to_string()),
    };

    if addrs.is_empty() {
        return (-1, "failed to resolve address".to_string());
    }

    match TcpStream::connect(&addrs[..]) {
        Ok(stream) => {
            let mut next = NEXT_CONN_HANDLE.lock().unwrap();
            let handle = *next;
            *next += 1;

            let mut connections = CONNECTIONS.lock().unwrap();
            connections.insert(handle, stream);
            (handle, String::new())
        }
        Err(e) => (-1, e.to_string()),
    }
}

// Listen announces on the local network address.
pub fn Listen(network: String, address: String) -> (i32, String) {
    if network != "tcp" && network != "tcp4" && network != "tcp6" {
        return (-1, format!("unsupported network: {}", network));
    }

    match TcpListener::bind(&address) {
        Ok(listener) => {
            let mut next = NEXT_LISTEN_HANDLE.lock().unwrap();
            let handle = *next;
            *next += 1;

            let mut listeners = LISTENERS.lock().unwrap();
            listeners.insert(handle, listener);
            (handle, String::new())
        }
        Err(e) => (-1, e.to_string()),
    }
}

// Accept waits for and returns the next connection to the listener.
pub fn Accept(listener: i32) -> (i32, String) {
    let listener_ref = {
        let listeners = LISTENERS.lock().unwrap();
        match listeners.get(&listener) {
            Some(l) => {
                match l.try_clone() {
                    Ok(cloned) => cloned,
                    Err(e) => return (-1, e.to_string()),
                }
            }
            None => return (-1, "invalid listener handle".to_string()),
        }
    };

    match listener_ref.accept() {
        Ok((stream, _addr)) => {
            let mut next = NEXT_CONN_HANDLE.lock().unwrap();
            let handle = *next;
            *next += 1;

            let mut connections = CONNECTIONS.lock().unwrap();
            connections.insert(handle, stream);
            (handle, String::new())
        }
        Err(e) => (-1, e.to_string()),
    }
}

// Read reads up to size bytes from the connection.
pub fn Read(conn: i32, size: i32) -> (Vec<u8>, i32, String) {
    use std::io::Read as IoRead;
    let mut connections = CONNECTIONS.lock().unwrap();
    match connections.get_mut(&conn) {
        Some(stream) => {
            let mut buf = vec![0u8; size as usize];
            match stream.read(&mut buf) {
                Ok(n) => {
                    buf.truncate(n);
                    (buf, n as i32, String::new())
                }
                Err(e) => (Vec::new(), 0, e.to_string()),
            }
        }
        None => (Vec::new(), 0, "invalid connection handle".to_string()),
    }
}

// Write writes data to the connection.
pub fn Write(conn: i32, data: Vec<u8>) -> (i32, String) {
    use std::io::Write as IoWrite;
    let mut connections = CONNECTIONS.lock().unwrap();
    match connections.get_mut(&conn) {
        Some(stream) => match stream.write_all(&data) {
            Ok(_) => (data.len() as i32, String::new()),
            Err(e) => (0, e.to_string()),
        },
        None => (0, "invalid connection handle".to_string()),
    }
}

// Close closes the connection.
pub fn Close(conn: i32) -> String {
    let mut connections = CONNECTIONS.lock().unwrap();
    match connections.remove(&conn) {
        Some(stream) => {
            drop(stream);
            String::new()
        }
        None => "invalid connection handle".to_string(),
    }
}

// CloseListener closes the listener.
pub fn CloseListener(listener: i32) -> String {
    let mut listeners = LISTENERS.lock().unwrap();
    match listeners.remove(&listener) {
        Some(l) => {
            drop(l);
            String::new()
        }
        None => "invalid listener handle".to_string(),
    }
}

// SetReadTimeout sets read timeout in milliseconds for a connection.
pub fn SetReadTimeout(conn: i32, timeout_ms: i32) -> String {
    let mut connections = CONNECTIONS.lock().unwrap();
    match connections.get_mut(&conn) {
        Some(stream) => {
            let timeout = if timeout_ms > 0 {
                Some(Duration::from_millis(timeout_ms as u64))
            } else {
                None
            };

            match stream.set_read_timeout(timeout) {
                Ok(_) => String::new(),
                Err(e) => e.to_string(),
            }
        }
        None => "invalid connection handle".to_string(),
    }
}
