//go:build sdl2

// Package graphics provides a cross-platform 2D graphics API.
// This package is designed to be transpiled to C++, C#, and Rust.
//
// For Go execution, this uses CGO to bind to the graphics library.
// This file uses the SDL2 backend. Build with: go build -tags sdl2
package graphics

import sdl "runtime/graphics/go/sdl2"

// Window represents a graphics window with an SDL2 renderer.
type Window struct {
	handle   int64 // Pointer to SDL_Window
	renderer int64 // Pointer to SDL_Renderer
	width    int32
	height   int32
	running  bool
}

// Color represents an RGBA color.
type Color struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

// Rect represents a rectangle.
type Rect struct {
	X      int32
	Y      int32
	Width  int32
	Height int32
}

// --- Color constructors ---

func NewColor(r uint8, g uint8, b uint8, a uint8) Color {
	return Color{R: r, G: g, B: b, A: a}
}

func Black() Color {
	return Color{R: 0, G: 0, B: 0, A: 255}
}

func White() Color {
	return Color{R: 255, G: 255, B: 255, A: 255}
}

func Red() Color {
	return Color{R: 255, G: 0, B: 0, A: 255}
}

func Green() Color {
	return Color{R: 0, G: 255, B: 0, A: 255}
}

func Blue() Color {
	return Color{R: 0, G: 0, B: 255, A: 255}
}

// --- Rect constructor ---

func NewRect(x int32, y int32, width int32, height int32) Rect {
	return Rect{X: x, Y: y, Width: width, Height: height}
}

// --- Window management ---

// CreateWindow creates a new window with the specified title and dimensions.
func CreateWindow(title string, width int32, height int32) Window {
	handle, renderer, ok := sdl.CreateWindow(title, width, height)
	if !ok {
		return Window{width: width, height: height, running: false}
	}
	return Window{
		handle:   handle,
		renderer: renderer,
		width:    width,
		height:   height,
		running:  true,
	}
}

// CloseWindow closes the window and releases resources.
func CloseWindow(w Window) {
	sdl.CloseWindow(w.handle, w.renderer)
}

// IsRunning returns true if the window is still open.
func IsRunning(w Window) bool {
	return w.running
}

// PollEvents processes pending events and returns updated window and false if quit requested.
func PollEvents(w Window) (Window, bool) {
	if sdl.PollEvents(w.handle) {
		w.running = false
		return w, false
	}
	return w, true
}

// GetLastKey returns the ASCII code of the last key pressed (0 if none).
// Must be called after PollEvents to get the key from the current frame.
func GetLastKey() int {
	return sdl.GetLastKey()
}

// GetMouse returns mouse position and button state.
// Returns: x, y, buttons (bit 0=left, bit 1=right, bit 2=middle)
func GetMouse(w Window) (int32, int32, int32) {
	return sdl.GetMouse(w.handle)
}

// GetWidth returns the window width.
func GetWidth(w Window) int32 {
	return w.width
}

// GetHeight returns the window height.
func GetHeight(w Window) int32 {
	return w.height
}

// GetScreenSize returns the screen resolution (SDL2 cross-platform).
func GetScreenSize() (int32, int32) {
	return sdl.GetScreenSize()
}

// CreateWindowFullscreen creates a fullscreen window.
func CreateWindowFullscreen(title string, width int32, height int32) Window {
	handle, renderer, ok := sdl.CreateWindowFullscreen(title, width, height)
	if !ok {
		return Window{width: width, height: height, running: false}
	}
	return Window{
		handle:   handle,
		renderer: renderer,
		width:    width,
		height:   height,
		running:  true,
	}
}

// --- Rendering ---

// Clear clears the window with the specified color.
func Clear(w Window, c Color) {
	sdl.Clear(w.renderer, c.R, c.G, c.B, c.A)
}

// Present presents the rendered content to the screen.
func Present(w Window) {
	sdl.Present(w.renderer)
}

// --- Drawing primitives ---

// DrawRect draws a rectangle outline.
func DrawRect(w Window, rect Rect, c Color) {
	sdl.DrawRect(w.renderer, rect.X, rect.Y, rect.Width, rect.Height, c.R, c.G, c.B, c.A)
}

// FillRect draws a filled rectangle.
func FillRect(w Window, rect Rect, c Color) {
	sdl.FillRect(w.renderer, rect.X, rect.Y, rect.Width, rect.Height, c.R, c.G, c.B, c.A)
}

// DrawLine draws a line between two points.
func DrawLine(w Window, x1 int32, y1 int32, x2 int32, y2 int32, c Color) {
	sdl.DrawLine(w.renderer, x1, y1, x2, y2, c.R, c.G, c.B, c.A)
}

// DrawPoint draws a single pixel.
func DrawPoint(w Window, x int32, y int32, c Color) {
	sdl.DrawPoint(w.renderer, x, y, c.R, c.G, c.B, c.A)
}

// DrawCircle draws a circle outline.
func DrawCircle(w Window, centerX int32, centerY int32, radius int32, c Color) {
	sdl.DrawCircle(w.renderer, centerX, centerY, radius, c.R, c.G, c.B, c.A)
}

// FillCircle draws a filled circle.
func FillCircle(w Window, centerX int32, centerY int32, radius int32, c Color) {
	sdl.FillCircle(w.renderer, centerX, centerY, radius, c.R, c.G, c.B, c.A)
}

// RunLoop runs the main loop, calling frameFunc each frame.
// frameFunc receives the window and returns true to continue, false to stop.
// This is the preferred way to write cross-platform graphics code that works in browsers.
func RunLoop(w Window, frameFunc func(Window) bool) {
	for {
		var running bool
		w, running = PollEvents(w)
		if !running {
			break
		}
		if !frameFunc(w) {
			break
		}
	}
}

// RunLoopWithState runs the main loop with explicit state threading.
// The state is passed to frameFunc and returned each frame, avoiding closure capture.
// This eliminates the need to clone state in Rust's FnMut closures.
// frameFunc receives the window and state, returns updated state and true to continue.
func RunLoopWithState[S any](w Window, state S, frameFunc func(Window, S) (S, bool)) {
	for {
		var running bool
		w, running = PollEvents(w)
		if !running {
			break
		}
		var cont bool
		state, cont = frameFunc(w, state)
		if !cont {
			break
		}
	}
}
