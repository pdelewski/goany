// graphics_runtime_sdl2.hpp - SDL2 runtime for goany graphics package
// This file provides the native implementations for the graphics package using SDL2.
// Requires: SDL2 library installed on the system

#ifndef GRAPHICS_RUNTIME_SDL2_HPP
#define GRAPHICS_RUNTIME_SDL2_HPP

#include <SDL.h>
#include <string>
#include <tuple>
#include <cstdint>
#include <functional>

namespace graphics {

struct Color {
    uint8_t R;
    uint8_t G;
    uint8_t B;
    uint8_t A;
};

struct Rect {
    int32_t X;
    int32_t Y;
    int32_t Width;
    int32_t Height;
};

struct Window {
    int64_t handle;   // SDL_Window*
    int64_t renderer; // SDL_Renderer*
    int32_t width;
    int32_t height;
    bool running;
};

// --- Color constructors ---

inline Color NewColor(uint8_t r, uint8_t g, uint8_t b, uint8_t a) {
    return Color{r, g, b, a};
}

inline Color Black() { return Color{0, 0, 0, 255}; }
inline Color White() { return Color{255, 255, 255, 255}; }
inline Color Red() { return Color{255, 0, 0, 255}; }
inline Color Green() { return Color{0, 255, 0, 255}; }
inline Color Blue() { return Color{0, 0, 255, 255}; }

// --- Rect constructor ---

inline Rect NewRect(int32_t x, int32_t y, int32_t width, int32_t height) {
    return Rect{x, y, width, height};
}

// Global to store last key pressed
static int lastKeyPressed = 0;

// --- Window management ---

inline Window CreateWindow(const std::string& title, int32_t width, int32_t height) {
    if (SDL_Init(SDL_INIT_VIDEO) < 0) {
        return Window{0, 0, width, height, false};
    }

    SDL_Window* sdlWindow = SDL_CreateWindow(
        title.c_str(),
        SDL_WINDOWPOS_CENTERED,
        SDL_WINDOWPOS_CENTERED,
        width,
        height,
        SDL_WINDOW_SHOWN
    );

    if (!sdlWindow) {
        return Window{0, 0, width, height, false};
    }

    SDL_Renderer* sdlRenderer = SDL_CreateRenderer(
        sdlWindow, -1,
        SDL_RENDERER_ACCELERATED | SDL_RENDERER_PRESENTVSYNC
    );

    if (!sdlRenderer) {
        SDL_DestroyWindow(sdlWindow);
        return Window{0, 0, width, height, false};
    }

    return Window{
        reinterpret_cast<int64_t>(sdlWindow),
        reinterpret_cast<int64_t>(sdlRenderer),
        width,
        height,
        true
    };
}

inline void CloseWindow(Window w) {
    if (w.renderer) {
        SDL_DestroyRenderer(reinterpret_cast<SDL_Renderer*>(w.renderer));
    }
    if (w.handle) {
        SDL_DestroyWindow(reinterpret_cast<SDL_Window*>(w.handle));
    }
    SDL_Quit();
}

inline bool IsRunning(Window w) {
    return w.running;
}

inline std::tuple<Window, bool> PollEvents(Window w) {
    SDL_Event event;
    lastKeyPressed = 0;
    while (SDL_PollEvent(&event)) {
        if (event.type == SDL_QUIT) {
            w.running = false;
            return std::make_tuple(w, false);
        }
        if (event.type == SDL_KEYDOWN) {
            SDL_Keycode key = event.key.keysym.sym;
            // Convert SDL keycode to ASCII for printable characters
            if (key >= SDLK_SPACE && key <= SDLK_z) {
                // Check for shift modifier for uppercase
                if (event.key.keysym.mod & KMOD_SHIFT) {
                    if (key >= SDLK_a && key <= SDLK_z) {
                        lastKeyPressed = key - 32; // Convert to uppercase
                    } else {
                        lastKeyPressed = key;
                    }
                } else {
                    lastKeyPressed = key;
                }
            } else if (key == SDLK_RETURN) {
                lastKeyPressed = 13; // Enter
            } else if (key == SDLK_BACKSPACE) {
                lastKeyPressed = 8; // Backspace
            }
        }
    }
    return std::make_tuple(w, true);
}

inline int GetLastKey() {
    return lastKeyPressed;
}

// GetMouse returns mouse position and button state
inline std::tuple<int32_t, int32_t, int32_t> GetMouse(Window w) {
    int x, y;
    Uint32 buttons = SDL_GetMouseState(&x, &y);
    // Convert SDL button mask to our format: left=1, right=2, middle=4
    int32_t result = 0;
    if (buttons & SDL_BUTTON(1)) result |= 1; // left
    if (buttons & SDL_BUTTON(3)) result |= 2; // right
    if (buttons & SDL_BUTTON(2)) result |= 4; // middle
    return {static_cast<int32_t>(x), static_cast<int32_t>(y), result};
}

inline int32_t GetWidth(Window w) { return w.width; }
inline int32_t GetHeight(Window w) { return w.height; }

// GetScreenSize returns screen resolution (SDL2 cross-platform)
inline std::tuple<int32_t, int32_t> GetScreenSize() {
    SDL_DisplayMode mode;
    if (SDL_GetCurrentDisplayMode(0, &mode) == 0) {
        return {mode.w, mode.h};
    }
    return {1920, 1080};
}

// CreateWindowFullscreen creates a fullscreen window
inline Window CreateWindowFullscreen(const std::string& title, int32_t width, int32_t height) {
    if (SDL_Init(SDL_INIT_VIDEO) < 0) {
        return Window{0, 0, width, height, false};
    }

    SDL_Window* window = SDL_CreateWindow(title.c_str(),
        SDL_WINDOWPOS_CENTERED, SDL_WINDOWPOS_CENTERED,
        width, height, SDL_WINDOW_SHOWN | SDL_WINDOW_FULLSCREEN_DESKTOP);
    if (!window) {
        return Window{0, 0, width, height, false};
    }

    SDL_Renderer* renderer = SDL_CreateRenderer(window, -1,
        SDL_RENDERER_ACCELERATED | SDL_RENDERER_PRESENTVSYNC);
    if (!renderer) {
        SDL_DestroyWindow(window);
        return Window{0, 0, width, height, false};
    }

    return Window{
        reinterpret_cast<int64_t>(window),
        reinterpret_cast<int64_t>(renderer),
        width, height, true
    };
}

// --- Rendering ---

inline void Clear(Window w, Color c) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
    SDL_RenderClear(renderer);
}

inline void Present(Window w) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_RenderPresent(renderer);
}

// --- Drawing primitives ---

inline void DrawRect(Window w, Rect rect, Color c) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
    SDL_Rect sdlRect = {rect.X, rect.Y, rect.Width, rect.Height};
    SDL_RenderDrawRect(renderer, &sdlRect);
}

inline void FillRect(Window w, Rect rect, Color c) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
    SDL_Rect sdlRect = {rect.X, rect.Y, rect.Width, rect.Height};
    SDL_RenderFillRect(renderer, &sdlRect);
}

inline void DrawLine(Window w, int32_t x1, int32_t y1, int32_t x2, int32_t y2, Color c) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
    SDL_RenderDrawLine(renderer, x1, y1, x2, y2);
}

inline void DrawPoint(Window w, int32_t x, int32_t y, Color c) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);
    SDL_RenderDrawPoint(renderer, x, y);
}

inline void DrawCircle(Window w, int32_t centerX, int32_t centerY, int32_t radius, Color c) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);

    // Bresenham's circle algorithm
    int32_t x = radius;
    int32_t y = 0;
    int32_t err = 0;

    while (x >= y) {
        SDL_RenderDrawPoint(renderer, centerX + x, centerY + y);
        SDL_RenderDrawPoint(renderer, centerX + y, centerY + x);
        SDL_RenderDrawPoint(renderer, centerX - y, centerY + x);
        SDL_RenderDrawPoint(renderer, centerX - x, centerY + y);
        SDL_RenderDrawPoint(renderer, centerX - x, centerY - y);
        SDL_RenderDrawPoint(renderer, centerX - y, centerY - x);
        SDL_RenderDrawPoint(renderer, centerX + y, centerY - x);
        SDL_RenderDrawPoint(renderer, centerX + x, centerY - y);

        y++;
        err += 1 + 2 * y;
        if (2 * (err - x) + 1 > 0) {
            x--;
            err += 1 - 2 * x;
        }
    }
}

inline void FillCircle(Window w, int32_t centerX, int32_t centerY, int32_t radius, Color c) {
    SDL_Renderer* renderer = reinterpret_cast<SDL_Renderer*>(w.renderer);
    SDL_SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A);

    for (int32_t y = -radius; y <= radius; y++) {
        for (int32_t x = -radius; x <= radius; x++) {
            if (x * x + y * y <= radius * radius) {
                SDL_RenderDrawPoint(renderer, centerX + x, centerY + y);
            }
        }
    }
}

// RunLoop runs the main loop, calling frameFunc each frame.
// frameFunc receives the window and returns true to continue, false to stop.
// This is the preferred way to write cross-platform graphics code that works in browsers.
inline void RunLoop(Window w, std::function<bool(Window)> frameFunc) {
    while (true) {
        bool running;
        std::tie(w, running) = PollEvents(w);
        if (!running) {
            break;
        }
        if (!frameFunc(w)) {
            break;
        }
    }
}

// RunLoopWithState runs the main loop with explicit state threading.
// The state is passed to frameFunc and returned each frame, avoiding closure capture.
// frameFunc receives the window and state, returns updated state and true to continue.
template<typename S, typename F>
inline void RunLoopWithState(Window w, S state, F frameFunc) {
    while (true) {
        bool running;
        std::tie(w, running) = PollEvents(w);
        if (!running) {
            break;
        }
        bool cont;
        std::tie(state, cont) = frameFunc(w, std::move(state));
        if (!cont) {
            break;
        }
    }
}

} // namespace graphics

#endif // GRAPHICS_RUNTIME_SDL2_HPP
