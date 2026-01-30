package main

import (
	"libs/gui"
	"runtime/graphics"
)

func main() {
	screenW, screenH := graphics.GetScreenSize()
	winW := screenW
	winH := screenH - 40
	w := graphics.CreateWindow("GUI Widgets Demo", int32(winW), int32(winH))
	ctx := gui.NewContext()

	// State variables
	showDemo := true
	showAnother := false
	showWidgets := true
	enabled := true
	volume := 0.5
	brightness := 75.0
	counter := 0
	progress := 0.35

	// Menu state
	menuState := gui.NewMenuState()

	// Tab state
	tabState := gui.NewTabState()
	tabLabels := []string{"Basic", "Lists", "Tree", "Inputs"}

	// Tree state
	treeState := gui.NewTreeNodeState()

	// Text input state
	textInput := gui.NewTextInputState("Hello")
	textInput2 := gui.NewTextInputState("")

	// ListBox state
	listItems := []string{"Apple", "Banana", "Cherry", "Date", "Elderberry", "Fig", "Grape", "Honeydew"}
	selectedItem := int32(0)
	scrollOffset := int32(0)

	// Spinner value
	spinnerValue := int32(50)

	// Radio button selection
	radioSelection := 0

	// Color picker
	pickedColor := graphics.NewColor(100, 150, 200, 255)

	// Draggable window states
	demoWin := gui.NewWindowState(20, 45, 350, 420)
	widgetsWin := gui.NewWindowState(390, 45, 380, 550)
	anotherWin := gui.NewWindowState(790, 45, 250, 180)
	infoWin := gui.NewWindowState(790, 240, 250, 200)
	sphereWin := gui.NewWindowState(790, 455, 250, 250)

	var clicked bool
	var fileMenuOpen bool
	var viewMenuOpen bool
	var dropY int32
	var dropX int32
	var fileDropX int32
	var viewDropX int32

	graphics.RunLoop(w, func(w graphics.Window) bool {
		// Update input first
		ctx = gui.UpdateInput(ctx, w)

		// Clear with very dark background
		graphics.Clear(w, graphics.NewColor(30, 30, 30, 255))

		// Menu bar at top - just draw the bar and detect which menu is open
		ctx, menuState = gui.BeginMenuBar(ctx, w, menuState, 0, 0, graphics.GetWidth(w))

		// File menu header
		ctx, menuState, fileMenuOpen = gui.Menu(ctx, w, menuState, "File")
		if fileMenuOpen {
			fileDropX = menuState.CurrentMenuX - menuState.CurrentMenuW
		}

		// View menu header
		ctx, menuState, viewMenuOpen = gui.Menu(ctx, w, menuState, "View")
		if viewMenuOpen {
			viewDropX = menuState.CurrentMenuX - menuState.CurrentMenuW
		}

		ctx, menuState = gui.EndMenuBar(ctx, menuState)

		// --- Z-ordered window rendering ---
		pass := int32(0)
		numPasses := gui.WindowPassCount(ctx)
		for pass < numPasses {
			ctx = gui.BeginWindowPass(ctx, pass)

			// Main demo panel (draggable)
			if showDemo {
				ctx, demoWin = gui.DraggablePanel(ctx, w, "Demo Window", demoWin)
				if ctx.DrawContent {
					ctx = gui.BeginLayout(ctx, demoWin.X+10, demoWin.Y+50, 6)

					ctx = gui.AutoLabel(ctx, w, "Hello from goany GUI!")

					gui.Separator(ctx, w, demoWin.X+10, ctx.CursorY-2, 330)
					ctx.CursorY = ctx.CursorY + 4

					// Buttons in a row
					ctx, clicked = gui.Button(ctx, w, "Click", demoWin.X+10, ctx.CursorY, 80, 26)
					if clicked {
						counter = counter + 1
					}
					ctx, clicked = gui.Button(ctx, w, "Reset", demoWin.X+100, ctx.CursorY, 80, 26)
					if clicked {
						counter = 0
						volume = 0.5
						brightness = 75.0
						progress = 0.35
					}
					gui.Label(ctx, w, "Count: "+intToString(counter), demoWin.X+190, ctx.CursorY+4)
					ctx = gui.NextRow(ctx, 26)

					gui.Separator(ctx, w, demoWin.X+10, ctx.CursorY-2, 330)
					ctx.CursorY = ctx.CursorY + 4

					// Checkboxes
					ctx, showDemo = gui.AutoCheckbox(ctx, w, "Show Demo Window", showDemo)
					ctx, showWidgets = gui.AutoCheckbox(ctx, w, "Show Widgets Window", showWidgets)
					ctx, showAnother = gui.AutoCheckbox(ctx, w, "Show Another Window", showAnother)
					ctx, enabled = gui.AutoCheckbox(ctx, w, "Enable Feature", enabled)

					gui.Separator(ctx, w, demoWin.X+10, ctx.CursorY-2, 330)
					ctx.CursorY = ctx.CursorY + 4

					// Sliders
					ctx, volume = gui.AutoSlider(ctx, w, "Volume", 320, 0.0, 1.0, volume)
					ctx, brightness = gui.AutoSlider(ctx, w, "Bright", 320, 0.0, 100.0, brightness)

					gui.Separator(ctx, w, demoWin.X+10, ctx.CursorY-2, 330)
					ctx.CursorY = ctx.CursorY + 4

					// Progress bar
					ctx = gui.AutoLabel(ctx, w, "Progress Bar:")
					gui.ProgressBar(ctx, w, demoWin.X+10, ctx.CursorY, 320, 20, progress)
					ctx = gui.NextRow(ctx, 24)

					// Animate progress
					progress = progress + 0.002
					if progress > 1.0 {
						progress = 0.0
					}
				}
			}

			// Widgets showcase window
			if showWidgets {
				ctx, widgetsWin = gui.DraggablePanel(ctx, w, "Widgets Showcase", widgetsWin)
				if ctx.DrawContent {
					// Tab bar
					ctx, tabState = gui.TabBar(ctx, w, tabState, tabLabels, widgetsWin.X+10, widgetsWin.Y+35)

					contentY := widgetsWin.Y + 70

					if tabState.ActiveTab == 0 {
						// Basic widgets tab
						ctx = gui.BeginLayout(ctx, widgetsWin.X+15, contentY, 6)

						ctx = gui.AutoLabel(ctx, w, "Radio Buttons:")
						ctx, clicked = gui.AutoRadioButton(ctx, w, "Option A", radioSelection == 0)
						if clicked {
							radioSelection = 0
						}
						ctx, clicked = gui.AutoRadioButton(ctx, w, "Option B", radioSelection == 1)
						if clicked {
							radioSelection = 1
						}
						ctx, clicked = gui.AutoRadioButton(ctx, w, "Option C", radioSelection == 2)
						if clicked {
							radioSelection = 2
						}

						gui.Separator(ctx, w, widgetsWin.X+15, ctx.CursorY, 350)
						ctx.CursorY = ctx.CursorY + 8

						ctx = gui.AutoLabel(ctx, w, "Spinner:")
						ctx, spinnerValue = gui.Spinner(ctx, w, "Value", widgetsWin.X+15, ctx.CursorY, spinnerValue, 0, 100)
						ctx = gui.NextRow(ctx, 28)

						gui.Separator(ctx, w, widgetsWin.X+15, ctx.CursorY, 350)
						ctx.CursorY = ctx.CursorY + 8

						// Color picker
						ctx, pickedColor = gui.ColorPicker(ctx, w, "Color Picker:", widgetsWin.X+15, ctx.CursorY, 340, pickedColor)

					} else if tabState.ActiveTab == 1 {
						// Lists tab
						ctx = gui.BeginLayout(ctx, widgetsWin.X+15, contentY, 6)

						ctx = gui.AutoLabel(ctx, w, "List Box (select an item):")
						ctx, selectedItem, scrollOffset = gui.ListBox(ctx, w, listItems, widgetsWin.X+15, ctx.CursorY, 200, 150, selectedItem, scrollOffset)
						ctx.CursorY = ctx.CursorY + 155

						ctx = gui.AutoLabel(ctx, w, "Selected: "+listItems[selectedItem])

					} else if tabState.ActiveTab == 2 {
						// Tree tab
						ctx = gui.BeginLayout(ctx, widgetsWin.X+15, contentY, 4)

						ctx = gui.AutoLabel(ctx, w, "Tree View:")

						var expanded bool
						nodeY := ctx.CursorY

						ctx, treeState, expanded = gui.TreeNode(ctx, w, treeState, "Root", widgetsWin.X+15, nodeY, 0)
						nodeY = nodeY + 20

						if expanded {
							ctx, treeState, expanded = gui.TreeNode(ctx, w, treeState, "Branch 1", widgetsWin.X+15, nodeY, 20)
							nodeY = nodeY + 20

							if expanded {
								ctx, clicked = gui.TreeLeaf(ctx, w, "Leaf 1.1", widgetsWin.X+15, nodeY, 40)
								nodeY = nodeY + 20
								ctx, clicked = gui.TreeLeaf(ctx, w, "Leaf 1.2", widgetsWin.X+15, nodeY, 40)
								nodeY = nodeY + 20
								ctx, clicked = gui.TreeLeaf(ctx, w, "Leaf 1.3", widgetsWin.X+15, nodeY, 40)
								nodeY = nodeY + 20
							}

							ctx, treeState, expanded = gui.TreeNode(ctx, w, treeState, "Branch 2", widgetsWin.X+15, nodeY, 20)
							nodeY = nodeY + 20

							if expanded {
								ctx, clicked = gui.TreeLeaf(ctx, w, "Leaf 2.1", widgetsWin.X+15, nodeY, 40)
								nodeY = nodeY + 20
								ctx, clicked = gui.TreeLeaf(ctx, w, "Leaf 2.2", widgetsWin.X+15, nodeY, 40)
								nodeY = nodeY + 20
							}

							ctx, clicked = gui.TreeLeaf(ctx, w, "Leaf 3", widgetsWin.X+15, nodeY, 20)
							nodeY = nodeY + 20
						}

					} else if tabState.ActiveTab == 3 {
						// Inputs tab
						ctx = gui.BeginLayout(ctx, widgetsWin.X+15, contentY, 8)

						ctx = gui.AutoLabel(ctx, w, "Text Input:")
						gui.Label(ctx, w, "Name:", widgetsWin.X+15, ctx.CursorY+4)
						ctx, textInput = gui.TextInput(ctx, w, textInput, widgetsWin.X+70, ctx.CursorY, 200)
						ctx = gui.NextRow(ctx, 28)

						gui.Label(ctx, w, "Email:", widgetsWin.X+15, ctx.CursorY+4)
						ctx, textInput2 = gui.TextInput(ctx, w, textInput2, widgetsWin.X+70, ctx.CursorY, 200)
						ctx = gui.NextRow(ctx, 28)

						gui.Separator(ctx, w, widgetsWin.X+15, ctx.CursorY, 350)
						ctx.CursorY = ctx.CursorY + 8

						ctx = gui.AutoLabel(ctx, w, "Entered text:")
						ctx = gui.AutoLabel(ctx, w, "  Name: "+textInput.Text)
						ctx = gui.AutoLabel(ctx, w, "  Email: "+textInput2.Text)
					}
				}
			}

			// Another window
			if showAnother {
				ctx, anotherWin = gui.DraggablePanel(ctx, w, "Another Window", anotherWin)
				if ctx.DrawContent {
					gui.Label(ctx, w, "This is another panel!", anotherWin.X+10, anotherWin.Y+50)
					ctx, clicked = gui.Button(ctx, w, "Close", anotherWin.X+10, anotherWin.Y+90, 100, 26)
					if clicked {
						showAnother = false
					}
				}
			}

			// Info panel
			ctx, infoWin = gui.DraggablePanel(ctx, w, "Info", infoWin)
			if ctx.DrawContent {
				ctx = gui.BeginLayout(ctx, infoWin.X+10, infoWin.Y+50, 4)
				ctx = gui.AutoLabel(ctx, w, "Application Stats:")
				ctx = gui.AutoLabel(ctx, w, "  Volume: "+floatToString(volume))
				ctx = gui.AutoLabel(ctx, w, "  Brightness: "+floatToString(brightness))
				ctx = gui.AutoLabel(ctx, w, "  Clicks: "+intToString(counter))
				ctx = gui.AutoLabel(ctx, w, "  Spinner: "+intToString(int(spinnerValue)))
				ctx = gui.AutoLabel(ctx, w, "  Radio: "+intToString(radioSelection))

				// Tooltip demo (show when hovering over info panel title area)
				if ctx.MouseX >= infoWin.X && ctx.MouseX <= infoWin.X+infoWin.Width &&
					ctx.MouseY >= infoWin.Y && ctx.MouseY <= infoWin.Y+30 {
					gui.Tooltip(ctx, w, "Drag to move this panel", ctx.MouseX, ctx.MouseY)
				}
			}

			// Sphere window
			ctx, sphereWin = gui.DraggablePanel(ctx, w, "3D Sphere", sphereWin)
			if ctx.DrawContent {
				drawSphere(w, sphereWin.X+125, sphereWin.Y+150, 80,
					graphics.NewColor(180, 180, 190, 255))
			}

			pass = pass + 1
		}
		ctx = gui.EndWindowPasses(ctx)

		// Quit button at bottom right
		ctx, clicked = gui.Button(ctx, w, "Quit", 1150, 900, 100, 30)
		if clicked {
			return false
		}

		// Draw menu dropdowns LAST so they appear on top of windows
		if fileMenuOpen {
			dropX = fileDropX
			ctx, dropY = gui.BeginDropdown(ctx, w, menuState, dropX, 5) // 5 items in File menu
			ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "New", dropX, dropY, 0)
			if clicked {
				counter = 0
			}
			ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "Open", dropX, dropY, 1)
			ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "Save", dropX, dropY, 2)
			gui.MenuItemSeparator(ctx, w, dropX, dropY, 3)
			ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "Exit", dropX, dropY, 4)
			if clicked {
				return false
			}
		}

		if viewMenuOpen {
			dropX = viewDropX
			ctx, dropY = gui.BeginDropdown(ctx, w, menuState, dropX, 3) // 3 items in View menu
			ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "Demo Window", dropX, dropY, 0)
			if clicked {
				showDemo = !showDemo
			}
			ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "Widgets Window", dropX, dropY, 1)
			if clicked {
				showWidgets = !showWidgets
			}
			ctx, menuState, clicked = gui.MenuItem(ctx, w, menuState, "Another Window", dropX, dropY, 2)
			if clicked {
				showAnother = !showAnother
			}
		}

		graphics.Present(w)
		return true
	})

	graphics.CloseWindow(w)
}

// intToString converts an integer to a string
func intToString(n int) string {
	digitStrings := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
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
		result = digitStrings[digit] + result
		n = n / 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

// floatToString converts a float to a string with 2 decimal places
func floatToString(f float64) string {
	// Integer part
	intPart := int(f)
	// Fractional part (2 decimals)
	fracPart := int((f - float64(intPart)) * 100)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	fracStr := intToString(fracPart)
	if fracPart < 10 {
		fracStr = "0" + fracStr
	}
	return intToString(intPart) + "." + fracStr
}

// sinTable returns sin values for 0-354 degrees in 6-degree steps, scaled by 1000
func sinTable() []int {
	return []int{0, 105, 208, 309, 407, 500, 588, 669, 743, 809,
		866, 914, 951, 978, 995, 1000, 995, 978, 951, 914,
		866, 809, 743, 669, 588, 500, 407, 309, 208, 105,
		0, -105, -208, -309, -407, -500, -588, -669, -743, -809,
		-866, -914, -951, -978, -995, -1000, -995, -978, -951, -914,
		-866, -809, -743, -669, -588, -500, -407, -309, -208, -105}
}

// cosTable returns cos values for 0-354 degrees in 6-degree steps, scaled by 1000
func cosTable() []int {
	return []int{1000, 995, 978, 951, 914, 866, 809, 743, 669, 588,
		500, 407, 309, 208, 105, 0, -105, -208, -309, -407,
		-500, -588, -669, -743, -809, -866, -914, -951, -978, -995,
		-1000, -995, -978, -951, -914, -866, -809, -743, -669, -588,
		-500, -407, -309, -208, -105, 0, 105, 208, 309, 407,
		500, 588, 669, 743, 809, 866, 914, 951, 978, 995}
}

// drawSphere draws a triangulated wireframe sphere using orthographic projection
func drawSphere(w graphics.Window, cx int32, cy int32, radius int32, color graphics.Color) {
	sin := sinTable()
	cos := cosTable()

	// Latitude indices: south pole (-90°) to north pole (+90°) in 30° steps
	// -90°=idx45, -60°=idx50, -30°=idx55, 0°=idx0, 30°=idx5, 60°=idx10, 90°=idx15
	latIdx := []int{45, 50, 55, 0, 5, 10, 15}

	// 12 longitude divisions (30° = 5 table entries each)
	numLon := 12

	lat := 0
	for lat < 6 {
		li0 := latIdx[lat]
		li1 := latIdx[lat+1]
		sinLat0 := sin[li0]
		cosLat0 := cos[li0]
		sinLat1 := sin[li1]
		cosLat1 := cos[li1]
		y0 := int32(sinLat0) * radius / 1000
		y1 := int32(sinLat1) * radius / 1000

		lon := 0
		for lon < numLon {
			lonIdx0 := (lon * 5) % 60
			lonIdx1 := ((lon + 1) * 5) % 60

			x00 := int32((cosLat0 * cos[lonIdx0]) / 1000) * radius / 1000
			x01 := int32((cosLat0 * cos[lonIdx1]) / 1000) * radius / 1000
			x10 := int32((cosLat1 * cos[lonIdx0]) / 1000) * radius / 1000
			x11 := int32((cosLat1 * cos[lonIdx1]) / 1000) * radius / 1000

			// Latitude edges (horizontal)
			graphics.DrawLine(w, cx+x00, cy-y0, cx+x01, cy-y0, color)
			graphics.DrawLine(w, cx+x10, cy-y1, cx+x11, cy-y1, color)
			// Longitude edge (vertical)
			graphics.DrawLine(w, cx+x00, cy-y0, cx+x10, cy-y1, color)
			// Diagonal to form triangles
			graphics.DrawLine(w, cx+x00, cy-y0, cx+x11, cy-y1, color)

			lon = lon + 1
		}
		lat = lat + 1
	}
}
