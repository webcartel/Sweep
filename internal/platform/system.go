package platform

// System answers questions about the running Windows that are not about files
// or processes. The scanner asks it for the OS build, to skip rules that name
// a newer one.
type System interface {
	OSBuild() (int, error)
}
