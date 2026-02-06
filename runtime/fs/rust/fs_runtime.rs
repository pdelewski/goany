use std::fs as stdfs;
use std::path::Path;

// ReadFile reads the entire file and returns its content as a string.
pub fn ReadFile(path: String) -> (String, String) {
    match stdfs::read_to_string(&path) {
        Ok(content) => (content, String::new()),
        Err(e) => (String::new(), e.to_string()),
    }
}

// WriteFile writes content to a file.
pub fn WriteFile(path: String, content: String) -> String {
    match stdfs::write(&path, &content) {
        Ok(_) => String::new(),
        Err(e) => e.to_string(),
    }
}

// Exists checks if a file or directory exists.
pub fn Exists(path: String) -> bool {
    Path::new(&path).exists()
}

// Remove deletes a file.
pub fn Remove(path: String) -> String {
    match stdfs::remove_file(&path) {
        Ok(_) => String::new(),
        Err(e) => e.to_string(),
    }
}

// Mkdir creates a directory.
pub fn Mkdir(path: String) -> String {
    match stdfs::create_dir(&path) {
        Ok(_) => String::new(),
        Err(e) => e.to_string(),
    }
}

// MkdirAll creates a directory and all parent directories.
pub fn MkdirAll(path: String) -> String {
    match stdfs::create_dir_all(&path) {
        Ok(_) => String::new(),
        Err(e) => e.to_string(),
    }
}

// RemoveAll removes a file or directory and all its contents.
pub fn RemoveAll(path: String) -> String {
    let p = Path::new(&path);
    if p.is_file() {
        match stdfs::remove_file(&path) {
            Ok(_) => String::new(),
            Err(e) => e.to_string(),
        }
    } else if p.is_dir() {
        match stdfs::remove_dir_all(&path) {
            Ok(_) => String::new(),
            Err(e) => e.to_string(),
        }
    } else {
        String::new() // Path doesn't exist, no error
    }
}
