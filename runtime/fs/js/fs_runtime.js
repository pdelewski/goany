const fs = {
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
            // Ignore error if directory already exists
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
    }
};
