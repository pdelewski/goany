// GraphicsRuntimeTigr.cs - tigr runtime for goany graphics package
// Requires: tigr.c to be compiled to a native library via pre-build step
//
// The .csproj should include a pre-build target that compiles tigr.c:
// - macOS: cc -shared -o libtigr.dylib tigr.c -framework OpenGL -framework Cocoa
// - Linux: gcc -shared -fPIC -o libtigr.so tigr.c -lGL -lX11
// - Windows: cl /LD tigr.c opengl32.lib gdi32.lib user32.lib /Fe:tigr.dll

using System;
using System.Runtime.InteropServices;

public static class graphics
{
    // GetScreenSize returns the usable screen area via native screen_helper.c
    internal static class ScreenInfo
    {
        private const string LibName = "tigr";

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        private static extern void getScreenSize(out int width, out int height);

        public static (int, int) GetSize()
        {
            int w, h;
            getScreenSize(out w, out h);
            return (w, h);
        }
    }

    // --- P/Invoke declarations for tigr ---
    internal static class Tigr
    {
        private const string LibName = "tigr";

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern IntPtr tigrWindow(int w, int h, string title, int flags);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrFree(IntPtr bmp);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern int tigrClosed(IntPtr bmp);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrUpdate(IntPtr bmp);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrClear(IntPtr bmp, uint color);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrPlot(IntPtr bmp, int x, int y, uint color);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrLine(IntPtr bmp, int x0, int y0, int x1, int y1, uint color);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrRect(IntPtr bmp, int x, int y, int w, int h, uint color);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrFillRect(IntPtr bmp, int x, int y, int w, int h, uint color);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrCircle(IntPtr bmp, int cx, int cy, int r, uint color);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrFillCircle(IntPtr bmp, int cx, int cy, int r, uint color);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern int tigrReadChar(IntPtr bmp);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern int tigrKeyDown(IntPtr bmp, int key);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern int tigrKeyHeld(IntPtr bmp, int key);

        [DllImport(LibName, CallingConvention = CallingConvention.Cdecl)]
        public static extern void tigrMouse(IntPtr bmp, out int x, out int y, out int buttons);

        // tigr key constants (from tigr.h TKey enum starting at TK_PAD0=128)
        public const int TK_BACKSPACE = 156;
        public const int TK_TAB = 157;
        public const int TK_RETURN = 158;
        public const int TK_SHIFT = 159;
        public const int TK_ESCAPE = 164;
        public const int TK_SPACE = 165;
        public const int TK_LEFT = 170;
        public const int TK_UP = 171;
        public const int TK_RIGHT = 172;
        public const int TK_DOWN = 173;

        // Helper to create RGBA color as uint (tigr uses ABGR format internally)
        public static uint RGBA(byte r, byte g, byte b, byte a)
        {
            return (uint)((a << 24) | (b << 16) | (g << 8) | r);
        }
    }

    // --- Public API types ---

    public struct Color
    {
        public byte R;
        public byte G;
        public byte B;
        public byte A;

        public Color(byte r, byte g, byte b, byte a)
        {
            R = r; G = g; B = b; A = a;
        }
    }

    public struct Rect
    {
        public int X;
        public int Y;
        public int Width;
        public int Height;

        public Rect(int x, int y, int width, int height)
        {
            X = x; Y = y; Width = width; Height = height;
        }
    }

    public struct Window
    {
        public long handle;    // Tigr*
        public long renderer;  // Same as handle for tigr
        public int width;
        public int height;
        public bool running;
    }

    // Key state tracking for reliable single-press detection
    private static bool[] prevKeyState = new bool[7];
    private static int lastKeyPressed = 0;

    // --- Helper function ---
    private static uint ColorToUint(Color c)
    {
        return Tigr.RGBA(c.R, c.G, c.B, c.A);
    }

    // --- Color constructors ---

    public static Color NewColor(byte r, byte g, byte b, byte a)
    {
        return new Color(r, g, b, a);
    }

    public static Color Black() { return new Color(0, 0, 0, 255); }
    public static Color White() { return new Color(255, 255, 255, 255); }
    public static Color Red() { return new Color(255, 0, 0, 255); }
    public static Color Green() { return new Color(0, 255, 0, 255); }
    public static Color Blue() { return new Color(0, 0, 255, 255); }

    // --- Rect constructor ---

    public static Rect NewRect(int x, int y, int width, int height)
    {
        return new Rect(x, y, width, height);
    }

    // --- Window management ---

    public static Window CreateWindow(string title, int width, int height)
    {
        IntPtr win = Tigr.tigrWindow(width, height, title, 0);

        if (win == IntPtr.Zero)
        {
            return new Window
            {
                handle = 0,
                renderer = 0,
                width = width,
                height = height,
                running = false
            };
        }

        return new Window
        {
            handle = win.ToInt64(),
            renderer = win.ToInt64(),
            width = width,
            height = height,
            running = true
        };
    }

    public static Window CreateWindowFullscreen(string title, int width, int height)
    {
        // TIGR_FULLSCREEN = 64
        IntPtr win = Tigr.tigrWindow(width, height, title, 64);

        if (win == IntPtr.Zero)
        {
            return new Window
            {
                handle = 0,
                renderer = 0,
                width = width,
                height = height,
                running = false
            };
        }

        return new Window
        {
            handle = win.ToInt64(),
            renderer = win.ToInt64(),
            width = width,
            height = height,
            running = true
        };
    }

    public static void CloseWindow(Window w)
    {
        if (w.handle != 0)
        {
            Tigr.tigrFree(new IntPtr(w.handle));
        }
    }

    public static bool IsRunning(Window w)
    {
        return w.running;
    }

    public static (Window, bool) PollEvents(Window w)
    {
        if (w.handle == 0)
        {
            return (w, false);
        }

        IntPtr win = new IntPtr(w.handle);

        // Check if window should close BEFORE update
        if (Tigr.tigrClosed(win) != 0)
        {
            w.running = false;
            return (w, false);
        }

        // Call tigrUpdate to process events and present previous frame
        Tigr.tigrUpdate(win);

        // Reset last key
        lastKeyPressed = 0;

        // Use tigrReadChar for character input
        // Note: Ignore '\n' (10) as some systems send CRLF for Enter - we use '\r' (13) only
        int ch = Tigr.tigrReadChar(win);
        if (ch > 0 && ch < 128 && ch != 10)
        {
            lastKeyPressed = ch;
        }

        // Also check for DEL (127) which some systems use for backspace
        if (ch == 127)
        {
            lastKeyPressed = 8;  // Normalize to backspace
        }

        // Check special keys using our own state tracking
        // ALWAYS read current state and update prevKeyState to avoid double-detection
        bool[] currKeyState = new bool[7];
        currKeyState[0] = Tigr.tigrKeyHeld(win, Tigr.TK_RETURN) != 0;
        currKeyState[1] = Tigr.tigrKeyHeld(win, Tigr.TK_BACKSPACE) != 0;
        currKeyState[2] = Tigr.tigrKeyHeld(win, Tigr.TK_ESCAPE) != 0;
        currKeyState[3] = Tigr.tigrKeyHeld(win, Tigr.TK_LEFT) != 0;
        currKeyState[4] = Tigr.tigrKeyHeld(win, Tigr.TK_RIGHT) != 0;
        currKeyState[5] = Tigr.tigrKeyHeld(win, Tigr.TK_UP) != 0;
        currKeyState[6] = Tigr.tigrKeyHeld(win, Tigr.TK_DOWN) != 0;

        // Only detect key press if no character was read via tigrReadChar
        if (lastKeyPressed == 0)
        {
            // Detect key press (transition from not pressed to pressed)
            // Note: TK_RETURN (index 0) is NOT detected here - Enter is handled solely via tigrReadChar
            // to avoid double-detection due to timing differences between tigrReadChar and tigrKeyHeld
            if (currKeyState[1] && !prevKeyState[1]) lastKeyPressed = 8;
            else if (currKeyState[2] && !prevKeyState[2]) lastKeyPressed = 27;
            else if (currKeyState[3] && !prevKeyState[3]) lastKeyPressed = 256;
            else if (currKeyState[4] && !prevKeyState[4]) lastKeyPressed = 257;
            else if (currKeyState[5] && !prevKeyState[5]) lastKeyPressed = 258;
            else if (currKeyState[6] && !prevKeyState[6]) lastKeyPressed = 259;
        }

        // ALWAYS update previous state to prevent double-detection
        for (int i = 0; i < 7; i++)
        {
            prevKeyState[i] = currKeyState[i];
        }

        // Check if window was closed during event processing
        if (Tigr.tigrClosed(win) != 0)
        {
            w.running = false;
            return (w, false);
        }

        return (w, true);
    }

    public static int GetLastKey()
    {
        return lastKeyPressed;
    }

    public static (int, int, int) GetMouse(Window w)
    {
        if (w.handle != 0)
        {
            Tigr.tigrMouse(new IntPtr(w.handle), out int x, out int y, out int buttons);
            return (x, y, buttons);
        }
        return (0, 0, 0);
    }

    public static int GetWidth(Window w) { return w.width; }
    public static int GetHeight(Window w) { return w.height; }

    public static (int, int) GetScreenSize()
    {
        return ScreenInfo.GetSize();
    }

    // --- Rendering ---

    public static void Clear(Window w, Color c)
    {
        if (w.handle != 0)
        {
            Tigr.tigrClear(new IntPtr(w.handle), ColorToUint(c));
        }
    }

    public static void Present(Window w)
    {
        // tigrUpdate is called in PollEvents to ensure events are processed before key checks.
        // This function exists for API compatibility.
    }

    // --- Drawing primitives ---

    public static void DrawRect(Window w, Rect rect, Color c)
    {
        if (w.handle != 0)
        {
            Tigr.tigrRect(new IntPtr(w.handle), rect.X, rect.Y, rect.Width, rect.Height, ColorToUint(c));
        }
    }

    public static void FillRect(Window w, Rect rect, Color c)
    {
        if (w.handle != 0)
        {
            Tigr.tigrFillRect(new IntPtr(w.handle), rect.X, rect.Y, rect.Width, rect.Height, ColorToUint(c));
        }
    }

    public static void DrawLine(Window w, int x1, int y1, int x2, int y2, Color c)
    {
        if (w.handle != 0)
        {
            Tigr.tigrLine(new IntPtr(w.handle), x1, y1, x2, y2, ColorToUint(c));
        }
    }

    public static void DrawPoint(Window w, int x, int y, Color c)
    {
        if (w.handle != 0)
        {
            Tigr.tigrPlot(new IntPtr(w.handle), x, y, ColorToUint(c));
        }
    }

    public static void DrawCircle(Window w, int centerX, int centerY, int radius, Color c)
    {
        if (w.handle != 0)
        {
            Tigr.tigrCircle(new IntPtr(w.handle), centerX, centerY, radius, ColorToUint(c));
        }
    }

    public static void FillCircle(Window w, int centerX, int centerY, int radius, Color c)
    {
        if (w.handle != 0)
        {
            Tigr.tigrFillCircle(new IntPtr(w.handle), centerX, centerY, radius, ColorToUint(c));
        }
    }

    // RunLoop runs the main loop, calling frameFunc each frame.
    // frameFunc receives the window and returns true to continue, false to stop.
    // This is the preferred way to write cross-platform graphics code that works in browsers.
    public static void RunLoop(Window w, Func<Window, bool> frameFunc)
    {
        while (true)
        {
            bool running;
            (w, running) = PollEvents(w);
            if (!running)
            {
                break;
            }
            if (!frameFunc(w))
            {
                break;
            }
        }
    }

    // RunLoopWithState runs the main loop with explicit state threading.
    // The state is passed to frameFunc and returned each frame, avoiding closure capture.
    // frameFunc receives the window and state, returns updated state and true to continue.
    public static void RunLoopWithState<S>(Window w, S state, Func<Window, S, (S, bool)> frameFunc)
    {
        while (true)
        {
            bool running;
            (w, running) = PollEvents(w);
            if (!running)
            {
                break;
            }
            bool cont;
            (state, cont) = frameFunc(w, state);
            if (!cont)
            {
                break;
            }
        }
    }
}
