// GraphicsRuntime.cs - SDL2 runtime for goany graphics package
// Requires: SDL2-CS NuGet package

using System;
using SDL2;

public static class graphics
{
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
        public long handle;   // SDL_Window*
        public long renderer; // SDL_Renderer*
        public int width;
        public int height;
        public bool running;
    }

    // Global to store last key pressed
    private static int lastKeyPressed = 0;

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
        if (SDL.SDL_Init(SDL.SDL_INIT_VIDEO) < 0)
        {
            return new Window { handle = 0, renderer = 0, width = width, height = height, running = false };
        }

        IntPtr sdlWindow = SDL.SDL_CreateWindow(
            title,
            SDL.SDL_WINDOWPOS_CENTERED,
            SDL.SDL_WINDOWPOS_CENTERED,
            width,
            height,
            SDL.SDL_WindowFlags.SDL_WINDOW_SHOWN
        );

        if (sdlWindow == IntPtr.Zero)
        {
            return new Window { handle = 0, renderer = 0, width = width, height = height, running = false };
        }

        IntPtr sdlRenderer = SDL.SDL_CreateRenderer(
            sdlWindow, -1,
            SDL.SDL_RendererFlags.SDL_RENDERER_ACCELERATED | SDL.SDL_RendererFlags.SDL_RENDERER_PRESENTVSYNC
        );

        if (sdlRenderer == IntPtr.Zero)
        {
            SDL.SDL_DestroyWindow(sdlWindow);
            return new Window { handle = 0, renderer = 0, width = width, height = height, running = false };
        }

        return new Window
        {
            handle = sdlWindow.ToInt64(),
            renderer = sdlRenderer.ToInt64(),
            width = width,
            height = height,
            running = true
        };
    }

    public static void CloseWindow(Window w)
    {
        if (w.renderer != 0)
        {
            SDL.SDL_DestroyRenderer(new IntPtr(w.renderer));
        }
        if (w.handle != 0)
        {
            SDL.SDL_DestroyWindow(new IntPtr(w.handle));
        }
        SDL.SDL_Quit();
    }

    public static bool IsRunning(Window w)
    {
        return w.running;
    }

    public static (Window, bool) PollEvents(Window w)
    {
        lastKeyPressed = 0;
        SDL.SDL_Event e;
        while (SDL.SDL_PollEvent(out e) != 0)
        {
            if (e.type == SDL.SDL_EventType.SDL_QUIT)
            {
                w.running = false;
                return (w, false);
            }
            if (e.type == SDL.SDL_EventType.SDL_KEYDOWN)
            {
                SDL.SDL_Keycode key = e.key.keysym.sym;
                // Convert SDL keycode to ASCII for printable characters
                if (key >= SDL.SDL_Keycode.SDLK_SPACE && key <= SDL.SDL_Keycode.SDLK_z)
                {
                    // Check for shift modifier for uppercase
                    SDL.SDL_Keymod mod = (SDL.SDL_Keymod)e.key.keysym.mod;
                    if ((mod & (SDL.SDL_Keymod.KMOD_LSHIFT | SDL.SDL_Keymod.KMOD_RSHIFT)) != 0)
                    {
                        if (key >= SDL.SDL_Keycode.SDLK_a && key <= SDL.SDL_Keycode.SDLK_z)
                        {
                            lastKeyPressed = (int)key - 32; // Convert to uppercase
                        }
                        else
                        {
                            lastKeyPressed = (int)key;
                        }
                    }
                    else
                    {
                        lastKeyPressed = (int)key;
                    }
                }
                else if (key == SDL.SDL_Keycode.SDLK_RETURN)
                {
                    lastKeyPressed = 13; // Enter
                }
                else if (key == SDL.SDL_Keycode.SDLK_BACKSPACE)
                {
                    lastKeyPressed = 8; // Backspace
                }
            }
        }
        return (w, true);
    }

    public static int GetLastKey()
    {
        return lastKeyPressed;
    }

    // GetMouse returns mouse position and button state
    public static (int, int, int) GetMouse(Window w)
    {
        int x, y;
        uint buttons = SDL.SDL_GetMouseState(out x, out y);
        int result = 0;
        if ((buttons & SDL.SDL_BUTTON(SDL.SDL_BUTTON_LEFT)) != 0) result |= 1;
        if ((buttons & SDL.SDL_BUTTON(SDL.SDL_BUTTON_RIGHT)) != 0) result |= 2;
        if ((buttons & SDL.SDL_BUTTON(SDL.SDL_BUTTON_MIDDLE)) != 0) result |= 4;
        return (x, y, result);
    }

    public static int GetWidth(Window w) { return w.width; }
    public static int GetHeight(Window w) { return w.height; }

    // GetScreenSize returns screen resolution (SDL2 cross-platform)
    public static (int, int) GetScreenSize()
    {
        SDL.SDL_DisplayMode mode;
        if (SDL.SDL_GetCurrentDisplayMode(0, out mode) == 0)
        {
            return (mode.w, mode.h);
        }
        return (1920, 1080);
    }

    // CreateWindowFullscreen creates a fullscreen window
    public static Window CreateWindowFullscreen(string title, int width, int height)
    {
        if (SDL.SDL_Init(SDL.SDL_INIT_VIDEO) < 0)
        {
            return new Window { handle = 0, renderer = 0, width = width, height = height, running = false };
        }

        IntPtr window = SDL.SDL_CreateWindow(title,
            SDL.SDL_WINDOWPOS_CENTERED, SDL.SDL_WINDOWPOS_CENTERED,
            width, height, SDL.SDL_WindowFlags.SDL_WINDOW_SHOWN | SDL.SDL_WindowFlags.SDL_WINDOW_FULLSCREEN_DESKTOP);
        if (window == IntPtr.Zero)
        {
            return new Window { handle = 0, renderer = 0, width = width, height = height, running = false };
        }

        IntPtr renderer = SDL.SDL_CreateRenderer(window, -1,
            SDL.SDL_RendererFlags.SDL_RENDERER_ACCELERATED | SDL.SDL_RendererFlags.SDL_RENDERER_PRESENTVSYNC);
        if (renderer == IntPtr.Zero)
        {
            SDL.SDL_DestroyWindow(window);
            return new Window { handle = 0, renderer = 0, width = width, height = height, running = false };
        }

        return new Window
        {
            handle = window.ToInt64(),
            renderer = renderer.ToInt64(),
            width = width,
            height = height,
            running = true
        };
    }

    // --- Rendering ---

    public static void Clear(Window w, Color c)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
        SDL.SDL_RenderClear(renderer);
    }

    public static void Present(Window w)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_RenderPresent(renderer);
    }

    // --- Drawing primitives ---

    public static void DrawRect(Window w, Rect rect, Color c)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
        SDL.SDL_Rect sdlRect = new SDL.SDL_Rect { x = rect.X, y = rect.Y, w = rect.Width, h = rect.Height };
        SDL.SDL_RenderDrawRect(renderer, ref sdlRect);
    }

    public static void FillRect(Window w, Rect rect, Color c)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
        SDL.SDL_Rect sdlRect = new SDL.SDL_Rect { x = rect.X, y = rect.Y, w = rect.Width, h = rect.Height };
        SDL.SDL_RenderFillRect(renderer, ref sdlRect);
    }

    public static void DrawLine(Window w, int x1, int y1, int x2, int y2, Color c)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
        SDL.SDL_RenderDrawLine(renderer, x1, y1, x2, y2);
    }

    public static void DrawPoint(Window w, int x, int y, Color c)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
        SDL.SDL_RenderDrawPoint(renderer, x, y);
    }

    public static void DrawCircle(Window w, int centerX, int centerY, int radius, Color c)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);

        // Bresenham's circle algorithm
        int x = radius;
        int y = 0;
        int err = 0;

        while (x >= y)
        {
            SDL.SDL_RenderDrawPoint(renderer, centerX + x, centerY + y);
            SDL.SDL_RenderDrawPoint(renderer, centerX + y, centerY + x);
            SDL.SDL_RenderDrawPoint(renderer, centerX - y, centerY + x);
            SDL.SDL_RenderDrawPoint(renderer, centerX - x, centerY + y);
            SDL.SDL_RenderDrawPoint(renderer, centerX - x, centerY - y);
            SDL.SDL_RenderDrawPoint(renderer, centerX - y, centerY - x);
            SDL.SDL_RenderDrawPoint(renderer, centerX + y, centerY - x);
            SDL.SDL_RenderDrawPoint(renderer, centerX + x, centerY - y);

            y++;
            err += 1 + 2 * y;
            if (2 * (err - x) + 1 > 0)
            {
                x--;
                err += 1 - 2 * x;
            }
        }
    }

    public static void FillCircle(Window w, int centerX, int centerY, int radius, Color c)
    {
        IntPtr renderer = new IntPtr(w.renderer);
        SDL.SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);

        for (int y = -radius; y <= radius; y++)
        {
            for (int x = -radius; x <= radius; x++)
            {
                if (x * x + y * y <= radius * radius)
                {
                    SDL.SDL_RenderDrawPoint(renderer, centerX + x, centerY + y);
                }
            }
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
