const fs = {
    // Seek whence constants
    SeekStart: 0,
    SeekCurrent: 1,
    SeekEnd: 2,

    // File handle management
    _handles: {},
    _nextHandle: 1,
    _positions: {},

    ReadFile: function(path) {
        try {
            var nodeFs = require('fs');
            var content = nodeFs.readFileSync(path, 'utf8');
            return [content, ""];
        } catch(e) {
            return ["", e.message];
        }
    },

    WriteFile: function(path, content) {
        try {
            var nodeFs = require('fs');
            nodeFs.writeFileSync(path, content, 'utf8');
            return "";
        } catch(e) {
            return e.message;
        }
    },

    Exists: function(path) {
        try {
            var nodeFs = require('fs');
            return nodeFs.existsSync(path);
        } catch(e) {
            return false;
        }
    },

    Remove: function(path) {
        try {
            var nodeFs = require('fs');
            nodeFs.unlinkSync(path);
            return "";
        } catch(e) {
            return e.message;
        }
    },

    Mkdir: function(path) {
        try {
            var nodeFs = require('fs');
            nodeFs.mkdirSync(path);
            return "";
        } catch(e) {
            if (e.code === 'EEXIST') {
                return "";
            }
            return e.message;
        }
    },

    MkdirAll: function(path) {
        try {
            var nodeFs = require('fs');
            nodeFs.mkdirSync(path, { recursive: true });
            return "";
        } catch(e) {
            return e.message;
        }
    },

    RemoveAll: function(path) {
        try {
            var nodeFs = require('fs');
            if (nodeFs.existsSync(path)) {
                nodeFs.rmSync(path, { recursive: true, force: true });
            }
            return "";
        } catch(e) {
            return e.message;
        }
    },

    Open: function(path) {
        try {
            var nodeFs = require('fs');
            var fd = nodeFs.openSync(path, 'r');
            var handle = fs._nextHandle++;
            fs._handles[handle] = fd;
            fs._positions[handle] = 0;
            return [handle, ""];
        } catch(e) {
            return [-1, e.message];
        }
    },

    Create: function(path) {
        try {
            var nodeFs = require('fs');
            var fd = nodeFs.openSync(path, 'w+');
            var handle = fs._nextHandle++;
            fs._handles[handle] = fd;
            fs._positions[handle] = 0;
            return [handle, ""];
        } catch(e) {
            return [-1, e.message];
        }
    },

    Close: function(handle) {
        try {
            var nodeFs = require('fs');
            var fd = fs._handles[handle];
            if (fd === undefined) {
                return "invalid file handle";
            }
            nodeFs.closeSync(fd);
            delete fs._handles[handle];
            delete fs._positions[handle];
            return "";
        } catch(e) {
            return e.message;
        }
    },

    Read: function(handle, size) {
        try {
            var nodeFs = require('fs');
            var fd = fs._handles[handle];
            if (fd === undefined) {
                return [[], 0, "invalid file handle"];
            }
            var buffer = Buffer.alloc(size);
            var bytesRead = nodeFs.readSync(fd, buffer, 0, size, fs._positions[handle]);
            fs._positions[handle] += bytesRead;
            // Convert Buffer to array of bytes
            var data = Array.from(buffer.slice(0, bytesRead));
            return [data, bytesRead, ""];
        } catch(e) {
            return [[], 0, e.message];
        }
    },

    ReadAt: function(handle, offset, size) {
        try {
            var nodeFs = require('fs');
            var fd = fs._handles[handle];
            if (fd === undefined) {
                return [[], 0, "invalid file handle"];
            }
            var buffer = Buffer.alloc(size);
            var bytesRead = nodeFs.readSync(fd, buffer, 0, size, offset);
            // Don't update position for ReadAt
            var data = Array.from(buffer.slice(0, bytesRead));
            return [data, bytesRead, ""];
        } catch(e) {
            return [[], 0, e.message];
        }
    },

    Write: function(handle, data) {
        try {
            var nodeFs = require('fs');
            var fd = fs._handles[handle];
            if (fd === undefined) {
                return [0, "invalid file handle"];
            }
            var buffer = Buffer.from(data);
            var bytesWritten = nodeFs.writeSync(fd, buffer, 0, buffer.length, fs._positions[handle]);
            fs._positions[handle] += bytesWritten;
            return [bytesWritten, ""];
        } catch(e) {
            return [0, e.message];
        }
    },

    Seek: function(handle, offset, whence) {
        try {
            var nodeFs = require('fs');
            var fd = fs._handles[handle];
            if (fd === undefined) {
                return [0, "invalid file handle"];
            }
            var newPos;
            switch (whence) {
                case fs.SeekStart:
                    newPos = offset;
                    break;
                case fs.SeekCurrent:
                    newPos = fs._positions[handle] + offset;
                    break;
                case fs.SeekEnd:
                    var stats = nodeFs.fstatSync(fd);
                    newPos = stats.size + offset;
                    break;
                default:
                    return [0, "invalid whence value"];
            }
            fs._positions[handle] = newPos;
            return [newPos, ""];
        } catch(e) {
            return [0, e.message];
        }
    },

    Size: function(handle) {
        try {
            var nodeFs = require('fs');
            var fd = fs._handles[handle];
            if (fd === undefined) {
                return [0, "invalid file handle"];
            }
            var stats = nodeFs.fstatSync(fd);
            return [stats.size, ""];
        } catch(e) {
            return [0, e.message];
        }
    }
};
