package version

// Passed in via `ldflags "-X ..."`
var edgelbVersionString string

// Version returns the version
func Version() string {
	return edgelbVersionString
}
