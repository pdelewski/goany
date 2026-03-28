module libs/macho

go 1.22.5

require (
	libs/binreader v0.0.0
	runtime/fs v0.0.0
)

replace (
	libs/binreader => ../binreader
	runtime/fs => ../../runtime/fs
)
