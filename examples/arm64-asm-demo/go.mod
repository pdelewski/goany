module arm64-asm-demo

go 1.22.5

require (
	libs/arm64asm v0.0.0
	libs/binreader v0.0.0
	libs/macho v0.0.0
	runtime/fs v0.0.0
)

replace (
	libs/arm64asm => ../../libs/arm64asm
	libs/binreader => ../../libs/binreader
	libs/macho => ../../libs/macho
	runtime/fs => ../../runtime/fs
)
