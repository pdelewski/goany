#ifndef NET_RUNTIME_HPP
#define NET_RUNTIME_HPP

#include <string>
#include <tuple>
#include <vector>
#include <map>
#include <mutex>
#include <cstring>

#ifdef _WIN32
    #include <winsock2.h>
    #include <ws2tcpip.h>
    #pragma comment(lib, "ws2_32.lib")
    typedef SOCKET socket_t;
    #define INVALID_SOCK INVALID_SOCKET
    #define CLOSE_SOCKET closesocket
    #define SOCK_ERROR SOCKET_ERROR
#else
    #include <sys/socket.h>
    #include <netinet/in.h>
    #include <arpa/inet.h>
    #include <netdb.h>
    #include <unistd.h>
    #include <fcntl.h>
    #include <sys/time.h>
    typedef int socket_t;
    #define INVALID_SOCK -1
    #define CLOSE_SOCKET close
    #define SOCK_ERROR -1
#endif

namespace net {

// Connection and listener handle management
static std::map<int, socket_t> connections;
static std::map<int, socket_t> listeners;
static int nextConnHandle = 1;
static int nextListenHandle = 1;
static std::mutex connMutex;
static std::mutex listenMutex;
static bool wsaInitialized = false;

// Initialize Winsock on Windows
static void initNetwork() {
#ifdef _WIN32
    if (!wsaInitialized) {
        WSADATA wsaData;
        if (WSAStartup(MAKEWORD(2, 2), &wsaData) == 0) {
            wsaInitialized = true;
        }
    }
#endif
}

// Parse address string "host:port" into host and port
static bool parseAddress(const std::string& address, std::string& host, int& port) {
    size_t colonPos = address.rfind(':');
    if (colonPos == std::string::npos) {
        return false;
    }
    host = address.substr(0, colonPos);
    try {
        port = std::stoi(address.substr(colonPos + 1));
    } catch (...) {
        return false;
    }
    return true;
}

// Dial connects to the address on the named network.
static std::tuple<int, std::string> Dial(const std::string& network, const std::string& address) {
    initNetwork();

    if (network != "tcp" && network != "tcp4" && network != "tcp6") {
        return std::make_tuple(-1, std::string("unsupported network: " + network));
    }

    std::string host;
    int port;
    if (!parseAddress(address, host, port)) {
        return std::make_tuple(-1, std::string("invalid address format"));
    }

    socket_t sock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (sock == INVALID_SOCK) {
        return std::make_tuple(-1, std::string("failed to create socket"));
    }

    struct sockaddr_in serverAddr;
    std::memset(&serverAddr, 0, sizeof(serverAddr));
    serverAddr.sin_family = AF_INET;
    serverAddr.sin_port = htons(static_cast<uint16_t>(port));

    // Resolve hostname
    struct hostent* he = gethostbyname(host.c_str());
    if (he == nullptr) {
        CLOSE_SOCKET(sock);
        return std::make_tuple(-1, std::string("failed to resolve hostname"));
    }
    std::memcpy(&serverAddr.sin_addr, he->h_addr_list[0], he->h_length);

    if (connect(sock, (struct sockaddr*)&serverAddr, sizeof(serverAddr)) == SOCK_ERROR) {
        CLOSE_SOCKET(sock);
        return std::make_tuple(-1, std::string("failed to connect"));
    }

    std::lock_guard<std::mutex> lock(connMutex);
    int handle = nextConnHandle++;
    connections[handle] = sock;
    return std::make_tuple(handle, std::string(""));
}

// Listen announces on the local network address.
static std::tuple<int, std::string> Listen(const std::string& network, const std::string& address) {
    initNetwork();

    if (network != "tcp" && network != "tcp4" && network != "tcp6") {
        return std::make_tuple(-1, std::string("unsupported network: " + network));
    }

    std::string host;
    int port;
    if (!parseAddress(address, host, port)) {
        return std::make_tuple(-1, std::string("invalid address format"));
    }

    socket_t sock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (sock == INVALID_SOCK) {
        return std::make_tuple(-1, std::string("failed to create socket"));
    }

    // Allow address reuse
    int opt = 1;
#ifdef _WIN32
    setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (const char*)&opt, sizeof(opt));
#else
    setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));
#endif

    struct sockaddr_in serverAddr;
    std::memset(&serverAddr, 0, sizeof(serverAddr));
    serverAddr.sin_family = AF_INET;
    serverAddr.sin_port = htons(static_cast<uint16_t>(port));

    if (host.empty() || host == "0.0.0.0") {
        serverAddr.sin_addr.s_addr = INADDR_ANY;
    } else if (host == "localhost" || host == "127.0.0.1") {
        serverAddr.sin_addr.s_addr = inet_addr("127.0.0.1");
    } else {
        serverAddr.sin_addr.s_addr = inet_addr(host.c_str());
    }

    if (bind(sock, (struct sockaddr*)&serverAddr, sizeof(serverAddr)) == SOCK_ERROR) {
        CLOSE_SOCKET(sock);
        return std::make_tuple(-1, std::string("failed to bind"));
    }

    if (listen(sock, SOMAXCONN) == SOCK_ERROR) {
        CLOSE_SOCKET(sock);
        return std::make_tuple(-1, std::string("failed to listen"));
    }

    std::lock_guard<std::mutex> lock(listenMutex);
    int handle = nextListenHandle++;
    listeners[handle] = sock;
    return std::make_tuple(handle, std::string(""));
}

// Accept waits for and returns the next connection to the listener.
static std::tuple<int, std::string> Accept(int listener) {
    socket_t listenerSock;
    {
        std::lock_guard<std::mutex> lock(listenMutex);
        auto it = listeners.find(listener);
        if (it == listeners.end()) {
            return std::make_tuple(-1, std::string("invalid listener handle"));
        }
        listenerSock = it->second;
    }

    struct sockaddr_in clientAddr;
    socklen_t clientLen = sizeof(clientAddr);
    socket_t clientSock = accept(listenerSock, (struct sockaddr*)&clientAddr, &clientLen);
    if (clientSock == INVALID_SOCK) {
        return std::make_tuple(-1, std::string("accept failed"));
    }

    std::lock_guard<std::mutex> lock(connMutex);
    int handle = nextConnHandle++;
    connections[handle] = clientSock;
    return std::make_tuple(handle, std::string(""));
}

// Read reads up to size bytes from the connection.
static std::tuple<std::vector<uint8_t>, int, std::string> Read(int conn, int size) {
    socket_t sock;
    {
        std::lock_guard<std::mutex> lock(connMutex);
        auto it = connections.find(conn);
        if (it == connections.end()) {
            return std::make_tuple(std::vector<uint8_t>(), 0, std::string("invalid connection handle"));
        }
        sock = it->second;
    }

    std::vector<uint8_t> buf(size);
    int bytesRead = recv(sock, reinterpret_cast<char*>(buf.data()), size, 0);
    if (bytesRead == SOCK_ERROR) {
        return std::make_tuple(std::vector<uint8_t>(), 0, std::string("read error"));
    }

    buf.resize(bytesRead);
    return std::make_tuple(buf, bytesRead, std::string(""));
}

// Write writes data to the connection.
static std::tuple<int, std::string> Write(int conn, const std::vector<uint8_t>& data) {
    socket_t sock;
    {
        std::lock_guard<std::mutex> lock(connMutex);
        auto it = connections.find(conn);
        if (it == connections.end()) {
            return std::make_tuple(0, std::string("invalid connection handle"));
        }
        sock = it->second;
    }

    int bytesSent = send(sock, reinterpret_cast<const char*>(data.data()), static_cast<int>(data.size()), 0);
    if (bytesSent == SOCK_ERROR) {
        return std::make_tuple(0, std::string("write error"));
    }

    return std::make_tuple(bytesSent, std::string(""));
}

// Close closes the connection.
static std::string Close(int conn) {
    std::lock_guard<std::mutex> lock(connMutex);
    auto it = connections.find(conn);
    if (it == connections.end()) {
        return "invalid connection handle";
    }

    CLOSE_SOCKET(it->second);
    connections.erase(it);
    return "";
}

// CloseListener closes the listener.
static std::string CloseListener(int listener) {
    std::lock_guard<std::mutex> lock(listenMutex);
    auto it = listeners.find(listener);
    if (it == listeners.end()) {
        return "invalid listener handle";
    }

    CLOSE_SOCKET(it->second);
    listeners.erase(it);
    return "";
}

// SetReadTimeout sets read timeout in milliseconds for a connection.
static std::string SetReadTimeout(int conn, int timeoutMs) {
    socket_t sock;
    {
        std::lock_guard<std::mutex> lock(connMutex);
        auto it = connections.find(conn);
        if (it == connections.end()) {
            return "invalid connection handle";
        }
        sock = it->second;
    }

#ifdef _WIN32
    DWORD timeout = timeoutMs;
    if (setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&timeout, sizeof(timeout)) == SOCK_ERROR) {
        return "failed to set timeout";
    }
#else
    struct timeval tv;
    tv.tv_sec = timeoutMs / 1000;
    tv.tv_usec = (timeoutMs % 1000) * 1000;
    if (setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv)) < 0) {
        return "failed to set timeout";
    }
#endif

    return "";
}

} // namespace net

#endif // NET_RUNTIME_HPP
