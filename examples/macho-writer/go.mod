module macho-writer

go 1.22.5

require (
	libs/binreader v0.0.0
	libs/macho v0.0.0
	runtime/fs v0.0.0
)

replace (
	libs/binreader => ../../libs/binreader
	libs/macho => ../../libs/macho
	runtime/fs => ../../runtime/fs
)
