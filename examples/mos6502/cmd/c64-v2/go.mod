module c64-v2

go 1.23

require runtime/graphics v0.0.0

require mos6502lib v0.0.0

replace runtime/graphics => ../../../../runtime/graphics

replace mos6502lib => ../../lib
