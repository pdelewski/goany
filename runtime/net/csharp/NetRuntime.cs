using System;
using System.Collections.Generic;
using System.Net;
using System.Net.Sockets;

public static class net
{
    // Connection and listener handle management
    private static Dictionary<int, Socket> connections = new Dictionary<int, Socket>();
    private static Dictionary<int, TcpListener> listeners = new Dictionary<int, TcpListener>();
    private static int nextConnHandle = 1;
    private static int nextListenHandle = 1;
    private static object connLock = new object();
    private static object listenLock = new object();

    // Dial connects to the address on the named network.
    public static (int, string) Dial(string network, string address)
    {
        try
        {
            if (network != "tcp" && network != "tcp4" && network != "tcp6")
            {
                return (-1, "unsupported network: " + network);
            }

            var parts = address.Split(':');
            if (parts.Length != 2)
            {
                return (-1, "invalid address format");
            }

            string host = parts[0];
            if (!int.TryParse(parts[1], out int port))
            {
                return (-1, "invalid port");
            }

            var socket = new Socket(AddressFamily.InterNetwork, SocketType.Stream, ProtocolType.Tcp);

            // Resolve hostname
            IPAddress[] addresses = Dns.GetHostAddresses(host);
            if (addresses.Length == 0)
            {
                return (-1, "failed to resolve hostname");
            }

            socket.Connect(new IPEndPoint(addresses[0], port));

            lock (connLock)
            {
                int handle = nextConnHandle++;
                connections[handle] = socket;
                return (handle, "");
            }
        }
        catch (Exception e)
        {
            return (-1, e.Message);
        }
    }

    // Listen announces on the local network address.
    public static (int, string) Listen(string network, string address)
    {
        try
        {
            if (network != "tcp" && network != "tcp4" && network != "tcp6")
            {
                return (-1, "unsupported network: " + network);
            }

            var parts = address.Split(':');
            if (parts.Length != 2)
            {
                return (-1, "invalid address format");
            }

            string host = parts[0];
            if (!int.TryParse(parts[1], out int port))
            {
                return (-1, "invalid port");
            }

            IPAddress bindAddr;
            if (string.IsNullOrEmpty(host) || host == "0.0.0.0")
            {
                bindAddr = IPAddress.Any;
            }
            else if (host == "localhost" || host == "127.0.0.1")
            {
                bindAddr = IPAddress.Loopback;
            }
            else
            {
                bindAddr = IPAddress.Parse(host);
            }

            var listener = new TcpListener(bindAddr, port);
            listener.Start();

            lock (listenLock)
            {
                int handle = nextListenHandle++;
                listeners[handle] = listener;
                return (handle, "");
            }
        }
        catch (Exception e)
        {
            return (-1, e.Message);
        }
    }

    // Accept waits for and returns the next connection to the listener.
    public static (int, string) Accept(int listener)
    {
        try
        {
            TcpListener tcpListener;
            lock (listenLock)
            {
                if (!listeners.TryGetValue(listener, out tcpListener))
                {
                    return (-1, "invalid listener handle");
                }
            }

            Socket clientSocket = tcpListener.AcceptSocket();

            lock (connLock)
            {
                int handle = nextConnHandle++;
                connections[handle] = clientSocket;
                return (handle, "");
            }
        }
        catch (Exception e)
        {
            return (-1, e.Message);
        }
    }

    // Read reads up to size bytes from the connection.
    public static (byte[], int, string) Read(int conn, int size)
    {
        try
        {
            Socket socket;
            lock (connLock)
            {
                if (!connections.TryGetValue(conn, out socket))
                {
                    return (new byte[0], 0, "invalid connection handle");
                }
            }

            byte[] buffer = new byte[size];
            int bytesRead = socket.Receive(buffer);

            if (bytesRead < size)
            {
                byte[] result = new byte[bytesRead];
                Array.Copy(buffer, result, bytesRead);
                return (result, bytesRead, "");
            }

            return (buffer, bytesRead, "");
        }
        catch (Exception e)
        {
            return (new byte[0], 0, e.Message);
        }
    }

    // Write writes data to the connection (byte array overload).
    public static (int, string) Write(int conn, byte[] data)
    {
        try
        {
            Socket socket;
            lock (connLock)
            {
                if (!connections.TryGetValue(conn, out socket))
                {
                    return (0, "invalid connection handle");
                }
            }

            int bytesSent = socket.Send(data);
            return (bytesSent, "");
        }
        catch (Exception e)
        {
            return (0, e.Message);
        }
    }

    // Write writes data to the connection (List<byte> overload for transpiled code).
    public static (int, string) Write(int conn, List<byte> data)
    {
        return Write(conn, data.ToArray());
    }

    // Close closes the connection.
    public static string Close(int conn)
    {
        try
        {
            lock (connLock)
            {
                if (!connections.TryGetValue(conn, out Socket socket))
                {
                    return "invalid connection handle";
                }

                socket.Shutdown(SocketShutdown.Both);
                socket.Close();
                connections.Remove(conn);
                return "";
            }
        }
        catch (Exception e)
        {
            return e.Message;
        }
    }

    // CloseListener closes the listener.
    public static string CloseListener(int listener)
    {
        try
        {
            lock (listenLock)
            {
                if (!listeners.TryGetValue(listener, out TcpListener tcpListener))
                {
                    return "invalid listener handle";
                }

                tcpListener.Stop();
                listeners.Remove(listener);
                return "";
            }
        }
        catch (Exception e)
        {
            return e.Message;
        }
    }

    // SetReadTimeout sets read timeout in milliseconds for a connection.
    public static string SetReadTimeout(int conn, int timeoutMs)
    {
        try
        {
            lock (connLock)
            {
                if (!connections.TryGetValue(conn, out Socket socket))
                {
                    return "invalid connection handle";
                }

                socket.ReceiveTimeout = timeoutMs;
                return "";
            }
        }
        catch (Exception e)
        {
            return e.Message;
        }
    }
}
