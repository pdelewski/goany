const net = {
    // Connection and listener handle management
    _connections: {},
    _listeners: {},
    _nextConnHandle: 1,
    _nextListenHandle: 1,
    _pendingData: {}, // Buffer for received data
    _pendingAccepts: {}, // Queue for pending accept operations

    Dial: function(network, address) {
        try {
            if (network !== "tcp" && network !== "tcp4" && network !== "tcp6") {
                return [-1, "unsupported network: " + network];
            }

            var parts = address.split(':');
            if (parts.length < 2) {
                return [-1, "invalid address format"];
            }

            var port = parseInt(parts[parts.length - 1]);
            var host = parts.slice(0, -1).join(':');
            if (isNaN(port)) {
                return [-1, "invalid port"];
            }

            var nodeNet = require('net');
            var socket = new nodeNet.Socket();
            var handle = net._nextConnHandle++;

            // Store socket immediately
            net._connections[handle] = socket;
            net._pendingData[handle] = [];

            // Buffer incoming data
            socket.on('data', function(data) {
                if (net._pendingData[handle]) {
                    net._pendingData[handle].push(data);
                }
            });

            // Synchronous-like connection using deasync
            var connected = false;
            var error = null;

            socket.connect(port, host, function() {
                connected = true;
            });

            socket.on('error', function(err) {
                error = err;
            });

            // Busy wait for connection (not ideal but necessary for sync API)
            var deasync = require('deasync');
            while (!connected && !error) {
                deasync.runLoopOnce();
            }

            if (error) {
                delete net._connections[handle];
                delete net._pendingData[handle];
                return [-1, error.message];
            }

            return [handle, ""];
        } catch (e) {
            return [-1, e.message];
        }
    },

    Listen: function(network, address) {
        try {
            if (network !== "tcp" && network !== "tcp4" && network !== "tcp6") {
                return [-1, "unsupported network: " + network];
            }

            var parts = address.split(':');
            if (parts.length < 2) {
                return [-1, "invalid address format"];
            }

            var port = parseInt(parts[parts.length - 1]);
            var host = parts.slice(0, -1).join(':');
            if (isNaN(port)) {
                return [-1, "invalid port"];
            }

            if (host === "" || host === "0.0.0.0") {
                host = "0.0.0.0";
            }

            var nodeNet = require('net');
            var server = nodeNet.createServer();
            var handle = net._nextListenHandle++;

            net._listeners[handle] = server;
            net._pendingAccepts[handle] = [];

            // Queue incoming connections
            server.on('connection', function(socket) {
                net._pendingAccepts[handle].push(socket);
            });

            // Synchronous listen
            var listening = false;
            var error = null;

            server.listen(port, host, function() {
                listening = true;
            });

            server.on('error', function(err) {
                error = err;
            });

            var deasync = require('deasync');
            while (!listening && !error) {
                deasync.runLoopOnce();
            }

            if (error) {
                delete net._listeners[handle];
                delete net._pendingAccepts[handle];
                return [-1, error.message];
            }

            return [handle, ""];
        } catch (e) {
            return [-1, e.message];
        }
    },

    Accept: function(listener) {
        try {
            var server = net._listeners[listener];
            if (!server) {
                return [-1, "invalid listener handle"];
            }

            var deasync = require('deasync');

            // Wait for a connection
            while (net._pendingAccepts[listener].length === 0) {
                deasync.runLoopOnce();
            }

            var socket = net._pendingAccepts[listener].shift();
            var handle = net._nextConnHandle++;

            net._connections[handle] = socket;
            net._pendingData[handle] = [];

            // Buffer incoming data
            socket.on('data', function(data) {
                if (net._pendingData[handle]) {
                    net._pendingData[handle].push(data);
                }
            });

            return [handle, ""];
        } catch (e) {
            return [-1, e.message];
        }
    },

    Read: function(conn, size) {
        try {
            var socket = net._connections[conn];
            if (!socket) {
                return [[], 0, "invalid connection handle"];
            }

            var deasync = require('deasync');

            // Wait for data if buffer is empty
            while (net._pendingData[conn].length === 0 && !socket.destroyed) {
                deasync.runLoopOnce();
            }

            if (socket.destroyed && net._pendingData[conn].length === 0) {
                return [[], 0, "connection closed"];
            }

            // Combine buffered data
            var combined = Buffer.concat(net._pendingData[conn]);
            net._pendingData[conn] = [];

            // Return up to size bytes
            var result;
            if (combined.length <= size) {
                result = combined;
            } else {
                result = combined.slice(0, size);
                // Put remaining data back
                net._pendingData[conn].push(combined.slice(size));
            }

            return [Array.from(result), result.length, ""];
        } catch (e) {
            return [[], 0, e.message];
        }
    },

    Write: function(conn, data) {
        try {
            var socket = net._connections[conn];
            if (!socket) {
                return [0, "invalid connection handle"];
            }

            var buffer = Buffer.from(data);
            var written = false;
            var error = null;

            socket.write(buffer, function(err) {
                if (err) {
                    error = err;
                }
                written = true;
            });

            var deasync = require('deasync');
            while (!written && !error) {
                deasync.runLoopOnce();
            }

            if (error) {
                return [0, error.message];
            }

            return [buffer.length, ""];
        } catch (e) {
            return [0, e.message];
        }
    },

    Close: function(conn) {
        try {
            var socket = net._connections[conn];
            if (!socket) {
                return "invalid connection handle";
            }

            socket.destroy();
            delete net._connections[conn];
            delete net._pendingData[conn];
            return "";
        } catch (e) {
            return e.message;
        }
    },

    CloseListener: function(listener) {
        try {
            var server = net._listeners[listener];
            if (!server) {
                return "invalid listener handle";
            }

            server.close();
            delete net._listeners[listener];
            delete net._pendingAccepts[listener];
            return "";
        } catch (e) {
            return e.message;
        }
    },

    SetReadTimeout: function(conn, timeoutMs) {
        try {
            var socket = net._connections[conn];
            if (!socket) {
                return "invalid connection handle";
            }

            socket.setTimeout(timeoutMs);
            return "";
        } catch (e) {
            return e.message;
        }
    }
};
