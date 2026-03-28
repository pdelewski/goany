module gguf-demo

go 1.22.5

require (
	libs/anyllm v0.0.0
	libs/binreader v0.0.0
	runtime/fs v0.0.0
	runtime/math v0.0.0
)

replace libs/anyllm => ../../libs/anyllm

replace libs/binreader => ../../libs/binreader

replace runtime/fs => ../../runtime/fs

replace runtime/math => ../../runtime/math
