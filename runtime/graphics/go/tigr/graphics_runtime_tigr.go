// Package tigr provides the CGO tigr implementation for the graphics package.
package tigr

/*
#cgo CFLAGS: -I.
#cgo darwin LDFLAGS: -framework OpenGL -framework Cocoa -framework CoreGraphics
#cgo linux LDFLAGS: -lGL -lX11
#cgo windows LDFLAGS: -lopengl32 -lgdi32 -luser32

#include "tigr.h"
#include <stdlib.h>

// Global to store last key pressed
static int lastKeyPressed = 0;

// Our own key state tracking for reliable single-press detection
// Index: 0=RETURN, 1=BACKSPACE, 2=ESCAPE, 3=LEFT, 4=RIGHT, 5=UP, 6=DOWN
static int prevKeyState[7] = {0, 0, 0, 0, 0, 0, 0};

// Helper to create window
static Tigr* createTigrWindow(const char* title, int width, int height) {
    return tigrWindow(width, height, title, TIGR_FIXED);
}

// Helper to create fullscreen window
static Tigr* createTigrWindowFullscreen(const char* title, int width, int height) {
    return tigrWindow(width, height, title, TIGR_FULLSCREEN);
}

// GetScreenSize returns the usable screen area in logical points (excludes menu bar and Dock)
#ifdef __APPLE__
#include <objc/runtime.h>
#include <objc/message.h>
#include <CoreGraphics/CGGeometry.h>
static void getScreenSize(int* width, int* height) {
    id screenClass = (id)objc_getClass("NSScreen");
    id mainScreen = ((id (*)(id, SEL))objc_msgSend)(screenClass, sel_registerName("mainScreen"));
    if (mainScreen) {
        typedef struct { CGPoint origin; CGSize size; } NSRect;
#if defined(__aarch64__)
        NSRect frame = ((NSRect (*)(id, SEL))objc_msgSend)(mainScreen, sel_registerName("visibleFrame"));
#else
        NSRect frame;
        ((void (*)(NSRect*, id, SEL))objc_msgSend_stret)(&frame, mainScreen, sel_registerName("visibleFrame"));
#endif
        *width = (int)frame.size.width;
        *height = (int)frame.size.height;
    } else {
        *width = 1024;
        *height = 768;
    }
}
#elif defined(_WIN32)
#include <windows.h>
static void getScreenSize(int* width, int* height) {
    *width = GetSystemMetrics(SM_CXSCREEN);
    *height = GetSystemMetrics(SM_CYSCREEN);
}
#elif defined(__linux__)
#include <X11/Xlib.h>
static void getScreenSize(int* width, int* height) {
    Display* dpy = XOpenDisplay(NULL);
    if (dpy) {
        Screen* scr = DefaultScreenOfDisplay(dpy);
        *width = scr->width;
        *height = scr->height;
        XCloseDisplay(dpy);
    } else {
        *width = 1024;
        *height = 768;
    }
}
#else
static void getScreenSize(int* width, int* height) {
    *width = 1024;
    *height = 768;
}
#endif

// Poll events and return 1 if quit requested
static int pollTigrEvents(Tigr* win) {
    if (!win) return 1;

    if (tigrClosed(win)) {
        return 1;
    }

    // Call tigrUpdate to process events
    tigrUpdate(win);

    // Reset last key
    lastKeyPressed = 0;

    // Use tigrReadChar for character input
    // Note: Ignore '\n' (10) as some systems send CRLF for Enter
    int ch = tigrReadChar(win);
    if (ch > 0 && ch < 128 && ch != 10) {
        lastKeyPressed = ch;
    }

    // Also check for DEL (127) which macOS uses for backspace
    if (ch == 127) {
        lastKeyPressed = 8;  // Normalize to backspace
    }

    // Check special keys using our own state tracking
    int currKeyState[7];
    currKeyState[0] = tigrKeyHeld(win, TK_RETURN) != 0;
    currKeyState[1] = tigrKeyHeld(win, TK_BACKSPACE) != 0;
    currKeyState[2] = tigrKeyHeld(win, TK_ESCAPE) != 0;
    currKeyState[3] = tigrKeyHeld(win, TK_LEFT) != 0;
    currKeyState[4] = tigrKeyHeld(win, TK_RIGHT) != 0;
    currKeyState[5] = tigrKeyHeld(win, TK_UP) != 0;
    currKeyState[6] = tigrKeyHeld(win, TK_DOWN) != 0;

    // Only detect key press if no character was read via tigrReadChar
    if (lastKeyPressed == 0) {
        // Note: TK_RETURN (index 0) is NOT detected here - Enter is handled via tigrReadChar
        if (currKeyState[1] && !prevKeyState[1]) lastKeyPressed = 8;
        else if (currKeyState[2] && !prevKeyState[2]) lastKeyPressed = 27;
        else if (currKeyState[3] && !prevKeyState[3]) lastKeyPressed = 256;
        else if (currKeyState[4] && !prevKeyState[4]) lastKeyPressed = 257;
        else if (currKeyState[5] && !prevKeyState[5]) lastKeyPressed = 258;
        else if (currKeyState[6] && !prevKeyState[6]) lastKeyPressed = 259;
    }

    // ALWAYS update previous state
    for (int i = 0; i < 7; i++) {
        prevKeyState[i] = currKeyState[i];
    }

    // Check again after update
    if (tigrClosed(win)) {
        return 1;
    }

    return 0;
}

// Get the last key pressed (0 if none)
static int getLastKey() {
    return lastKeyPressed;
}

// Convert RGBA to TPixel
static TPixel rgbaToPixel(unsigned char r, unsigned char g, unsigned char b, unsigned char a) {
    return tigrRGBA(r, g, b, a);
}
*/
import "C"
import (
	"runtime"
	"unsafe"
)

func init() {
	// On macOS, graphics operations should be on the main thread.
	runtime.LockOSThread()
}

// CreateWindow creates a new window with the specified title and dimensions.
// Returns handle, renderer (same as handle for tigr), success
func CreateWindow(title string, width int32, height int32) (int64, int64, bool) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	win := C.createTigrWindow(cTitle, C.int(width), C.int(height))
	if win == nil {
		return 0, 0, false
	}

	handle := int64(uintptr(unsafe.Pointer(win)))
	return handle, handle, true
}

// CreateWindowFullscreen creates a fullscreen window with the specified buffer dimensions.
// Returns handle, renderer (same as handle for tigr), success
func CreateWindowFullscreen(title string, width int32, height int32) (int64, int64, bool) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	win := C.createTigrWindowFullscreen(cTitle, C.int(width), C.int(height))
	if win == nil {
		return 0, 0, false
	}

	handle := int64(uintptr(unsafe.Pointer(win)))
	return handle, handle, true
}

// CloseWindow closes the window and releases resources.
func CloseWindow(handle int64, renderer int64) {
	if handle != 0 {
		C.tigrFree((*C.Tigr)(unsafe.Pointer(uintptr(handle))))
	}
}

// PollEvents processes pending events for the given window and returns true if quit requested.
func PollEvents(handle int64) bool {
	if handle == 0 {
		return true
	}
	win := (*C.Tigr)(unsafe.Pointer(uintptr(handle)))
	return C.pollTigrEvents(win) != 0
}

// GetLastKey returns the ASCII code of the last key pressed (0 if none).
func GetLastKey() int {
	return int(C.getLastKey())
}

// Clear clears the window with the specified color.
func Clear(renderer int64, r uint8, g uint8, b uint8, a uint8) {
	win := (*C.Tigr)(unsafe.Pointer(uintptr(renderer)))
	C.tigrClear(win, C.rgbaToPixel(C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a)))
}

// Present presents the rendered content to the screen.
func Present(renderer int64) {
	// tigrUpdate is called in PollEvents, so this is a no-op for tigr
}

// DrawRect draws a rectangle outline.
func DrawRect(renderer int64, x int32, y int32, width int32, height int32, r uint8, g uint8, b uint8, a uint8) {
	win := (*C.Tigr)(unsafe.Pointer(uintptr(renderer)))
	C.tigrRect(win, C.int(x), C.int(y), C.int(width), C.int(height),
		C.rgbaToPixel(C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a)))
}

// FillRect draws a filled rectangle.
func FillRect(renderer int64, x int32, y int32, width int32, height int32, r uint8, g uint8, b uint8, a uint8) {
	win := (*C.Tigr)(unsafe.Pointer(uintptr(renderer)))
	C.tigrFillRect(win, C.int(x), C.int(y), C.int(width), C.int(height),
		C.rgbaToPixel(C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a)))
}

// DrawLine draws a line between two points.
func DrawLine(renderer int64, x1 int32, y1 int32, x2 int32, y2 int32, r uint8, g uint8, b uint8, a uint8) {
	win := (*C.Tigr)(unsafe.Pointer(uintptr(renderer)))
	C.tigrLine(win, C.int(x1), C.int(y1), C.int(x2), C.int(y2),
		C.rgbaToPixel(C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a)))
}

// DrawPoint draws a single pixel.
func DrawPoint(renderer int64, x int32, y int32, r uint8, g uint8, b uint8, a uint8) {
	win := (*C.Tigr)(unsafe.Pointer(uintptr(renderer)))
	C.tigrPlot(win, C.int(x), C.int(y),
		C.rgbaToPixel(C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a)))
}

// DrawCircle draws a circle outline.
func DrawCircle(renderer int64, centerX int32, centerY int32, radius int32, r uint8, g uint8, b uint8, a uint8) {
	win := (*C.Tigr)(unsafe.Pointer(uintptr(renderer)))
	C.tigrCircle(win, C.int(centerX), C.int(centerY), C.int(radius),
		C.rgbaToPixel(C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a)))
}

// FillCircle draws a filled circle.
func FillCircle(renderer int64, centerX int32, centerY int32, radius int32, r uint8, g uint8, b uint8, a uint8) {
	win := (*C.Tigr)(unsafe.Pointer(uintptr(renderer)))
	C.tigrFillCircle(win, C.int(centerX), C.int(centerY), C.int(radius),
		C.rgbaToPixel(C.uchar(r), C.uchar(g), C.uchar(b), C.uchar(a)))
}

// GetMouse returns mouse position and button state.
// Returns: x, y, buttons (bit 0=left, bit 1=right, bit 2=middle)
func GetMouse(handle int64) (int32, int32, int32) {
	if handle == 0 {
		return 0, 0, 0
	}
	win := (*C.Tigr)(unsafe.Pointer(uintptr(handle)))
	var x, y, buttons C.int
	C.tigrMouse(win, &x, &y, &buttons)
	return int32(x), int32(y), int32(buttons)
}

// GetScreenSize returns the screen resolution.
func GetScreenSize() (int32, int32) {
	var w, h C.int
	C.getScreenSize(&w, &h)
	return int32(w), int32(h)
}
