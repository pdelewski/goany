import java.io.*;
import java.nio.file.*;
import java.util.*;

class fs {
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
}
