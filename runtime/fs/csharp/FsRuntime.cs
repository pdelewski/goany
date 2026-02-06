public static class fs {
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
}
