// graphics_runtime_tigr.hpp - tigr runtime for goany graphics package
// This file provides the native implementations for the graphics package using tigr.
// tigr is a tiny graphics library (https://github.com/erkkah/tigr)

#ifndef GRAPHICS_RUNTIME_TIGR_HPP
#define GRAPHICS_RUNTIME_TIGR_HPP

#include "tigr.h"
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
    int64_t handle;   // Tigr*
    int64_t renderer; // Not used with tigr (same as handle)
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

// --- Helper to convert Color to TPixel ---
namespace detail {
    inline TPixel toTPixel(const Color& c) {
        return tigrRGBA(c.R, c.G, c.B, c.A);
    }
}

// Global to store last key pressed
static int lastKeyPressed = 0;

// Our own key state tracking for reliable single-press detection
static bool prevKeyState[8] = {false, false, false, false, false, false, false, false};
// Index: 0=RETURN, 1=BACKSPACE, 2=ESCAPE, 3=LEFT, 4=RIGHT, 5=UP, 6=DOWN, 7=reserved

// --- Window management ---

inline Window CreateWindow(const std::string& title, int32_t width, int32_t height) {
    Tigr* win = tigrWindow(width, height, title.c_str(), TIGR_FIXED);

    if (!win) {
        return Window{0, 0, width, height, false};
    }

    return Window{
        reinterpret_cast<int64_t>(win),
        reinterpret_cast<int64_t>(win),  // renderer same as handle for tigr
        width,
        height,
        true
    };
}

inline Window CreateWindowFullscreen(const std::string& title, int32_t width, int32_t height) {
    Tigr* win = tigrWindow(width, height, title.c_str(), TIGR_FULLSCREEN);

    if (!win) {
        return Window{0, 0, width, height, false};
    }

    return Window{
        reinterpret_cast<int64_t>(win),
        reinterpret_cast<int64_t>(win),  // renderer same as handle for tigr
        width,
        height,
        true
    };
}

inline void CloseWindow(Window w) {
    if (w.handle) {
        tigrFree(reinterpret_cast<Tigr*>(w.handle));
    }
}

inline bool IsRunning(Window w) {
    return w.running;
}

inline std::tuple<Window, bool> PollEvents(Window w) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);

    // Check if window should close BEFORE update (in case it was closed last frame)
    if (tigrClosed(win)) {
        w.running = false;
        return std::make_tuple(w, false);
    }

    // Call tigrUpdate to process events and present previous frame
    // This must happen BEFORE checking keys, as events are processed in tigrUpdate
    tigrUpdate(win);

    // Reset last key
    lastKeyPressed = 0;

    // Use tigrReadChar for character input (letters, numbers, space, etc.)
    // Note: Ignore '\n' (10) as some systems send CRLF for Enter - we use '\r' (13) only
    int ch = tigrReadChar(win);
    if (ch > 0 && ch < 128 && ch != 10) {
        lastKeyPressed = ch;
    }

    // Also check for DEL (127) which some systems use for backspace
    if (ch == 127) {
        lastKeyPressed = 8;  // Normalize to backspace
    }

    // Check special keys that don't produce characters
    // Use our own state tracking for reliable single-press detection
    // ALWAYS read current state and update prevKeyState to avoid double-detection
    bool currKeyState[7];
    currKeyState[0] = tigrKeyHeld(win, TK_RETURN) != 0;
    currKeyState[1] = tigrKeyHeld(win, TK_BACKSPACE) != 0;
    currKeyState[2] = tigrKeyHeld(win, TK_ESCAPE) != 0;
    currKeyState[3] = tigrKeyHeld(win, TK_LEFT) != 0;
    currKeyState[4] = tigrKeyHeld(win, TK_RIGHT) != 0;
    currKeyState[5] = tigrKeyHeld(win, TK_UP) != 0;
    currKeyState[6] = tigrKeyHeld(win, TK_DOWN) != 0;

    // Only detect key press if no character was read via tigrReadChar
    if (lastKeyPressed == 0) {
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
    for (int i = 0; i < 7; i++) {
        prevKeyState[i] = currKeyState[i];
    }

    // Check if window was closed during event processing
    if (tigrClosed(win)) {
        w.running = false;
        return std::make_tuple(w, false);
    }

    return std::make_tuple(w, true);
}

inline int GetLastKey() {
    return lastKeyPressed;
}

// GetMouse returns mouse position and button state
// Returns: x, y, buttons (bit 0=left, bit 1=right, bit 2=middle)
inline std::tuple<int32_t, int32_t, int32_t> GetMouse(Window w) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    int x, y, buttons;
    tigrMouse(win, &x, &y, &buttons);
    return std::make_tuple(static_cast<int32_t>(x), static_cast<int32_t>(y), static_cast<int32_t>(buttons));
}

inline int32_t GetWidth(Window w) { return w.width; }
inline int32_t GetHeight(Window w) { return w.height; }

// GetScreenSize returns a safe default size for tigr (windowed mode)
inline std::tuple<int32_t, int32_t> GetScreenSize() {
    return {1024, 768};
}

// --- Rendering ---

inline void Clear(Window w, Color c) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    tigrClear(win, detail::toTPixel(c));
}

inline void Present(Window w) {
    // tigrUpdate is called in PollEvents to ensure events are processed before key checks.
    // The actual rendering/present happens there. This function exists for API compatibility.
    (void)w;
}

// --- Drawing primitives ---

inline void DrawRect(Window w, Rect rect, Color c) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    tigrRect(win, rect.X, rect.Y, rect.Width, rect.Height, detail::toTPixel(c));
}

inline void FillRect(Window w, Rect rect, Color c) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    tigrFillRect(win, rect.X, rect.Y, rect.Width, rect.Height, detail::toTPixel(c));
}

inline void DrawLine(Window w, int32_t x1, int32_t y1, int32_t x2, int32_t y2, Color c) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    tigrLine(win, x1, y1, x2, y2, detail::toTPixel(c));
}

inline void DrawPoint(Window w, int32_t x, int32_t y, Color c) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    tigrPlot(win, x, y, detail::toTPixel(c));
}

inline void DrawCircle(Window w, int32_t centerX, int32_t centerY, int32_t radius, Color c) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    tigrCircle(win, centerX, centerY, radius, detail::toTPixel(c));
}

inline void FillCircle(Window w, int32_t centerX, int32_t centerY, int32_t radius, Color c) {
    Tigr* win = reinterpret_cast<Tigr*>(w.handle);
    tigrFillCircle(win, centerX, centerY, radius, detail::toTPixel(c));
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

#endif // GRAPHICS_RUNTIME_TIGR_HPP
