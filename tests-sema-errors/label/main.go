package main

// ERROR: Labeled statements are not supported
func main() {
label: // error: label not allowed
	_ = 0
	goto label // error: goto not allowed
}
