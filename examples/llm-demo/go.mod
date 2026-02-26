module llm-demo

go 1.22.5

require (
	libs/anyllm v0.0.0
	runtime/fs v0.0.0
)

replace libs/anyllm => ../../libs/anyllm

replace runtime/fs => ../../runtime/fs
