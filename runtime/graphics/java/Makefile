# Makefile for building the Java graphics JNI native library
# Requires JAVA_HOME to be set
# All source files (tigr.c, screen_helper.c, graphics_jni.c) are expected in the current directory

JAVA_HOME ?= $(shell /usr/libexec/java_home 2>/dev/null || echo $$JAVA_HOME)

UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
    # macOS
    CC = cc
    CFLAGS = -shared -fPIC -I"$(JAVA_HOME)/include" -I"$(JAVA_HOME)/include/darwin" -I.
    LDFLAGS = -framework OpenGL -framework Cocoa
    TARGET = libgraphics_jni.dylib
else ifeq ($(UNAME_S),Linux)
    # Linux
    CC = gcc
    CFLAGS = -shared -fPIC -I"$(JAVA_HOME)/include" -I"$(JAVA_HOME)/include/linux" -I.
    LDFLAGS = -lGL -lX11
    TARGET = libgraphics_jni.so
else
    # Windows (MinGW)
    CC = gcc
    CFLAGS = -shared -I"$(JAVA_HOME)/include" -I"$(JAVA_HOME)/include/win32" -I.
    LDFLAGS = -lopengl32 -lgdi32 -luser32
    TARGET = graphics_jni.dll
endif

SOURCES = graphics_jni.c tigr.c screen_helper.c

all: $(TARGET)

$(TARGET): $(SOURCES)
	@echo "Building $(TARGET) with JAVA_HOME=$(JAVA_HOME)"
	$(CC) $(CFLAGS) -o $@ $(SOURCES) $(LDFLAGS)

clean:
	rm -f $(TARGET)

.PHONY: all clean
