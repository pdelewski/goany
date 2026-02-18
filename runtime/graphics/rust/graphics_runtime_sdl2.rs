// graphics_runtime.rs - SDL2 runtime for goany graphics package
// Requires: sdl2 crate in Cargo.toml

use sdl2::event::Event;
use sdl2::keyboard::Keycode;
use sdl2::pixels::Color as SdlColor;
use sdl2::rect::Rect as SdlRect;
use sdl2::render::Canvas;
use sdl2::video::Window as SdlWindow;
use sdl2::Sdl;
use std::cell::RefCell;
use std::collections::HashMap;

// Thread-local storage for SDL context and canvases
thread_local! {
    static SDL_CONTEXT: RefCell<Option<Sdl>> = RefCell::new(None);
    static CANVASES: RefCell<HashMap<i64, Canvas<SdlWindow>>> = RefCell::new(HashMap::new());
    static NEXT_HANDLE: RefCell<i64> = RefCell::new(1);
    static LAST_KEY: RefCell<i32> = RefCell::new(0);
}

#[derive(Clone, Copy, Default, Debug, Hash, PartialEq, Eq)]
pub struct Color {
    pub R: u8,
    pub G: u8,
    pub B: u8,
    pub A: u8,
}

#[derive(Clone, Copy, Default, Debug, Hash, PartialEq, Eq)]
pub struct Rect {
    pub X: i32,
    pub Y: i32,
    pub Width: i32,
    pub Height: i32,
}

#[derive(Clone, Copy, Default, Debug, Hash, PartialEq, Eq)]
pub struct Window {
    pub handle: i64,
    pub renderer: i64,
    pub width: i32,
    pub height: i32,
    pub running: bool,
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
    SDL_CONTEXT.with(|ctx| {
        let mut ctx_ref = ctx.borrow_mut();
        if ctx_ref.is_none() {
            *ctx_ref = sdl2::init().ok();
        }

        if let Some(ref sdl) = *ctx_ref {
            let video = match sdl.video() {
                Ok(v) => v,
                Err(_) => return Window { handle: 0, renderer: 0, width, height, running: false },
            };

            let window = match video
                .window(&title, width as u32, height as u32)
                .position_centered()
                .build()
            {
                Ok(w) => w,
                Err(_) => return Window { handle: 0, renderer: 0, width, height, running: false },
            };

            let canvas = match window.into_canvas().accelerated().present_vsync().build() {
                Ok(c) => c,
                Err(_) => return Window { handle: 0, renderer: 0, width, height, running: false },
            };

            let handle = NEXT_HANDLE.with(|h| {
                let mut h_ref = h.borrow_mut();
                let current = *h_ref;
                *h_ref += 1;
                current
            });

            CANVASES.with(|c| {
                c.borrow_mut().insert(handle, canvas);
            });

            Window {
                handle,
                renderer: handle,
                width,
                height,
                running: true,
            }
        } else {
            Window { handle: 0, renderer: 0, width, height, running: false }
        }
    })
}

pub fn CloseWindow(mut w: Window) {
    CANVASES.with(|c| {
        c.borrow_mut().remove(&w.handle);
    });
}

pub fn IsRunning(mut w: Window) -> bool {
    w.running
}

pub fn PollEvents(mut w: Window) -> (Window, bool) {
    LAST_KEY.with(|k| *k.borrow_mut() = 0);
    SDL_CONTEXT.with(|ctx| {
        if let Some(ref sdl) = *ctx.borrow() {
            let mut event_pump = match sdl.event_pump() {
                Ok(ep) => ep,
                Err(_) => return (w, true),
            };

            for event in event_pump.poll_iter() {
                match event {
                    Event::Quit { .. } => {
                        w.running = false;
                        return (w, false);
                    }
                    Event::KeyDown { keycode: Some(key), keymod, .. } => {
                        let ascii = match key {
                            Keycode::Return => 13,
                            Keycode::Backspace => 8,
                            Keycode::Space => 32,
                            k if k as i32 >= Keycode::A as i32 && k as i32 <= Keycode::Z as i32 => {
                                let base = k as i32 - Keycode::A as i32;
                                if keymod.contains(sdl2::keyboard::Mod::LSHIFTMOD) ||
                                   keymod.contains(sdl2::keyboard::Mod::RSHIFTMOD) {
                                    65 + base // Uppercase A-Z
                                } else {
                                    97 + base // Lowercase a-z
                                }
                            }
                            k if k as i32 >= Keycode::Num0 as i32 && k as i32 <= Keycode::Num9 as i32 => {
                                48 + (k as i32 - Keycode::Num0 as i32)
                            }
                            _ => 0,
                        };
                        if ascii != 0 {
                            LAST_KEY.with(|k| *k.borrow_mut() = ascii);
                        }
                    }
                    _ => {}
                }
            }
        }
        (w, true)
    })
}

pub fn GetLastKey() -> i32 {
    LAST_KEY.with(|k| *k.borrow())
}

/// GetMouse returns mouse position and button state
pub fn GetMouse(w: Window) -> (i32, i32, i32) {
    SDL_CONTEXT.with(|ctx| {
        if let Some(ref sdl) = *ctx.borrow() {
            if let Ok(event_pump) = sdl.event_pump() {
                let state = event_pump.mouse_state();
                let mut buttons: i32 = 0;
                if state.left() { buttons |= 1; }
                if state.right() { buttons |= 2; }
                if state.middle() { buttons |= 4; }
                return (state.x(), state.y(), buttons);
            }
        }
        (0, 0, 0)
    })
}

pub fn GetWidth(mut w: Window) -> i32 { w.width }
pub fn GetHeight(mut w: Window) -> i32 { w.height }

/// GetScreenSize returns the screen resolution (SDL2 cross-platform)
pub fn GetScreenSize() -> (i32, i32) {
    SDL_CONTEXT.with(|ctx| {
        let mut ctx_ref = ctx.borrow_mut();
        if ctx_ref.is_none() {
            *ctx_ref = sdl2::init().ok();
        }
        if let Some(ref sdl) = *ctx_ref {
            if let Ok(video) = sdl.video() {
                if let Ok(mode) = video.current_display_mode(0) {
                    return (mode.w, mode.h);
                }
            }
        }
        (1920, 1080)
    })
}

/// CreateWindowFullscreen creates a fullscreen window
pub fn CreateWindowFullscreen(title: String, width: i32, height: i32) -> Window {
    SDL_CONTEXT.with(|ctx| {
        let mut ctx_ref = ctx.borrow_mut();
        if ctx_ref.is_none() {
            *ctx_ref = sdl2::init().ok();
        }

        if let Some(ref sdl) = *ctx_ref {
            let video = match sdl.video() {
                Ok(v) => v,
                Err(_) => return Window { handle: 0, renderer: 0, width, height, running: false },
            };

            let window = match video
                .window(&title, width as u32, height as u32)
                .position_centered()
                .fullscreen_desktop()
                .build()
            {
                Ok(w) => w,
                Err(_) => return Window { handle: 0, renderer: 0, width, height, running: false },
            };

            let canvas = match window.into_canvas().accelerated().present_vsync().build() {
                Ok(c) => c,
                Err(_) => return Window { handle: 0, renderer: 0, width, height, running: false },
            };

            let handle = NEXT_HANDLE.with(|h| {
                let mut h_ref = h.borrow_mut();
                let current = *h_ref;
                *h_ref += 1;
                current
            });

            CANVASES.with(|c| {
                c.borrow_mut().insert(handle, canvas);
            });

            return Window {
                handle,
                renderer: handle,
                width,
                height,
                running: true,
            };
        }
        Window { handle: 0, renderer: 0, width, height, running: false }
    })
}

// --- Rendering ---

pub fn Clear(mut w: Window, mut c: Color) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.set_draw_color(SdlColor::RGBA(c.R, c.G, c.B, c.A));
            canvas.clear();
        }
    });
}

pub fn Present(mut w: Window) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.present();
        }
    });
}

// --- Drawing primitives ---

pub fn DrawRect(mut w: Window, mut rect: Rect, mut c: Color) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.set_draw_color(SdlColor::RGBA(c.R, c.G, c.B, c.A));
            let _ = canvas.draw_rect(SdlRect::new(rect.X, rect.Y, rect.Width as u32, rect.Height as u32));
        }
    });
}

pub fn FillRect(mut w: Window, mut rect: Rect, mut c: Color) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.set_draw_color(SdlColor::RGBA(c.R, c.G, c.B, c.A));
            let _ = canvas.fill_rect(SdlRect::new(rect.X, rect.Y, rect.Width as u32, rect.Height as u32));
        }
    });
}

pub fn DrawLine(mut w: Window, x1: i32, y1: i32, x2: i32, y2: i32, mut c: Color) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.set_draw_color(SdlColor::RGBA(c.R, c.G, c.B, c.A));
            let _ = canvas.draw_line((x1, y1), (x2, y2));
        }
    });
}

pub fn DrawPoint(mut w: Window, x: i32, y: i32, mut c: Color) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.set_draw_color(SdlColor::RGBA(c.R, c.G, c.B, c.A));
            let _ = canvas.draw_point((x, y));
        }
    });
}

pub fn DrawCircle(mut w: Window, centerX: i32, centerY: i32, radius: i32, mut c: Color) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.set_draw_color(SdlColor::RGBA(c.R, c.G, c.B, c.A));

            // Bresenham's circle algorithm
            let mut x = radius;
            let mut y = 0;
            let mut err = 0;

            while x >= y {
                let _ = canvas.draw_point((centerX + x, centerY + y));
                let _ = canvas.draw_point((centerX + y, centerY + x));
                let _ = canvas.draw_point((centerX - y, centerY + x));
                let _ = canvas.draw_point((centerX - x, centerY + y));
                let _ = canvas.draw_point((centerX - x, centerY - y));
                let _ = canvas.draw_point((centerX - y, centerY - x));
                let _ = canvas.draw_point((centerX + y, centerY - x));
                let _ = canvas.draw_point((centerX + x, centerY - y));

                y += 1;
                err += 1 + 2 * y;
                if 2 * (err - x) + 1 > 0 {
                    x -= 1;
                    err += 1 - 2 * x;
                }
            }
        }
    });
}

pub fn FillCircle(mut w: Window, centerX: i32, centerY: i32, radius: i32, mut c: Color) {
    CANVASES.with(|canvases| {
        if let Some(canvas) = canvases.borrow_mut().get_mut(&w.handle) {
            canvas.set_draw_color(SdlColor::RGBA(c.R, c.G, c.B, c.A));

            for y in -radius..=radius {
                for x in -radius..=radius {
                    if x * x + y * y <= radius * radius {
                        let _ = canvas.draw_point((centerX + x, centerY + y));
                    }
                }
            }
        }
    });
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
