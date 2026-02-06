#ifndef FS_RUNTIME_HPP
#define FS_RUNTIME_HPP

#include <string>
#include <tuple>
#include <vector>
#include <fstream>
#include <sstream>
#include <filesystem>
#include <map>
#include <mutex>

namespace fs {

// Seek whence constants
constexpr int SeekStart = 0;
constexpr int SeekCurrent = 1;
constexpr int SeekEnd = 2;

// File handle management
static std::map<int, std::fstream*> fileHandles;
static int nextHandle = 1;
static std::mutex handlesMutex;

// ReadFile reads the entire file and returns its content as a string.
static std::tuple<std::string, std::string> ReadFile(const std::string& path) {
    std::ifstream file(path, std::ios::binary);
    if (!file.is_open()) {
        return std::make_tuple(std::string(""), std::string("failed to open file: " + path));
    }
    std::stringstream buffer;
    buffer << file.rdbuf();
    return std::make_tuple(buffer.str(), std::string(""));
}

// WriteFile writes content to a file.
static std::string WriteFile(const std::string& path, const std::string& content) {
    std::ofstream file(path, std::ios::binary);
    if (!file.is_open()) {
        return "failed to create file: " + path;
    }
    file << content;
    if (file.fail()) {
        return "failed to write to file: " + path;
    }
    return "";
}

// Exists checks if a file or directory exists.
static bool Exists(const std::string& path) {
    return std::filesystem::exists(path);
}

// Remove deletes a file.
static std::string Remove(const std::string& path) {
    std::error_code ec;
    if (!std::filesystem::remove(path, ec)) {
        if (ec) {
            return "failed to remove file: " + path + " (" + ec.message() + ")";
        }
    }
    return "";
}

// Mkdir creates a directory.
static std::string Mkdir(const std::string& path) {
    std::error_code ec;
    if (!std::filesystem::create_directory(path, ec)) {
        if (ec) {
            return "failed to create directory: " + path + " (" + ec.message() + ")";
        }
    }
    return "";
}

// MkdirAll creates a directory and all parent directories.
static std::string MkdirAll(const std::string& path) {
    std::error_code ec;
    if (!std::filesystem::create_directories(path, ec)) {
        if (ec) {
            return "failed to create directories: " + path + " (" + ec.message() + ")";
        }
    }
    return "";
}

// RemoveAll removes a file or directory and all its contents.
static std::string RemoveAll(const std::string& path) {
    std::error_code ec;
    std::filesystem::remove_all(path, ec);
    if (ec) {
        return "failed to remove: " + path + " (" + ec.message() + ")";
    }
    return "";
}

// Open opens a file for reading and returns a handle.
static std::tuple<int, std::string> Open(const std::string& path) {
    auto* f = new std::fstream(path, std::ios::in | std::ios::binary);
    if (!f->is_open()) {
        delete f;
        return std::make_tuple(-1, std::string("failed to open file: " + path));
    }

    std::lock_guard<std::mutex> lock(handlesMutex);
    int handle = nextHandle++;
    fileHandles[handle] = f;
    return std::make_tuple(handle, std::string(""));
}

// Create creates or truncates a file for writing and returns a handle.
static std::tuple<int, std::string> Create(const std::string& path) {
    auto* f = new std::fstream(path, std::ios::out | std::ios::binary | std::ios::trunc);
    if (!f->is_open()) {
        delete f;
        return std::make_tuple(-1, std::string("failed to create file: " + path));
    }

    std::lock_guard<std::mutex> lock(handlesMutex);
    int handle = nextHandle++;
    fileHandles[handle] = f;
    return std::make_tuple(handle, std::string(""));
}

// Close closes a file handle.
static std::string Close(int handle) {
    std::lock_guard<std::mutex> lock(handlesMutex);
    auto it = fileHandles.find(handle);
    if (it == fileHandles.end()) {
        return "invalid file handle";
    }

    it->second->close();
    delete it->second;
    fileHandles.erase(it);
    return "";
}

// Read reads up to size bytes from the file.
static std::tuple<std::vector<uint8_t>, int, std::string> Read(int handle, int size) {
    std::lock_guard<std::mutex> lock(handlesMutex);
    auto it = fileHandles.find(handle);
    if (it == fileHandles.end()) {
        return std::make_tuple(std::vector<uint8_t>(), 0, std::string("invalid file handle"));
    }

    std::vector<uint8_t> buf(size);
    it->second->read(reinterpret_cast<char*>(buf.data()), size);
    int bytesRead = static_cast<int>(it->second->gcount());
    buf.resize(bytesRead);

    if (it->second->bad()) {
        return std::make_tuple(std::vector<uint8_t>(), 0, std::string("read error"));
    }

    return std::make_tuple(buf, bytesRead, std::string(""));
}

// ReadAt reads up to size bytes from the file at the given offset.
static std::tuple<std::vector<uint8_t>, int, std::string> ReadAt(int handle, int64_t offset, int size) {
    std::lock_guard<std::mutex> lock(handlesMutex);
    auto it = fileHandles.find(handle);
    if (it == fileHandles.end()) {
        return std::make_tuple(std::vector<uint8_t>(), 0, std::string("invalid file handle"));
    }

    // Save current position
    auto currentPos = it->second->tellg();

    // Seek to offset
    it->second->seekg(offset, std::ios::beg);
    if (it->second->fail()) {
        it->second->clear();
        it->second->seekg(currentPos);
        return std::make_tuple(std::vector<uint8_t>(), 0, std::string("seek error"));
    }

    // Read data
    std::vector<uint8_t> buf(size);
    it->second->read(reinterpret_cast<char*>(buf.data()), size);
    int bytesRead = static_cast<int>(it->second->gcount());
    buf.resize(bytesRead);

    // Restore position
    it->second->clear(); // Clear EOF flag if set
    it->second->seekg(currentPos);

    return std::make_tuple(buf, bytesRead, std::string(""));
}

// Write writes data to the file.
static std::tuple<int, std::string> Write(int handle, const std::vector<uint8_t>& data) {
    std::lock_guard<std::mutex> lock(handlesMutex);
    auto it = fileHandles.find(handle);
    if (it == fileHandles.end()) {
        return std::make_tuple(0, std::string("invalid file handle"));
    }

    it->second->write(reinterpret_cast<const char*>(data.data()), data.size());
    if (it->second->fail()) {
        return std::make_tuple(0, std::string("write error"));
    }

    return std::make_tuple(static_cast<int>(data.size()), std::string(""));
}

// Seek sets the file position for the next read/write.
static std::tuple<int64_t, std::string> Seek(int handle, int64_t offset, int whence) {
    std::lock_guard<std::mutex> lock(handlesMutex);
    auto it = fileHandles.find(handle);
    if (it == fileHandles.end()) {
        return std::make_tuple(0LL, std::string("invalid file handle"));
    }

    std::ios_base::seekdir dir;
    switch (whence) {
        case SeekStart:   dir = std::ios::beg; break;
        case SeekCurrent: dir = std::ios::cur; break;
        case SeekEnd:     dir = std::ios::end; break;
        default:          return std::make_tuple(0LL, std::string("invalid whence value"));
    }

    it->second->seekg(offset, dir);
    it->second->seekp(offset, dir);

    if (it->second->fail()) {
        it->second->clear();
        return std::make_tuple(0LL, std::string("seek error"));
    }

    return std::make_tuple(static_cast<int64_t>(it->second->tellg()), std::string(""));
}

// Size returns the size of the file in bytes.
static std::tuple<int64_t, std::string> Size(int handle) {
    std::lock_guard<std::mutex> lock(handlesMutex);
    auto it = fileHandles.find(handle);
    if (it == fileHandles.end()) {
        return std::make_tuple(0LL, std::string("invalid file handle"));
    }

    // Save current position
    auto currentPos = it->second->tellg();

    // Seek to end to get size
    it->second->seekg(0, std::ios::end);
    int64_t size = static_cast<int64_t>(it->second->tellg());

    // Restore position
    it->second->seekg(currentPos);

    return std::make_tuple(size, std::string(""));
}

} // namespace fs

#endif // FS_RUNTIME_HPP
