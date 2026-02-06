#ifndef FS_RUNTIME_HPP
#define FS_RUNTIME_HPP

#include <string>
#include <tuple>
#include <fstream>
#include <sstream>
#include <filesystem>

namespace fs {

// ReadFile reads the entire file and returns its content as a string.
static std::tuple<std::string, std::string> ReadFile(const std::string& path) {
    std::ifstream file(path);
    if (!file.is_open()) {
        return std::make_tuple(std::string(""), std::string("failed to open file: " + path));
    }
    std::stringstream buffer;
    buffer << file.rdbuf();
    return std::make_tuple(buffer.str(), std::string(""));
}

// WriteFile writes content to a file.
static std::string WriteFile(const std::string& path, const std::string& content) {
    std::ofstream file(path);
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
    if (std::remove(path.c_str()) != 0) {
        return "failed to remove file: " + path;
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

} // namespace fs

#endif // FS_RUNTIME_HPP
