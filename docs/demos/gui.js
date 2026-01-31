// Generated JavaScript code
"use strict";

// Runtime helpers
function len(arr) {
  if (typeof arr === "string") return arr.length;
  if (Array.isArray(arr)) return arr.length;
  return 0;
}

function append(arr, ...items) {
  // Handle nil/undefined slices like Go does
  if (arr == null) arr = [];
  // Clone plain objects to preserve Go's value semantics for structs
  // Use push for O(1) amortized instead of spread which is O(n)
  for (const item of items) {
    if (item && typeof item === "object" && !Array.isArray(item)) {
      arr.push({ ...item });
    } else {
      arr.push(item);
    }
  }
  return arr;
}

function stringFormat(fmt, ...args) {
  let i = 0;
  return fmt.replace(/%[sdvfxc%]/g, (match) => {
    if (match === "%%") return "%";
    if (i >= args.length) return match;
    const arg = args[i++];
    switch (match) {
      case "%s":
        return String(arg);
      case "%d":
        return parseInt(arg, 10);
      case "%f":
        return parseFloat(arg);
      case "%v":
        return String(arg);
      case "%x":
        return parseInt(arg, 10).toString(16);
      case "%c":
        return String.fromCharCode(arg);
      default:
        return arg;
    }
  });
}

// printf - like fmt.Printf (no newline)
function printf(fmt, ...args) {
  const str = stringFormat(fmt, ...args);
  if (typeof process !== "undefined" && process.stdout) {
    process.stdout.write(str);
  } else {
    // Browser fallback - accumulate output
    if (typeof window !== "undefined") {
      window._printBuffer = (window._printBuffer || "") + str;
    }
  }
}

// print - like fmt.Print (no newline)
function print(...args) {
  const str = args.map((a) => String(a)).join(" ");
  if (typeof process !== "undefined" && process.stdout) {
    process.stdout.write(str);
  } else {
    if (typeof window !== "undefined") {
      window._printBuffer = (window._printBuffer || "") + str;
    }
  }
}

function make(type, length, capacity) {
  if (Array.isArray(type)) {
    return new Array(length || 0).fill(type[0] === "number" ? 0 : null);
  }
  return [];
}

// Type conversion functions
// Handle string-to-int conversion for character codes (Go rune semantics)
function int8(v) {
  return typeof v === "string" ? v.charCodeAt(0) | 0 : v | 0;
}
function int16(v) {
  return typeof v === "string" ? v.charCodeAt(0) | 0 : v | 0;
}
function int32(v) {
  return typeof v === "string" ? v.charCodeAt(0) | 0 : v | 0;
}
function int64(v) {
  return typeof v === "string" ? v.charCodeAt(0) : v;
} // BigInt not used for simplicity
function int(v) {
  return typeof v === "string" ? v.charCodeAt(0) | 0 : v | 0;
}
function uint8(v) {
  return typeof v === "string" ? v.charCodeAt(0) & 0xff : (v | 0) & 0xff;
}
function uint16(v) {
  return typeof v === "string" ? v.charCodeAt(0) & 0xffff : (v | 0) & 0xffff;
}
function uint32(v) {
  return typeof v === "string" ? v.charCodeAt(0) >>> 0 : (v | 0) >>> 0;
}
function uint64(v) {
  return typeof v === "string" ? v.charCodeAt(0) : v;
} // BigInt not used for simplicity
function float32(v) {
  return v;
}
function float64(v) {
  return v;
}
function string(v) {
  return String(v);
}
function bool(v) {
  return Boolean(v);
}

// Graphics runtime for Canvas
const graphics = {
  canvas: null,
  ctx: null,
  running: true,
  keys: {},
  lastKey: 0,
  mouseX: 0,
  mouseY: 0,
  mouseDown: false,

  _setupEventListeners: function () {
    window.addEventListener("keydown", (e) => {
      this.keys[e.key] = true;
      // Store ASCII code for GetLastKey
      if (e.key.length === 1) {
        this.lastKey = e.key.charCodeAt(0);
      } else {
        // Map special keys to ASCII codes
        const specialKeys = {
          Enter: 13,
          Backspace: 8,
          Tab: 9,
          Escape: 27,
          ArrowUp: 38,
          ArrowDown: 40,
          ArrowLeft: 37,
          ArrowRight: 39,
          Delete: 127,
          Space: 32,
        };
        if (specialKeys[e.key]) {
          this.lastKey = specialKeys[e.key];
        }
      }
    });
    window.addEventListener("keyup", (e) => {
      this.keys[e.key] = false;
    });
    this.canvas.addEventListener("mousemove", (e) => {
      const rect = this.canvas.getBoundingClientRect();
      this.mouseX = e.clientX - rect.left;
      this.mouseY = e.clientY - rect.top;
    });
    this.canvas.addEventListener("mousedown", () => {
      this.mouseDown = true;
    });
    this.canvas.addEventListener("mouseup", () => {
      this.mouseDown = false;
    });
  },

  CreateWindow: function (title, width, height) {
    this.canvas = document.createElement("canvas");
    this.canvas.width = width;
    this.canvas.height = height;
    this.ctx = this.canvas.getContext("2d");
    document.body.appendChild(this.canvas);
    document.title = title;

    this.windowObj = {
      canvas: this.canvas,
      width: width,
      height: height,
    };

    this._setupEventListeners();
    return this.windowObj;
  },

  CreateWindowFullscreen: function (title, width, height) {
    document.body.style.margin = "0";
    document.body.style.padding = "0";
    document.body.style.overflow = "hidden";

    this.canvas = document.createElement("canvas");
    this.canvas.width = window.innerWidth;
    this.canvas.height = window.innerHeight;
    this.canvas.style.display = "block";
    this.ctx = this.canvas.getContext("2d");
    document.body.appendChild(this.canvas);
    document.title = title;

    this.windowObj = {
      canvas: this.canvas,
      width: this.canvas.width,
      height: this.canvas.height,
    };

    window.addEventListener("resize", () => {
      this.canvas.width = window.innerWidth;
      this.canvas.height = window.innerHeight;
      this.windowObj.width = this.canvas.width;
      this.windowObj.height = this.canvas.height;
    });

    this._setupEventListeners();
    return this.windowObj;
  },

  NewColor: function (r, g, b, a) {
    return { R: r, G: g, B: b, A: a !== undefined ? a : 255 };
  },

  // Color helper functions
  Red: function () {
    return { R: 255, G: 0, B: 0, A: 255 };
  },
  Green: function () {
    return { R: 0, G: 255, B: 0, A: 255 };
  },
  Blue: function () {
    return { R: 0, G: 0, B: 255, A: 255 };
  },
  White: function () {
    return { R: 255, G: 255, B: 255, A: 255 };
  },
  Black: function () {
    return { R: 0, G: 0, B: 0, A: 255 };
  },

  Clear: function (canvas, color) {
    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.fillRect(0, 0, canvas.width, canvas.height);
  },

  FillRect: function (canvas, rect, color) {
    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.fillRect(rect.x, rect.y, rect.width, rect.height);
  },

  DrawRect: function (canvas, rect, color) {
    this.ctx.strokeStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.strokeRect(rect.x, rect.y, rect.width, rect.height);
  },

  NewRect: function (x, y, width, height) {
    return { x, y, width, height };
  },

  FillCircle: function (canvas, centerX, centerY, radius, color) {
    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.beginPath();
    this.ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
    this.ctx.fill();
  },

  DrawCircle: function (canvas, centerX, centerY, radius, color) {
    this.ctx.strokeStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.beginPath();
    this.ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
    this.ctx.stroke();
  },

  DrawPoint: function (canvas, x, y, color) {
    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.fillRect(x, y, 1, 1);
  },

  DrawLine: function (canvas, x1, y1, x2, y2, color) {
    this.ctx.strokeStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.beginPath();
    this.ctx.moveTo(x1, y1);
    this.ctx.lineTo(x2, y2);
    this.ctx.stroke();
  },

  SetPixel: function (canvas, x, y, color) {
    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;
    this.ctx.fillRect(x, y, 1, 1);
  },

  PollEvents: function (canvas) {
    return [canvas, this.running];
  },

  Update: function (canvas) {
    // Canvas updates automatically
  },

  KeyDown: function (canvas, key) {
    return this.keys[key] || false;
  },

  GetLastKey: function () {
    const key = this.lastKey;
    this.lastKey = 0; // Clear after reading (like native backends)
    return key;
  },

  GetMousePos: function (canvas) {
    return [this.mouseX, this.mouseY];
  },

  GetMouse: function (canvas) {
    // Returns [x, y, buttons] like other backends
    return [this.mouseX, this.mouseY, this.mouseDown ? 1 : 0];
  },

  GetWidth: function (w) {
    return w.width;
  },

  GetHeight: function (w) {
    return w.height;
  },

  GetScreenSize: function () {
    return [window.screen.width, window.screen.height];
  },

  MouseDown: function (canvas) {
    return this.mouseDown;
  },

  Closed: function (canvas) {
    return !this.running;
  },

  Free: function (canvas) {
    if (canvas && canvas.parentNode) {
      canvas.parentNode.removeChild(canvas);
    }
  },

  Present: function (canvas) {
    // Canvas updates automatically, no-op
  },

  CloseWindow: function (canvas) {
    // In browser context, don't immediately close - RunLoop is async
    // The canvas will remain until the page is closed
  },

  RunLoop: function (canvas, frameFunc) {
    const self = this;
    function loop() {
      if (!self.running) return;
      // Poll events (update internal state)
      const result = frameFunc(canvas);
      if (result === false) {
        self.running = false;
        return;
      }
      requestAnimationFrame(loop);
    }
    requestAnimationFrame(loop);
  },

  RunLoopWithState: function (canvas, state, frameFunc) {
    const self = this;
    function loop() {
      if (!self.running) return;
      // Call frameFunc with canvas and state, returns [newState, shouldContinue]
      const result = frameFunc(canvas, state);
      state = result[0];
      if (result[1] === false) {
        self.running = false;
        return;
      }
      requestAnimationFrame(loop);
    }
    requestAnimationFrame(loop);
  },
};

const gui = {
  NewWindowState: function (x, y, width, height) {
    return {
      X: x,
      Y: y,
      Width: width,
      Height: height,
      Dragging: false,
      DragOffsetX: 0,
      DragOffsetY: 0,
    };
  },
  NewMenuState: function () {
    return {
      OpenMenuID: 0,
      MenuBarX: 0,
      MenuBarY: 0,
      MenuBarH: 0,
      CurrentMenuX: 0,
      CurrentMenuW: 0,
      ClickedOutside: false,
    };
  },
  DefaultStyle: function () {
    return {
      BackgroundColor: graphics.NewColor(45, 45, 48, 250),
      TextColor: graphics.NewColor(240, 240, 245, 255),
      ButtonColor: graphics.NewColor(90, 90, 95, 200),
      ButtonHoverColor: graphics.NewColor(110, 110, 115, 230),
      ButtonActiveColor: graphics.NewColor(70, 70, 75, 255),
      CheckboxColor: graphics.NewColor(60, 60, 65, 255),
      CheckmarkColor: graphics.NewColor(200, 200, 210, 255),
      SliderTrackColor: graphics.NewColor(130, 130, 140, 220),
      SliderKnobColor: graphics.NewColor(180, 180, 190, 255),
      BorderColor: graphics.NewColor(65, 65, 70, 255),
      FrameBgColor: graphics.NewColor(50, 50, 55, 220),
      TitleBgColor: graphics.NewColor(75, 75, 80, 255),
      FontSize: 1,
      Padding: 8,
      ButtonHeight: 24,
      SliderHeight: 20,
      CheckboxSize: 18,
      FrameRounding: 3,
    };
  },
  NewContext: function () {
    return {
      Style: this.DefaultStyle(),
      MouseX: 0,
      MouseY: 0,
      MouseDown: false,
      MouseClicked: false,
      MouseReleased: false,
      HotID: 0,
      ActiveID: 0,
      ReleasedID: 0,
      CursorX: 0,
      CursorY: 0,
      Spacing: 0,
      WindowZOrder: [],
      RenderPass: 0,
      DrawContent: false,
      ZOrderReady: false,
    };
  },
  GenID: function (label) {
    let hash = int32(5381);
    for (let i = 0; i < len(label); i++) {
      hash = (hash << 5) + hash + int32(label.charCodeAt(i));
    }
    if (hash < 0) {
      hash = -hash;
    }
    return hash;
  },
  UpdateInput: function (ctx, w) {
    let prevDown = ctx.MouseDown;
    let [x, y, buttons] = graphics.GetMouse(w);
    ctx.MouseX = x;
    ctx.MouseY = y;
    ctx.MouseDown = (buttons & 1) != 0;
    ctx.MouseClicked = ctx.MouseDown && !prevDown;
    ctx.MouseReleased = !ctx.MouseDown && prevDown;
    ctx.ReleasedID = 0;
    if (ctx.MouseReleased) {
      ctx.ReleasedID = ctx.ActiveID;
      ctx.ActiveID = 0;
    }
    ctx.HotID = 0;
    return ctx;
  },
  drawChar: function (w, charCode, x, y, scale, color) {
    if (charCode < 32 || charCode > 127) {
      charCode = 32;
    }
    let offset = (charCode - 32) * 8;
    let font = this.getFontData();
    for (let row = int32(0); row < 8; row++) {
      let rowData = font[offset + int(row)];
      for (let col = int32(0); col < 8; col++) {
        if ((rowData & (0x80 >> col)) != 0) {
          for (let sy = int32(0); sy < scale; sy++) {
            for (let sx = int32(0); sx < scale; sx++) {
              graphics.DrawPoint(
                w,
                x + col * scale + sx,
                y + row * scale + sy,
                color,
              );
            }
          }
        }
      }
    }
  },
  DrawText: function (w, text, x, y, scale, color) {
    let curX = x;
    for (let i = 0; i < len(text); i++) {
      let ch = int(text.charCodeAt(i));
      this.drawChar(w, ch, curX, y, scale, color);
      curX = curX + 8 * scale;
    }
  },
  TextWidth: function (text, scale) {
    return int32(len(text)) * 8 * scale;
  },
  TextHeight: function (scale) {
    return 8 * scale;
  },
  pointInRect: function (px, py, x, y, w, h) {
    return px >= x && px < x + w && py >= y && py < y + h;
  },
  drawHGradient: function (w, x, y, width, height, r1, g1, b1, r2, g2, b2) {
    let step = int32(3);
    let i = int32(0);
    for (; i < width; ) {
      let sw = step;
      if (i + sw > width) {
        sw = width - i;
      }
      let t = ((i * 1000) / width) | 0;
      let r = r1 + ((((r2 - r1) * t) / 1000) | 0);
      let g = g1 + ((((g2 - g1) * t) / 1000) | 0);
      let b = b1 + ((((b2 - b1) * t) / 1000) | 0);
      graphics.FillRect(
        w,
        graphics.NewRect(x + i, y, sw, height),
        graphics.NewColor(uint8(r), uint8(g), uint8(b), 255),
      );
      i = i + step;
    }
  },
  registerWindow: function (ctx, id) {
    let i = 0;
    for (; i < len(ctx.WindowZOrder); ) {
      if (ctx.WindowZOrder[i] == id) {
        return ctx;
      }
      i = i + 1;
    }
    ctx.WindowZOrder = append(ctx.WindowZOrder, id);
    return ctx;
  },
  bringWindowToFront: function (ctx, id) {
    let newOrder = [];
    let i = 0;
    for (; i < len(ctx.WindowZOrder); ) {
      if (ctx.WindowZOrder[i] != id) {
        newOrder = append(newOrder, ctx.WindowZOrder[i]);
      }
      i = i + 1;
    }
    newOrder = append(newOrder, id);
    ctx.WindowZOrder = newOrder;
    return ctx;
  },
  WindowPassCount: function (ctx) {
    if (!ctx.ZOrderReady || len(ctx.WindowZOrder) == 0) {
      return 1;
    }
    return int32(len(ctx.WindowZOrder));
  },
  BeginWindowPass: function (ctx, pass) {
    ctx.RenderPass = pass;
    ctx.DrawContent = false;
    return ctx;
  },
  EndWindowPasses: function (ctx) {
    if (!ctx.ZOrderReady && len(ctx.WindowZOrder) > 0) {
      ctx.ZOrderReady = true;
    }
    return ctx;
  },
  Label: function (ctx, w, text, x, y) {
    this.DrawText(w, text, x, y, ctx.Style.FontSize, ctx.Style.TextColor);
  },
  Button: function (ctx, w, label, x, y, width, height) {
    let id = this.GenID(label);
    let hovered = this.pointInRect(ctx.MouseX, ctx.MouseY, x, y, width, height);
    if (hovered) {
      ctx.HotID = id;
      if (ctx.MouseClicked) {
        ctx.ActiveID = id;
      }
    }
    let bgColor = { R: 0, G: 0, B: 0, A: 0 };
    let pressOffset = int32(0);
    if (ctx.ActiveID == id && hovered) {
      bgColor = ctx.Style.ButtonActiveColor;
      pressOffset = 1;
    } else if (ctx.HotID == id) {
      bgColor = ctx.Style.ButtonHoverColor;
    } else {
      bgColor = ctx.Style.ButtonColor;
    }
    graphics.FillRect(
      w,
      graphics.NewRect(x + 2, y + 2, width, height),
      graphics.NewColor(0, 0, 0, 70),
    );
    graphics.FillRect(w, graphics.NewRect(x, y, width, height), bgColor);
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + width - 2,
      y + 1,
      graphics.NewColor(255, 255, 255, 100),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 2,
      x + width - 2,
      y + 2,
      graphics.NewColor(255, 255, 255, 60),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 3,
      x + width - 2,
      y + 3,
      graphics.NewColor(255, 255, 255, 30),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + 1,
      y + height - 2,
      graphics.NewColor(255, 255, 255, 80),
    );
    graphics.DrawLine(
      w,
      x + 2,
      y + 2,
      x + 2,
      y + height - 3,
      graphics.NewColor(255, 255, 255, 40),
    );
    graphics.DrawLine(
      w,
      x + 2,
      y + height - 1,
      x + width - 1,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 120),
    );
    graphics.DrawLine(
      w,
      x + 2,
      y + height - 2,
      x + width - 2,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 70),
    );
    graphics.DrawLine(
      w,
      x + 2,
      y + height - 3,
      x + width - 2,
      y + height - 3,
      graphics.NewColor(0, 0, 0, 30),
    );
    graphics.DrawLine(
      w,
      x + width - 1,
      y + 2,
      x + width - 1,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 120),
    );
    graphics.DrawLine(
      w,
      x + width - 2,
      y + 3,
      x + width - 2,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(x, y, width, height),
      graphics.NewColor(30, 30, 35, 255),
    );
    let textW = this.TextWidth(label, ctx.Style.FontSize);
    let textH = this.TextHeight(ctx.Style.FontSize);
    let textX = x + (((width - textW) / 2) | 0) + pressOffset;
    let textY = y + (((height - textH) / 2) | 0) + pressOffset;
    this.DrawText(
      w,
      label,
      textX,
      textY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    let clicked = ctx.ReleasedID == id && ctx.MouseReleased && hovered;
    return [ctx, clicked];
  },
  Checkbox: function (ctx, w, label, x, y, value) {
    let id = this.GenID(label);
    let boxSize = ctx.Style.CheckboxSize;
    let labelW = this.TextWidth(label, ctx.Style.FontSize);
    let totalW = boxSize + ctx.Style.Padding + labelW;
    let hovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      x,
      y,
      totalW,
      boxSize,
    );
    if (hovered) {
      ctx.HotID = id;
      if (ctx.MouseClicked) {
        ctx.ActiveID = id;
      }
    }
    let boxColor = { R: 0, G: 0, B: 0, A: 0 };
    if (ctx.HotID == id) {
      boxColor = ctx.Style.ButtonHoverColor;
    } else {
      boxColor = ctx.Style.FrameBgColor;
    }
    graphics.FillRect(
      w,
      graphics.NewRect(x + 1, y + 1, boxSize, boxSize),
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.FillRect(w, graphics.NewRect(x, y, boxSize, boxSize), boxColor);
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + boxSize - 2,
      y + 1,
      graphics.NewColor(255, 255, 255, 90),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 2,
      x + boxSize - 3,
      y + 2,
      graphics.NewColor(255, 255, 255, 50),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + 1,
      y + boxSize - 2,
      graphics.NewColor(255, 255, 255, 70),
    );
    graphics.DrawLine(
      w,
      x + 2,
      y + 2,
      x + 2,
      y + boxSize - 3,
      graphics.NewColor(255, 255, 255, 35),
    );
    graphics.DrawLine(
      w,
      x + 2,
      y + boxSize - 1,
      x + boxSize - 1,
      y + boxSize - 1,
      graphics.NewColor(0, 0, 0, 100),
    );
    graphics.DrawLine(
      w,
      x + 3,
      y + boxSize - 2,
      x + boxSize - 2,
      y + boxSize - 2,
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.DrawLine(
      w,
      x + boxSize - 1,
      y + 2,
      x + boxSize - 1,
      y + boxSize - 1,
      graphics.NewColor(0, 0, 0, 100),
    );
    graphics.DrawLine(
      w,
      x + boxSize - 2,
      y + 3,
      x + boxSize - 2,
      y + boxSize - 2,
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(x, y, boxSize, boxSize),
      graphics.NewColor(60, 65, 75, 255),
    );
    if (value) {
      let checkColor = ctx.Style.CheckmarkColor;
      let cx = x + ((boxSize / 2) | 0);
      let cy = y + ((boxSize / 2) | 0);
      graphics.DrawLine(w, cx - 5, cy - 1, cx - 2, cy + 3, checkColor);
      graphics.DrawLine(w, cx - 5, cy, cx - 2, cy + 4, checkColor);
      graphics.DrawLine(w, cx - 4, cy, cx - 1, cy + 4, checkColor);
      graphics.DrawLine(w, cx - 2, cy + 3, cx + 5, cy - 4, checkColor);
      graphics.DrawLine(w, cx - 2, cy + 4, cx + 5, cy - 3, checkColor);
      graphics.DrawLine(w, cx - 1, cy + 4, cx + 6, cy - 3, checkColor);
    }
    let labelX = x + boxSize + ctx.Style.Padding;
    let labelY =
      y + (((boxSize - this.TextHeight(ctx.Style.FontSize)) / 2) | 0);
    this.DrawText(
      w,
      label,
      labelX,
      labelY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    let newValue = value;
    if (ctx.ReleasedID == id && ctx.MouseReleased && hovered) {
      newValue = !value;
    }
    return [ctx, newValue];
  },
  Slider: function (ctx, w, label, x, y, width, min, max, value) {
    let id = this.GenID(label);
    let height = ctx.Style.SliderHeight;
    let grabW = int32(12);
    let labelW = this.TextWidth(label, ctx.Style.FontSize);
    let labelY = y + (((height - this.TextHeight(ctx.Style.FontSize)) / 2) | 0);
    this.DrawText(w, label, x, labelY, ctx.Style.FontSize, ctx.Style.TextColor);
    let trackX = x + labelW + ctx.Style.Padding;
    let trackW = width - labelW - ctx.Style.Padding;
    if (value < min) {
      value = min;
    }
    if (value > max) {
      value = max;
    }
    let rangeVal = max - min;
    if (rangeVal == 0) {
      rangeVal = 1;
    }
    let t = (value - min) / rangeVal;
    let grabRange = trackW - grabW;
    let grabX = trackX + int32(float64(grabRange) * t);
    let hovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      trackX,
      y,
      trackW,
      height,
    );
    if (hovered) {
      ctx.HotID = id;
      if (ctx.MouseClicked) {
        ctx.ActiveID = id;
      }
    }
    graphics.FillRect(
      w,
      graphics.NewRect(trackX + 1, y + 1, trackW, height),
      graphics.NewColor(0, 0, 0, 40),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(trackX, y, trackW, height),
      ctx.Style.FrameBgColor,
    );
    graphics.DrawLine(
      w,
      trackX + 1,
      y + 1,
      trackX + trackW - 2,
      y + 1,
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.DrawLine(
      w,
      trackX + 1,
      y + 1,
      trackX + 1,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 60),
    );
    let fillW = grabX - trackX + ((grabW / 2) | 0);
    if (fillW > 0) {
      graphics.FillRect(
        w,
        graphics.NewRect(trackX + 2, y + 2, fillW - 2, height - 4),
        ctx.Style.SliderTrackColor,
      );
      graphics.DrawLine(
        w,
        trackX + 2,
        y + 2,
        trackX + fillW - 1,
        y + 2,
        graphics.NewColor(255, 255, 255, 40),
      );
    }
    graphics.DrawRect(
      w,
      graphics.NewRect(trackX, y, trackW, height),
      graphics.NewColor(50, 55, 65, 255),
    );
    let grabColor = { R: 0, G: 0, B: 0, A: 0 };
    if (ctx.ActiveID == id) {
      grabColor = ctx.Style.ButtonActiveColor;
    } else if (ctx.HotID == id) {
      grabColor = ctx.Style.ButtonHoverColor;
    } else {
      grabColor = ctx.Style.SliderKnobColor;
    }
    graphics.FillRect(
      w,
      graphics.NewRect(grabX + 2, y + 2, grabW, height),
      graphics.NewColor(0, 0, 0, 70),
    );
    graphics.FillRect(w, graphics.NewRect(grabX, y, grabW, height), grabColor);
    graphics.DrawLine(
      w,
      grabX + 1,
      y + 1,
      grabX + grabW - 2,
      y + 1,
      graphics.NewColor(255, 255, 255, 100),
    );
    graphics.DrawLine(
      w,
      grabX + 1,
      y + 2,
      grabX + grabW - 3,
      y + 2,
      graphics.NewColor(255, 255, 255, 50),
    );
    graphics.DrawLine(
      w,
      grabX + 1,
      y + 1,
      grabX + 1,
      y + height - 2,
      graphics.NewColor(255, 255, 255, 80),
    );
    graphics.DrawLine(
      w,
      grabX + 2,
      y + 2,
      grabX + 2,
      y + height - 3,
      graphics.NewColor(255, 255, 255, 40),
    );
    graphics.DrawLine(
      w,
      grabX + 2,
      y + height - 1,
      grabX + grabW - 1,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 120),
    );
    graphics.DrawLine(
      w,
      grabX + 3,
      y + height - 2,
      grabX + grabW - 2,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.DrawLine(
      w,
      grabX + grabW - 1,
      y + 2,
      grabX + grabW - 1,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 120),
    );
    graphics.DrawLine(
      w,
      grabX + grabW - 2,
      y + 3,
      grabX + grabW - 2,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(grabX, y, grabW, height),
      graphics.NewColor(40, 45, 55, 255),
    );
    if (ctx.ActiveID == id && ctx.MouseDown) {
      let mouseT =
        float64(ctx.MouseX - trackX - ((grabW / 2) | 0)) / float64(grabRange);
      if (mouseT < 0) {
        mouseT = 0;
      }
      if (mouseT > 1) {
        mouseT = 1;
      }
      value = min + mouseT * rangeVal;
    }
    return [ctx, value];
  },
  Panel: function (ctx, w, title, x, y, width, height) {
    let titleH = this.TextHeight(ctx.Style.FontSize) + ctx.Style.Padding * 2;
    graphics.FillRect(
      w,
      graphics.NewRect(x + 4, y + 4, width, height),
      graphics.NewColor(0, 0, 0, 40),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(x + 3, y + 3, width, height),
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(x + 2, y + 2, width, height),
      graphics.NewColor(0, 0, 0, 60),
    );
    this.drawHGradient(w, x, y, width, titleH, 25, 25, 30, 85, 85, 90);
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + width - 2,
      y + 1,
      graphics.NewColor(255, 255, 255, 60),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 2,
      x + width - 2,
      y + 2,
      graphics.NewColor(255, 255, 255, 25),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + 1,
      y + titleH - 2,
      graphics.NewColor(255, 255, 255, 35),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + titleH - 2,
      x + width - 2,
      y + titleH - 2,
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + titleH - 1,
      x + width - 2,
      y + titleH - 1,
      graphics.NewColor(0, 0, 0, 80),
    );
    this.DrawText(
      w,
      title,
      x + ctx.Style.Padding,
      y + (((titleH - this.TextHeight(ctx.Style.FontSize)) / 2) | 0),
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    graphics.FillRect(
      w,
      graphics.NewRect(x, y + titleH, width, height - titleH),
      ctx.Style.BackgroundColor,
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + titleH,
      x + width - 2,
      y + titleH,
      graphics.NewColor(255, 255, 255, 15),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(x, y, width, height),
      graphics.NewColor(40, 45, 55, 255),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + titleH + 1,
      x + 1,
      y + height - 2,
      graphics.NewColor(255, 255, 255, 10),
    );
  },
  DraggablePanel: function (ctx, w, title, state) {
    let idStr = title;
    idStr += "_panel";
    let id = this.GenID(idStr);
    let titleH = this.TextHeight(ctx.Style.FontSize) + ctx.Style.Padding * 2;
    ctx = this.registerWindow(ctx, id);
    let shouldRender = true;
    if (ctx.ZOrderReady && len(ctx.WindowZOrder) > 0) {
      shouldRender = false;
      if (
        ctx.RenderPass >= 0 &&
        ctx.RenderPass < int32(len(ctx.WindowZOrder))
      ) {
        if (ctx.WindowZOrder[ctx.RenderPass] == id) {
          shouldRender = true;
        }
      }
    }
    if (!shouldRender) {
      ctx.DrawContent = false;
      return [ctx, state];
    }
    let inWindow = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      state.X,
      state.Y,
      state.Width,
      state.Height,
    );
    if (inWindow && ctx.MouseClicked) {
      ctx = this.bringWindowToFront(ctx, id);
    }
    let inTitleBar = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      state.X,
      state.Y,
      state.Width,
      titleH,
    );
    if (inTitleBar && ctx.MouseClicked) {
      state.Dragging = true;
      state.DragOffsetX = ctx.MouseX - state.X;
      state.DragOffsetY = ctx.MouseY - state.Y;
      ctx.ActiveID = id;
    }
    if (state.Dragging && ctx.MouseDown) {
      state.X = ctx.MouseX - state.DragOffsetX;
      state.Y = ctx.MouseY - state.DragOffsetY;
    }
    if (state.Dragging && ctx.MouseReleased) {
      state.Dragging = false;
      if (ctx.ActiveID == id) {
        ctx.ActiveID = 0;
      }
    }
    this.Panel(ctx, w, title, state.X, state.Y, state.Width, state.Height);
    ctx.DrawContent = true;
    return [ctx, state];
  },
  Separator: function (ctx, w, x, y, width) {
    graphics.DrawLine(
      w,
      x,
      y,
      x + width,
      y,
      graphics.NewColor(25, 30, 40, 255),
    );
    graphics.DrawLine(
      w,
      x,
      y + 1,
      x + width,
      y + 1,
      graphics.NewColor(65, 70, 80, 255),
    );
  },
  BeginMenuBar: function (ctx, w, state, x, y, width) {
    let height = this.TextHeight(ctx.Style.FontSize) + ctx.Style.Padding * 2;
    this.drawHGradient(w, x, y, width, height, 25, 25, 30, 85, 85, 90);
    graphics.DrawLine(
      w,
      x,
      y + 1,
      x + width - 1,
      y + 1,
      graphics.NewColor(255, 255, 255, 25),
    );
    graphics.DrawLine(
      w,
      x,
      y + height - 1,
      x + width,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.DrawLine(
      w,
      x,
      y + height,
      x + width,
      y + height,
      graphics.NewColor(0, 0, 0, 30),
    );
    state.MenuBarX = x;
    state.MenuBarY = y;
    state.MenuBarH = height;
    state.CurrentMenuX = x + ctx.Style.Padding;
    state.ClickedOutside = ctx.MouseClicked;
    return [ctx, state];
  },
  EndMenuBar: function (ctx, state) {
    if (state.ClickedOutside && state.OpenMenuID != 0) {
      state.OpenMenuID = 0;
    }
    return [ctx, state];
  },
  Menu: function (ctx, w, state, label) {
    let id = this.GenID(label);
    let padding = ctx.Style.Padding;
    let textW = this.TextWidth(label, ctx.Style.FontSize);
    let textH = this.TextHeight(ctx.Style.FontSize);
    let menuW = textW + padding * 2;
    let menuH = state.MenuBarH;
    let x = state.CurrentMenuX;
    let y = state.MenuBarY;
    let hovered = this.pointInRect(ctx.MouseX, ctx.MouseY, x, y, menuW, menuH);
    let isOpen = state.OpenMenuID == id;
    if (hovered || isOpen) {
      graphics.FillRect(
        w,
        graphics.NewRect(x, y + 1, menuW, menuH - 2),
        ctx.Style.ButtonHoverColor,
      );
      graphics.DrawLine(
        w,
        x + 1,
        y + 2,
        x + menuW - 2,
        y + 2,
        graphics.NewColor(255, 255, 255, 30),
      );
      if (ctx.MouseClicked) {
        if (isOpen) {
          state.OpenMenuID = 0;
          isOpen = false;
        } else {
          state.OpenMenuID = id;
          isOpen = true;
        }
        state.ClickedOutside = false;
      }
    }
    let textY = y + (((menuH - textH) / 2) | 0);
    this.DrawText(
      w,
      label,
      x + padding,
      textY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    state.CurrentMenuW = menuW;
    state.CurrentMenuX = x + menuW;
    return [ctx, state, isOpen];
  },
  BeginDropdown: function (ctx, w, state, dropX, itemCount) {
    let dropY = state.MenuBarY + state.MenuBarH;
    let padding = ctx.Style.Padding;
    let textH = this.TextHeight(ctx.Style.FontSize);
    let itemH = textH + padding * 2;
    let itemW = int32(160);
    let totalH = itemH * itemCount;
    graphics.FillRect(
      w,
      graphics.NewRect(dropX + 3, dropY + 3, itemW, totalH),
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(dropX + 2, dropY + 2, itemW, totalH),
      graphics.NewColor(0, 0, 0, 40),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(dropX, dropY, itemW, totalH),
      graphics.NewColor(45, 45, 48, 255),
    );
    return [ctx, dropY];
  },
  MenuItem: function (ctx, w, state, label, dropX, dropY, itemIndex) {
    let padding = ctx.Style.Padding;
    let textH = this.TextHeight(ctx.Style.FontSize);
    let itemH = textH + padding * 2;
    let itemW = int32(160);
    let y = dropY + itemIndex * itemH;
    graphics.FillRect(
      w,
      graphics.NewRect(dropX, y, itemW, itemH),
      ctx.Style.BackgroundColor,
    );
    let hovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      dropX,
      y,
      itemW,
      itemH,
    );
    let clicked = false;
    if (hovered) {
      graphics.FillRect(
        w,
        graphics.NewRect(dropX + 1, y + 1, itemW - 2, itemH - 2),
        ctx.Style.ButtonHoverColor,
      );
      state.ClickedOutside = false;
      if (ctx.MouseClicked) {
        clicked = true;
        state.OpenMenuID = 0;
      }
    }
    let textY = y + (((itemH - textH) / 2) | 0);
    this.DrawText(
      w,
      label,
      dropX + padding,
      textY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(dropX, dropY, itemW, (itemIndex + 1) * itemH),
      graphics.NewColor(50, 55, 65, 255),
    );
    return [ctx, state, clicked];
  },
  MenuItemSeparator: function (ctx, w, dropX, dropY, itemIndex) {
    let padding = ctx.Style.Padding;
    let textH = this.TextHeight(ctx.Style.FontSize);
    let itemH = textH + padding * 2;
    let itemW = int32(160);
    let y = dropY + itemIndex * itemH + ((itemH / 2) | 0);
    graphics.FillRect(
      w,
      graphics.NewRect(dropX, dropY + itemIndex * itemH, itemW, itemH),
      ctx.Style.BackgroundColor,
    );
    graphics.DrawLine(
      w,
      dropX + padding,
      y,
      dropX + itemW - padding,
      y,
      graphics.NewColor(30, 35, 45, 255),
    );
    graphics.DrawLine(
      w,
      dropX + padding,
      y + 1,
      dropX + itemW - padding,
      y + 1,
      graphics.NewColor(70, 75, 85, 255),
    );
  },
  BeginToolbar: function (ctx, w, x, y, width) {
    let height =
      this.TextHeight(ctx.Style.FontSize) + ctx.Style.Padding * 2 - 2;
    let padding = ctx.Style.Padding;
    graphics.FillRect(
      w,
      graphics.NewRect(x, y, width, height),
      graphics.NewColor(60, 60, 65, 255),
    );
    graphics.DrawLine(
      w,
      x,
      y,
      x + width - 1,
      y,
      graphics.NewColor(255, 255, 255, 20),
    );
    graphics.DrawLine(
      w,
      x,
      y + height - 1,
      x + width,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.DrawLine(
      w,
      x,
      y + height,
      x + width,
      y + height,
      graphics.NewColor(0, 0, 0, 25),
    );
    let state = { X: x + padding, Y: y, Height: height };
    return [ctx, state];
  },
  ToolbarButton: function (ctx, w, state, label) {
    let id = this.GenID("tb_" + label);
    let padding = ctx.Style.Padding;
    let textW = this.TextWidth(label, ctx.Style.FontSize);
    let textH = this.TextHeight(ctx.Style.FontSize);
    let btnW = textW + padding * 2;
    let btnH = state.Height - 6;
    let x = state.X;
    let y = state.Y + 3;
    let hovered = this.pointInRect(ctx.MouseX, ctx.MouseY, x, y, btnW, btnH);
    let clicked = false;
    if (hovered) {
      ctx.HotID = id;
      if (ctx.MouseClicked) {
        ctx.ActiveID = id;
      }
    }
    if (ctx.ActiveID == id && hovered) {
      graphics.FillRect(
        w,
        graphics.NewRect(x, y, btnW, btnH),
        ctx.Style.ButtonActiveColor,
      );
      graphics.DrawLine(
        w,
        x,
        y,
        x + btnW - 1,
        y,
        graphics.NewColor(0, 0, 0, 60),
      );
      graphics.DrawLine(
        w,
        x,
        y,
        x,
        y + btnH - 1,
        graphics.NewColor(0, 0, 0, 40),
      );
    } else if (ctx.HotID == id) {
      graphics.FillRect(
        w,
        graphics.NewRect(x, y, btnW, btnH),
        ctx.Style.ButtonHoverColor,
      );
      graphics.DrawLine(
        w,
        x + 1,
        y + 1,
        x + btnW - 2,
        y + 1,
        graphics.NewColor(255, 255, 255, 50),
      );
      graphics.DrawLine(
        w,
        x + 1,
        y + 1,
        x + 1,
        y + btnH - 2,
        graphics.NewColor(255, 255, 255, 30),
      );
      graphics.DrawLine(
        w,
        x + 1,
        y + btnH - 1,
        x + btnW - 1,
        y + btnH - 1,
        graphics.NewColor(0, 0, 0, 50),
      );
      graphics.DrawLine(
        w,
        x + btnW - 1,
        y + 1,
        x + btnW - 1,
        y + btnH - 1,
        graphics.NewColor(0, 0, 0, 30),
      );
    }
    if (ctx.ReleasedID == id && hovered) {
      clicked = true;
    }
    let textX = x + (((btnW - textW) / 2) | 0);
    let textY = y + (((btnH - textH) / 2) | 0);
    this.DrawText(
      w,
      label,
      textX,
      textY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    state.X = x + btnW + 2;
    return [ctx, state, clicked];
  },
  ToolbarSeparator: function (w, state) {
    let x = state.X + 3;
    let y = state.Y + 5;
    let h = state.Height - 10;
    graphics.DrawLine(w, x, y, x, y + h, graphics.NewColor(0, 0, 0, 80));
    graphics.DrawLine(
      w,
      x + 1,
      y,
      x + 1,
      y + h,
      graphics.NewColor(255, 255, 255, 40),
    );
    state.X = x + 8;
    return state;
  },
  EndToolbar: function (ctx, state) {
    return ctx;
  },
  BeginLayout: function (ctx, x, y, spacing) {
    ctx.CursorX = x;
    ctx.CursorY = y;
    ctx.Spacing = spacing;
    return ctx;
  },
  NextRow: function (ctx, height) {
    ctx.CursorY = ctx.CursorY + height + ctx.Spacing;
    return ctx;
  },
  AutoLabel: function (ctx, w, text) {
    this.Label(ctx, w, text, ctx.CursorX, ctx.CursorY);
    ctx = this.NextRow(ctx, this.TextHeight(ctx.Style.FontSize));
    return ctx;
  },
  AutoButton: function (ctx, w, label, width, height) {
    let result = false;
    [ctx, result] = this.Button(
      ctx,
      w,
      label,
      ctx.CursorX,
      ctx.CursorY,
      width,
      height,
    );
    ctx = this.NextRow(ctx, height);
    return [ctx, result];
  },
  AutoCheckbox: function (ctx, w, label, value) {
    let result = false;
    [ctx, result] = this.Checkbox(
      ctx,
      w,
      label,
      ctx.CursorX,
      ctx.CursorY,
      value,
    );
    ctx = this.NextRow(ctx, ctx.Style.CheckboxSize);
    return [ctx, result];
  },
  AutoSlider: function (ctx, w, label, width, min, max, value) {
    let result = 0;
    [ctx, result] = this.Slider(
      ctx,
      w,
      label,
      ctx.CursorX,
      ctx.CursorY,
      width,
      min,
      max,
      value,
    );
    ctx = this.NextRow(ctx, ctx.Style.SliderHeight);
    return [ctx, result];
  },
  NewTabState: function () {
    return { ActiveTab: 0 };
  },
  TabBar: function (ctx, w, state, labels, x, y) {
    let padding = ctx.Style.Padding;
    let tabH = this.TextHeight(ctx.Style.FontSize) + padding * 2;
    let tabX = x;
    for (let i = 0; i < len(labels); i++) {
      let label = labels[i];
      let tabW = this.TextWidth(label, ctx.Style.FontSize) + padding * 3;
      let isActive = int32(i) == state.ActiveTab;
      let hovered = this.pointInRect(
        ctx.MouseX,
        ctx.MouseY,
        tabX,
        y,
        tabW,
        tabH,
      );
      let bgColor = { R: 0, G: 0, B: 0, A: 0 };
      if (isActive) {
        bgColor = ctx.Style.BackgroundColor;
      } else if (hovered) {
        bgColor = ctx.Style.ButtonHoverColor;
      } else {
        bgColor = graphics.NewColor(35, 40, 50, 255);
      }
      if (isActive) {
        graphics.FillRect(
          w,
          graphics.NewRect(tabX + 1, y + 1, tabW, tabH),
          graphics.NewColor(0, 0, 0, 40),
        );
      }
      graphics.FillRect(w, graphics.NewRect(tabX, y, tabW, tabH), bgColor);
      if (isActive) {
        graphics.DrawLine(
          w,
          tabX + 1,
          y + 1,
          tabX + tabW - 2,
          y + 1,
          graphics.NewColor(255, 255, 255, 80),
        );
        graphics.DrawLine(
          w,
          tabX + 1,
          y + 1,
          tabX + 1,
          y + tabH - 1,
          graphics.NewColor(255, 255, 255, 50),
        );
      }
      graphics.DrawLine(
        w,
        tabX,
        y,
        tabX + tabW,
        y,
        graphics.NewColor(60, 65, 75, 255),
      );
      graphics.DrawLine(
        w,
        tabX,
        y,
        tabX,
        y + tabH,
        graphics.NewColor(60, 65, 75, 255),
      );
      graphics.DrawLine(
        w,
        tabX + tabW,
        y,
        tabX + tabW,
        y + tabH,
        graphics.NewColor(60, 65, 75, 255),
      );
      if (!isActive) {
        graphics.DrawLine(
          w,
          tabX,
          y + tabH,
          tabX + tabW,
          y + tabH,
          graphics.NewColor(60, 65, 75, 255),
        );
      }
      let textY = y + (((tabH - this.TextHeight(ctx.Style.FontSize)) / 2) | 0);
      this.DrawText(
        w,
        label,
        tabX + padding,
        textY,
        ctx.Style.FontSize,
        ctx.Style.TextColor,
      );
      if (hovered && ctx.MouseClicked) {
        state.ActiveTab = int32(i);
      }
      tabX = tabX + tabW;
    }
    return [ctx, state];
  },
  NewTreeNodeState: function () {
    return { ExpandedIDs: [] };
  },
  isExpanded: function (state, id) {
    for (let i = 0; i < len(state.ExpandedIDs); i++) {
      if (state.ExpandedIDs[i] == id) {
        return true;
      }
    }
    return false;
  },
  toggleExpanded: function (state, id) {
    for (let i = 0; i < len(state.ExpandedIDs); i++) {
      if (state.ExpandedIDs[i] == id) {
        let newIDs = [];
        for (let j = 0; j < len(state.ExpandedIDs); j++) {
          if (state.ExpandedIDs[j] != id) {
            newIDs = append(newIDs, state.ExpandedIDs[j]);
          }
        }
        state.ExpandedIDs = newIDs;
        return state;
      }
    }
    state.ExpandedIDs = append(state.ExpandedIDs, id);
    return state;
  },
  TreeNode: function (ctx, w, state, label, x, y, indent) {
    let id = this.GenID(label);
    let padding = ctx.Style.Padding;
    let nodeH = this.TextHeight(ctx.Style.FontSize) + padding;
    let arrowSize = int32(8);
    let actualX = x + indent;
    let arrowHovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      actualX,
      y,
      arrowSize + padding,
      nodeH,
    );
    let labelHovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      actualX + arrowSize + padding,
      y,
      this.TextWidth(label, ctx.Style.FontSize) + padding,
      nodeH,
    );
    let expanded = this.isExpanded(state, id);
    if (arrowHovered || labelHovered) {
      let totalW =
        arrowSize +
        padding +
        this.TextWidth(label, ctx.Style.FontSize) +
        padding;
      graphics.FillRect(
        w,
        graphics.NewRect(actualX, y, totalW, nodeH),
        graphics.NewColor(255, 255, 255, 20),
      );
    }
    let arrowX = actualX + ((arrowSize / 2) | 0);
    let arrowY = y + ((nodeH / 2) | 0);
    let arrowColor = ctx.Style.TextColor;
    if (expanded) {
      graphics.DrawLine(
        w,
        arrowX - 3,
        arrowY - 2,
        arrowX,
        arrowY + 2,
        arrowColor,
      );
      graphics.DrawLine(
        w,
        arrowX,
        arrowY + 2,
        arrowX + 3,
        arrowY - 2,
        arrowColor,
      );
      graphics.DrawLine(
        w,
        arrowX - 2,
        arrowY - 2,
        arrowX,
        arrowY + 1,
        arrowColor,
      );
      graphics.DrawLine(
        w,
        arrowX,
        arrowY + 1,
        arrowX + 2,
        arrowY - 2,
        arrowColor,
      );
    } else {
      graphics.DrawLine(
        w,
        arrowX - 2,
        arrowY - 3,
        arrowX + 2,
        arrowY,
        arrowColor,
      );
      graphics.DrawLine(
        w,
        arrowX + 2,
        arrowY,
        arrowX - 2,
        arrowY + 3,
        arrowColor,
      );
      graphics.DrawLine(
        w,
        arrowX - 2,
        arrowY - 2,
        arrowX + 1,
        arrowY,
        arrowColor,
      );
      graphics.DrawLine(
        w,
        arrowX + 1,
        arrowY,
        arrowX - 2,
        arrowY + 2,
        arrowColor,
      );
    }
    this.DrawText(
      w,
      label,
      actualX + arrowSize + padding,
      y + (((nodeH - this.TextHeight(ctx.Style.FontSize)) / 2) | 0),
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    if (arrowHovered && ctx.MouseClicked) {
      state = this.toggleExpanded(state, id);
      expanded = !expanded;
    }
    return [ctx, state, expanded];
  },
  TreeLeaf: function (ctx, w, label, x, y, indent) {
    let padding = ctx.Style.Padding;
    let nodeH = this.TextHeight(ctx.Style.FontSize) + padding;
    let bulletSize = int32(4);
    let actualX = x + indent;
    let hovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      actualX,
      y,
      this.TextWidth(label, ctx.Style.FontSize) + padding * 2 + bulletSize,
      nodeH,
    );
    let clicked = false;
    if (hovered) {
      let totalW =
        bulletSize +
        padding +
        this.TextWidth(label, ctx.Style.FontSize) +
        padding;
      graphics.FillRect(
        w,
        graphics.NewRect(actualX, y, totalW, nodeH),
        graphics.NewColor(255, 255, 255, 20),
      );
      if (ctx.MouseClicked) {
        clicked = true;
      }
    }
    let bulletX = actualX + ((bulletSize / 2) | 0) + 2;
    let bulletY = y + ((nodeH / 2) | 0);
    graphics.FillCircle(w, bulletX, bulletY, 2, ctx.Style.TextColor);
    this.DrawText(
      w,
      label,
      actualX + bulletSize + padding,
      y + (((nodeH - this.TextHeight(ctx.Style.FontSize)) / 2) | 0),
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    return [ctx, clicked];
  },
  ProgressBar: function (ctx, w, x, y, width, height, progress) {
    if (progress < 0) {
      progress = 0;
    }
    if (progress > 1) {
      progress = 1;
    }
    graphics.FillRect(
      w,
      graphics.NewRect(x + 1, y + 1, width, height),
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(x, y, width, height),
      ctx.Style.FrameBgColor,
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + width - 2,
      y + 1,
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + 1,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 60),
    );
    let fillW = int32(float64(width - 4) * progress);
    if (fillW > 0) {
      let fillColor = ctx.Style.SliderTrackColor;
      graphics.FillRect(
        w,
        graphics.NewRect(x + 2, y + 2, fillW, height - 4),
        fillColor,
      );
      graphics.DrawLine(
        w,
        x + 2,
        y + 2,
        x + fillW,
        y + 2,
        graphics.NewColor(255, 255, 255, 60),
      );
      graphics.DrawLine(
        w,
        x + 2,
        y + 3,
        x + fillW,
        y + 3,
        graphics.NewColor(255, 255, 255, 30),
      );
    }
    graphics.DrawRect(
      w,
      graphics.NewRect(x, y, width, height),
      graphics.NewColor(50, 55, 65, 255),
    );
  },
  RadioButton: function (ctx, w, label, x, y, selected) {
    let id = this.GenID(label);
    let radioSize = ctx.Style.CheckboxSize;
    let radius = (radioSize / 2) | 0;
    let labelW = this.TextWidth(label, ctx.Style.FontSize);
    let totalW = radioSize + ctx.Style.Padding + labelW;
    let hovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      x,
      y,
      totalW,
      radioSize,
    );
    if (hovered) {
      ctx.HotID = id;
      if (ctx.MouseClicked) {
        ctx.ActiveID = id;
      }
    }
    let centerX = x + radius;
    let centerY = y + radius;
    graphics.DrawCircle(
      w,
      centerX + 1,
      centerY + 1,
      radius,
      graphics.NewColor(0, 0, 0, 60),
    );
    let bgColor = { R: 0, G: 0, B: 0, A: 0 };
    if (hovered) {
      bgColor = ctx.Style.ButtonHoverColor;
    } else {
      bgColor = ctx.Style.FrameBgColor;
    }
    graphics.FillCircle(w, centerX, centerY, radius - 1, bgColor);
    graphics.DrawCircle(
      w,
      centerX - 1,
      centerY - 1,
      radius - 2,
      graphics.NewColor(255, 255, 255, 50),
    );
    graphics.DrawCircle(
      w,
      centerX,
      centerY,
      radius,
      graphics.NewColor(60, 65, 75, 255),
    );
    if (selected) {
      let dotColor = ctx.Style.CheckmarkColor;
      graphics.FillCircle(w, centerX, centerY, radius - 4, dotColor);
      graphics.DrawCircle(
        w,
        centerX - 1,
        centerY - 1,
        radius - 5,
        graphics.NewColor(255, 255, 255, 60),
      );
    }
    let labelX = x + radioSize + ctx.Style.Padding;
    let labelY =
      y + (((radioSize - this.TextHeight(ctx.Style.FontSize)) / 2) | 0);
    this.DrawText(
      w,
      label,
      labelX,
      labelY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    let clicked = ctx.ReleasedID == id && ctx.MouseReleased && hovered;
    return [ctx, clicked];
  },
  NewTextInputState: function (initialText) {
    return {
      Text: initialText,
      CursorPos: int32(len(initialText)),
      Active: false,
      BlinkTimer: 0,
    };
  },
  TextInput: function (ctx, w, state, x, y, width) {
    let idStr = "textinput_";
    let id = this.GenID(idStr) + x * 1000 + y;
    let height = ctx.Style.ButtonHeight;
    let padding = ctx.Style.Padding;
    let hovered = this.pointInRect(ctx.MouseX, ctx.MouseY, x, y, width, height);
    if (hovered && ctx.MouseClicked) {
      state.Active = true;
      ctx.ActiveID = id;
    } else if (ctx.MouseClicked && !hovered) {
      state.Active = false;
      if (ctx.ActiveID == id) {
        ctx.ActiveID = 0;
      }
    }
    graphics.FillRect(
      w,
      graphics.NewRect(x + 1, y + 1, width, height),
      graphics.NewColor(0, 0, 0, 50),
    );
    let bgColor = { R: 0, G: 0, B: 0, A: 0 };
    if (state.Active) {
      bgColor = graphics.NewColor(55, 60, 70, 255);
    } else {
      bgColor = ctx.Style.FrameBgColor;
    }
    graphics.FillRect(w, graphics.NewRect(x, y, width, height), bgColor);
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + width - 2,
      y + 1,
      graphics.NewColor(0, 0, 0, 80),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + 1,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 80),
    );
    let textY = y + (((height - this.TextHeight(ctx.Style.FontSize)) / 2) | 0);
    this.DrawText(
      w,
      state.Text,
      x + padding,
      textY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    if (state.Active) {
      state.BlinkTimer = state.BlinkTimer + 1;
      if (state.BlinkTimer > 60) {
        state.BlinkTimer = 0;
      }
      if (state.BlinkTimer < 30) {
        let cursorX =
          x + padding + this.TextWidth(state.Text, ctx.Style.FontSize);
        graphics.DrawLine(
          w,
          cursorX,
          y + 4,
          cursorX,
          y + height - 4,
          ctx.Style.TextColor,
        );
      }
    }
    let borderColor = { R: 0, G: 0, B: 0, A: 0 };
    if (state.Active) {
      borderColor = ctx.Style.CheckmarkColor;
    } else if (hovered) {
      borderColor = graphics.NewColor(80, 90, 110, 255);
    } else {
      borderColor = graphics.NewColor(50, 55, 65, 255);
    }
    graphics.DrawRect(w, graphics.NewRect(x, y, width, height), borderColor);
    if (state.Active) {
      let key = graphics.GetLastKey();
      if (key > 0) {
        if (key == 8) {
          if (len(state.Text) > 0) {
            state.Text = this.removeLastChar(state.Text);
          }
        } else if (key >= 32 && key < 127) {
          state.Text = state.Text + this.asciiToString(key);
        }
      }
    }
    return [ctx, state];
  },
  ListBox: function (
    ctx,
    w,
    items,
    x,
    y,
    width,
    height,
    selectedIndex,
    scrollOffset,
  ) {
    let padding = ctx.Style.Padding;
    let itemH = this.TextHeight(ctx.Style.FontSize) + padding;
    graphics.FillRect(
      w,
      graphics.NewRect(x + 2, y + 2, width, height),
      graphics.NewColor(0, 0, 0, 50),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(x, y, width, height),
      ctx.Style.FrameBgColor,
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + width - 2,
      y + 1,
      graphics.NewColor(0, 0, 0, 80),
    );
    graphics.DrawLine(
      w,
      x + 1,
      y + 1,
      x + 1,
      y + height - 2,
      graphics.NewColor(0, 0, 0, 80),
    );
    let visibleCount = ((height - 4) / itemH) | 0;
    let startIndex = (scrollOffset / itemH) | 0;
    if (startIndex < 0) {
      startIndex = 0;
    }
    for (
      let i = int32(0);
      i < visibleCount && int(startIndex + i) < len(items);
      i++
    ) {
      let itemIndex = startIndex + i;
      let item = items[itemIndex];
      let itemY = y + 2 + i * itemH;
      if (itemIndex == selectedIndex) {
        graphics.FillRect(
          w,
          graphics.NewRect(x + 2, itemY, width - 4, itemH),
          ctx.Style.ButtonHoverColor,
        );
      }
      if (
        this.pointInRect(ctx.MouseX, ctx.MouseY, x + 2, itemY, width - 4, itemH)
      ) {
        if (itemIndex != selectedIndex) {
          graphics.FillRect(
            w,
            graphics.NewRect(x + 2, itemY, width - 4, itemH),
            graphics.NewColor(255, 255, 255, 20),
          );
        }
        if (ctx.MouseClicked) {
          selectedIndex = itemIndex;
        }
      }
      let textY =
        itemY + (((itemH - this.TextHeight(ctx.Style.FontSize)) / 2) | 0);
      this.DrawText(
        w,
        item,
        x + padding,
        textY,
        ctx.Style.FontSize,
        ctx.Style.TextColor,
      );
    }
    graphics.DrawRect(
      w,
      graphics.NewRect(x, y, width, height),
      graphics.NewColor(50, 55, 65, 255),
    );
    let totalHeight = int32(len(items)) * itemH;
    if (totalHeight > height) {
      let scrollbarW = int32(8);
      let scrollbarX = x + width - scrollbarW - 2;
      let scrollbarH = height - 4;
      let thumbH = ((height * scrollbarH) / totalHeight) | 0;
      if (thumbH < 20) {
        thumbH = 20;
      }
      let maxScroll = totalHeight - height;
      let thumbY =
        y + 2 + ((((scrollbarH - thumbH) * scrollOffset) / maxScroll) | 0);
      graphics.FillRect(
        w,
        graphics.NewRect(scrollbarX, y + 2, scrollbarW, scrollbarH),
        graphics.NewColor(30, 35, 45, 255),
      );
      graphics.FillRect(
        w,
        graphics.NewRect(scrollbarX, thumbY, scrollbarW, thumbH),
        graphics.NewColor(80, 90, 110, 255),
      );
      graphics.DrawLine(
        w,
        scrollbarX + 1,
        thumbY + 1,
        scrollbarX + scrollbarW - 2,
        thumbY + 1,
        graphics.NewColor(255, 255, 255, 40),
      );
    }
    return [ctx, selectedIndex, scrollOffset];
  },
  AutoProgressBar: function (ctx, w, width, height, progress) {
    this.ProgressBar(ctx, w, ctx.CursorX, ctx.CursorY, width, height, progress);
    ctx = this.NextRow(ctx, height);
    return ctx;
  },
  AutoRadioButton: function (ctx, w, label, selected) {
    let clicked = false;
    [ctx, clicked] = this.RadioButton(
      ctx,
      w,
      label,
      ctx.CursorX,
      ctx.CursorY,
      selected,
    );
    ctx = this.NextRow(ctx, ctx.Style.CheckboxSize);
    return [ctx, clicked];
  },
  Tooltip: function (ctx, w, text, x, y) {
    let padding = ctx.Style.Padding;
    let textW = this.TextWidth(text, ctx.Style.FontSize);
    let textH = this.TextHeight(ctx.Style.FontSize);
    let tipW = textW + padding * 2;
    let tipH = textH + padding * 2;
    let tipX = x + 10;
    let tipY = y + 10;
    graphics.FillRect(
      w,
      graphics.NewRect(tipX + 2, tipY + 2, tipW, tipH),
      graphics.NewColor(0, 0, 0, 80),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(tipX, tipY, tipW, tipH),
      graphics.NewColor(60, 60, 65, 250),
    );
    graphics.DrawLine(
      w,
      tipX + 1,
      tipY + 1,
      tipX + tipW - 2,
      tipY + 1,
      graphics.NewColor(255, 255, 255, 50),
    );
    graphics.DrawLine(
      w,
      tipX + 1,
      tipY + 1,
      tipX + 1,
      tipY + tipH - 2,
      graphics.NewColor(255, 255, 255, 30),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(tipX, tipY, tipW, tipH),
      graphics.NewColor(80, 85, 95, 255),
    );
    this.DrawText(
      w,
      text,
      tipX + padding,
      tipY + padding,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
  },
  Spinner: function (ctx, w, label, x, y, value, minVal, maxVal) {
    let padding = ctx.Style.Padding;
    let height = ctx.Style.ButtonHeight;
    let btnW = height;
    let valueW = int32(50);
    let labelW = this.TextWidth(label, ctx.Style.FontSize);
    let labelY = y + (((height - this.TextHeight(ctx.Style.FontSize)) / 2) | 0);
    this.DrawText(w, label, x, labelY, ctx.Style.FontSize, ctx.Style.TextColor);
    let spinX = x + labelW + padding;
    let minusHovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      spinX,
      y,
      btnW,
      height,
    );
    let minusBg = { R: 0, G: 0, B: 0, A: 0 };
    if (minusHovered) {
      minusBg = ctx.Style.ButtonHoverColor;
    } else {
      minusBg = ctx.Style.ButtonColor;
    }
    graphics.FillRect(w, graphics.NewRect(spinX, y, btnW, height), minusBg);
    graphics.DrawLine(
      w,
      spinX + 1,
      y + 1,
      spinX + btnW - 2,
      y + 1,
      graphics.NewColor(255, 255, 255, 70),
    );
    graphics.DrawLine(
      w,
      spinX + 1,
      y + height - 1,
      spinX + btnW - 1,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 80),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(spinX, y, btnW, height),
      graphics.NewColor(50, 55, 65, 255),
    );
    graphics.DrawLine(
      w,
      spinX + 4,
      y + ((height / 2) | 0),
      spinX + btnW - 4,
      y + ((height / 2) | 0),
      ctx.Style.TextColor,
    );
    graphics.DrawLine(
      w,
      spinX + 4,
      y + ((height / 2) | 0) + 1,
      spinX + btnW - 4,
      y + ((height / 2) | 0) + 1,
      ctx.Style.TextColor,
    );
    if (minusHovered && ctx.MouseClicked && value > minVal) {
      value = value - 1;
    }
    let valueX = spinX + btnW;
    graphics.FillRect(
      w,
      graphics.NewRect(valueX, y, valueW, height),
      ctx.Style.FrameBgColor,
    );
    graphics.DrawLine(
      w,
      valueX + 1,
      y + 1,
      valueX + valueW - 2,
      y + 1,
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(valueX, y, valueW, height),
      graphics.NewColor(50, 55, 65, 255),
    );
    let valueStr = this.intToString(value);
    let valueTextW = this.TextWidth(valueStr, ctx.Style.FontSize);
    let valueTextX = valueX + (((valueW - valueTextW) / 2) | 0);
    this.DrawText(
      w,
      valueStr,
      valueTextX,
      labelY,
      ctx.Style.FontSize,
      ctx.Style.TextColor,
    );
    let plusX = valueX + valueW;
    let plusHovered = this.pointInRect(
      ctx.MouseX,
      ctx.MouseY,
      plusX,
      y,
      btnW,
      height,
    );
    let plusBg = { R: 0, G: 0, B: 0, A: 0 };
    if (plusHovered) {
      plusBg = ctx.Style.ButtonHoverColor;
    } else {
      plusBg = ctx.Style.ButtonColor;
    }
    graphics.FillRect(w, graphics.NewRect(plusX, y, btnW, height), plusBg);
    graphics.DrawLine(
      w,
      plusX + 1,
      y + 1,
      plusX + btnW - 2,
      y + 1,
      graphics.NewColor(255, 255, 255, 70),
    );
    graphics.DrawLine(
      w,
      plusX + 1,
      y + height - 1,
      plusX + btnW - 1,
      y + height - 1,
      graphics.NewColor(0, 0, 0, 80),
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(plusX, y, btnW, height),
      graphics.NewColor(50, 55, 65, 255),
    );
    graphics.DrawLine(
      w,
      plusX + 4,
      y + ((height / 2) | 0),
      plusX + btnW - 4,
      y + ((height / 2) | 0),
      ctx.Style.TextColor,
    );
    graphics.DrawLine(
      w,
      plusX + 4,
      y + ((height / 2) | 0) + 1,
      plusX + btnW - 4,
      y + ((height / 2) | 0) + 1,
      ctx.Style.TextColor,
    );
    graphics.DrawLine(
      w,
      plusX + ((btnW / 2) | 0),
      y + 4,
      plusX + ((btnW / 2) | 0),
      y + height - 4,
      ctx.Style.TextColor,
    );
    graphics.DrawLine(
      w,
      plusX + ((btnW / 2) | 0) + 1,
      y + 4,
      plusX + ((btnW / 2) | 0) + 1,
      y + height - 4,
      ctx.Style.TextColor,
    );
    if (plusHovered && ctx.MouseClicked && value < maxVal) {
      value = value + 1;
    }
    return [ctx, value];
  },
  intToString: function (n) {
    let digitChars = ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9"];
    if (n == 0) {
      return "0";
    }
    let negative = false;
    if (n < 0) {
      negative = true;
      n = -n;
    }
    let result = "";
    for (; n > 0; ) {
      let digit = n % 10;
      result = digitChars[digit] + result;
      n = (n / 10) | 0;
    }
    if (negative) {
      result = "-" + result;
    }
    return result;
  },
  removeLastChar: function (s) {
    let slen = len(s);
    if (slen == 0) {
      return s;
    }
    let result = "";
    for (let i = 0; i < slen - 1; i++) {
      result = result + this.asciiToString(int(s.charCodeAt(i)));
    }
    return result;
  },
  asciiToString: function (code) {
    let chars = [
      " ",
      "!",
      '"',
      "#",
      "$",
      "%",
      "&",
      "'",
      "(",
      ")",
      "*",
      "+",
      ",",
      "-",
      ".",
      "/",
      "0",
      "1",
      "2",
      "3",
      "4",
      "5",
      "6",
      "7",
      "8",
      "9",
      ":",
      ";",
      "<",
      "=",
      ">",
      "?",
      "@",
      "A",
      "B",
      "C",
      "D",
      "E",
      "F",
      "G",
      "H",
      "I",
      "J",
      "K",
      "L",
      "M",
      "N",
      "O",
      "P",
      "Q",
      "R",
      "S",
      "T",
      "U",
      "V",
      "W",
      "X",
      "Y",
      "Z",
      "[",
      "\\",
      "]",
      "^",
      "_",
      "`",
      "a",
      "b",
      "c",
      "d",
      "e",
      "f",
      "g",
      "h",
      "i",
      "j",
      "k",
      "l",
      "m",
      "n",
      "o",
      "p",
      "q",
      "r",
      "s",
      "t",
      "u",
      "v",
      "w",
      "x",
      "y",
      "z",
      "{",
      "|",
      "}",
      "~",
    ];
    if (code >= 32 && code <= 126) {
      return chars[code - 32];
    }
    return "";
  },
  ColorPicker: function (ctx, w, label, x, y, width, color) {
    let padding = ctx.Style.Padding;
    let sliderH = ctx.Style.SliderHeight;
    let previewSize = int32(40);
    this.DrawText(w, label, x, y, ctx.Style.FontSize, ctx.Style.TextColor);
    y = y + this.TextHeight(ctx.Style.FontSize) + padding;
    graphics.FillRect(
      w,
      graphics.NewRect(x + 1, y + 1, previewSize, previewSize),
      graphics.NewColor(0, 0, 0, 60),
    );
    graphics.FillRect(
      w,
      graphics.NewRect(x, y, previewSize, previewSize),
      color,
    );
    graphics.DrawRect(
      w,
      graphics.NewRect(x, y, previewSize, previewSize),
      graphics.NewColor(60, 65, 75, 255),
    );
    let sliderX = x + previewSize + padding;
    let sliderW = width - previewSize - padding;
    let rVal = 0;
    let gVal = 0;
    let bVal = 0;
    [ctx, rVal] = this.Slider(
      ctx,
      w,
      "R",
      sliderX,
      y,
      sliderW,
      0,
      255,
      float64(color.R),
    );
    color.R = uint8(rVal);
    y = y + sliderH + 2;
    [ctx, gVal] = this.Slider(
      ctx,
      w,
      "G",
      sliderX,
      y,
      sliderW,
      0,
      255,
      float64(color.G),
    );
    color.G = uint8(gVal);
    y = y + sliderH + 2;
    [ctx, bVal] = this.Slider(
      ctx,
      w,
      "B",
      sliderX,
      y,
      sliderW,
      0,
      255,
      float64(color.B),
    );
    color.B = uint8(bVal);
    return [ctx, color];
  },
  getFontData: function () {
    return [
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x18, 0x18,
      0x18, 0x00, 0x18, 0x00, 0x6c, 0x6c, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00,
      0x6c, 0x6c, 0xfe, 0x6c, 0xfe, 0x6c, 0x6c, 0x00, 0x18, 0x3e, 0x60, 0x3c,
      0x06, 0x7c, 0x18, 0x00, 0x00, 0xc6, 0xcc, 0x18, 0x30, 0x66, 0xc6, 0x00,
      0x38, 0x6c, 0x38, 0x76, 0xdc, 0xcc, 0x76, 0x00, 0x18, 0x18, 0x30, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x0c, 0x18, 0x30, 0x30, 0x30, 0x18, 0x0c, 0x00,
      0x30, 0x18, 0x0c, 0x0c, 0x0c, 0x18, 0x30, 0x00, 0x00, 0x66, 0x3c, 0xff,
      0x3c, 0x66, 0x00, 0x00, 0x00, 0x18, 0x18, 0x7e, 0x18, 0x18, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x30, 0x00, 0x00, 0x00, 0x7e,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x00,
      0x06, 0x0c, 0x18, 0x30, 0x60, 0xc0, 0x80, 0x00, 0x7c, 0xc6, 0xce, 0xd6,
      0xe6, 0xc6, 0x7c, 0x00, 0x18, 0x38, 0x18, 0x18, 0x18, 0x18, 0x7e, 0x00,
      0x7c, 0xc6, 0x06, 0x1c, 0x30, 0x66, 0xfe, 0x00, 0x7c, 0xc6, 0x06, 0x3c,
      0x06, 0xc6, 0x7c, 0x00, 0x1c, 0x3c, 0x6c, 0xcc, 0xfe, 0x0c, 0x1e, 0x00,
      0xfe, 0xc0, 0xc0, 0xfc, 0x06, 0xc6, 0x7c, 0x00, 0x38, 0x60, 0xc0, 0xfc,
      0xc6, 0xc6, 0x7c, 0x00, 0xfe, 0xc6, 0x0c, 0x18, 0x30, 0x30, 0x30, 0x00,
      0x7c, 0xc6, 0xc6, 0x7c, 0xc6, 0xc6, 0x7c, 0x00, 0x7c, 0xc6, 0xc6, 0x7e,
      0x06, 0x0c, 0x78, 0x00, 0x00, 0x18, 0x18, 0x00, 0x00, 0x18, 0x18, 0x00,
      0x00, 0x18, 0x18, 0x00, 0x00, 0x18, 0x18, 0x30, 0x06, 0x0c, 0x18, 0x30,
      0x18, 0x0c, 0x06, 0x00, 0x00, 0x00, 0x7e, 0x00, 0x00, 0x7e, 0x00, 0x00,
      0x60, 0x30, 0x18, 0x0c, 0x18, 0x30, 0x60, 0x00, 0x7c, 0xc6, 0x0c, 0x18,
      0x18, 0x00, 0x18, 0x00, 0x7c, 0xc6, 0xde, 0xde, 0xde, 0xc0, 0x78, 0x00,
      0x38, 0x6c, 0xc6, 0xfe, 0xc6, 0xc6, 0xc6, 0x00, 0xfc, 0x66, 0x66, 0x7c,
      0x66, 0x66, 0xfc, 0x00, 0x3c, 0x66, 0xc0, 0xc0, 0xc0, 0x66, 0x3c, 0x00,
      0xf8, 0x6c, 0x66, 0x66, 0x66, 0x6c, 0xf8, 0x00, 0xfe, 0x62, 0x68, 0x78,
      0x68, 0x62, 0xfe, 0x00, 0xfe, 0x62, 0x68, 0x78, 0x68, 0x60, 0xf0, 0x00,
      0x3c, 0x66, 0xc0, 0xc0, 0xce, 0x66, 0x3a, 0x00, 0xc6, 0xc6, 0xc6, 0xfe,
      0xc6, 0xc6, 0xc6, 0x00, 0x3c, 0x18, 0x18, 0x18, 0x18, 0x18, 0x3c, 0x00,
      0x1e, 0x0c, 0x0c, 0x0c, 0xcc, 0xcc, 0x78, 0x00, 0xe6, 0x66, 0x6c, 0x78,
      0x6c, 0x66, 0xe6, 0x00, 0xf0, 0x60, 0x60, 0x60, 0x62, 0x66, 0xfe, 0x00,
      0xc6, 0xee, 0xfe, 0xfe, 0xd6, 0xc6, 0xc6, 0x00, 0xc6, 0xe6, 0xf6, 0xde,
      0xce, 0xc6, 0xc6, 0x00, 0x7c, 0xc6, 0xc6, 0xc6, 0xc6, 0xc6, 0x7c, 0x00,
      0xfc, 0x66, 0x66, 0x7c, 0x60, 0x60, 0xf0, 0x00, 0x7c, 0xc6, 0xc6, 0xc6,
      0xd6, 0xde, 0x7c, 0x06, 0xfc, 0x66, 0x66, 0x7c, 0x6c, 0x66, 0xe6, 0x00,
      0x7c, 0xc6, 0x60, 0x38, 0x0c, 0xc6, 0x7c, 0x00, 0x7e, 0x7e, 0x5a, 0x18,
      0x18, 0x18, 0x3c, 0x00, 0xc6, 0xc6, 0xc6, 0xc6, 0xc6, 0xc6, 0x7c, 0x00,
      0xc6, 0xc6, 0xc6, 0xc6, 0xc6, 0x6c, 0x38, 0x00, 0xc6, 0xc6, 0xc6, 0xd6,
      0xd6, 0xfe, 0x6c, 0x00, 0xc6, 0xc6, 0x6c, 0x38, 0x6c, 0xc6, 0xc6, 0x00,
      0x66, 0x66, 0x66, 0x3c, 0x18, 0x18, 0x3c, 0x00, 0xfe, 0xc6, 0x8c, 0x18,
      0x32, 0x66, 0xfe, 0x00, 0x3c, 0x30, 0x30, 0x30, 0x30, 0x30, 0x3c, 0x00,
      0xc0, 0x60, 0x30, 0x18, 0x0c, 0x06, 0x02, 0x00, 0x3c, 0x0c, 0x0c, 0x0c,
      0x0c, 0x0c, 0x3c, 0x00, 0x10, 0x38, 0x6c, 0xc6, 0x00, 0x00, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x30, 0x18, 0x0c, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x78, 0x0c, 0x7c, 0xcc, 0x76, 0x00,
      0xe0, 0x60, 0x7c, 0x66, 0x66, 0x66, 0xdc, 0x00, 0x00, 0x00, 0x7c, 0xc6,
      0xc0, 0xc6, 0x7c, 0x00, 0x1c, 0x0c, 0x7c, 0xcc, 0xcc, 0xcc, 0x76, 0x00,
      0x00, 0x00, 0x7c, 0xc6, 0xfe, 0xc0, 0x7c, 0x00, 0x3c, 0x66, 0x60, 0xf8,
      0x60, 0x60, 0xf0, 0x00, 0x00, 0x00, 0x76, 0xcc, 0xcc, 0x7c, 0x0c, 0xf8,
      0xe0, 0x60, 0x6c, 0x76, 0x66, 0x66, 0xe6, 0x00, 0x18, 0x00, 0x38, 0x18,
      0x18, 0x18, 0x3c, 0x00, 0x06, 0x00, 0x06, 0x06, 0x06, 0x66, 0x66, 0x3c,
      0xe0, 0x60, 0x66, 0x6c, 0x78, 0x6c, 0xe6, 0x00, 0x38, 0x18, 0x18, 0x18,
      0x18, 0x18, 0x3c, 0x00, 0x00, 0x00, 0xec, 0xfe, 0xd6, 0xd6, 0xd6, 0x00,
      0x00, 0x00, 0xdc, 0x66, 0x66, 0x66, 0x66, 0x00, 0x00, 0x00, 0x7c, 0xc6,
      0xc6, 0xc6, 0x7c, 0x00, 0x00, 0x00, 0xdc, 0x66, 0x66, 0x7c, 0x60, 0xf0,
      0x00, 0x00, 0x76, 0xcc, 0xcc, 0x7c, 0x0c, 0x1e, 0x00, 0x00, 0xdc, 0x76,
      0x60, 0x60, 0xf0, 0x00, 0x00, 0x00, 0x7e, 0xc0, 0x7c, 0x06, 0xfc, 0x00,
      0x30, 0x30, 0xfc, 0x30, 0x30, 0x36, 0x1c, 0x00, 0x00, 0x00, 0xcc, 0xcc,
      0xcc, 0xcc, 0x76, 0x00, 0x00, 0x00, 0xc6, 0xc6, 0xc6, 0x6c, 0x38, 0x00,
      0x00, 0x00, 0xc6, 0xd6, 0xd6, 0xfe, 0x6c, 0x00, 0x00, 0x00, 0xc6, 0x6c,
      0x38, 0x6c, 0xc6, 0x00, 0x00, 0x00, 0xc6, 0xc6, 0xc6, 0x7e, 0x06, 0xfc,
      0x00, 0x00, 0xfe, 0x8c, 0x18, 0x32, 0xfe, 0x00, 0x0e, 0x18, 0x18, 0x70,
      0x18, 0x18, 0x0e, 0x00, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x00,
      0x70, 0x18, 0x18, 0x0e, 0x18, 0x18, 0x70, 0x00, 0x76, 0xdc, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
    ];
  },
};

function main() {
  let [screenW, screenH] = graphics.GetScreenSize();
  let winW = screenW;
  let winH = screenH - 40;
  let w = graphics.CreateWindow("GUI Widgets Demo", int32(winW), int32(winH));
  let ctx = gui.NewContext();
  let showDemo = true;
  let showAnother = false;
  let showWidgets = true;
  let enabled = true;
  let volume = 0.5;
  let brightness = 75.0;
  let counter = 0;
  let progress = 0.35;
  let menuState = gui.NewMenuState();
  let tabState = gui.NewTabState();
  let tabLabels = ["Basic", "Lists", "Tree", "Inputs"];
  let treeState = gui.NewTreeNodeState();
  let textInput = gui.NewTextInputState("Hello");
  let textInput2 = gui.NewTextInputState("");
  let listItems = [
    "Apple",
    "Banana",
    "Cherry",
    "Date",
    "Elderberry",
    "Fig",
    "Grape",
    "Honeydew",
  ];
  let selectedItem = int32(0);
  let scrollOffset = int32(0);
  let spinnerValue = int32(50);
  let radioSelection = 0;
  let pickedColor = graphics.NewColor(100, 150, 200, 255);
  let demoWin = gui.NewWindowState(20, 70, 350, 420);
  let widgetsWin = gui.NewWindowState(390, 70, 380, 550);
  let anotherWin = gui.NewWindowState(790, 70, 250, 180);
  let infoWin = gui.NewWindowState(790, 265, 250, 200);
  let sphereWin = gui.NewWindowState(790, 480, 250, 250);
  let clicked = false;
  let fileMenuOpen = false;
  let viewMenuOpen = false;
  let dropY = 0;
  let dropX = 0;
  let fileDropX = 0;
  let viewDropX = 0;
  let toolbarState = { X: 0, Y: 0, Height: 0 };
  graphics.RunLoop(w, function (w) {
    ctx = gui.UpdateInput(ctx, w);
    graphics.Clear(w, graphics.NewColor(30, 30, 30, 255));
    [ctx, menuState] = gui.BeginMenuBar(
      ctx,
      w,
      menuState,
      0,
      0,
      graphics.GetWidth(w),
    );
    [ctx, menuState, fileMenuOpen] = gui.Menu(ctx, w, menuState, "File");
    if (fileMenuOpen) {
      fileDropX = menuState.CurrentMenuX - menuState.CurrentMenuW;
    }
    [ctx, menuState, viewMenuOpen] = gui.Menu(ctx, w, menuState, "View");
    if (viewMenuOpen) {
      viewDropX = menuState.CurrentMenuX - menuState.CurrentMenuW;
    }
    [ctx, menuState] = gui.EndMenuBar(ctx, menuState);
    [ctx, toolbarState] = gui.BeginToolbar(
      ctx,
      w,
      0,
      menuState.MenuBarH,
      graphics.GetWidth(w),
    );
    [ctx, toolbarState, clicked] = gui.ToolbarButton(
      ctx,
      w,
      toolbarState,
      "New",
    );
    if (clicked) {
      counter = 0;
    }
    [ctx, toolbarState, clicked] = gui.ToolbarButton(
      ctx,
      w,
      toolbarState,
      "Open",
    );
    [ctx, toolbarState, clicked] = gui.ToolbarButton(
      ctx,
      w,
      toolbarState,
      "Save",
    );
    toolbarState = gui.ToolbarSeparator(w, toolbarState);
    [ctx, toolbarState, clicked] = gui.ToolbarButton(
      ctx,
      w,
      toolbarState,
      "Undo",
    );
    [ctx, toolbarState, clicked] = gui.ToolbarButton(
      ctx,
      w,
      toolbarState,
      "Redo",
    );
    toolbarState = gui.ToolbarSeparator(w, toolbarState);
    [ctx, toolbarState, clicked] = gui.ToolbarButton(
      ctx,
      w,
      toolbarState,
      "Zoom In",
    );
    [ctx, toolbarState, clicked] = gui.ToolbarButton(
      ctx,
      w,
      toolbarState,
      "Zoom Out",
    );
    ctx = gui.EndToolbar(ctx, toolbarState);
    let pass = int32(0);
    let numPasses = gui.WindowPassCount(ctx);
    for (; pass < numPasses; ) {
      ctx = gui.BeginWindowPass(ctx, pass);
      if (showDemo) {
        [ctx, demoWin] = gui.DraggablePanel(ctx, w, "Demo Window", demoWin);
        if (ctx.DrawContent) {
          ctx = gui.BeginLayout(ctx, demoWin.X + 10, demoWin.Y + 50, 6);
          ctx = gui.AutoLabel(ctx, w, "Hello from goany GUI!");
          gui.Separator(ctx, w, demoWin.X + 10, ctx.CursorY - 2, 330);
          ctx.CursorY = ctx.CursorY + 4;
          [ctx, clicked] = gui.Button(
            ctx,
            w,
            "Click",
            demoWin.X + 10,
            ctx.CursorY,
            80,
            26,
          );
          if (clicked) {
            counter = counter + 1;
          }
          [ctx, clicked] = gui.Button(
            ctx,
            w,
            "Reset",
            demoWin.X + 100,
            ctx.CursorY,
            80,
            26,
          );
          if (clicked) {
            counter = 0;
            volume = 0.5;
            brightness = 75.0;
            progress = 0.35;
          }
          gui.Label(
            ctx,
            w,
            "Count: " + intToString(counter),
            demoWin.X + 190,
            ctx.CursorY + 4,
          );
          ctx = gui.NextRow(ctx, 26);
          gui.Separator(ctx, w, demoWin.X + 10, ctx.CursorY - 2, 330);
          ctx.CursorY = ctx.CursorY + 4;
          [ctx, showDemo] = gui.AutoCheckbox(
            ctx,
            w,
            "Show Demo Window",
            showDemo,
          );
          [ctx, showWidgets] = gui.AutoCheckbox(
            ctx,
            w,
            "Show Widgets Window",
            showWidgets,
          );
          [ctx, showAnother] = gui.AutoCheckbox(
            ctx,
            w,
            "Show Another Window",
            showAnother,
          );
          [ctx, enabled] = gui.AutoCheckbox(ctx, w, "Enable Feature", enabled);
          gui.Separator(ctx, w, demoWin.X + 10, ctx.CursorY - 2, 330);
          ctx.CursorY = ctx.CursorY + 4;
          [ctx, volume] = gui.AutoSlider(
            ctx,
            w,
            "Volume",
            320,
            0.0,
            1.0,
            volume,
          );
          [ctx, brightness] = gui.AutoSlider(
            ctx,
            w,
            "Bright",
            320,
            0.0,
            100.0,
            brightness,
          );
          gui.Separator(ctx, w, demoWin.X + 10, ctx.CursorY - 2, 330);
          ctx.CursorY = ctx.CursorY + 4;
          ctx = gui.AutoLabel(ctx, w, "Progress Bar:");
          gui.ProgressBar(
            ctx,
            w,
            demoWin.X + 10,
            ctx.CursorY,
            320,
            20,
            progress,
          );
          ctx = gui.NextRow(ctx, 24);
          progress = progress + 0.002;
          if (progress > 1.0) {
            progress = 0.0;
          }
        }
      }
      if (showWidgets) {
        [ctx, widgetsWin] = gui.DraggablePanel(
          ctx,
          w,
          "Widgets Showcase",
          widgetsWin,
        );
        if (ctx.DrawContent) {
          [ctx, tabState] = gui.TabBar(
            ctx,
            w,
            tabState,
            tabLabels,
            widgetsWin.X + 10,
            widgetsWin.Y + 35,
          );
          let contentY = widgetsWin.Y + 70;
          if (tabState.ActiveTab == 0) {
            ctx = gui.BeginLayout(ctx, widgetsWin.X + 15, contentY, 6);
            ctx = gui.AutoLabel(ctx, w, "Radio Buttons:");
            [ctx, clicked] = gui.AutoRadioButton(
              ctx,
              w,
              "Option A",
              radioSelection == 0,
            );
            if (clicked) {
              radioSelection = 0;
            }
            [ctx, clicked] = gui.AutoRadioButton(
              ctx,
              w,
              "Option B",
              radioSelection == 1,
            );
            if (clicked) {
              radioSelection = 1;
            }
            [ctx, clicked] = gui.AutoRadioButton(
              ctx,
              w,
              "Option C",
              radioSelection == 2,
            );
            if (clicked) {
              radioSelection = 2;
            }
            gui.Separator(ctx, w, widgetsWin.X + 15, ctx.CursorY, 350);
            ctx.CursorY = ctx.CursorY + 8;
            ctx = gui.AutoLabel(ctx, w, "Spinner:");
            [ctx, spinnerValue] = gui.Spinner(
              ctx,
              w,
              "Value",
              widgetsWin.X + 15,
              ctx.CursorY,
              spinnerValue,
              0,
              100,
            );
            ctx = gui.NextRow(ctx, 28);
            gui.Separator(ctx, w, widgetsWin.X + 15, ctx.CursorY, 350);
            ctx.CursorY = ctx.CursorY + 8;
            [ctx, pickedColor] = gui.ColorPicker(
              ctx,
              w,
              "Color Picker:",
              widgetsWin.X + 15,
              ctx.CursorY,
              340,
              pickedColor,
            );
          } else if (tabState.ActiveTab == 1) {
            ctx = gui.BeginLayout(ctx, widgetsWin.X + 15, contentY, 6);
            ctx = gui.AutoLabel(ctx, w, "List Box (select an item):");
            [ctx, selectedItem, scrollOffset] = gui.ListBox(
              ctx,
              w,
              listItems,
              widgetsWin.X + 15,
              ctx.CursorY,
              200,
              150,
              selectedItem,
              scrollOffset,
            );
            ctx.CursorY = ctx.CursorY + 155;
            ctx = gui.AutoLabel(ctx, w, "Selected: " + listItems[selectedItem]);
          } else if (tabState.ActiveTab == 2) {
            ctx = gui.BeginLayout(ctx, widgetsWin.X + 15, contentY, 4);
            ctx = gui.AutoLabel(ctx, w, "Tree View:");
            let expanded = false;
            let nodeY = ctx.CursorY;
            [ctx, treeState, expanded] = gui.TreeNode(
              ctx,
              w,
              treeState,
              "Root",
              widgetsWin.X + 15,
              nodeY,
              0,
            );
            nodeY = nodeY + 20;
            if (expanded) {
              [ctx, treeState, expanded] = gui.TreeNode(
                ctx,
                w,
                treeState,
                "Branch 1",
                widgetsWin.X + 15,
                nodeY,
                20,
              );
              nodeY = nodeY + 20;
              if (expanded) {
                [ctx, clicked] = gui.TreeLeaf(
                  ctx,
                  w,
                  "Leaf 1.1",
                  widgetsWin.X + 15,
                  nodeY,
                  40,
                );
                nodeY = nodeY + 20;
                [ctx, clicked] = gui.TreeLeaf(
                  ctx,
                  w,
                  "Leaf 1.2",
                  widgetsWin.X + 15,
                  nodeY,
                  40,
                );
                nodeY = nodeY + 20;
                [ctx, clicked] = gui.TreeLeaf(
                  ctx,
                  w,
                  "Leaf 1.3",
                  widgetsWin.X + 15,
                  nodeY,
                  40,
                );
                nodeY = nodeY + 20;
              }
              [ctx, treeState, expanded] = gui.TreeNode(
                ctx,
                w,
                treeState,
                "Branch 2",
                widgetsWin.X + 15,
                nodeY,
                20,
              );
              nodeY = nodeY + 20;
              if (expanded) {
                [ctx, clicked] = gui.TreeLeaf(
                  ctx,
                  w,
                  "Leaf 2.1",
                  widgetsWin.X + 15,
                  nodeY,
                  40,
                );
                nodeY = nodeY + 20;
                [ctx, clicked] = gui.TreeLeaf(
                  ctx,
                  w,
                  "Leaf 2.2",
                  widgetsWin.X + 15,
                  nodeY,
                  40,
                );
                nodeY = nodeY + 20;
              }
              [ctx, clicked] = gui.TreeLeaf(
                ctx,
                w,
                "Leaf 3",
                widgetsWin.X + 15,
                nodeY,
                20,
              );
              nodeY = nodeY + 20;
            }
          } else if (tabState.ActiveTab == 3) {
            ctx = gui.BeginLayout(ctx, widgetsWin.X + 15, contentY, 8);
            ctx = gui.AutoLabel(ctx, w, "Text Input:");
            gui.Label(ctx, w, "Name:", widgetsWin.X + 15, ctx.CursorY + 4);
            [ctx, textInput] = gui.TextInput(
              ctx,
              w,
              textInput,
              widgetsWin.X + 70,
              ctx.CursorY,
              200,
            );
            ctx = gui.NextRow(ctx, 28);
            gui.Label(ctx, w, "Email:", widgetsWin.X + 15, ctx.CursorY + 4);
            [ctx, textInput2] = gui.TextInput(
              ctx,
              w,
              textInput2,
              widgetsWin.X + 70,
              ctx.CursorY,
              200,
            );
            ctx = gui.NextRow(ctx, 28);
            gui.Separator(ctx, w, widgetsWin.X + 15, ctx.CursorY, 350);
            ctx.CursorY = ctx.CursorY + 8;
            ctx = gui.AutoLabel(ctx, w, "Entered text:");
            ctx = gui.AutoLabel(ctx, w, "  Name: " + textInput.Text);
            ctx = gui.AutoLabel(ctx, w, "  Email: " + textInput2.Text);
          }
        }
      }
      if (showAnother) {
        [ctx, anotherWin] = gui.DraggablePanel(
          ctx,
          w,
          "Another Window",
          anotherWin,
        );
        if (ctx.DrawContent) {
          gui.Label(
            ctx,
            w,
            "This is another panel!",
            anotherWin.X + 10,
            anotherWin.Y + 50,
          );
          [ctx, clicked] = gui.Button(
            ctx,
            w,
            "Close",
            anotherWin.X + 10,
            anotherWin.Y + 90,
            100,
            26,
          );
          if (clicked) {
            showAnother = false;
          }
        }
      }
      [ctx, infoWin] = gui.DraggablePanel(ctx, w, "Info", infoWin);
      if (ctx.DrawContent) {
        ctx = gui.BeginLayout(ctx, infoWin.X + 10, infoWin.Y + 50, 4);
        ctx = gui.AutoLabel(ctx, w, "Application Stats:");
        ctx = gui.AutoLabel(ctx, w, "  Volume: " + floatToString(volume));
        ctx = gui.AutoLabel(
          ctx,
          w,
          "  Brightness: " + floatToString(brightness),
        );
        ctx = gui.AutoLabel(ctx, w, "  Clicks: " + intToString(counter));
        ctx = gui.AutoLabel(
          ctx,
          w,
          "  Spinner: " + intToString(int(spinnerValue)),
        );
        ctx = gui.AutoLabel(ctx, w, "  Radio: " + intToString(radioSelection));
        if (
          ctx.MouseX >= infoWin.X &&
          ctx.MouseX <= infoWin.X + infoWin.Width &&
          ctx.MouseY >= infoWin.Y &&
          ctx.MouseY <= infoWin.Y + 30
        ) {
          gui.Tooltip(
            ctx,
            w,
            "Drag to move this panel",
            ctx.MouseX,
            ctx.MouseY,
          );
        }
      }
      [ctx, sphereWin] = gui.DraggablePanel(ctx, w, "3D Sphere", sphereWin);
      if (ctx.DrawContent) {
        drawSphere(
          w,
          sphereWin.X + 125,
          sphereWin.Y + 150,
          80,
          graphics.NewColor(180, 180, 190, 255),
        );
      }
      pass = pass + 1;
    }
    ctx = gui.EndWindowPasses(ctx);
    [ctx, clicked] = gui.Button(ctx, w, "Quit", 1150, 900, 100, 30);
    if (clicked) {
      return false;
    }
    if (fileMenuOpen) {
      dropX = fileDropX;
      [ctx, dropY] = gui.BeginDropdown(ctx, w, menuState, dropX, 5);
      [ctx, menuState, clicked] = gui.MenuItem(
        ctx,
        w,
        menuState,
        "New",
        dropX,
        dropY,
        0,
      );
      if (clicked) {
        counter = 0;
      }
      [ctx, menuState, clicked] = gui.MenuItem(
        ctx,
        w,
        menuState,
        "Open",
        dropX,
        dropY,
        1,
      );
      [ctx, menuState, clicked] = gui.MenuItem(
        ctx,
        w,
        menuState,
        "Save",
        dropX,
        dropY,
        2,
      );
      gui.MenuItemSeparator(ctx, w, dropX, dropY, 3);
      [ctx, menuState, clicked] = gui.MenuItem(
        ctx,
        w,
        menuState,
        "Exit",
        dropX,
        dropY,
        4,
      );
      if (clicked) {
        return false;
      }
    }
    if (viewMenuOpen) {
      dropX = viewDropX;
      [ctx, dropY] = gui.BeginDropdown(ctx, w, menuState, dropX, 3);
      [ctx, menuState, clicked] = gui.MenuItem(
        ctx,
        w,
        menuState,
        "Demo Window",
        dropX,
        dropY,
        0,
      );
      if (clicked) {
        showDemo = !showDemo;
      }
      [ctx, menuState, clicked] = gui.MenuItem(
        ctx,
        w,
        menuState,
        "Widgets Window",
        dropX,
        dropY,
        1,
      );
      if (clicked) {
        showWidgets = !showWidgets;
      }
      [ctx, menuState, clicked] = gui.MenuItem(
        ctx,
        w,
        menuState,
        "Another Window",
        dropX,
        dropY,
        2,
      );
      if (clicked) {
        showAnother = !showAnother;
      }
    }
    graphics.Present(w);
    return true;
  });
  graphics.CloseWindow(w);
}

function intToString(n) {
  let digitStrings = ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9"];
  if (n == 0) {
    return "0";
  }
  let negative = false;
  if (n < 0) {
    negative = true;
    n = -n;
  }
  let result = "";
  for (; n > 0; ) {
    let digit = n % 10;
    result = digitStrings[digit] + result;
    n = (n / 10) | 0;
  }
  if (negative) {
    result = "-" + result;
  }
  return result;
}

function floatToString(f) {
  let intPart = int(f);
  let fracPart = int((f - float64(intPart)) * 100);
  if (fracPart < 0) {
    fracPart = -fracPart;
  }
  let fracStr = intToString(fracPart);
  if (fracPart < 10) {
    fracStr = "0" + fracStr;
  }
  return intToString(intPart) + "." + fracStr;
}

function sinTable() {
  return [
    0, 105, 208, 309, 407, 500, 588, 669, 743, 809, 866, 914, 951, 978, 995,
    1000, 995, 978, 951, 914, 866, 809, 743, 669, 588, 500, 407, 309, 208, 105,
    0, -105, -208, -309, -407, -500, -588, -669, -743, -809, -866, -914, -951,
    -978, -995, -1000, -995, -978, -951, -914, -866, -809, -743, -669, -588,
    -500, -407, -309, -208, -105,
  ];
}

function cosTable() {
  return [
    1000, 995, 978, 951, 914, 866, 809, 743, 669, 588, 500, 407, 309, 208, 105,
    0, -105, -208, -309, -407, -500, -588, -669, -743, -809, -866, -914, -951,
    -978, -995, -1000, -995, -978, -951, -914, -866, -809, -743, -669, -588,
    -500, -407, -309, -208, -105, 0, 105, 208, 309, 407, 500, 588, 669, 743,
    809, 866, 914, 951, 978, 995,
  ];
}

function drawSphere(w, cx, cy, radius, color) {
  let sin = sinTable();
  let cos = cosTable();
  let latIdx = [45, 50, 55, 0, 5, 10, 15];
  let numLon = 12;
  let lat = 0;
  for (; lat < 6; ) {
    let li0 = latIdx[lat];
    let li1 = latIdx[lat + 1];
    let sinLat0 = sin[li0];
    let cosLat0 = cos[li0];
    let sinLat1 = sin[li1];
    let cosLat1 = cos[li1];
    let y0 = ((int32(sinLat0) * radius) / 1000) | 0;
    let y1 = ((int32(sinLat1) * radius) / 1000) | 0;
    let lon = 0;
    for (; lon < numLon; ) {
      let lonIdx0 = (lon * 5) % 60;
      let lonIdx1 = ((lon + 1) * 5) % 60;
      let x00 = (int32(((cosLat0 * cos[lonIdx0]) / 1000) | 0) * radius) / 1000;
      let x01 = (int32(((cosLat0 * cos[lonIdx1]) / 1000) | 0) * radius) / 1000;
      let x10 = (int32(((cosLat1 * cos[lonIdx0]) / 1000) | 0) * radius) / 1000;
      let x11 = (int32(((cosLat1 * cos[lonIdx1]) / 1000) | 0) * radius) / 1000;
      graphics.DrawLine(w, cx + x00, cy - y0, cx + x01, cy - y0, color);
      graphics.DrawLine(w, cx + x10, cy - y1, cx + x11, cy - y1, color);
      graphics.DrawLine(w, cx + x00, cy - y0, cx + x10, cy - y1, color);
      graphics.DrawLine(w, cx + x00, cy - y0, cx + x11, cy - y1, color);
      lon = lon + 1;
    }
    lat = lat + 1;
  }
}

// Run main
main();
