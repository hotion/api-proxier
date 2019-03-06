package plugin

// PlgStatus ...
type PlgStatus string

const (
	// Reloading ...
	Reloading PlgStatus = "reloading"
	// Stopped ...
	Stopped PlgStatus = "stopped"
	// Working ...
	Working PlgStatus = "working"
)

// Plugin type Plugin want to save all plugin
type Plugin interface {
	// Handle method to mainly deal with work
	Handle(ctx *Context)

	// Status get plugin status of current plugin.
	Status() PlgStatus

	// Enabled get plugin enabled or not, if not enabled will be skipped.
	Enabled() bool

	// Name method get plgugin name
	Name() string

	// Enable to enabled or disable current plugin
	Enable(enabled bool)
}
