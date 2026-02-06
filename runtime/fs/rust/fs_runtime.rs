use std::fs as stdfs;
use std::path::Path;
use std::io::{Read, Write, Seek, SeekFrom};
use std::collections::HashMap;
use std::sync::Mutex;

// Seek whence constants
pub const SeekStart: i32 = 0;
pub const SeekCurrent: i32 = 1;
pub const SeekEnd: i32 = 2;

// File handle management
lazy_static::lazy_static! {
    static ref FILE_HANDLES: Mutex<HashMap<i32, stdfs::File>> = Mutex::new(HashMap::new());
    static ref NEXT_HANDLE: Mutex<i32> = Mutex::new(1);
}

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

// Open opens a file for reading and returns a handle.
pub fn Open(path: String) -> (i32, String) {
    match stdfs::File::open(&path) {
        Ok(file) => {
            let mut next = NEXT_HANDLE.lock().unwrap();
            let handle = *next;
            *next += 1;

            let mut handles = FILE_HANDLES.lock().unwrap();
            handles.insert(handle, file);
            (handle, String::new())
        }
        Err(e) => (-1, e.to_string()),
    }
}

// Create creates or truncates a file for writing and returns a handle.
pub fn Create(path: String) -> (i32, String) {
    match stdfs::File::create(&path) {
        Ok(file) => {
            let mut next = NEXT_HANDLE.lock().unwrap();
            let handle = *next;
            *next += 1;

            let mut handles = FILE_HANDLES.lock().unwrap();
            handles.insert(handle, file);
            (handle, String::new())
        }
        Err(e) => (-1, e.to_string()),
    }
}

// Close closes a file handle.
pub fn Close(handle: i32) -> String {
    let mut handles = FILE_HANDLES.lock().unwrap();
    match handles.remove(&handle) {
        Some(_) => String::new(), // File is closed when dropped
        None => "invalid file handle".to_string(),
    }
}

// Read reads up to size bytes from the file.
pub fn Read(handle: i32, size: i32) -> (Vec<u8>, i32, String) {
    let mut handles = FILE_HANDLES.lock().unwrap();
    match handles.get_mut(&handle) {
        Some(file) => {
            let mut buf = vec![0u8; size as usize];
            match file.read(&mut buf) {
                Ok(n) => {
                    buf.truncate(n);
                    (buf, n as i32, String::new())
                }
                Err(e) => (Vec::new(), 0, e.to_string()),
            }
        }
        None => (Vec::new(), 0, "invalid file handle".to_string()),
    }
}

// ReadAt reads up to size bytes from the file at the given offset.
pub fn ReadAt(handle: i32, offset: i64, size: i32) -> (Vec<u8>, i32, String) {
    let mut handles = FILE_HANDLES.lock().unwrap();
    match handles.get_mut(&handle) {
        Some(file) => {
            // Save current position
            let current_pos = match file.stream_position() {
                Ok(pos) => pos,
                Err(e) => return (Vec::new(), 0, e.to_string()),
            };

            // Seek to offset
            if let Err(e) = file.seek(SeekFrom::Start(offset as u64)) {
                return (Vec::new(), 0, e.to_string());
            }

            // Read data
            let mut buf = vec![0u8; size as usize];
            let result = match file.read(&mut buf) {
                Ok(n) => {
                    buf.truncate(n);
                    (buf, n as i32, String::new())
                }
                Err(e) => (Vec::new(), 0, e.to_string()),
            };

            // Restore position
            let _ = file.seek(SeekFrom::Start(current_pos));

            result
        }
        None => (Vec::new(), 0, "invalid file handle".to_string()),
    }
}

// Write writes data to the file.
pub fn Write(handle: i32, data: Vec<u8>) -> (i32, String) {
    let mut handles = FILE_HANDLES.lock().unwrap();
    match handles.get_mut(&handle) {
        Some(file) => {
            match file.write_all(&data) {
                Ok(_) => (data.len() as i32, String::new()),
                Err(e) => (0, e.to_string()),
            }
        }
        None => (0, "invalid file handle".to_string()),
    }
}

// Seek sets the file position for the next read/write.
pub fn Seek(handle: i32, offset: i64, whence: i32) -> (i64, String) {
    let mut handles = FILE_HANDLES.lock().unwrap();
    match handles.get_mut(&handle) {
        Some(file) => {
            let seek_from = match whence {
                0 => SeekFrom::Start(offset as u64),  // SeekStart
                1 => SeekFrom::Current(offset),        // SeekCurrent
                2 => SeekFrom::End(offset),            // SeekEnd
                _ => return (0, "invalid whence value".to_string()),
            };

            match file.seek(seek_from) {
                Ok(pos) => (pos as i64, String::new()),
                Err(e) => (0, e.to_string()),
            }
        }
        None => (0, "invalid file handle".to_string()),
    }
}

// Size returns the size of the file in bytes.
pub fn Size(handle: i32) -> (i64, String) {
    let handles = FILE_HANDLES.lock().unwrap();
    match handles.get(&handle) {
        Some(file) => {
            match file.metadata() {
                Ok(metadata) => (metadata.len() as i64, String::new()),
                Err(e) => (0, e.to_string()),
            }
        }
        None => (0, "invalid file handle".to_string()),
    }
}
