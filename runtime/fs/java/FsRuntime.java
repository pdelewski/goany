import java.io.*;
import java.nio.file.*;
import java.util.*;

class fs {
    // Seek whence constants
    public static final int SeekStart = 0;
    public static final int SeekCurrent = 1;
    public static final int SeekEnd = 2;

    // File handle management
    private static final Map<Integer, RandomAccessFile> fileHandles = new HashMap<>();
    private static int nextHandle = 1;

    public static class FileInfo {
        public String Name;
        public long Size;
        public boolean IsDir;

        public FileInfo(String name, long size, boolean isDir) {
            this.Name = name;
            this.Size = size;
            this.IsDir = isDir;
        }
    }

    // ReadFile reads the entire file and returns its contents.
    public static Object[] ReadFile(String path) {
        try {
            String content = Files.readString(Path.of(path));
            return new Object[]{content, ""};
        } catch (Exception e) {
            return new Object[]{"", e.getMessage()};
        }
    }

    // WriteFile writes content to a file, creating it if necessary.
    public static String WriteFile(String path, String content) {
        try {
            Files.writeString(Path.of(path), content);
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // WriteFileAppend appends content to a file.
    public static String WriteFileAppend(String path, String content) {
        try {
            Files.writeString(Path.of(path), content, StandardOpenOption.CREATE, StandardOpenOption.APPEND);
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // ReadFileChunk reads a chunk of the file starting at offset.
    public static Object[] ReadFileChunk(String path, long offset, int size) {
        try (RandomAccessFile file = new RandomAccessFile(path, "r")) {
            file.seek(offset);
            byte[] buffer = new byte[size];
            int bytesRead = file.read(buffer);

            if (bytesRead == -1) {
                return new Object[]{"", 0, ""};
            }

            String content = new String(buffer, 0, bytesRead);
            return new Object[]{content, bytesRead, ""};
        } catch (Exception e) {
            return new Object[]{"", 0, e.getMessage()};
        }
    }

    // ReadDir reads the directory and returns a list of entries.
    public static Object[] ReadDir(String path) {
        try {
            File dir = new File(path);
            String[] entries = dir.list();
            if (entries == null) {
                return new Object[]{new ArrayList<String>(), "not a directory or does not exist"};
            }
            ArrayList<String> result = new ArrayList<>(Arrays.asList(entries));
            return new Object[]{result, ""};
        } catch (Exception e) {
            return new Object[]{new ArrayList<String>(), e.getMessage()};
        }
    }

    // Mkdir creates a directory.
    public static String Mkdir(String path) {
        try {
            Files.createDirectories(Path.of(path));
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // MkdirAll creates a directory and all parent directories.
    public static String MkdirAll(String path) {
        try {
            Files.createDirectories(Path.of(path));
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // Remove removes a file or empty directory.
    public static String Remove(String path) {
        try {
            Files.delete(Path.of(path));
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // RemoveAll removes a file or directory and all its contents.
    public static String RemoveAll(String path) {
        try {
            Path pathObj = Path.of(path);
            if (Files.isDirectory(pathObj)) {
                Files.walk(pathObj)
                    .sorted(Comparator.reverseOrder())
                    .forEach(p -> {
                        try {
                            Files.delete(p);
                        } catch (IOException e) {
                            // Ignore
                        }
                    });
            } else {
                Files.deleteIfExists(pathObj);
            }
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // Stat returns information about a file.
    public static Object[] Stat(String path) {
        try {
            File file = new File(path);
            if (!file.exists()) {
                return new Object[]{new FileInfo("", 0, false), "file does not exist"};
            }
            return new Object[]{new FileInfo(file.getName(), file.length(), file.isDirectory()), ""};
        } catch (Exception e) {
            return new Object[]{new FileInfo("", 0, false), e.getMessage()};
        }
    }

    // Exists checks if a file or directory exists.
    public static boolean Exists(String path) {
        return Files.exists(Path.of(path));
    }

    // IsDir checks if a path is a directory.
    public static boolean IsDir(String path) {
        return Files.isDirectory(Path.of(path));
    }

    // Rename renames a file or directory.
    public static String Rename(String oldPath, String newPath) {
        try {
            Files.move(Path.of(oldPath), Path.of(newPath), StandardCopyOption.REPLACE_EXISTING);
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // Copy copies a file.
    public static String Copy(String src, String dst) {
        try {
            Files.copy(Path.of(src), Path.of(dst), StandardCopyOption.REPLACE_EXISTING);
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // Open opens a file for reading and returns a handle.
    public static Object[] Open(String path) {
        try {
            RandomAccessFile file = new RandomAccessFile(path, "r");
            synchronized (fileHandles) {
                int handle = nextHandle++;
                fileHandles.put(handle, file);
                return new Object[]{handle, ""};
            }
        } catch (Exception e) {
            return new Object[]{-1, e.getMessage()};
        }
    }

    // Create creates or truncates a file for writing and returns a handle.
    public static Object[] Create(String path) {
        try {
            RandomAccessFile file = new RandomAccessFile(path, "rw");
            file.setLength(0); // Truncate
            synchronized (fileHandles) {
                int handle = nextHandle++;
                fileHandles.put(handle, file);
                return new Object[]{handle, ""};
            }
        } catch (Exception e) {
            return new Object[]{-1, e.getMessage()};
        }
    }

    // Close closes a file handle.
    public static String Close(int handle) {
        try {
            RandomAccessFile file;
            synchronized (fileHandles) {
                file = fileHandles.remove(handle);
                if (file == null) {
                    return "invalid file handle";
                }
            }
            file.close();
            return "";
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // Read reads up to size bytes from the file.
    // Returns (data, bytesRead, error).
    public static Object[] Read(int handle, int size) {
        try {
            RandomAccessFile file;
            synchronized (fileHandles) {
                file = fileHandles.get(handle);
                if (file == null) {
                    return new Object[]{new ArrayList<Byte>(), 0, "invalid file handle"};
                }
            }

            byte[] buffer = new byte[size];
            int bytesRead = file.read(buffer);

            if (bytesRead == -1) {
                return new Object[]{new ArrayList<Byte>(), 0, ""};
            }

            // Convert to ArrayList<Byte> for compatibility with slice representation
            ArrayList<Byte> result = new ArrayList<>(bytesRead);
            for (int i = 0; i < bytesRead; i++) {
                result.add(buffer[i]);
            }

            return new Object[]{result, bytesRead, ""};
        } catch (Exception e) {
            return new Object[]{new ArrayList<Byte>(), 0, e.getMessage()};
        }
    }

    // ReadAt reads up to size bytes from the file at the given offset.
    // Does not change the file position.
    // Returns (data, bytesRead, error).
    public static Object[] ReadAt(int handle, long offset, int size) {
        try {
            RandomAccessFile file;
            synchronized (fileHandles) {
                file = fileHandles.get(handle);
                if (file == null) {
                    return new Object[]{new ArrayList<Byte>(), 0, "invalid file handle"};
                }
            }

            // Save current position
            long currentPos = file.getFilePointer();

            // Seek and read
            file.seek(offset);
            byte[] buffer = new byte[size];
            int bytesRead = file.read(buffer);

            // Restore position
            file.seek(currentPos);

            if (bytesRead == -1) {
                return new Object[]{new ArrayList<Byte>(), 0, ""};
            }

            // Convert to ArrayList<Byte> for compatibility with slice representation
            ArrayList<Byte> result = new ArrayList<>(bytesRead);
            for (int i = 0; i < bytesRead; i++) {
                result.add(buffer[i]);
            }

            return new Object[]{result, bytesRead, ""};
        } catch (Exception e) {
            return new Object[]{new ArrayList<Byte>(), 0, e.getMessage()};
        }
    }

    // Write writes data to the file.
    // Returns (bytesWritten, error).
    public static Object[] Write(int handle, ArrayList<Byte> data) {
        try {
            RandomAccessFile file;
            synchronized (fileHandles) {
                file = fileHandles.get(handle);
                if (file == null) {
                    return new Object[]{0, "invalid file handle"};
                }
            }

            byte[] buffer = new byte[data.size()];
            for (int i = 0; i < data.size(); i++) {
                buffer[i] = data.get(i);
            }

            file.write(buffer);
            return new Object[]{buffer.length, ""};
        } catch (Exception e) {
            return new Object[]{0, e.getMessage()};
        }
    }

    // Seek sets the file position for the next read/write.
    // whence: SeekStart (0), SeekCurrent (1), or SeekEnd (2).
    // Returns (newOffset, error).
    public static Object[] Seek(int handle, long offset, int whence) {
        try {
            RandomAccessFile file;
            synchronized (fileHandles) {
                file = fileHandles.get(handle);
                if (file == null) {
                    return new Object[]{0L, "invalid file handle"};
                }
            }

            long newPos;
            switch (whence) {
                case SeekStart:
                    file.seek(offset);
                    newPos = offset;
                    break;
                case SeekCurrent:
                    newPos = file.getFilePointer() + offset;
                    file.seek(newPos);
                    break;
                case SeekEnd:
                    newPos = file.length() + offset;
                    file.seek(newPos);
                    break;
                default:
                    return new Object[]{0L, "invalid whence value"};
            }

            return new Object[]{newPos, ""};
        } catch (Exception e) {
            return new Object[]{0L, e.getMessage()};
        }
    }

    // Size returns the size of the file in bytes.
    // Returns (size, error).
    public static Object[] Size(int handle) {
        try {
            RandomAccessFile file;
            synchronized (fileHandles) {
                file = fileHandles.get(handle);
                if (file == null) {
                    return new Object[]{0L, "invalid file handle"};
                }
            }

            return new Object[]{file.length(), ""};
        } catch (Exception e) {
            return new Object[]{0L, e.getMessage()};
        }
    }
}
