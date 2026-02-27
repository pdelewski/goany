module libs/anyllm

go 1.22.5

require (
	runtime/fs v0.0.0
	runtime/math v0.0.0
)

replace (
	runtime/fs => ../../runtime/fs
	runtime/math => ../../runtime/math
)
