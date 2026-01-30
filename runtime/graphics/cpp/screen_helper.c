// screen_helper.c - Platform-specific screen size detection
// Compiled alongside tigr.c via build.rs

#ifdef __APPLE__
#include <objc/runtime.h>
#include <objc/message.h>
#include <CoreGraphics/CGGeometry.h>

typedef struct { CGPoint origin; CGSize size; } ScreenNSRect;

void getScreenSize(int* width, int* height) {
    id screenClass = (id)objc_getClass("NSScreen");
    id mainScreen = ((id (*)(id, SEL))objc_msgSend)(screenClass, sel_registerName("mainScreen"));
    if (mainScreen) {
#if defined(__aarch64__)
        ScreenNSRect frame = ((ScreenNSRect (*)(id, SEL))objc_msgSend)(mainScreen, sel_registerName("visibleFrame"));
#else
        ScreenNSRect frame;
        ((void (*)(ScreenNSRect*, id, SEL))objc_msgSend_stret)(&frame, mainScreen, sel_registerName("visibleFrame"));
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

void getScreenSize(int* width, int* height) {
    *width = GetSystemMetrics(SM_CXSCREEN);
    *height = GetSystemMetrics(SM_CYSCREEN);
}

#elif defined(__linux__)
#include <X11/Xlib.h>

void getScreenSize(int* width, int* height) {
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

void getScreenSize(int* width, int* height) {
    *width = 1024;
    *height = 768;
}

#endif
