package gui

import graphics "runtime/graphics"

// --- Core Types ---

// GuiContext holds minimal state for immediate-mode GUI
type GuiContext struct {
	// Input state (updated each frame)
	MouseX        int32
	MouseY        int32
	MouseDown     bool
	MouseClicked  bool
	MouseReleased bool

	// Widget interaction tracking
	HotID      int32
	ActiveID   int32
	ReleasedID int32 // ID that was active when mouse was released

	// Layout cursor
	CursorX int32
	CursorY int32
	Spacing int32

	// Current style
	Style GuiStyle

	// Z-order management
	WindowZOrder []int32 // Window IDs in back-to-front render order
	RenderPass   int32   // Current render pass index
	DrawContent  bool    // Set by DraggablePanel: draw this window's content?
	ZOrderReady  bool    // True after first frame (bootstrap flag)
}

// WindowState holds position and drag state for a draggable window
type WindowState struct {
	X           int32
	Y           int32
	Width       int32
	Height      int32
	Dragging    bool
	DragOffsetX int32
	DragOffsetY int32
}

// NewWindowState creates a new window state at the given position
func NewWindowState(x int32, y int32, width int32, height int32) WindowState {
	return WindowState{
		X:      x,
		Y:      y,
		Width:  width,
		Height: height,
	}
}

// MenuState holds state for menu bar and dropdowns
type MenuState struct {
	OpenMenuID   int32 // ID of currently open menu (0 = none)
	MenuBarX     int32 // Menu bar position for dropdown alignment
	MenuBarY     int32
	MenuBarH     int32 // Menu bar height
	CurrentMenuX int32 // X position of current menu header
	CurrentMenuW int32 // Width of current menu header
	ClickedOutside bool // Flag to close menu on outside click
}

// NewMenuState creates a new menu state
func NewMenuState() MenuState {
	return MenuState{}
}

// GuiStyle defines colors and dimensions for widgets
type GuiStyle struct {
	// Colors
	BackgroundColor   graphics.Color
	TextColor         graphics.Color
	ButtonColor       graphics.Color
	ButtonHoverColor  graphics.Color
	ButtonActiveColor graphics.Color
	CheckboxColor     graphics.Color
	CheckmarkColor    graphics.Color
	SliderTrackColor  graphics.Color
	SliderKnobColor   graphics.Color
	BorderColor       graphics.Color
	FrameBgColor      graphics.Color // Background for input widgets
	TitleBgColor      graphics.Color // Panel/window title background

	// Dimensions
	FontSize     int32
	Padding      int32
	ButtonHeight int32
	SliderHeight int32
	CheckboxSize int32
	FrameRounding int32 // Visual depth simulation
}

// --- Initialization ---

// DefaultStyle returns a modern, polished dark theme style
func DefaultStyle() GuiStyle {
	return GuiStyle{
		// Modern dark theme with better contrast and softer colors
		BackgroundColor:   graphics.NewColor(45, 45, 48, 250),    // Window body - neutral gray
		TextColor:         graphics.NewColor(240, 240, 245, 255), // Soft white text
		ButtonColor:       graphics.NewColor(90, 90, 95, 200),    // Button normal - gray
		ButtonHoverColor:  graphics.NewColor(110, 110, 115, 230), // Button hover - lighter gray
		ButtonActiveColor: graphics.NewColor(70, 70, 75, 255),    // Button pressed - darker gray
		CheckboxColor:     graphics.NewColor(60, 60, 65, 255),    // Checkbox/frame bg - gray
		CheckmarkColor:    graphics.NewColor(200, 200, 210, 255), // Checkmark - light gray
		SliderTrackColor:  graphics.NewColor(130, 130, 140, 220), // Slider filled - medium gray
		SliderKnobColor:   graphics.NewColor(180, 180, 190, 255), // Slider grab - light gray
		BorderColor:       graphics.NewColor(65, 65, 70, 255),    // Subtle borders
		FrameBgColor:      graphics.NewColor(50, 50, 55, 220),    // Input field bg - dark gray
		TitleBgColor:      graphics.NewColor(75, 75, 80, 255),    // Title bar - neutral gray
		FontSize:          1,
		Padding:           8,
		ButtonHeight:      24,
		SliderHeight:      20,
		CheckboxSize:      18,
		FrameRounding:     3,
	}
}

// NewContext creates a new GUI context with default style
func NewContext() GuiContext {
	return GuiContext{
		Style: DefaultStyle(),
	}
}

// --- Widget ID Generation ---

// GenID generates a unique ID for a widget based on its label
// Uses simple hash for portability across all backends
func GenID(label string) int32 {
	hash := int32(5381) // djb2 hash starting value
	for i := 0; i < len(label); i++ {
		hash = ((hash << 5) + hash) + int32(label[i])
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// --- Input Handling ---

// UpdateInput must be called once per frame before any GUI widgets
// Returns updated context (goany doesn't support pointers)
func UpdateInput(ctx GuiContext, w graphics.Window) GuiContext {
	// Store previous state
	prevDown := ctx.MouseDown

	// Get current mouse state
	x, y, buttons := graphics.GetMouse(w)
	ctx.MouseX = x
	ctx.MouseY = y
	ctx.MouseDown = (buttons & 1) != 0

	// Detect click/release events
	ctx.MouseClicked = ctx.MouseDown && !prevDown
	ctx.MouseReleased = !ctx.MouseDown && prevDown

	// Save which widget was active when mouse released (for click detection)
	ctx.ReleasedID = 0
	if ctx.MouseReleased {
		ctx.ReleasedID = ctx.ActiveID
		ctx.ActiveID = 0
	}

	// Reset hot ID each frame
	ctx.HotID = 0

	return ctx
}

// --- Text Rendering ---

// drawChar renders a single character from the bitmap font
func drawChar(w graphics.Window, charCode int, x int32, y int32, scale int32, color graphics.Color) {
	if charCode < 32 || charCode > 127 {
		charCode = 32
	}
	offset := (charCode - 32) * 8
	font := getFontData()
	for row := int32(0); row < 8; row++ {
		rowData := font[offset+int(row)]
		for col := int32(0); col < 8; col++ {
			if (rowData & (0x80 >> col)) != 0 {
				// Draw scaled pixel
				for sy := int32(0); sy < scale; sy++ {
					for sx := int32(0); sx < scale; sx++ {
						graphics.DrawPoint(w, x+col*scale+sx, y+row*scale+sy, color)
					}
				}
			}
		}
	}
}

// DrawText renders a string at (x, y) with the given color
// scale: 1 = 8px, 2 = 16px, etc.
func DrawText(w graphics.Window, text string, x int32, y int32, scale int32, color graphics.Color) {
	curX := x
	for i := 0; i < len(text); i++ {
		ch := int(text[i])
		drawChar(w, ch, curX, y, scale, color)
		curX = curX + (8 * scale)
	}
}

// TextWidth returns the width in pixels of a text string
func TextWidth(text string, scale int32) int32 {
	return int32(len(text)) * 8 * scale
}

// TextHeight returns the height in pixels (fixed for bitmap font)
func TextHeight(scale int32) int32 {
	return 8 * scale
}

// --- Helper Functions ---

// pointInRect checks if a point is inside a rectangle
func pointInRect(px int32, py int32, x int32, y int32, w int32, h int32) bool {
	return px >= x && px < x+w && py >= y && py < y+h
}

// --- Z-Order Management ---

// registerWindow adds a window ID to the z-order if not already present
func registerWindow(ctx GuiContext, id int32) GuiContext {
	i := 0
	for i < len(ctx.WindowZOrder) {
		if ctx.WindowZOrder[i] == id {
			return ctx
		}
		i = i + 1
	}
	ctx.WindowZOrder = append(ctx.WindowZOrder, id)
	return ctx
}

// bringWindowToFront moves the given window ID to the front (end) of the z-order
func bringWindowToFront(ctx GuiContext, id int32) GuiContext {
	newOrder := []int32{}
	i := 0
	for i < len(ctx.WindowZOrder) {
		if ctx.WindowZOrder[i] != id {
			newOrder = append(newOrder, ctx.WindowZOrder[i])
		}
		i = i + 1
	}
	newOrder = append(newOrder, id)
	ctx.WindowZOrder = newOrder
	return ctx
}

// WindowPassCount returns the number of render passes needed for z-ordered windows
func WindowPassCount(ctx GuiContext) int32 {
	if !ctx.ZOrderReady || len(ctx.WindowZOrder) == 0 {
		return 1
	}
	return int32(len(ctx.WindowZOrder))
}

// BeginWindowPass sets up the context for a specific render pass
func BeginWindowPass(ctx GuiContext, pass int32) GuiContext {
	ctx.RenderPass = pass
	ctx.DrawContent = false
	return ctx
}

// EndWindowPasses marks z-order as ready after the first full frame
func EndWindowPasses(ctx GuiContext) GuiContext {
	if !ctx.ZOrderReady && len(ctx.WindowZOrder) > 0 {
		ctx.ZOrderReady = true
	}
	return ctx
}

// --- Widgets ---

// Label draws static text
func Label(ctx GuiContext, w graphics.Window, text string, x int32, y int32) {
	DrawText(w, text, x, y, ctx.Style.FontSize, ctx.Style.TextColor)
}

// Button returns updated context and true if the button was clicked this frame
func Button(ctx GuiContext, w graphics.Window, label string, x int32, y int32, width int32, height int32) (GuiContext, bool) {
	id := GenID(label)

	// Hit test
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, x, y, width, height)

	// Update hot/active state
	if hovered {
		ctx.HotID = id
		if ctx.MouseClicked {
			ctx.ActiveID = id
		}
	}

	// Determine visual state and offset for press effect
	var bgColor graphics.Color
	pressOffset := int32(0)
	if ctx.ActiveID == id && hovered {
		bgColor = ctx.Style.ButtonActiveColor
		pressOffset = 1
	} else if ctx.HotID == id {
		bgColor = ctx.Style.ButtonHoverColor
	} else {
		bgColor = ctx.Style.ButtonColor
	}

	// Draw button shadow (offset dark rectangle for depth)
	graphics.FillRect(w, graphics.NewRect(x+2, y+2, width, height), graphics.NewColor(0, 0, 0, 70))

	// Main button fill
	graphics.FillRect(w, graphics.NewRect(x, y, width, height), bgColor)

	// Convex effect - bright top edge (multiple lines for gradient)
	graphics.DrawLine(w, x+1, y+1, x+width-2, y+1, graphics.NewColor(255, 255, 255, 100))
	graphics.DrawLine(w, x+1, y+2, x+width-2, y+2, graphics.NewColor(255, 255, 255, 60))
	graphics.DrawLine(w, x+1, y+3, x+width-2, y+3, graphics.NewColor(255, 255, 255, 30))

	// Convex effect - bright left edge
	graphics.DrawLine(w, x+1, y+1, x+1, y+height-2, graphics.NewColor(255, 255, 255, 80))
	graphics.DrawLine(w, x+2, y+2, x+2, y+height-3, graphics.NewColor(255, 255, 255, 40))

	// Convex effect - dark bottom edge (multiple lines for gradient)
	graphics.DrawLine(w, x+2, y+height-1, x+width-1, y+height-1, graphics.NewColor(0, 0, 0, 120))
	graphics.DrawLine(w, x+2, y+height-2, x+width-2, y+height-2, graphics.NewColor(0, 0, 0, 70))
	graphics.DrawLine(w, x+2, y+height-3, x+width-2, y+height-3, graphics.NewColor(0, 0, 0, 30))

	// Convex effect - dark right edge
	graphics.DrawLine(w, x+width-1, y+2, x+width-1, y+height-1, graphics.NewColor(0, 0, 0, 120))
	graphics.DrawLine(w, x+width-2, y+3, x+width-2, y+height-2, graphics.NewColor(0, 0, 0, 60))

	// Outer border
	graphics.DrawRect(w, graphics.NewRect(x, y, width, height), graphics.NewColor(30, 30, 35, 255))

	// Draw centered label with press offset
	textW := TextWidth(label, ctx.Style.FontSize)
	textH := TextHeight(ctx.Style.FontSize)
	textX := x + (width-textW)/2 + pressOffset
	textY := y + (height-textH)/2 + pressOffset
	DrawText(w, label, textX, textY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Return context and true if clicked (was active when mouse released while hovered)
	clicked := ctx.ReleasedID == id && ctx.MouseReleased && hovered
	return ctx, clicked
}

// Checkbox draws a checkbox and returns updated context and new value
func Checkbox(ctx GuiContext, w graphics.Window, label string, x int32, y int32, value bool) (GuiContext, bool) {
	id := GenID(label)
	boxSize := ctx.Style.CheckboxSize

	// Hit test (checkbox + label area)
	labelW := TextWidth(label, ctx.Style.FontSize)
	totalW := boxSize + ctx.Style.Padding + labelW
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, x, y, totalW, boxSize)

	// Update hot/active state
	if hovered {
		ctx.HotID = id
		if ctx.MouseClicked {
			ctx.ActiveID = id
		}
	}

	// Determine visual state
	var boxColor graphics.Color
	if ctx.HotID == id {
		boxColor = ctx.Style.ButtonHoverColor
	} else {
		boxColor = ctx.Style.FrameBgColor
	}

	// Draw checkbox shadow
	graphics.FillRect(w, graphics.NewRect(x+1, y+1, boxSize, boxSize), graphics.NewColor(0, 0, 0, 60))

	// Draw checkbox frame
	graphics.FillRect(w, graphics.NewRect(x, y, boxSize, boxSize), boxColor)

	// Convex effect - bright top-left edges
	graphics.DrawLine(w, x+1, y+1, x+boxSize-2, y+1, graphics.NewColor(255, 255, 255, 90))
	graphics.DrawLine(w, x+1, y+2, x+boxSize-3, y+2, graphics.NewColor(255, 255, 255, 50))
	graphics.DrawLine(w, x+1, y+1, x+1, y+boxSize-2, graphics.NewColor(255, 255, 255, 70))
	graphics.DrawLine(w, x+2, y+2, x+2, y+boxSize-3, graphics.NewColor(255, 255, 255, 35))

	// Convex effect - dark bottom-right edges
	graphics.DrawLine(w, x+2, y+boxSize-1, x+boxSize-1, y+boxSize-1, graphics.NewColor(0, 0, 0, 100))
	graphics.DrawLine(w, x+3, y+boxSize-2, x+boxSize-2, y+boxSize-2, graphics.NewColor(0, 0, 0, 50))
	graphics.DrawLine(w, x+boxSize-1, y+2, x+boxSize-1, y+boxSize-1, graphics.NewColor(0, 0, 0, 100))
	graphics.DrawLine(w, x+boxSize-2, y+3, x+boxSize-2, y+boxSize-2, graphics.NewColor(0, 0, 0, 50))

	// Border
	graphics.DrawRect(w, graphics.NewRect(x, y, boxSize, boxSize), graphics.NewColor(60, 65, 75, 255))

	// Draw checkmark if checked
	if value {
		checkColor := ctx.Style.CheckmarkColor
		// Draw a thicker, more visible checkmark
		cx := x + boxSize/2
		cy := y + boxSize/2
		// Left part of tick (going down-right) - thicker
		graphics.DrawLine(w, cx-5, cy-1, cx-2, cy+3, checkColor)
		graphics.DrawLine(w, cx-5, cy, cx-2, cy+4, checkColor)
		graphics.DrawLine(w, cx-4, cy, cx-1, cy+4, checkColor)
		// Right part of tick (going up-right) - thicker
		graphics.DrawLine(w, cx-2, cy+3, cx+5, cy-4, checkColor)
		graphics.DrawLine(w, cx-2, cy+4, cx+5, cy-3, checkColor)
		graphics.DrawLine(w, cx-1, cy+4, cx+6, cy-3, checkColor)
	}

	// Draw label
	labelX := x + boxSize + ctx.Style.Padding
	labelY := y + (boxSize-TextHeight(ctx.Style.FontSize))/2
	DrawText(w, label, labelX, labelY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Toggle on click
	newValue := value
	if ctx.ReleasedID == id && ctx.MouseReleased && hovered {
		newValue = !value
	}
	return ctx, newValue
}

// Slider draws a horizontal slider and returns updated context and new value
// value and result are in range [min, max]
func Slider(ctx GuiContext, w graphics.Window, label string, x int32, y int32, width int32, min float64, max float64, value float64) (GuiContext, float64) {
	id := GenID(label)
	height := ctx.Style.SliderHeight
	grabW := int32(12) // Smaller grab handle like ImGui

	// Draw label to the left
	labelW := TextWidth(label, ctx.Style.FontSize)
	labelY := y + (height-TextHeight(ctx.Style.FontSize))/2
	DrawText(w, label, x, labelY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Slider track starts after label
	trackX := x + labelW + ctx.Style.Padding
	trackW := width - labelW - ctx.Style.Padding

	// Clamp value
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}

	// Calculate grab position
	rangeVal := max - min
	if rangeVal == 0 {
		rangeVal = 1
	}
	t := (value - min) / rangeVal
	grabRange := trackW - grabW
	grabX := trackX + int32(float64(grabRange)*t)

	// Hit test (entire slider track area)
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, trackX, y, trackW, height)

	// Update hot/active state
	if hovered {
		ctx.HotID = id
		if ctx.MouseClicked {
			ctx.ActiveID = id
		}
	}

	// Draw track shadow (inset effect)
	graphics.FillRect(w, graphics.NewRect(trackX+1, y+1, trackW, height), graphics.NewColor(0, 0, 0, 40))

	// Draw track background
	graphics.FillRect(w, graphics.NewRect(trackX, y, trackW, height), ctx.Style.FrameBgColor)

	// Inner shadow for inset look
	graphics.DrawLine(w, trackX+1, y+1, trackX+trackW-2, y+1, graphics.NewColor(0, 0, 0, 60))
	graphics.DrawLine(w, trackX+1, y+1, trackX+1, y+height-2, graphics.NewColor(0, 0, 0, 60))

	// Draw filled portion (from start to grab) with gradient effect
	fillW := grabX - trackX + grabW/2
	if fillW > 0 {
		graphics.FillRect(w, graphics.NewRect(trackX+2, y+2, fillW-2, height-4), ctx.Style.SliderTrackColor)
		// Highlight on top of fill
		graphics.DrawLine(w, trackX+2, y+2, trackX+fillW-1, y+2, graphics.NewColor(255, 255, 255, 40))
	}

	// Track border
	graphics.DrawRect(w, graphics.NewRect(trackX, y, trackW, height), graphics.NewColor(50, 55, 65, 255))

	// Draw grab handle with better styling
	var grabColor graphics.Color
	if ctx.ActiveID == id {
		grabColor = ctx.Style.ButtonActiveColor
	} else if ctx.HotID == id {
		grabColor = ctx.Style.ButtonHoverColor
	} else {
		grabColor = ctx.Style.SliderKnobColor
	}
	// Grab handle shadow
	graphics.FillRect(w, graphics.NewRect(grabX+2, y+2, grabW, height), graphics.NewColor(0, 0, 0, 70))
	// Grab handle fill
	graphics.FillRect(w, graphics.NewRect(grabX, y, grabW, height), grabColor)

	// Convex effect - bright top-left
	graphics.DrawLine(w, grabX+1, y+1, grabX+grabW-2, y+1, graphics.NewColor(255, 255, 255, 100))
	graphics.DrawLine(w, grabX+1, y+2, grabX+grabW-3, y+2, graphics.NewColor(255, 255, 255, 50))
	graphics.DrawLine(w, grabX+1, y+1, grabX+1, y+height-2, graphics.NewColor(255, 255, 255, 80))
	graphics.DrawLine(w, grabX+2, y+2, grabX+2, y+height-3, graphics.NewColor(255, 255, 255, 40))

	// Convex effect - dark bottom-right
	graphics.DrawLine(w, grabX+2, y+height-1, grabX+grabW-1, y+height-1, graphics.NewColor(0, 0, 0, 120))
	graphics.DrawLine(w, grabX+3, y+height-2, grabX+grabW-2, y+height-2, graphics.NewColor(0, 0, 0, 60))
	graphics.DrawLine(w, grabX+grabW-1, y+2, grabX+grabW-1, y+height-1, graphics.NewColor(0, 0, 0, 120))
	graphics.DrawLine(w, grabX+grabW-2, y+3, grabX+grabW-2, y+height-2, graphics.NewColor(0, 0, 0, 60))

	// Grab handle border
	graphics.DrawRect(w, graphics.NewRect(grabX, y, grabW, height), graphics.NewColor(40, 45, 55, 255))

	// Handle dragging
	if ctx.ActiveID == id && ctx.MouseDown {
		// Calculate new value from mouse position
		mouseT := float64(ctx.MouseX-trackX-grabW/2) / float64(grabRange)
		if mouseT < 0 {
			mouseT = 0
		}
		if mouseT > 1 {
			mouseT = 1
		}
		value = min + mouseT*rangeVal
	}

	return ctx, value
}

// --- Panels and Frames ---

// Panel draws a panel/window background with title
func Panel(ctx GuiContext, w graphics.Window, title string, x int32, y int32, width int32, height int32) {
	titleH := TextHeight(ctx.Style.FontSize) + ctx.Style.Padding*2

	// Draw drop shadow (multiple layers for soft shadow effect)
	graphics.FillRect(w, graphics.NewRect(x+4, y+4, width, height), graphics.NewColor(0, 0, 0, 40))
	graphics.FillRect(w, graphics.NewRect(x+3, y+3, width, height), graphics.NewColor(0, 0, 0, 50))
	graphics.FillRect(w, graphics.NewRect(x+2, y+2, width, height), graphics.NewColor(0, 0, 0, 60))

	// Draw title bar background
	graphics.FillRect(w, graphics.NewRect(x, y, width, titleH), ctx.Style.TitleBgColor)

	// Convex effect - bright top edge
	graphics.DrawLine(w, x+1, y+1, x+width-2, y+1, graphics.NewColor(255, 255, 255, 80))
	graphics.DrawLine(w, x+1, y+2, x+width-2, y+2, graphics.NewColor(255, 255, 255, 40))
	graphics.DrawLine(w, x+1, y+1, x+1, y+titleH-2, graphics.NewColor(255, 255, 255, 50))

	// Convex effect - darker bottom of title bar
	graphics.DrawLine(w, x+1, y+titleH-2, x+width-2, y+titleH-2, graphics.NewColor(0, 0, 0, 50))
	graphics.DrawLine(w, x+1, y+titleH-1, x+width-2, y+titleH-1, graphics.NewColor(0, 0, 0, 80))

	// Title text centered vertically, left padded
	DrawText(w, title, x+ctx.Style.Padding, y+(titleH-TextHeight(ctx.Style.FontSize))/2, ctx.Style.FontSize, ctx.Style.TextColor)

	// Draw panel body
	graphics.FillRect(w, graphics.NewRect(x, y+titleH, width, height-titleH), ctx.Style.BackgroundColor)

	// Inner body highlight (top edge below title)
	graphics.DrawLine(w, x+1, y+titleH, x+width-2, y+titleH, graphics.NewColor(255, 255, 255, 15))

	// Outer border for the whole window
	graphics.DrawRect(w, graphics.NewRect(x, y, width, height), graphics.NewColor(40, 45, 55, 255))

	// Inner border highlight (top-left edges)
	graphics.DrawLine(w, x+1, y+titleH+1, x+1, y+height-2, graphics.NewColor(255, 255, 255, 10))
}

// DraggablePanel draws a draggable panel/window and returns updated context and window state
func DraggablePanel(ctx GuiContext, w graphics.Window, title string, state WindowState) (GuiContext, WindowState) {
	// Generate ID from title
	idStr := title
	idStr += "_panel"
	id := GenID(idStr)
	titleH := TextHeight(ctx.Style.FontSize) + ctx.Style.Padding*2

	// Register in z-order
	ctx = registerWindow(ctx, id)

	// Determine if this window should render on the current pass
	shouldRender := true
	if ctx.ZOrderReady && len(ctx.WindowZOrder) > 0 {
		shouldRender = false
		if ctx.RenderPass >= 0 && ctx.RenderPass < int32(len(ctx.WindowZOrder)) {
			if ctx.WindowZOrder[ctx.RenderPass] == id {
				shouldRender = true
			}
		}
	}

	if !shouldRender {
		ctx.DrawContent = false
		return ctx, state
	}

	// Click anywhere in window brings to front
	inWindow := pointInRect(ctx.MouseX, ctx.MouseY, state.X, state.Y, state.Width, state.Height)
	if inWindow && ctx.MouseClicked {
		ctx = bringWindowToFront(ctx, id)
	}

	// Drag start (title bar only)
	inTitleBar := pointInRect(ctx.MouseX, ctx.MouseY, state.X, state.Y, state.Width, titleH)
	if inTitleBar && ctx.MouseClicked {
		state.Dragging = true
		state.DragOffsetX = ctx.MouseX - state.X
		state.DragOffsetY = ctx.MouseY - state.Y
		ctx.ActiveID = id
	}

	// Handle dragging
	if state.Dragging && ctx.MouseDown {
		state.X = ctx.MouseX - state.DragOffsetX
		state.Y = ctx.MouseY - state.DragOffsetY
	}

	// Handle drag end
	if state.Dragging && ctx.MouseReleased {
		state.Dragging = false
		if ctx.ActiveID == id {
			ctx.ActiveID = 0
		}
	}

	// Draw the panel at current position
	Panel(ctx, w, title, state.X, state.Y, state.Width, state.Height)

	ctx.DrawContent = true
	return ctx, state
}

// Separator draws a horizontal separator line with subtle 3D effect
func Separator(ctx GuiContext, w graphics.Window, x int32, y int32, width int32) {
	graphics.DrawLine(w, x, y, x+width, y, graphics.NewColor(25, 30, 40, 255))
	graphics.DrawLine(w, x, y+1, x+width, y+1, graphics.NewColor(65, 70, 80, 255))
}

// --- Menu System ---

// BeginMenuBar starts a menu bar at the given position
func BeginMenuBar(ctx GuiContext, w graphics.Window, state MenuState, x int32, y int32, width int32) (GuiContext, MenuState) {
	height := TextHeight(ctx.Style.FontSize) + ctx.Style.Padding*2

	// Draw menu bar background with gradient effect
	graphics.FillRect(w, graphics.NewRect(x, y, width, height), ctx.Style.TitleBgColor)
	// Top highlight
	graphics.DrawLine(w, x, y+1, x+width-1, y+1, graphics.NewColor(255, 255, 255, 30))
	// Bottom shadow
	graphics.DrawLine(w, x, y+height-1, x+width, y+height-1, graphics.NewColor(0, 0, 0, 60))
	graphics.DrawLine(w, x, y+height, x+width, y+height, graphics.NewColor(0, 0, 0, 30))

	// Store menu bar info for dropdown positioning
	state.MenuBarX = x
	state.MenuBarY = y
	state.MenuBarH = height
	state.CurrentMenuX = x + ctx.Style.Padding

	// Check for click outside any menu to close
	state.ClickedOutside = ctx.MouseClicked

	return ctx, state
}

// EndMenuBar finishes the menu bar and handles click-outside-to-close
func EndMenuBar(ctx GuiContext, state MenuState) (GuiContext, MenuState) {
	// If clicked outside and no menu item was hit, close menu
	if state.ClickedOutside && state.OpenMenuID != 0 {
		state.OpenMenuID = 0
	}
	return ctx, state
}

// Menu draws a menu header and returns true if the dropdown should be shown
func Menu(ctx GuiContext, w graphics.Window, state MenuState, label string) (GuiContext, MenuState, bool) {
	id := GenID(label)
	padding := ctx.Style.Padding
	textW := TextWidth(label, ctx.Style.FontSize)
	textH := TextHeight(ctx.Style.FontSize)
	menuW := textW + padding*2
	menuH := state.MenuBarH

	x := state.CurrentMenuX
	y := state.MenuBarY

	// Hit test
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, x, y, menuW, menuH)

	// Determine if this menu is open
	isOpen := state.OpenMenuID == id

	// Draw background if hovered or open
	if hovered || isOpen {
		graphics.FillRect(w, graphics.NewRect(x, y+1, menuW, menuH-2), ctx.Style.ButtonHoverColor)
		// Highlight effect
		graphics.DrawLine(w, x+1, y+2, x+menuW-2, y+2, graphics.NewColor(255, 255, 255, 30))
		// If clicked, toggle menu
		if ctx.MouseClicked {
			if isOpen {
				state.OpenMenuID = 0
				isOpen = false
			} else {
				state.OpenMenuID = id
				isOpen = true
			}
			state.ClickedOutside = false // Click was on menu, not outside
		}
	}

	// Draw label centered vertically
	textY := y + (menuH-textH)/2
	DrawText(w, label, x+padding, textY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Store position for dropdown and advance for next menu
	state.CurrentMenuW = menuW
	state.CurrentMenuX = x + menuW

	return ctx, state, isOpen
}

// BeginDropdown starts a dropdown menu area, returns the dropdown Y position
// dropX is the X position of the dropdown, itemCount is the total number of items
func BeginDropdown(ctx GuiContext, w graphics.Window, state MenuState, dropX int32, itemCount int32) (GuiContext, int32) {
	// Dropdown appears below menu bar, aligned with current menu header
	dropY := state.MenuBarY + state.MenuBarH

	// Draw dropdown shadow and background
	padding := ctx.Style.Padding
	textH := TextHeight(ctx.Style.FontSize)
	itemH := textH + padding*2
	itemW := int32(160)
	totalH := itemH * itemCount

	// Shadow
	graphics.FillRect(w, graphics.NewRect(dropX+3, dropY+3, itemW, totalH), graphics.NewColor(0, 0, 0, 50))
	graphics.FillRect(w, graphics.NewRect(dropX+2, dropY+2, itemW, totalH), graphics.NewColor(0, 0, 0, 40))

	// Solid opaque background for entire dropdown (prevents gaps between items)
	graphics.FillRect(w, graphics.NewRect(dropX, dropY, itemW, totalH), graphics.NewColor(45, 45, 48, 255))

	return ctx, dropY
}

// MenuItem draws a menu item and returns true if clicked
func MenuItem(ctx GuiContext, w graphics.Window, state MenuState, label string, dropX int32, dropY int32, itemIndex int32) (GuiContext, MenuState, bool) {
	padding := ctx.Style.Padding
	textH := TextHeight(ctx.Style.FontSize)
	itemH := textH + padding*2
	itemW := int32(160) // Fixed width for menu items

	y := dropY + itemIndex*itemH

	// Draw item background
	graphics.FillRect(w, graphics.NewRect(dropX, y, itemW, itemH), ctx.Style.BackgroundColor)

	// Hit test
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, dropX, y, itemW, itemH)
	clicked := false

	if hovered {
		// Hover highlight with gradient effect
		graphics.FillRect(w, graphics.NewRect(dropX+1, y+1, itemW-2, itemH-2), ctx.Style.ButtonHoverColor)
		state.ClickedOutside = false // Click is on menu item
		if ctx.MouseClicked {
			clicked = true
			state.OpenMenuID = 0 // Close menu after click
		}
	}

	// Draw label
	textY := y + (itemH-textH)/2
	DrawText(w, label, dropX+padding, textY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Draw border around dropdown (cleaner look)
	graphics.DrawRect(w, graphics.NewRect(dropX, dropY, itemW, (itemIndex+1)*itemH), graphics.NewColor(50, 55, 65, 255))

	return ctx, state, clicked
}

// MenuItemSeparator draws a separator line in a dropdown menu
func MenuItemSeparator(ctx GuiContext, w graphics.Window, dropX int32, dropY int32, itemIndex int32) {
	padding := ctx.Style.Padding
	textH := TextHeight(ctx.Style.FontSize)
	itemH := textH + padding*2
	itemW := int32(160)

	y := dropY + itemIndex*itemH + itemH/2

	graphics.FillRect(w, graphics.NewRect(dropX, dropY+itemIndex*itemH, itemW, itemH), ctx.Style.BackgroundColor)
	// Draw separator with subtle 3D effect
	graphics.DrawLine(w, dropX+padding, y, dropX+itemW-padding, y, graphics.NewColor(30, 35, 45, 255))
	graphics.DrawLine(w, dropX+padding, y+1, dropX+itemW-padding, y+1, graphics.NewColor(70, 75, 85, 255))
}

// --- Layout Helpers ---

// BeginLayout starts auto-layout at the given position
func BeginLayout(ctx GuiContext, x int32, y int32, spacing int32) GuiContext {
	ctx.CursorX = x
	ctx.CursorY = y
	ctx.Spacing = spacing
	return ctx
}

// NextRow moves cursor to next row
func NextRow(ctx GuiContext, height int32) GuiContext {
	ctx.CursorY = ctx.CursorY + height + ctx.Spacing
	return ctx
}

// AutoLabel draws label at cursor position and advances cursor
func AutoLabel(ctx GuiContext, w graphics.Window, text string) GuiContext {
	Label(ctx, w, text, ctx.CursorX, ctx.CursorY)
	ctx = NextRow(ctx, TextHeight(ctx.Style.FontSize))
	return ctx
}

// AutoButton draws button at cursor position and advances cursor
func AutoButton(ctx GuiContext, w graphics.Window, label string, width int32, height int32) (GuiContext, bool) {
	var result bool
	ctx, result = Button(ctx, w, label, ctx.CursorX, ctx.CursorY, width, height)
	ctx = NextRow(ctx, height)
	return ctx, result
}

// AutoCheckbox draws checkbox at cursor position and advances cursor
func AutoCheckbox(ctx GuiContext, w graphics.Window, label string, value bool) (GuiContext, bool) {
	var result bool
	ctx, result = Checkbox(ctx, w, label, ctx.CursorX, ctx.CursorY, value)
	ctx = NextRow(ctx, ctx.Style.CheckboxSize)
	return ctx, result
}

// AutoSlider draws slider at cursor position and advances cursor
func AutoSlider(ctx GuiContext, w graphics.Window, label string, width int32, min float64, max float64, value float64) (GuiContext, float64) {
	var result float64
	ctx, result = Slider(ctx, w, label, ctx.CursorX, ctx.CursorY, width, min, max, value)
	ctx = NextRow(ctx, ctx.Style.SliderHeight)
	return ctx, result
}

// --- Additional Widgets ---

// TabState holds state for a tab bar
type TabState struct {
	ActiveTab int32
}

// NewTabState creates a new tab state
func NewTabState() TabState {
	return TabState{ActiveTab: 0}
}

// TabBar draws a tab bar and returns the active tab index
func TabBar(ctx GuiContext, w graphics.Window, state TabState, labels []string, x int32, y int32) (GuiContext, TabState) {
	padding := ctx.Style.Padding
	tabH := TextHeight(ctx.Style.FontSize) + padding*2
	tabX := x

	for i := 0; i < len(labels); i++ {
		label := labels[i]
		tabW := TextWidth(label, ctx.Style.FontSize) + padding*3
		isActive := int32(i) == state.ActiveTab

		// Hit test
		hovered := pointInRect(ctx.MouseX, ctx.MouseY, tabX, y, tabW, tabH)

		// Determine colors
		var bgColor graphics.Color
		if isActive {
			bgColor = ctx.Style.BackgroundColor
		} else if hovered {
			bgColor = ctx.Style.ButtonHoverColor
		} else {
			bgColor = graphics.NewColor(35, 40, 50, 255)
		}

		// Draw tab shadow (only for active)
		if isActive {
			graphics.FillRect(w, graphics.NewRect(tabX+1, y+1, tabW, tabH), graphics.NewColor(0, 0, 0, 40))
		}

		// Draw tab background
		graphics.FillRect(w, graphics.NewRect(tabX, y, tabW, tabH), bgColor)

		// Convex effect for active tab
		if isActive {
			graphics.DrawLine(w, tabX+1, y+1, tabX+tabW-2, y+1, graphics.NewColor(255, 255, 255, 80))
			graphics.DrawLine(w, tabX+1, y+1, tabX+1, y+tabH-1, graphics.NewColor(255, 255, 255, 50))
		}

		// Draw border
		graphics.DrawLine(w, tabX, y, tabX+tabW, y, graphics.NewColor(60, 65, 75, 255))
		graphics.DrawLine(w, tabX, y, tabX, y+tabH, graphics.NewColor(60, 65, 75, 255))
		graphics.DrawLine(w, tabX+tabW, y, tabX+tabW, y+tabH, graphics.NewColor(60, 65, 75, 255))

		// Don't draw bottom border for active tab (connects to content)
		if !isActive {
			graphics.DrawLine(w, tabX, y+tabH, tabX+tabW, y+tabH, graphics.NewColor(60, 65, 75, 255))
		}

		// Draw label
		textY := y + (tabH-TextHeight(ctx.Style.FontSize))/2
		DrawText(w, label, tabX+padding, textY, ctx.Style.FontSize, ctx.Style.TextColor)

		// Handle click
		if hovered && ctx.MouseClicked {
			state.ActiveTab = int32(i)
		}

		tabX = tabX + tabW
	}

	return ctx, state
}

// TreeNodeState holds expanded/collapsed state for tree nodes
type TreeNodeState struct {
	ExpandedIDs []int32 // List of expanded node IDs
}

// NewTreeNodeState creates a new tree node state
func NewTreeNodeState() TreeNodeState {
	return TreeNodeState{ExpandedIDs: []int32{}}
}

// isExpanded checks if a node ID is in the expanded list
func isExpanded(state TreeNodeState, id int32) bool {
	for i := 0; i < len(state.ExpandedIDs); i++ {
		if state.ExpandedIDs[i] == id {
			return true
		}
	}
	return false
}

// toggleExpanded adds or removes an ID from the expanded list
func toggleExpanded(state TreeNodeState, id int32) TreeNodeState {
	// Check if already expanded
	for i := 0; i < len(state.ExpandedIDs); i++ {
		if state.ExpandedIDs[i] == id {
			// Remove it (collapse)
			newIDs := []int32{}
			for j := 0; j < len(state.ExpandedIDs); j++ {
				if state.ExpandedIDs[j] != id {
					newIDs = append(newIDs, state.ExpandedIDs[j])
				}
			}
			state.ExpandedIDs = newIDs
			return state
		}
	}
	// Not found, add it (expand)
	state.ExpandedIDs = append(state.ExpandedIDs, id)
	return state
}

// TreeNode draws a tree node with expand/collapse arrow, returns true if expanded
func TreeNode(ctx GuiContext, w graphics.Window, state TreeNodeState, label string, x int32, y int32, indent int32) (GuiContext, TreeNodeState, bool) {
	id := GenID(label)
	padding := ctx.Style.Padding
	nodeH := TextHeight(ctx.Style.FontSize) + padding
	arrowSize := int32(8)

	// Calculate actual x with indent
	actualX := x + indent

	// Hit test for arrow area
	arrowHovered := pointInRect(ctx.MouseX, ctx.MouseY, actualX, y, arrowSize+padding, nodeH)
	// Hit test for label area
	labelHovered := pointInRect(ctx.MouseX, ctx.MouseY, actualX+arrowSize+padding, y, TextWidth(label, ctx.Style.FontSize)+padding, nodeH)

	// Get expanded state
	expanded := isExpanded(state, id)

	// Draw hover highlight
	if arrowHovered || labelHovered {
		totalW := arrowSize + padding + TextWidth(label, ctx.Style.FontSize) + padding
		graphics.FillRect(w, graphics.NewRect(actualX, y, totalW, nodeH), graphics.NewColor(255, 255, 255, 20))
	}

	// Draw expand/collapse arrow
	arrowX := actualX + arrowSize/2
	arrowY := y + nodeH/2
	arrowColor := ctx.Style.TextColor
	if expanded {
		// Down arrow (expanded)
		graphics.DrawLine(w, arrowX-3, arrowY-2, arrowX, arrowY+2, arrowColor)
		graphics.DrawLine(w, arrowX, arrowY+2, arrowX+3, arrowY-2, arrowColor)
		graphics.DrawLine(w, arrowX-2, arrowY-2, arrowX, arrowY+1, arrowColor)
		graphics.DrawLine(w, arrowX, arrowY+1, arrowX+2, arrowY-2, arrowColor)
	} else {
		// Right arrow (collapsed)
		graphics.DrawLine(w, arrowX-2, arrowY-3, arrowX+2, arrowY, arrowColor)
		graphics.DrawLine(w, arrowX+2, arrowY, arrowX-2, arrowY+3, arrowColor)
		graphics.DrawLine(w, arrowX-2, arrowY-2, arrowX+1, arrowY, arrowColor)
		graphics.DrawLine(w, arrowX+1, arrowY, arrowX-2, arrowY+2, arrowColor)
	}

	// Draw label
	DrawText(w, label, actualX+arrowSize+padding, y+(nodeH-TextHeight(ctx.Style.FontSize))/2, ctx.Style.FontSize, ctx.Style.TextColor)

	// Handle click on arrow to toggle
	if arrowHovered && ctx.MouseClicked {
		state = toggleExpanded(state, id)
		expanded = !expanded
	}

	return ctx, state, expanded
}

// TreeLeaf draws a tree leaf (no expand arrow)
func TreeLeaf(ctx GuiContext, w graphics.Window, label string, x int32, y int32, indent int32) (GuiContext, bool) {
	padding := ctx.Style.Padding
	nodeH := TextHeight(ctx.Style.FontSize) + padding
	bulletSize := int32(4)

	// Calculate actual x with indent
	actualX := x + indent

	// Hit test
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, actualX, y, TextWidth(label, ctx.Style.FontSize)+padding*2+bulletSize, nodeH)
	clicked := false

	// Draw hover highlight
	if hovered {
		totalW := bulletSize + padding + TextWidth(label, ctx.Style.FontSize) + padding
		graphics.FillRect(w, graphics.NewRect(actualX, y, totalW, nodeH), graphics.NewColor(255, 255, 255, 20))
		if ctx.MouseClicked {
			clicked = true
		}
	}

	// Draw bullet point
	bulletX := actualX + bulletSize/2 + 2
	bulletY := y + nodeH/2
	graphics.FillCircle(w, bulletX, bulletY, 2, ctx.Style.TextColor)

	// Draw label
	DrawText(w, label, actualX+bulletSize+padding, y+(nodeH-TextHeight(ctx.Style.FontSize))/2, ctx.Style.FontSize, ctx.Style.TextColor)

	return ctx, clicked
}

// ProgressBar draws a horizontal progress bar
func ProgressBar(ctx GuiContext, w graphics.Window, x int32, y int32, width int32, height int32, progress float64) {
	// Clamp progress
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	// Draw shadow
	graphics.FillRect(w, graphics.NewRect(x+1, y+1, width, height), graphics.NewColor(0, 0, 0, 50))

	// Draw background (inset)
	graphics.FillRect(w, graphics.NewRect(x, y, width, height), ctx.Style.FrameBgColor)

	// Inner shadow for inset look
	graphics.DrawLine(w, x+1, y+1, x+width-2, y+1, graphics.NewColor(0, 0, 0, 60))
	graphics.DrawLine(w, x+1, y+1, x+1, y+height-2, graphics.NewColor(0, 0, 0, 60))

	// Draw filled portion
	fillW := int32(float64(width-4) * progress)
	if fillW > 0 {
		fillColor := ctx.Style.SliderTrackColor
		graphics.FillRect(w, graphics.NewRect(x+2, y+2, fillW, height-4), fillColor)

		// Convex highlight on fill
		graphics.DrawLine(w, x+2, y+2, x+fillW, y+2, graphics.NewColor(255, 255, 255, 60))
		graphics.DrawLine(w, x+2, y+3, x+fillW, y+3, graphics.NewColor(255, 255, 255, 30))
	}

	// Border
	graphics.DrawRect(w, graphics.NewRect(x, y, width, height), graphics.NewColor(50, 55, 65, 255))
}

// RadioButton draws a radio button and returns true if selected
func RadioButton(ctx GuiContext, w graphics.Window, label string, x int32, y int32, selected bool) (GuiContext, bool) {
	id := GenID(label)
	radioSize := ctx.Style.CheckboxSize
	radius := radioSize / 2

	// Hit test
	labelW := TextWidth(label, ctx.Style.FontSize)
	totalW := radioSize + ctx.Style.Padding + labelW
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, x, y, totalW, radioSize)

	// Update hot/active state
	if hovered {
		ctx.HotID = id
		if ctx.MouseClicked {
			ctx.ActiveID = id
		}
	}

	// Draw radio button circle
	centerX := x + radius
	centerY := y + radius

	// Outer circle shadow
	graphics.DrawCircle(w, centerX+1, centerY+1, radius, graphics.NewColor(0, 0, 0, 60))

	// Outer circle background
	var bgColor graphics.Color
	if hovered {
		bgColor = ctx.Style.ButtonHoverColor
	} else {
		bgColor = ctx.Style.FrameBgColor
	}
	graphics.FillCircle(w, centerX, centerY, radius-1, bgColor)

	// Convex highlight
	graphics.DrawCircle(w, centerX-1, centerY-1, radius-2, graphics.NewColor(255, 255, 255, 50))

	// Outer circle border
	graphics.DrawCircle(w, centerX, centerY, radius, graphics.NewColor(60, 65, 75, 255))

	// Draw inner dot if selected
	if selected {
		dotColor := ctx.Style.CheckmarkColor
		graphics.FillCircle(w, centerX, centerY, radius-4, dotColor)
		// Highlight on dot
		graphics.DrawCircle(w, centerX-1, centerY-1, radius-5, graphics.NewColor(255, 255, 255, 60))
	}

	// Draw label
	labelX := x + radioSize + ctx.Style.Padding
	labelY := y + (radioSize-TextHeight(ctx.Style.FontSize))/2
	DrawText(w, label, labelX, labelY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Return true if clicked
	clicked := ctx.ReleasedID == id && ctx.MouseReleased && hovered
	return ctx, clicked
}

// TextInputState holds state for text input
type TextInputState struct {
	Text       string
	CursorPos  int32
	Active     bool
	BlinkTimer int32
}

// NewTextInputState creates a new text input state
func NewTextInputState(initialText string) TextInputState {
	return TextInputState{
		Text:      initialText,
		CursorPos: int32(len(initialText)),
	}
}

// TextInput draws a text input field and returns updated state
func TextInput(ctx GuiContext, w graphics.Window, state TextInputState, x int32, y int32, width int32) (GuiContext, TextInputState) {
	idStr := "textinput_"
	// Use position as part of ID since we don't have a label
	id := GenID(idStr) + x*1000 + y
	height := ctx.Style.ButtonHeight
	padding := ctx.Style.Padding

	// Hit test
	hovered := pointInRect(ctx.MouseX, ctx.MouseY, x, y, width, height)

	// Handle click to activate
	if hovered && ctx.MouseClicked {
		state.Active = true
		ctx.ActiveID = id
	} else if ctx.MouseClicked && !hovered {
		state.Active = false
		if ctx.ActiveID == id {
			ctx.ActiveID = 0
		}
	}

	// Draw shadow
	graphics.FillRect(w, graphics.NewRect(x+1, y+1, width, height), graphics.NewColor(0, 0, 0, 50))

	// Draw background
	var bgColor graphics.Color
	if state.Active {
		bgColor = graphics.NewColor(55, 60, 70, 255)
	} else {
		bgColor = ctx.Style.FrameBgColor
	}
	graphics.FillRect(w, graphics.NewRect(x, y, width, height), bgColor)

	// Inset shadow
	graphics.DrawLine(w, x+1, y+1, x+width-2, y+1, graphics.NewColor(0, 0, 0, 80))
	graphics.DrawLine(w, x+1, y+1, x+1, y+height-2, graphics.NewColor(0, 0, 0, 80))

	// Draw text
	textY := y + (height-TextHeight(ctx.Style.FontSize))/2
	DrawText(w, state.Text, x+padding, textY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Draw cursor if active
	if state.Active {
		state.BlinkTimer = state.BlinkTimer + 1
		if state.BlinkTimer > 60 {
			state.BlinkTimer = 0
		}
		if state.BlinkTimer < 30 {
			cursorX := x + padding + TextWidth(state.Text, ctx.Style.FontSize)
			graphics.DrawLine(w, cursorX, y+4, cursorX, y+height-4, ctx.Style.TextColor)
		}
	}

	// Border (highlight if active)
	var borderColor graphics.Color
	if state.Active {
		borderColor = ctx.Style.CheckmarkColor
	} else if hovered {
		borderColor = graphics.NewColor(80, 90, 110, 255)
	} else {
		borderColor = graphics.NewColor(50, 55, 65, 255)
	}
	graphics.DrawRect(w, graphics.NewRect(x, y, width, height), borderColor)

	// Handle keyboard input when active
	if state.Active {
		key := graphics.GetLastKey()
		if key > 0 {
			if key == 8 { // Backspace
				if len(state.Text) > 0 {
					state.Text = removeLastChar(state.Text)
				}
			} else if key >= 32 && key < 127 { // Printable characters
				state.Text = state.Text + asciiToString(key)
			}
		}
	}

	return ctx, state
}

// ListBox draws a scrollable list and returns the selected index
func ListBox(ctx GuiContext, w graphics.Window, items []string, x int32, y int32, width int32, height int32, selectedIndex int32, scrollOffset int32) (GuiContext, int32, int32) {
	padding := ctx.Style.Padding
	itemH := TextHeight(ctx.Style.FontSize) + padding

	// Draw shadow
	graphics.FillRect(w, graphics.NewRect(x+2, y+2, width, height), graphics.NewColor(0, 0, 0, 50))

	// Draw background
	graphics.FillRect(w, graphics.NewRect(x, y, width, height), ctx.Style.FrameBgColor)

	// Inset shadow
	graphics.DrawLine(w, x+1, y+1, x+width-2, y+1, graphics.NewColor(0, 0, 0, 80))
	graphics.DrawLine(w, x+1, y+1, x+1, y+height-2, graphics.NewColor(0, 0, 0, 80))

	// Calculate visible items
	visibleCount := (height - 4) / itemH
	startIndex := scrollOffset / itemH
	if startIndex < 0 {
		startIndex = 0
	}

	// Draw items
	for i := int32(0); i < visibleCount && int(startIndex+i) < len(items); i++ {
		itemIndex := startIndex + i
		item := items[itemIndex]
		itemY := y + 2 + i*itemH

		// Check if this item is selected
		if itemIndex == selectedIndex {
			graphics.FillRect(w, graphics.NewRect(x+2, itemY, width-4, itemH), ctx.Style.ButtonHoverColor)
		}

		// Hit test
		if pointInRect(ctx.MouseX, ctx.MouseY, x+2, itemY, width-4, itemH) {
			if itemIndex != selectedIndex {
				graphics.FillRect(w, graphics.NewRect(x+2, itemY, width-4, itemH), graphics.NewColor(255, 255, 255, 20))
			}
			if ctx.MouseClicked {
				selectedIndex = itemIndex
			}
		}

		// Draw item text
		textY := itemY + (itemH-TextHeight(ctx.Style.FontSize))/2
		DrawText(w, item, x+padding, textY, ctx.Style.FontSize, ctx.Style.TextColor)
	}

	// Draw border
	graphics.DrawRect(w, graphics.NewRect(x, y, width, height), graphics.NewColor(50, 55, 65, 255))

	// Draw scrollbar if needed
	totalHeight := int32(len(items)) * itemH
	if totalHeight > height {
		scrollbarW := int32(8)
		scrollbarX := x + width - scrollbarW - 2
		scrollbarH := height - 4
		thumbH := (height * scrollbarH) / totalHeight
		if thumbH < 20 {
			thumbH = 20
		}
		maxScroll := totalHeight - height
		thumbY := y + 2 + ((scrollbarH - thumbH) * scrollOffset) / maxScroll

		// Scrollbar track
		graphics.FillRect(w, graphics.NewRect(scrollbarX, y+2, scrollbarW, scrollbarH), graphics.NewColor(30, 35, 45, 255))

		// Scrollbar thumb
		graphics.FillRect(w, graphics.NewRect(scrollbarX, thumbY, scrollbarW, thumbH), graphics.NewColor(80, 90, 110, 255))
		// Convex effect
		graphics.DrawLine(w, scrollbarX+1, thumbY+1, scrollbarX+scrollbarW-2, thumbY+1, graphics.NewColor(255, 255, 255, 40))
	}

	return ctx, selectedIndex, scrollOffset
}

// --- Auto-layout versions of new widgets ---

// AutoProgressBar draws a progress bar at cursor position
func AutoProgressBar(ctx GuiContext, w graphics.Window, width int32, height int32, progress float64) GuiContext {
	ProgressBar(ctx, w, ctx.CursorX, ctx.CursorY, width, height, progress)
	ctx = NextRow(ctx, height)
	return ctx
}

// AutoRadioButton draws a radio button at cursor position
func AutoRadioButton(ctx GuiContext, w graphics.Window, label string, selected bool) (GuiContext, bool) {
	var clicked bool
	ctx, clicked = RadioButton(ctx, w, label, ctx.CursorX, ctx.CursorY, selected)
	ctx = NextRow(ctx, ctx.Style.CheckboxSize)
	return ctx, clicked
}

// Tooltip draws a tooltip at the given position
func Tooltip(ctx GuiContext, w graphics.Window, text string, x int32, y int32) {
	padding := ctx.Style.Padding
	textW := TextWidth(text, ctx.Style.FontSize)
	textH := TextHeight(ctx.Style.FontSize)
	tipW := textW + padding*2
	tipH := textH + padding*2

	// Adjust position to stay on screen
	tipX := x + 10
	tipY := y + 10

	// Draw shadow
	graphics.FillRect(w, graphics.NewRect(tipX+2, tipY+2, tipW, tipH), graphics.NewColor(0, 0, 0, 80))

	// Draw background
	graphics.FillRect(w, graphics.NewRect(tipX, tipY, tipW, tipH), graphics.NewColor(60, 60, 65, 250))

	// Convex highlight
	graphics.DrawLine(w, tipX+1, tipY+1, tipX+tipW-2, tipY+1, graphics.NewColor(255, 255, 255, 50))
	graphics.DrawLine(w, tipX+1, tipY+1, tipX+1, tipY+tipH-2, graphics.NewColor(255, 255, 255, 30))

	// Border
	graphics.DrawRect(w, graphics.NewRect(tipX, tipY, tipW, tipH), graphics.NewColor(80, 85, 95, 255))

	// Draw text
	DrawText(w, text, tipX+padding, tipY+padding, ctx.Style.FontSize, ctx.Style.TextColor)
}

// Spinner draws a numeric spinner with +/- buttons
func Spinner(ctx GuiContext, w graphics.Window, label string, x int32, y int32, value int32, minVal int32, maxVal int32) (GuiContext, int32) {
	padding := ctx.Style.Padding
	height := ctx.Style.ButtonHeight
	btnW := height
	valueW := int32(50)
	labelW := TextWidth(label, ctx.Style.FontSize)

	// Draw label
	labelY := y + (height-TextHeight(ctx.Style.FontSize))/2
	DrawText(w, label, x, labelY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Position for spinner controls
	spinX := x + labelW + padding

	// Minus button
	minusHovered := pointInRect(ctx.MouseX, ctx.MouseY, spinX, y, btnW, height)
	var minusBg graphics.Color
	if minusHovered {
		minusBg = ctx.Style.ButtonHoverColor
	} else {
		minusBg = ctx.Style.ButtonColor
	}
	graphics.FillRect(w, graphics.NewRect(spinX, y, btnW, height), minusBg)
	// Convex effect
	graphics.DrawLine(w, spinX+1, y+1, spinX+btnW-2, y+1, graphics.NewColor(255, 255, 255, 70))
	graphics.DrawLine(w, spinX+1, y+height-1, spinX+btnW-1, y+height-1, graphics.NewColor(0, 0, 0, 80))
	graphics.DrawRect(w, graphics.NewRect(spinX, y, btnW, height), graphics.NewColor(50, 55, 65, 255))
	// Minus sign
	graphics.DrawLine(w, spinX+4, y+height/2, spinX+btnW-4, y+height/2, ctx.Style.TextColor)
	graphics.DrawLine(w, spinX+4, y+height/2+1, spinX+btnW-4, y+height/2+1, ctx.Style.TextColor)

	if minusHovered && ctx.MouseClicked && value > minVal {
		value = value - 1
	}

	// Value display
	valueX := spinX + btnW
	graphics.FillRect(w, graphics.NewRect(valueX, y, valueW, height), ctx.Style.FrameBgColor)
	graphics.DrawLine(w, valueX+1, y+1, valueX+valueW-2, y+1, graphics.NewColor(0, 0, 0, 60))
	graphics.DrawRect(w, graphics.NewRect(valueX, y, valueW, height), graphics.NewColor(50, 55, 65, 255))

	// Draw value centered
	valueStr := intToString(value)
	valueTextW := TextWidth(valueStr, ctx.Style.FontSize)
	valueTextX := valueX + (valueW-valueTextW)/2
	DrawText(w, valueStr, valueTextX, labelY, ctx.Style.FontSize, ctx.Style.TextColor)

	// Plus button
	plusX := valueX + valueW
	plusHovered := pointInRect(ctx.MouseX, ctx.MouseY, plusX, y, btnW, height)
	var plusBg graphics.Color
	if plusHovered {
		plusBg = ctx.Style.ButtonHoverColor
	} else {
		plusBg = ctx.Style.ButtonColor
	}
	graphics.FillRect(w, graphics.NewRect(plusX, y, btnW, height), plusBg)
	// Convex effect
	graphics.DrawLine(w, plusX+1, y+1, plusX+btnW-2, y+1, graphics.NewColor(255, 255, 255, 70))
	graphics.DrawLine(w, plusX+1, y+height-1, plusX+btnW-1, y+height-1, graphics.NewColor(0, 0, 0, 80))
	graphics.DrawRect(w, graphics.NewRect(plusX, y, btnW, height), graphics.NewColor(50, 55, 65, 255))
	// Plus sign
	graphics.DrawLine(w, plusX+4, y+height/2, plusX+btnW-4, y+height/2, ctx.Style.TextColor)
	graphics.DrawLine(w, plusX+4, y+height/2+1, plusX+btnW-4, y+height/2+1, ctx.Style.TextColor)
	graphics.DrawLine(w, plusX+btnW/2, y+4, plusX+btnW/2, y+height-4, ctx.Style.TextColor)
	graphics.DrawLine(w, plusX+btnW/2+1, y+4, plusX+btnW/2+1, y+height-4, ctx.Style.TextColor)

	if plusHovered && ctx.MouseClicked && value < maxVal {
		value = value + 1
	}

	return ctx, value
}

// Helper to convert int to string (simple implementation)
func intToString(n int32) string {
	digitChars := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	result := ""
	for n > 0 {
		digit := n % 10
		result = digitChars[digit] + result
		n = n / 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

// removeLastChar removes the last character from a string
func removeLastChar(s string) string {
	slen := len(s)
	if slen == 0 {
		return s
	}
	// Build result by taking chars one at a time using asciiToString
	result := ""
	for i := 0; i < slen-1; i++ {
		result = result + asciiToString(int(s[i]))
	}
	return result
}

// asciiToString converts an ASCII code (32-126) to a single-character string
func asciiToString(code int) string {
	// Printable ASCII characters
	chars := []string{
		" ", "!", "\"", "#", "$", "%", "&", "'", "(", ")", "*", "+", ",", "-", ".", "/",
		"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", ":", ";", "<", "=", ">", "?",
		"@", "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O",
		"P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "[", "\\", "]", "^", "_",
		"`", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o",
		"p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "{", "|", "}", "~",
	}
	if code >= 32 && code <= 126 {
		return chars[code-32]
	}
	return ""
}

// ColorPicker draws a simple color picker with RGB sliders
func ColorPicker(ctx GuiContext, w graphics.Window, label string, x int32, y int32, width int32, color graphics.Color) (GuiContext, graphics.Color) {
	padding := ctx.Style.Padding
	sliderH := ctx.Style.SliderHeight
	previewSize := int32(40)

	// Draw label
	DrawText(w, label, x, y, ctx.Style.FontSize, ctx.Style.TextColor)
	y = y + TextHeight(ctx.Style.FontSize) + padding

	// Draw color preview
	graphics.FillRect(w, graphics.NewRect(x+1, y+1, previewSize, previewSize), graphics.NewColor(0, 0, 0, 60))
	graphics.FillRect(w, graphics.NewRect(x, y, previewSize, previewSize), color)
	graphics.DrawRect(w, graphics.NewRect(x, y, previewSize, previewSize), graphics.NewColor(60, 65, 75, 255))

	// RGB sliders
	sliderX := x + previewSize + padding
	sliderW := width - previewSize - padding

	var rVal float64
	var gVal float64
	var bVal float64

	// Red slider
	ctx, rVal = Slider(ctx, w, "R", sliderX, y, sliderW, 0, 255, float64(color.R))
	color.R = uint8(rVal)
	y = y + sliderH + 2

	// Green slider
	ctx, gVal = Slider(ctx, w, "G", sliderX, y, sliderW, 0, 255, float64(color.G))
	color.G = uint8(gVal)
	y = y + sliderH + 2

	// Blue slider
	ctx, bVal = Slider(ctx, w, "B", sliderX, y, sliderW, 0, 255, float64(color.B))
	color.B = uint8(bVal)

	return ctx, color
}

// --- Embedded 8x8 Bitmap Font ---
// Each character is 8 bytes, where each byte represents one row
// Bit 7 is leftmost pixel, bit 0 is rightmost pixel

func getFontData() []uint8 {
	return []uint8{
	// Character 32: ' ' (space)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 33: '!'
	0x18, 0x18, 0x18, 0x18, 0x18, 0x00, 0x18, 0x00,
	// Character 34: '"'
	0x6C, 0x6C, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 35: '#'
	0x6C, 0x6C, 0xFE, 0x6C, 0xFE, 0x6C, 0x6C, 0x00,
	// Character 36: '$'
	0x18, 0x3E, 0x60, 0x3C, 0x06, 0x7C, 0x18, 0x00,
	// Character 37: '%'
	0x00, 0xC6, 0xCC, 0x18, 0x30, 0x66, 0xC6, 0x00,
	// Character 38: '&'
	0x38, 0x6C, 0x38, 0x76, 0xDC, 0xCC, 0x76, 0x00,
	// Character 39: '''
	0x18, 0x18, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 40: '('
	0x0C, 0x18, 0x30, 0x30, 0x30, 0x18, 0x0C, 0x00,
	// Character 41: ')'
	0x30, 0x18, 0x0C, 0x0C, 0x0C, 0x18, 0x30, 0x00,
	// Character 42: '*'
	0x00, 0x66, 0x3C, 0xFF, 0x3C, 0x66, 0x00, 0x00,
	// Character 43: '+'
	0x00, 0x18, 0x18, 0x7E, 0x18, 0x18, 0x00, 0x00,
	// Character 44: ','
	0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x30,
	// Character 45: '-'
	0x00, 0x00, 0x00, 0x7E, 0x00, 0x00, 0x00, 0x00,
	// Character 46: '.'
	0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x00,
	// Character 47: '/'
	0x06, 0x0C, 0x18, 0x30, 0x60, 0xC0, 0x80, 0x00,
	// Character 48: '0'
	0x7C, 0xC6, 0xCE, 0xD6, 0xE6, 0xC6, 0x7C, 0x00,
	// Character 49: '1'
	0x18, 0x38, 0x18, 0x18, 0x18, 0x18, 0x7E, 0x00,
	// Character 50: '2'
	0x7C, 0xC6, 0x06, 0x1C, 0x30, 0x66, 0xFE, 0x00,
	// Character 51: '3'
	0x7C, 0xC6, 0x06, 0x3C, 0x06, 0xC6, 0x7C, 0x00,
	// Character 52: '4'
	0x1C, 0x3C, 0x6C, 0xCC, 0xFE, 0x0C, 0x1E, 0x00,
	// Character 53: '5'
	0xFE, 0xC0, 0xC0, 0xFC, 0x06, 0xC6, 0x7C, 0x00,
	// Character 54: '6'
	0x38, 0x60, 0xC0, 0xFC, 0xC6, 0xC6, 0x7C, 0x00,
	// Character 55: '7'
	0xFE, 0xC6, 0x0C, 0x18, 0x30, 0x30, 0x30, 0x00,
	// Character 56: '8'
	0x7C, 0xC6, 0xC6, 0x7C, 0xC6, 0xC6, 0x7C, 0x00,
	// Character 57: '9'
	0x7C, 0xC6, 0xC6, 0x7E, 0x06, 0x0C, 0x78, 0x00,
	// Character 58: ':'
	0x00, 0x18, 0x18, 0x00, 0x00, 0x18, 0x18, 0x00,
	// Character 59: ';'
	0x00, 0x18, 0x18, 0x00, 0x00, 0x18, 0x18, 0x30,
	// Character 60: '<'
	0x06, 0x0C, 0x18, 0x30, 0x18, 0x0C, 0x06, 0x00,
	// Character 61: '='
	0x00, 0x00, 0x7E, 0x00, 0x00, 0x7E, 0x00, 0x00,
	// Character 62: '>'
	0x60, 0x30, 0x18, 0x0C, 0x18, 0x30, 0x60, 0x00,
	// Character 63: '?'
	0x7C, 0xC6, 0x0C, 0x18, 0x18, 0x00, 0x18, 0x00,
	// Character 64: '@'
	0x7C, 0xC6, 0xDE, 0xDE, 0xDE, 0xC0, 0x78, 0x00,
	// Character 65: 'A'
	0x38, 0x6C, 0xC6, 0xFE, 0xC6, 0xC6, 0xC6, 0x00,
	// Character 66: 'B'
	0xFC, 0x66, 0x66, 0x7C, 0x66, 0x66, 0xFC, 0x00,
	// Character 67: 'C'
	0x3C, 0x66, 0xC0, 0xC0, 0xC0, 0x66, 0x3C, 0x00,
	// Character 68: 'D'
	0xF8, 0x6C, 0x66, 0x66, 0x66, 0x6C, 0xF8, 0x00,
	// Character 69: 'E'
	0xFE, 0x62, 0x68, 0x78, 0x68, 0x62, 0xFE, 0x00,
	// Character 70: 'F'
	0xFE, 0x62, 0x68, 0x78, 0x68, 0x60, 0xF0, 0x00,
	// Character 71: 'G'
	0x3C, 0x66, 0xC0, 0xC0, 0xCE, 0x66, 0x3A, 0x00,
	// Character 72: 'H'
	0xC6, 0xC6, 0xC6, 0xFE, 0xC6, 0xC6, 0xC6, 0x00,
	// Character 73: 'I'
	0x3C, 0x18, 0x18, 0x18, 0x18, 0x18, 0x3C, 0x00,
	// Character 74: 'J'
	0x1E, 0x0C, 0x0C, 0x0C, 0xCC, 0xCC, 0x78, 0x00,
	// Character 75: 'K'
	0xE6, 0x66, 0x6C, 0x78, 0x6C, 0x66, 0xE6, 0x00,
	// Character 76: 'L'
	0xF0, 0x60, 0x60, 0x60, 0x62, 0x66, 0xFE, 0x00,
	// Character 77: 'M'
	0xC6, 0xEE, 0xFE, 0xFE, 0xD6, 0xC6, 0xC6, 0x00,
	// Character 78: 'N'
	0xC6, 0xE6, 0xF6, 0xDE, 0xCE, 0xC6, 0xC6, 0x00,
	// Character 79: 'O'
	0x7C, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0x7C, 0x00,
	// Character 80: 'P'
	0xFC, 0x66, 0x66, 0x7C, 0x60, 0x60, 0xF0, 0x00,
	// Character 81: 'Q'
	0x7C, 0xC6, 0xC6, 0xC6, 0xD6, 0xDE, 0x7C, 0x06,
	// Character 82: 'R'
	0xFC, 0x66, 0x66, 0x7C, 0x6C, 0x66, 0xE6, 0x00,
	// Character 83: 'S'
	0x7C, 0xC6, 0x60, 0x38, 0x0C, 0xC6, 0x7C, 0x00,
	// Character 84: 'T'
	0x7E, 0x7E, 0x5A, 0x18, 0x18, 0x18, 0x3C, 0x00,
	// Character 85: 'U'
	0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0x7C, 0x00,
	// Character 86: 'V'
	0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0x6C, 0x38, 0x00,
	// Character 87: 'W'
	0xC6, 0xC6, 0xC6, 0xD6, 0xD6, 0xFE, 0x6C, 0x00,
	// Character 88: 'X'
	0xC6, 0xC6, 0x6C, 0x38, 0x6C, 0xC6, 0xC6, 0x00,
	// Character 89: 'Y'
	0x66, 0x66, 0x66, 0x3C, 0x18, 0x18, 0x3C, 0x00,
	// Character 90: 'Z'
	0xFE, 0xC6, 0x8C, 0x18, 0x32, 0x66, 0xFE, 0x00,
	// Character 91: '['
	0x3C, 0x30, 0x30, 0x30, 0x30, 0x30, 0x3C, 0x00,
	// Character 92: '\'
	0xC0, 0x60, 0x30, 0x18, 0x0C, 0x06, 0x02, 0x00,
	// Character 93: ']'
	0x3C, 0x0C, 0x0C, 0x0C, 0x0C, 0x0C, 0x3C, 0x00,
	// Character 94: '^'
	0x10, 0x38, 0x6C, 0xC6, 0x00, 0x00, 0x00, 0x00,
	// Character 95: '_'
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF,
	// Character 96: '`'
	0x30, 0x18, 0x0C, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 97: 'a'
	0x00, 0x00, 0x78, 0x0C, 0x7C, 0xCC, 0x76, 0x00,
	// Character 98: 'b'
	0xE0, 0x60, 0x7C, 0x66, 0x66, 0x66, 0xDC, 0x00,
	// Character 99: 'c'
	0x00, 0x00, 0x7C, 0xC6, 0xC0, 0xC6, 0x7C, 0x00,
	// Character 100: 'd'
	0x1C, 0x0C, 0x7C, 0xCC, 0xCC, 0xCC, 0x76, 0x00,
	// Character 101: 'e'
	0x00, 0x00, 0x7C, 0xC6, 0xFE, 0xC0, 0x7C, 0x00,
	// Character 102: 'f'
	0x3C, 0x66, 0x60, 0xF8, 0x60, 0x60, 0xF0, 0x00,
	// Character 103: 'g'
	0x00, 0x00, 0x76, 0xCC, 0xCC, 0x7C, 0x0C, 0xF8,
	// Character 104: 'h'
	0xE0, 0x60, 0x6C, 0x76, 0x66, 0x66, 0xE6, 0x00,
	// Character 105: 'i'
	0x18, 0x00, 0x38, 0x18, 0x18, 0x18, 0x3C, 0x00,
	// Character 106: 'j'
	0x06, 0x00, 0x06, 0x06, 0x06, 0x66, 0x66, 0x3C,
	// Character 107: 'k'
	0xE0, 0x60, 0x66, 0x6C, 0x78, 0x6C, 0xE6, 0x00,
	// Character 108: 'l'
	0x38, 0x18, 0x18, 0x18, 0x18, 0x18, 0x3C, 0x00,
	// Character 109: 'm'
	0x00, 0x00, 0xEC, 0xFE, 0xD6, 0xD6, 0xD6, 0x00,
	// Character 110: 'n'
	0x00, 0x00, 0xDC, 0x66, 0x66, 0x66, 0x66, 0x00,
	// Character 111: 'o'
	0x00, 0x00, 0x7C, 0xC6, 0xC6, 0xC6, 0x7C, 0x00,
	// Character 112: 'p'
	0x00, 0x00, 0xDC, 0x66, 0x66, 0x7C, 0x60, 0xF0,
	// Character 113: 'q'
	0x00, 0x00, 0x76, 0xCC, 0xCC, 0x7C, 0x0C, 0x1E,
	// Character 114: 'r'
	0x00, 0x00, 0xDC, 0x76, 0x60, 0x60, 0xF0, 0x00,
	// Character 115: 's'
	0x00, 0x00, 0x7E, 0xC0, 0x7C, 0x06, 0xFC, 0x00,
	// Character 116: 't'
	0x30, 0x30, 0xFC, 0x30, 0x30, 0x36, 0x1C, 0x00,
	// Character 117: 'u'
	0x00, 0x00, 0xCC, 0xCC, 0xCC, 0xCC, 0x76, 0x00,
	// Character 118: 'v'
	0x00, 0x00, 0xC6, 0xC6, 0xC6, 0x6C, 0x38, 0x00,
	// Character 119: 'w'
	0x00, 0x00, 0xC6, 0xD6, 0xD6, 0xFE, 0x6C, 0x00,
	// Character 120: 'x'
	0x00, 0x00, 0xC6, 0x6C, 0x38, 0x6C, 0xC6, 0x00,
	// Character 121: 'y'
	0x00, 0x00, 0xC6, 0xC6, 0xC6, 0x7E, 0x06, 0xFC,
	// Character 122: 'z'
	0x00, 0x00, 0xFE, 0x8C, 0x18, 0x32, 0xFE, 0x00,
	// Character 123: '{'
	0x0E, 0x18, 0x18, 0x70, 0x18, 0x18, 0x0E, 0x00,
	// Character 124: '|'
	0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x00,
	// Character 125: '}'
	0x70, 0x18, 0x18, 0x0E, 0x18, 0x18, 0x70, 0x00,
	// Character 126: '~'
	0x76, 0xDC, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 127: DEL (block character)
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	}
}
