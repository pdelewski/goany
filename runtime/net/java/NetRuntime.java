import java.net.*;
import java.io.*;
import java.util.*;

class net {
    private static final Map<Integer, Socket> connections = new HashMap<>();
    private static final Map<Integer, ServerSocket> listeners = new HashMap<>();
    private static int nextConnHandle = 1;
    private static int nextListenHandle = 1;
    private static final Object connLock = new Object();
    private static final Object listenLock = new Object();

    // Dial connects to the address on the named network.
    public static Object[] Dial(String network, String address) {
        try {
            if (!network.equals("tcp") && !network.equals("tcp4") && !network.equals("tcp6")) {
                return new Object[]{-1, "unsupported network: " + network};
            }

            String[] parts = address.split(":");
            if (parts.length != 2) {
                return new Object[]{-1, "invalid address format"};
            }

            String host = parts[0];
            int port;
            try {
                port = Integer.parseInt(parts[1]);
            } catch (NumberFormatException e) {
                return new Object[]{-1, "invalid port"};
            }

            Socket socket = new Socket(host, port);

            synchronized (connLock) {
                int handle = nextConnHandle++;
                connections.put(handle, socket);
                return new Object[]{handle, ""};
            }
        } catch (Exception e) {
            return new Object[]{-1, e.getMessage()};
        }
    }

    // Listen announces on the local network address.
    public static Object[] Listen(String network, String address) {
        try {
            if (!network.equals("tcp") && !network.equals("tcp4") && !network.equals("tcp6")) {
                return new Object[]{-1, "unsupported network: " + network};
            }

            String[] parts = address.split(":");
            if (parts.length != 2) {
                return new Object[]{-1, "invalid address format"};
            }

            String host = parts[0];
            int port;
            try {
                port = Integer.parseInt(parts[1]);
            } catch (NumberFormatException e) {
                return new Object[]{-1, "invalid port"};
            }

            InetAddress bindAddr;
            if (host.isEmpty() || host.equals("0.0.0.0")) {
                bindAddr = InetAddress.getByName("0.0.0.0");
            } else if (host.equals("localhost") || host.equals("127.0.0.1")) {
                bindAddr = InetAddress.getLoopbackAddress();
            } else {
                bindAddr = InetAddress.getByName(host);
            }

            ServerSocket serverSocket = new ServerSocket(port, 50, bindAddr);

            synchronized (listenLock) {
                int handle = nextListenHandle++;
                listeners.put(handle, serverSocket);
                return new Object[]{handle, ""};
            }
        } catch (Exception e) {
            return new Object[]{-1, e.getMessage()};
        }
    }

    // Accept waits for and returns the next connection to the listener.
    public static Object[] Accept(int listener) {
        try {
            ServerSocket serverSocket;
            synchronized (listenLock) {
                serverSocket = listeners.get(listener);
                if (serverSocket == null) {
                    return new Object[]{-1, "invalid listener handle"};
                }
            }

            Socket clientSocket = serverSocket.accept();

            synchronized (connLock) {
                int handle = nextConnHandle++;
                connections.put(handle, clientSocket);
                return new Object[]{handle, ""};
            }
        } catch (Exception e) {
            return new Object[]{-1, e.getMessage()};
        }
    }

    // Read reads up to size bytes from the connection.
    public static Object[] Read(int conn, int size) {
        try {
            Socket socket;
            synchronized (connLock) {
                socket = connections.get(conn);
                if (socket == null) {
                    return new Object[]{new ArrayList<Byte>(), 0, "invalid connection handle"};
                }
            }

            byte[] buffer = new byte[size];
            int bytesRead = socket.getInputStream().read(buffer);

            if (bytesRead == -1) {
                return new Object[]{new ArrayList<Byte>(), 0, "EOF"};
            }

            ArrayList<Byte> result = new ArrayList<>();
            for (int i = 0; i < bytesRead; i++) {
                result.add(buffer[i]);
            }

            return new Object[]{result, bytesRead, ""};
        } catch (Exception e) {
            return new Object[]{new ArrayList<Byte>(), 0, e.getMessage()};
        }
    }

    // Write writes data to the connection.
    public static Object[] Write(int conn, ArrayList<Byte> data) {
        try {
            Socket socket;
            synchronized (connLock) {
                socket = connections.get(conn);
                if (socket == null) {
                    return new Object[]{0, "invalid connection handle"};
                }
            }

            byte[] bytes = new byte[data.size()];
            for (int i = 0; i < data.size(); i++) {
                bytes[i] = data.get(i);
            }

            socket.getOutputStream().write(bytes);
            socket.getOutputStream().flush();

            return new Object[]{bytes.length, ""};
        } catch (Exception e) {
            return new Object[]{0, e.getMessage()};
        }
    }

    // Close closes the connection.
    public static String Close(int conn) {
        try {
            synchronized (connLock) {
                Socket socket = connections.get(conn);
                if (socket == null) {
                    return "invalid connection handle";
                }

                socket.close();
                connections.remove(conn);
                return "";
            }
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // CloseListener closes the listener.
    public static String CloseListener(int listener) {
        try {
            synchronized (listenLock) {
                ServerSocket serverSocket = listeners.get(listener);
                if (serverSocket == null) {
                    return "invalid listener handle";
                }

                serverSocket.close();
                listeners.remove(listener);
                return "";
            }
        } catch (Exception e) {
            return e.getMessage();
        }
    }

    // SetReadTimeout sets read timeout in milliseconds for a connection.
    public static String SetReadTimeout(int conn, int timeoutMs) {
        try {
            synchronized (connLock) {
                Socket socket = connections.get(conn);
                if (socket == null) {
                    return "invalid connection handle";
                }

                socket.setSoTimeout(timeoutMs);
                return "";
            }
        } catch (Exception e) {
            return e.getMessage();
        }
    }
}
