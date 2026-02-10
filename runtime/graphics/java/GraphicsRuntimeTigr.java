// GraphicsRuntime.java - tigr runtime for goany graphics package
// Requires: tigr.c + graphics_jni.c compiled to native library
//
// Build native library (run 'make' in the output directory):
// - macOS: cc -shared -o libgraphics_jni.dylib -I"$JAVA_HOME/include" -I"$JAVA_HOME/include/darwin" graphics_jni.c tigr.c screen_helper.c -framework OpenGL -framework Cocoa
// - Linux: gcc -shared -fPIC -o libgraphics_jni.so -I"$JAVA_HOME/include" -I"$JAVA_HOME/include/linux" graphics_jni.c tigr.c screen_helper.c -lGL -lX11
// - Windows: cl /LD graphics_jni.c tigr.c screen_helper.c /I"%JAVA_HOME%\include" /I"%JAVA_HOME%\include\win32" opengl32.lib gdi32.lib user32.lib /Fe:graphics_jni.dll
//
// Run the application:
// - macOS:   java -XstartOnFirstThread --enable-native-access=ALL-UNNAMED -Djava.library.path=. YourMainClass
// - Linux:   java --enable-native-access=ALL-UNNAMED -Djava.library.path=. YourMainClass
// - Windows: java --enable-native-access=ALL-UNNAMED -Djava.library.path=. YourMainClass
//
// IMPORTANT: On macOS, -XstartOnFirstThread is REQUIRED for GUI applications!

import java.util.function.BiFunction;
import java.util.function.Function;

class graphics {
    // Load native library
    static {
        try {
            System.loadLibrary("graphics_jni");
        } catch (UnsatisfiedLinkError e) {
            System.err.println("Failed to load graphics_jni native library: " + e.getMessage());
            System.err.println("Make sure libgraphics_jni.dylib/so/dll is in java.library.path");
        }
    }

    // --- Native method declarations ---
    private static native long nativeCreateWindow(String title, int width, int height, boolean fullscreen);
    private static native void nativeCloseWindow(long handle);
    private static native boolean nativePollEvents(long handle);
    private static native int nativeGetLastKey();
    private static native int[] nativeGetMouse(long handle); // returns [x, y, buttons]
    private static native int[] nativeGetScreenSize(); // returns [width, height]
    private static native void nativeClear(long handle, int r, int g, int b, int a);
    private static native void nativePresent(long handle);
    private static native void nativeDrawRect(long handle, int x, int y, int w, int h, int r, int g, int b, int a);
    private static native void nativeFillRect(long handle, int x, int y, int w, int h, int r, int g, int b, int a);
    private static native void nativeDrawLine(long handle, int x1, int y1, int x2, int y2, int r, int g, int b, int a);
    private static native void nativeDrawPoint(long handle, int x, int y, int r, int g, int b, int a);
    private static native void nativeDrawCircle(long handle, int cx, int cy, int radius, int r, int g, int b, int a);
    private static native void nativeFillCircle(long handle, int cx, int cy, int radius, int r, int g, int b, int a);
    private static native void nativeForceExit();  // macOS: bypass atexit handlers

    // --- Public API types ---

    public static class Color {
        public int R;
        public int G;
        public int B;
        public int A;

        public Color() {
            this.R = 0;
            this.G = 0;
            this.B = 0;
            this.A = 0;
        }

        public Color(int r, int g, int b, int a) {
            this.R = r;
            this.G = g;
            this.B = b;
            this.A = a;
        }

        public Color(Color other) {
            this.R = other.R;
            this.G = other.G;
            this.B = other.B;
            this.A = other.A;
        }
    }

    public static class Rect {
        public int X;
        public int Y;
        public int Width;
        public int Height;

        public Rect() {
            this.X = 0;
            this.Y = 0;
            this.Width = 0;
            this.Height = 0;
        }

        public Rect(int x, int y, int width, int height) {
            this.X = x;
            this.Y = y;
            this.Width = width;
            this.Height = height;
        }

        public Rect(Rect other) {
            this.X = other.X;
            this.Y = other.Y;
            this.Width = other.Width;
            this.Height = other.Height;
        }
    }

    public static class Window {
        public long handle;
        public long renderer;
        public int width;
        public int height;
        public boolean running;

        public Window() {
            this.handle = 0;
            this.renderer = 0;
            this.width = 0;
            this.height = 0;
            this.running = false;
        }

        public Window(long handle, long renderer, int width, int height, boolean running) {
            this.handle = handle;
            this.renderer = renderer;
            this.width = width;
            this.height = height;
            this.running = running;
        }

        public Window(Window other) {
            this.handle = other.handle;
            this.renderer = other.renderer;
            this.width = other.width;
            this.height = other.height;
            this.running = other.running;
        }
    }

    // --- Color constructors ---

    public static Color NewColor(int r, int g, int b, int a) {
        return new Color(r, g, b, a);
    }

    public static Color Black() {
        return new Color(0, 0, 0, 255);
    }

    public static Color White() {
        return new Color(255, 255, 255, 255);
    }

    public static Color Red() {
        return new Color(255, 0, 0, 255);
    }

    public static Color Green() {
        return new Color(0, 255, 0, 255);
    }

    public static Color Blue() {
        return new Color(0, 0, 255, 255);
    }

    // --- Rect constructor ---

    public static Rect NewRect(int x, int y, int width, int height) {
        return new Rect(x, y, width, height);
    }

    // --- Window management ---

    public static Window CreateWindow(String title, int width, int height) {
        long handle = nativeCreateWindow(title, width, height, false);
        if (handle == 0) {
            return new Window(0, 0, width, height, false);
        }
        return new Window(handle, handle, width, height, true);
    }

    public static Window CreateWindowFullscreen(String title, int width, int height) {
        long handle = nativeCreateWindow(title, width, height, true);
        if (handle == 0) {
            return new Window(0, 0, width, height, false);
        }
        return new Window(handle, handle, width, height, true);
    }

    public static void CloseWindow(Window w) {
        if (w.handle != 0) {
            nativeCloseWindow(w.handle);
            // On macOS, force exit to bypass atexit handlers that cause autorelease pool issues
            String os = System.getProperty("os.name").toLowerCase();
            if (os.contains("mac")) {
                nativeForceExit();
            }
        }
    }

    public static boolean IsRunning(Window w) {
        return w.running;
    }

    public static class PollEventsResult {
        public Window _0;
        public boolean _1;

        public PollEventsResult(Window w, boolean running) {
            this._0 = w;
            this._1 = running;
        }
    }

    public static PollEventsResult PollEvents(Window w) {
        if (nativePollEvents(w.handle)) {
            w.running = false;
            return new PollEventsResult(w, false);
        }
        return new PollEventsResult(w, true);
    }

    public static int GetLastKey() {
        return nativeGetLastKey();
    }

    public static class GetMouseResult {
        public int _0; // x
        public int _1; // y
        public int _2; // buttons

        public GetMouseResult(int x, int y, int buttons) {
            this._0 = x;
            this._1 = y;
            this._2 = buttons;
        }
    }

    public static GetMouseResult GetMouse(Window w) {
        int[] result = nativeGetMouse(w.handle);
        return new GetMouseResult(result[0], result[1], result[2]);
    }

    public static int GetWidth(Window w) {
        return w.width;
    }

    public static int GetHeight(Window w) {
        return w.height;
    }

    public static class GetScreenSizeResult {
        public int _0; // width
        public int _1; // height

        public GetScreenSizeResult(int width, int height) {
            this._0 = width;
            this._1 = height;
        }
    }

    public static GetScreenSizeResult GetScreenSize() {
        int[] result = nativeGetScreenSize();
        return new GetScreenSizeResult(result[0], result[1]);
    }

    // --- Rendering ---

    public static void Clear(Window w, Color c) {
        nativeClear(w.renderer, c.R, c.G, c.B, c.A);
    }

    public static void Present(Window w) {
        nativePresent(w.renderer);
    }

    // --- Drawing primitives ---

    public static void DrawRect(Window w, Rect rect, Color c) {
        nativeDrawRect(w.renderer, rect.X, rect.Y, rect.Width, rect.Height, c.R, c.G, c.B, c.A);
    }

    public static void FillRect(Window w, Rect rect, Color c) {
        nativeFillRect(w.renderer, rect.X, rect.Y, rect.Width, rect.Height, c.R, c.G, c.B, c.A);
    }

    public static void DrawLine(Window w, int x1, int y1, int x2, int y2, Color c) {
        nativeDrawLine(w.renderer, x1, y1, x2, y2, c.R, c.G, c.B, c.A);
    }

    public static void DrawPoint(Window w, int x, int y, Color c) {
        nativeDrawPoint(w.renderer, x, y, c.R, c.G, c.B, c.A);
    }

    public static void DrawCircle(Window w, int centerX, int centerY, int radius, Color c) {
        nativeDrawCircle(w.renderer, centerX, centerY, radius, c.R, c.G, c.B, c.A);
    }

    public static void FillCircle(Window w, int centerX, int centerY, int radius, Color c) {
        nativeFillCircle(w.renderer, centerX, centerY, radius, c.R, c.G, c.B, c.A);
    }

    // --- Run loops ---

    public static void RunLoop(Window w, Function<Window, Boolean> frameFunc) {
        while (true) {
            PollEventsResult result = PollEvents(w);
            w = result._0;
            if (!result._1) {
                break;
            }
            if (!frameFunc.apply(w)) {
                break;
            }
        }
    }

    public static <S> void RunLoopWithState(Window w, S state, BiFunction<Window, S, Object[]> frameFunc) {
        while (true) {
            PollEventsResult result = PollEvents(w);
            w = result._0;
            if (!result._1) {
                break;
            }
            Object[] funcResult = frameFunc.apply(w, state);
            @SuppressWarnings("unchecked")
            S newState = (S) funcResult[0];
            state = newState;
            Boolean cont = (Boolean) funcResult[1];
            if (!cont) {
                break;
            }
        }
    }
}
