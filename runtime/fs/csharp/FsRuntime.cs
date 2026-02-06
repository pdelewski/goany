public static class fs {
    // Seek whence constants
    public const int SeekStart = 0;
    public const int SeekCurrent = 1;
    public const int SeekEnd = 2;

    // File handle management
    private static readonly System.Collections.Generic.Dictionary<int, System.IO.FileStream> fileHandles =
        new System.Collections.Generic.Dictionary<int, System.IO.FileStream>();
    private static int nextHandle = 1;
    private static readonly object handleLock = new object();

    // ReadFile reads the entire file and returns its content as a string.
    public static (string, string) ReadFile(string path) {
        try {
            string content = System.IO.File.ReadAllText(path);
            return (content, "");
        } catch (System.Exception e) {
            return ("", e.Message);
        }
    }

    // WriteFile writes content to a file.
    public static string WriteFile(string path, string content) {
        try {
            System.IO.File.WriteAllText(path, content);
            return "";
        } catch (System.Exception e) {
            return e.Message;
        }
    }

    // Exists checks if a file or directory exists.
    public static bool Exists(string path) {
        return System.IO.File.Exists(path) || System.IO.Directory.Exists(path);
    }

    // Remove deletes a file.
    public static string Remove(string path) {
        try {
            System.IO.File.Delete(path);
            return "";
        } catch (System.Exception e) {
            return e.Message;
        }
    }

    // Mkdir creates a directory.
    public static string Mkdir(string path) {
        try {
            System.IO.Directory.CreateDirectory(path);
            return "";
        } catch (System.Exception e) {
            return e.Message;
        }
    }

    // MkdirAll creates a directory and all parent directories.
    public static string MkdirAll(string path) {
        try {
            System.IO.Directory.CreateDirectory(path);
            return "";
        } catch (System.Exception e) {
            return e.Message;
        }
    }

    // RemoveAll removes a file or directory and all its contents.
    public static string RemoveAll(string path) {
        try {
            if (System.IO.File.Exists(path)) {
                System.IO.File.Delete(path);
            } else if (System.IO.Directory.Exists(path)) {
                System.IO.Directory.Delete(path, true);
            }
            return "";
        } catch (System.Exception e) {
            return e.Message;
        }
    }

    // Open opens a file for reading and returns a handle.
    public static (int, string) Open(string path) {
        try {
            var stream = new System.IO.FileStream(path, System.IO.FileMode.Open, System.IO.FileAccess.Read);
            lock (handleLock) {
                int handle = nextHandle++;
                fileHandles[handle] = stream;
                return (handle, "");
            }
        } catch (System.Exception e) {
            return (-1, e.Message);
        }
    }

    // Create creates or truncates a file for writing and returns a handle.
    public static (int, string) Create(string path) {
        try {
            var stream = new System.IO.FileStream(path, System.IO.FileMode.Create, System.IO.FileAccess.ReadWrite);
            lock (handleLock) {
                int handle = nextHandle++;
                fileHandles[handle] = stream;
                return (handle, "");
            }
        } catch (System.Exception e) {
            return (-1, e.Message);
        }
    }

    // Close closes a file handle.
    public static string Close(int handle) {
        try {
            lock (handleLock) {
                if (!fileHandles.TryGetValue(handle, out var stream)) {
                    return "invalid file handle";
                }
                stream.Close();
                fileHandles.Remove(handle);
                return "";
            }
        } catch (System.Exception e) {
            return e.Message;
        }
    }

    // Read reads up to size bytes from the file.
    public static (byte[], int, string) Read(int handle, int size) {
        try {
            System.IO.FileStream stream;
            lock (handleLock) {
                if (!fileHandles.TryGetValue(handle, out stream)) {
                    return (new byte[0], 0, "invalid file handle");
                }
            }

            byte[] buffer = new byte[size];
            int bytesRead = stream.Read(buffer, 0, size);

            if (bytesRead < size) {
                byte[] result = new byte[bytesRead];
                System.Array.Copy(buffer, result, bytesRead);
                return (result, bytesRead, "");
            }

            return (buffer, bytesRead, "");
        } catch (System.Exception e) {
            return (new byte[0], 0, e.Message);
        }
    }

    // ReadAt reads up to size bytes from the file at the given offset.
    public static (byte[], int, string) ReadAt(int handle, long offset, int size) {
        try {
            System.IO.FileStream stream;
            lock (handleLock) {
                if (!fileHandles.TryGetValue(handle, out stream)) {
                    return (new byte[0], 0, "invalid file handle");
                }
            }

            // Save current position
            long currentPos = stream.Position;

            // Seek and read
            stream.Seek(offset, System.IO.SeekOrigin.Begin);
            byte[] buffer = new byte[size];
            int bytesRead = stream.Read(buffer, 0, size);

            // Restore position
            stream.Seek(currentPos, System.IO.SeekOrigin.Begin);

            if (bytesRead < size) {
                byte[] result = new byte[bytesRead];
                System.Array.Copy(buffer, result, bytesRead);
                return (result, bytesRead, "");
            }

            return (buffer, bytesRead, "");
        } catch (System.Exception e) {
            return (new byte[0], 0, e.Message);
        }
    }

    // Write writes data to the file.
    public static (int, string) Write(int handle, byte[] data) {
        try {
            System.IO.FileStream stream;
            lock (handleLock) {
                if (!fileHandles.TryGetValue(handle, out stream)) {
                    return (0, "invalid file handle");
                }
            }

            stream.Write(data, 0, data.Length);
            return (data.Length, "");
        } catch (System.Exception e) {
            return (0, e.Message);
        }
    }

    // Seek sets the file position for the next read/write.
    public static (long, string) Seek(int handle, long offset, int whence) {
        try {
            System.IO.FileStream stream;
            lock (handleLock) {
                if (!fileHandles.TryGetValue(handle, out stream)) {
                    return (0, "invalid file handle");
                }
            }

            System.IO.SeekOrigin origin;
            switch (whence) {
                case SeekStart:   origin = System.IO.SeekOrigin.Begin; break;
                case SeekCurrent: origin = System.IO.SeekOrigin.Current; break;
                case SeekEnd:     origin = System.IO.SeekOrigin.End; break;
                default:          return (0, "invalid whence value");
            }

            long newPos = stream.Seek(offset, origin);
            return (newPos, "");
        } catch (System.Exception e) {
            return (0, e.Message);
        }
    }

    // Size returns the size of the file in bytes.
    public static (long, string) Size(int handle) {
        try {
            System.IO.FileStream stream;
            lock (handleLock) {
                if (!fileHandles.TryGetValue(handle, out stream)) {
                    return (0, "invalid file handle");
                }
            }

            return (stream.Length, "");
        } catch (System.Exception e) {
            return (0, e.Message);
        }
    }
}
