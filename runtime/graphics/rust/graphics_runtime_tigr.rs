// graphics_runtime_tigr.rs - tigr runtime for goany graphics package
// Requires: tigr.c to be compiled via build.rs
//
// Add to Cargo.toml:
// [build-dependencies]
// cc = "1.0"
//
// Add build.rs:
// fn main() {
//     cc::Build::new()
//         .file("tigr.c")
//         .compile("tigr");
// }

use std::cell::RefCell;
use std::ffi::CString;
use std::os::raw::{c_char, c_int};

// --- FFI bindings for tigr ---

#[repr(C)]
#[derive(Clone, Copy)]
pub struct TPixel {
    pub r: u8,
    pub g: u8,
    pub b: u8,
    pub a: u8,
}

#[repr(C)]
pub struct Tigr {
    pub w: c_int,
    pub h: c_int,
    pub cx: c_int,
    pub cy: c_int,
    pub cw: c_int,
    pub ch: c_int,
    pub pix: *mut TPixel,
    pub handle: *mut std::ffi::c_void,
    pub blitMode: c_int,
}

// tigr key constants (from tigr.h TKey enum starting at TK_PAD0=128)
pub const TK_BACKSPACE: c_int = 156;
pub const TK_TAB: c_int = 157;
pub const TK_RETURN: c_int = 158;
pub const TK_SHIFT: c_int = 159;
pub const TK_ESCAPE: c_int = 164;
pub const TK_SPACE: c_int = 165;
pub const TK_LEFT: c_int = 170;
pub const TK_UP: c_int = 171;
pub const TK_RIGHT: c_int = 172;
pub const TK_DOWN: c_int = 173;

extern "C" {
    fn tigrWindow(w: c_int, h: c_int, title: *const c_char, flags: c_int) -> *mut Tigr;
    fn tigrFree(bmp: *mut Tigr);
    fn tigrClosed(bmp: *mut Tigr) -> c_int;
    fn tigrUpdate(bmp: *mut Tigr);
    fn tigrClear(bmp: *mut Tigr, color: TPixel);
    fn tigrPlot(bmp: *mut Tigr, x: c_int, y: c_int, pix: TPixel);
    fn tigrLine(bmp: *mut Tigr, x0: c_int, y0: c_int, x1: c_int, y1: c_int, color: TPixel);
    fn tigrRect(bmp: *mut Tigr, x: c_int, y: c_int, w: c_int, h: c_int, color: TPixel);
    fn tigrFillRect(bmp: *mut Tigr, x: c_int, y: c_int, w: c_int, h: c_int, color: TPixel);
    fn tigrCircle(bmp: *mut Tigr, x: c_int, y: c_int, r: c_int, color: TPixel);
    fn tigrFillCircle(bmp: *mut Tigr, x: c_int, y: c_int, r: c_int, color: TPixel);
    fn tigrReadChar(bmp: *mut Tigr) -> c_int;
    fn tigrKeyDown(bmp: *mut Tigr, key: c_int) -> c_int;
    fn tigrKeyHeld(bmp: *mut Tigr, key: c_int) -> c_int;
    fn tigrMouse(bmp: *mut Tigr, x: *mut c_int, y: *mut c_int, buttons: *mut c_int);
}

// Platform-specific screen size FFI
// On macOS, getScreenSize is implemented in screen_helper.c (compiled via build.rs)
#[cfg(target_os = "macos")]
extern "C" {
    fn getScreenSize(width: *mut c_int, height: *mut c_int);
}

#[cfg(target_os = "linux")]
extern "C" {
    fn XOpenDisplay(display_name: *const c_char) -> *mut std::ffi::c_void;
    fn XCloseDisplay(display: *mut std::ffi::c_void) -> c_int;
    fn XDefaultScreenOfDisplay(display: *mut std::ffi::c_void) -> *mut std::ffi::c_void;
    fn XWidthOfScreen(screen: *mut std::ffi::c_void) -> c_int;
    fn XHeightOfScreen(screen: *mut std::ffi::c_void) -> c_int;
}

#[cfg(target_os = "windows")]
extern "C" {
    fn GetSystemMetrics(nIndex: c_int) -> c_int;
}

// --- Thread-local storage ---

thread_local! {
    static LAST_KEY: RefCell<i32> = RefCell::new(0);
    // Our own key state tracking for reliable single-press detection
    // Index: 0=RETURN, 1=BACKSPACE, 2=ESCAPE, 3=LEFT, 4=RIGHT, 5=UP, 6=DOWN
    static PREV_KEY_STATE: RefCell<[bool; 7]> = RefCell::new([false; 7]);
}

// --- Public API types ---

#[derive(Clone, Copy, Default, Debug)]
pub struct Color {
    pub R: u8,
    pub G: u8,
    pub B: u8,
    pub A: u8,
}

#[derive(Clone, Copy)]
pub struct Rect {
    pub X: i32,
    pub Y: i32,
    pub Width: i32,
    pub Height: i32,
}

#[derive(Clone, Copy)]
pub struct Window {
    pub handle: i64,    // Tigr*
    pub renderer: i64,  // Same as handle for tigr
    pub width: i32,
    pub height: i32,
    pub running: bool,
}

// --- Helper functions ---

fn color_to_tpixel(c: Color) -> TPixel {
    TPixel { r: c.R, g: c.G, b: c.B, a: c.A }
}

// --- Color constructors ---

pub fn NewColor(r: u8, g: u8, b: u8, a: u8) -> Color {
    Color { R: r, G: g, B: b, A: a }
}

pub fn Black() -> Color { Color { R: 0, G: 0, B: 0, A: 255 } }
pub fn White() -> Color { Color { R: 255, G: 255, B: 255, A: 255 } }
pub fn Red() -> Color { Color { R: 255, G: 0, B: 0, A: 255 } }
pub fn Green() -> Color { Color { R: 0, G: 255, B: 0, A: 255 } }
pub fn Blue() -> Color { Color { R: 0, G: 0, B: 255, A: 255 } }

// --- Rect constructor ---

pub fn NewRect(x: i32, y: i32, width: i32, height: i32) -> Rect {
    Rect { X: x, Y: y, Width: width, Height: height }
}

// --- Window management ---

pub fn CreateWindow(title: String, width: i32, height: i32) -> Window {
    let c_title = CString::new(title).unwrap_or_else(|_| CString::new("Window").unwrap());

    let win = unsafe {
        tigrWindow(width as c_int, height as c_int, c_title.as_ptr(), 0)
    };

    if win.is_null() {
        return Window {
            handle: 0,
            renderer: 0,
            width,
            height,
            running: false,
        };
    }

    Window {
        handle: win as i64,
        renderer: win as i64,
        width,
        height,
        running: true,
    }
}

pub fn CreateWindowFullscreen(title: String, width: i32, height: i32) -> Window {
    let c_title = CString::new(title).unwrap_or_else(|_| CString::new("Window").unwrap());

    // TIGR_FULLSCREEN = 64
    let win = unsafe {
        tigrWindow(width as c_int, height as c_int, c_title.as_ptr(), 64)
    };

    if win.is_null() {
        return Window {
            handle: 0,
            renderer: 0,
            width,
            height,
            running: false,
        };
    }

    Window {
        handle: win as i64,
        renderer: win as i64,
        width,
        height,
        running: true,
    }
}

pub fn CloseWindow(w: Window) {
    if w.handle != 0 {
        unsafe {
            tigrFree(w.handle as *mut Tigr);
        }
    }
}

pub fn IsRunning(w: Window) -> bool {
    w.running
}

pub fn PollEvents(mut w: Window) -> (Window, bool) {
    if w.handle == 0 {
        return (w, false);
    }

    let win = w.handle as *mut Tigr;

    // Check if window should close BEFORE update (in case it was closed last frame)
    let closed = unsafe { tigrClosed(win) };
    if closed != 0 {
        w.running = false;
        return (w, false);
    }

    // Call tigrUpdate to process events and present previous frame
    // This must happen BEFORE checking keys, as events are processed in tigrUpdate
    unsafe { tigrUpdate(win); }

    // Reset last key
    LAST_KEY.with(|k| *k.borrow_mut() = 0);

    // Use tigrReadChar for character input
    // Note: Ignore '\n' (10) as some systems send CRLF for Enter - we use '\r' (13) only
    let ch = unsafe { tigrReadChar(win) };
    if ch > 0 && ch < 128 && ch != 10 {
        LAST_KEY.with(|k| *k.borrow_mut() = ch);
    }

    // Also check for DEL (127) which some systems use for backspace
    if ch == 127 {
        LAST_KEY.with(|k| *k.borrow_mut() = 8);  // Normalize to backspace
    }

    // Check special keys that don't produce characters
    // Use our own state tracking for reliable single-press detection
    // ALWAYS read current state and update prev_state to avoid double-detection
    PREV_KEY_STATE.with(|prev| {
        let mut prev_state = prev.borrow_mut();
        let curr_state: [bool; 7] = unsafe {[
            tigrKeyHeld(win, TK_RETURN) != 0,
            tigrKeyHeld(win, TK_BACKSPACE) != 0,
            tigrKeyHeld(win, TK_ESCAPE) != 0,
            tigrKeyHeld(win, TK_LEFT) != 0,
            tigrKeyHeld(win, TK_RIGHT) != 0,
            tigrKeyHeld(win, TK_UP) != 0,
            tigrKeyHeld(win, TK_DOWN) != 0,
        ]};

        // Only detect key press if no character was read via tigrReadChar
        LAST_KEY.with(|k| {
            if *k.borrow() == 0 {
                // Detect key press (transition from not pressed to pressed)
                // Note: TK_RETURN (index 0) is NOT detected here - Enter is handled solely via tigrReadChar
                // to avoid double-detection due to timing differences between tigrReadChar and tigrKeyHeld
                if curr_state[1] && !prev_state[1] {
                    *k.borrow_mut() = 8;
                } else if curr_state[2] && !prev_state[2] {
                    *k.borrow_mut() = 27;
                } else if curr_state[3] && !prev_state[3] {
                    *k.borrow_mut() = 256;
                } else if curr_state[4] && !prev_state[4] {
                    *k.borrow_mut() = 257;
                } else if curr_state[5] && !prev_state[5] {
                    *k.borrow_mut() = 258;
                } else if curr_state[6] && !prev_state[6] {
                    *k.borrow_mut() = 259;
                }
            }
        });

        // ALWAYS update previous state to prevent double-detection
        *prev_state = curr_state;
    });

    // Check if window was closed during event processing
    let closed = unsafe { tigrClosed(win) };
    if closed != 0 {
        w.running = false;
        return (w, false);
    }

    (w, true)
}

pub fn GetLastKey() -> i32 {
    LAST_KEY.with(|k| *k.borrow())
}

pub fn GetMouse(w: Window) -> (i32, i32, i32) {
    if w.handle != 0 {
        unsafe {
            let mut x: c_int = 0;
            let mut y: c_int = 0;
            let mut buttons: c_int = 0;
            tigrMouse(w.handle as *mut Tigr, &mut x, &mut y, &mut buttons);
            return (x as i32, y as i32, buttons as i32);
        }
    }
    (0, 0, 0)
}

pub fn GetWidth(w: Window) -> i32 { w.width }
pub fn GetHeight(w: Window) -> i32 { w.height }

/// GetScreenSize returns the actual screen resolution
pub fn GetScreenSize() -> (i32, i32) {
    #[cfg(target_os = "macos")]
    {
        unsafe {
            let mut w: c_int = 1024;
            let mut h: c_int = 768;
            getScreenSize(&mut w, &mut h);
            return (w, h);
        }
    }
    #[cfg(target_os = "windows")]
    {
        const SM_CXSCREEN: c_int = 0;
        const SM_CYSCREEN: c_int = 1;
        unsafe {
            return (GetSystemMetrics(SM_CXSCREEN), GetSystemMetrics(SM_CYSCREEN));
        }
    }
    #[cfg(target_os = "linux")]
    {
        unsafe {
            let dpy = XOpenDisplay(std::ptr::null());
            if !dpy.is_null() {
                let scr = XDefaultScreenOfDisplay(dpy);
                let w = XWidthOfScreen(scr);
                let h = XHeightOfScreen(scr);
                XCloseDisplay(dpy);
                return (w, h);
            }
        }
        return (1024, 768);
    }
    #[cfg(not(any(target_os = "macos", target_os = "windows", target_os = "linux")))]
    {
        (1024, 768)
    }
}

// --- Rendering ---

pub fn Clear(w: Window, c: Color) {
    if w.handle != 0 {
        unsafe {
            tigrClear(w.handle as *mut Tigr, color_to_tpixel(c));
        }
    }
}

pub fn Present(_w: Window) {
    // tigrUpdate is called in PollEvents to ensure events are processed before key checks.
    // The actual rendering/present happens there. This function exists for API compatibility.
}

// --- Drawing primitives ---

pub fn DrawRect(w: Window, rect: Rect, c: Color) {
    if w.handle != 0 {
        unsafe {
            tigrRect(
                w.handle as *mut Tigr,
                rect.X as c_int,
                rect.Y as c_int,
                rect.Width as c_int,
                rect.Height as c_int,
                color_to_tpixel(c),
            );
        }
    }
}

pub fn FillRect(w: Window, rect: Rect, c: Color) {
    if w.handle != 0 {
        unsafe {
            tigrFillRect(
                w.handle as *mut Tigr,
                rect.X as c_int,
                rect.Y as c_int,
                rect.Width as c_int,
                rect.Height as c_int,
                color_to_tpixel(c),
            );
        }
    }
}

pub fn DrawLine(w: Window, x1: i32, y1: i32, x2: i32, y2: i32, c: Color) {
    if w.handle != 0 {
        unsafe {
            tigrLine(
                w.handle as *mut Tigr,
                x1 as c_int,
                y1 as c_int,
                x2 as c_int,
                y2 as c_int,
                color_to_tpixel(c),
            );
        }
    }
}

pub fn DrawPoint(w: Window, x: i32, y: i32, c: Color) {
    if w.handle != 0 {
        unsafe {
            tigrPlot(w.handle as *mut Tigr, x as c_int, y as c_int, color_to_tpixel(c));
        }
    }
}

pub fn DrawCircle(w: Window, centerX: i32, centerY: i32, radius: i32, c: Color) {
    if w.handle != 0 {
        unsafe {
            tigrCircle(
                w.handle as *mut Tigr,
                centerX as c_int,
                centerY as c_int,
                radius as c_int,
                color_to_tpixel(c),
            );
        }
    }
}

pub fn FillCircle(w: Window, centerX: i32, centerY: i32, radius: i32, c: Color) {
    if w.handle != 0 {
        unsafe {
            tigrFillCircle(
                w.handle as *mut Tigr,
                centerX as c_int,
                centerY as c_int,
                radius as c_int,
                color_to_tpixel(c),
            );
        }
    }
}

/// RunLoop runs the main loop, calling frameFunc each frame.
/// frameFunc receives the window and returns true to continue, false to stop.
/// This is the preferred way to write cross-platform graphics code that works in browsers.
pub fn RunLoop<F>(mut w: Window, mut frameFunc: F)
where
    F: FnMut(Window) -> bool,
{
    loop {
        let running: bool;
        (w, running) = PollEvents(w);
        if !running {
            break;
        }
        if !frameFunc(w) {
            break;
        }
    }
}

/// RunLoopWithState runs the main loop with explicit state threading.
/// The state is passed to frameFunc and returned each frame, avoiding closure capture.
/// This eliminates the need to clone state in FnMut closures.
/// frameFunc receives the window and state, returns updated state and true to continue.
pub fn RunLoopWithState<S, F>(mut w: Window, mut state: S, mut frameFunc: F)
where
    F: FnMut(Window, S) -> (S, bool),
{
    loop {
        let running: bool;
        (w, running) = PollEvents(w);
        if !running {
            break;
        }
        let cont: bool;
        (state, cont) = frameFunc(w, state);
        if !cont {
            break;
        }
    }
}
