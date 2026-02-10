// graphics_jni.c - JNI wrapper for tigr graphics library
// Compile with:
// - macOS: cc -shared -o libgraphics_jni.dylib -I"$JAVA_HOME/include" -I"$JAVA_HOME/include/darwin" graphics_jni.c tigr.c screen_helper.c -framework OpenGL -framework Cocoa
// - Linux: gcc -shared -fPIC -o libgraphics_jni.so -I"$JAVA_HOME/include" -I"$JAVA_HOME/include/linux" graphics_jni.c tigr.c screen_helper.c -lGL -lX11
// - Windows: cl /LD graphics_jni.c tigr.c screen_helper.c /I"%JAVA_HOME%\include" /I"%JAVA_HOME%\include\win32" opengl32.lib gdi32.lib user32.lib /Fe:graphics_jni.dll

#include <jni.h>
#include "tigr.h"
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// macOS: Force TIGR cleanup before JVM tries to shutdown
#ifdef __APPLE__
#include <objc/runtime.h>
#include <objc/message.h>
#include <signal.h>

// Flag to track if we've closed the window properly
static int windowClosed = 0;

// Custom signal handler to prevent atexit from running after ObjC runtime issues
static void cleanExit(int sig) {
    _exit(0);
}
#endif

// Platform-specific screen size (implemented in screen_helper.c)
extern void getScreenSize(int* width, int* height);

// Global to store last key pressed
static int lastKeyPressed = 0;

// Key state tracking for reliable single-press detection
static int prevKeyState[8] = {0, 0, 0, 0, 0, 0, 0, 0};
// Index: 0=RETURN, 1=BACKSPACE, 2=ESCAPE, 3=LEFT, 4=RIGHT, 5=UP, 6=DOWN, 7=reserved

// Helper to convert RGBA to TPixel
static TPixel toTPixel(int r, int g, int b, int a) {
    return tigrRGBA((unsigned char)r, (unsigned char)g, (unsigned char)b, (unsigned char)a);
}

// --- JNI Native Method Implementations ---

JNIEXPORT jlong JNICALL Java_graphics_nativeCreateWindow(JNIEnv *env, jclass cls, jstring title, jint width, jint height, jboolean fullscreen) {
    const char *titleStr = (*env)->GetStringUTFChars(env, title, NULL);
    if (titleStr == NULL) {
        return 0; // OutOfMemoryError already thrown
    }

    int flags = fullscreen ? TIGR_FULLSCREEN : TIGR_FIXED;
    Tigr *win = tigrWindow(width, height, titleStr, flags);

    (*env)->ReleaseStringUTFChars(env, title, titleStr);

    return (jlong)(intptr_t)win;
}

JNIEXPORT void JNICALL Java_graphics_nativeCloseWindow(JNIEnv *env, jclass cls, jlong handle) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrFree(win);
#ifdef __APPLE__
        windowClosed = 1;
#endif
    }
}

// Special exit function to bypass atexit handlers that cause autorelease pool issues
JNIEXPORT void JNICALL Java_graphics_nativeForceExit(JNIEnv *env, jclass cls) {
#ifdef __APPLE__
    _exit(0);  // Bypass atexit handlers
#endif
}

JNIEXPORT jboolean JNICALL Java_graphics_nativePollEvents(JNIEnv *env, jclass cls, jlong handle) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win == NULL) {
        return JNI_TRUE; // Closed
    }

    // Update window (processes events)
    tigrUpdate(win);

    // Check for close
    if (tigrClosed(win)) {
        return JNI_TRUE;
    }

    // Reset last key
    lastKeyPressed = 0;

    // Check for character input
    int ch = tigrReadChar(win);
    if (ch > 0 && ch < 128) {
        lastKeyPressed = ch;
    }

    // Check for special keys with edge detection
    int specialKeys[] = {TK_RETURN, TK_BACKSPACE, TK_ESCAPE, TK_LEFT, TK_RIGHT, TK_UP, TK_DOWN};
    int keyCodes[] = {13, 8, 27, 37, 39, 38, 40}; // ASCII/virtual key codes

    for (int i = 0; i < 7; i++) {
        int currentState = tigrKeyHeld(win, specialKeys[i]);
        if (currentState && !prevKeyState[i]) {
            // Key just pressed
            lastKeyPressed = keyCodes[i];
        }
        prevKeyState[i] = currentState;
    }

    return JNI_FALSE; // Not closed
}

JNIEXPORT jint JNICALL Java_graphics_nativeGetLastKey(JNIEnv *env, jclass cls) {
    return lastKeyPressed;
}

JNIEXPORT jintArray JNICALL Java_graphics_nativeGetMouse(JNIEnv *env, jclass cls, jlong handle) {
    Tigr *win = (Tigr *)(intptr_t)handle;

    int x = 0, y = 0, buttons = 0;
    if (win != NULL) {
        tigrMouse(win, &x, &y, &buttons);
    }

    jintArray result = (*env)->NewIntArray(env, 3);
    if (result == NULL) {
        return NULL; // OutOfMemoryError
    }

    jint values[3] = {x, y, buttons};
    (*env)->SetIntArrayRegion(env, result, 0, 3, values);

    return result;
}

JNIEXPORT jintArray JNICALL Java_graphics_nativeGetScreenSize(JNIEnv *env, jclass cls) {
    int width = 0, height = 0;
    getScreenSize(&width, &height);

    jintArray result = (*env)->NewIntArray(env, 2);
    if (result == NULL) {
        return NULL; // OutOfMemoryError
    }

    jint values[2] = {width, height};
    (*env)->SetIntArrayRegion(env, result, 0, 2, values);

    return result;
}

JNIEXPORT void JNICALL Java_graphics_nativeClear(JNIEnv *env, jclass cls, jlong handle, jint r, jint g, jint b, jint a) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrClear(win, toTPixel(r, g, b, a));
    }
}

JNIEXPORT void JNICALL Java_graphics_nativePresent(JNIEnv *env, jclass cls, jlong handle) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrUpdate(win);
    }
}

JNIEXPORT void JNICALL Java_graphics_nativeDrawRect(JNIEnv *env, jclass cls, jlong handle, jint x, jint y, jint w, jint h, jint r, jint g, jint b, jint a) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrRect(win, x, y, w, h, toTPixel(r, g, b, a));
    }
}

JNIEXPORT void JNICALL Java_graphics_nativeFillRect(JNIEnv *env, jclass cls, jlong handle, jint x, jint y, jint w, jint h, jint r, jint g, jint b, jint a) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrFill(win, x, y, w, h, toTPixel(r, g, b, a));
    }
}

JNIEXPORT void JNICALL Java_graphics_nativeDrawLine(JNIEnv *env, jclass cls, jlong handle, jint x1, jint y1, jint x2, jint y2, jint r, jint g, jint b, jint a) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrLine(win, x1, y1, x2, y2, toTPixel(r, g, b, a));
    }
}

JNIEXPORT void JNICALL Java_graphics_nativeDrawPoint(JNIEnv *env, jclass cls, jlong handle, jint x, jint y, jint r, jint g, jint b, jint a) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrPlot(win, x, y, toTPixel(r, g, b, a));
    }
}

JNIEXPORT void JNICALL Java_graphics_nativeDrawCircle(JNIEnv *env, jclass cls, jlong handle, jint cx, jint cy, jint radius, jint r, jint g, jint b, jint a) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrCircle(win, cx, cy, radius, toTPixel(r, g, b, a));
    }
}

JNIEXPORT void JNICALL Java_graphics_nativeFillCircle(JNIEnv *env, jclass cls, jlong handle, jint cx, jint cy, jint radius, jint r, jint g, jint b, jint a) {
    Tigr *win = (Tigr *)(intptr_t)handle;
    if (win != NULL) {
        tigrFillCircle(win, cx, cy, radius, toTPixel(r, g, b, a));
    }
}
