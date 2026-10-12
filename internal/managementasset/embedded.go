package managementasset

import _ "embed"

//go:embed assets/management.html
var defaultEmbeddedManagementHTML []byte

// EmbeddedManagementHTML returns the embedded management.html asset content.
func EmbeddedManagementHTML() []byte {
	return defaultEmbeddedManagementHTML
}
