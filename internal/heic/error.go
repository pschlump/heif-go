package heic

import "fmt"

type decodeError struct {
	kind string
	what string
}

func (e *decodeError) Error() string { return e.kind + ": " + e.what }

func truncated(what string) error {
	return &decodeError{"truncated file: ran out of bytes reading", what}
}
func malformed(what string) error   { return &decodeError{"malformed file", what} }
func unsupported(what string) error { return &decodeError{"unsupported", what} }
func missingBox(what string) error  { return &decodeError{"required box is missing", what} }
func missingItem(id uint32) error {
	return &decodeError{"item is referenced but not present", fmt.Sprint(id)}
}
func missingParameterSet(what string) error {
	return &decodeError{"the bitstream names a parameter set that the file does not carry", what}
}
func cyclicDerivation(id uint32) error {
	return &decodeError{"item derives from itself, directly or indirectly", fmt.Sprint(id)}
}
