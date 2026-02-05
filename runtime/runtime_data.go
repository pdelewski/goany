package runtime

import _ "embed"

//go:embed std/hashmap.go
var HashmapGoSource string

//go:embed std/cpp/goany_panic.hpp
var PanicCppSource string

//go:embed std/csharp/GoanyPanic.cs
var PanicCsSource string

//go:embed std/rust/goany_panic.rs
var PanicRustSource string

//go:embed std/js/goany_panic.js
var PanicJsSource string
