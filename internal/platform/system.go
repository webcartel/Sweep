package platform

// System answers questions about the running Windows that are not about files
// or processes. The scanner asks it for the OS build, to skip rules that name
// a newer one.
type System interface {
	OSBuild() (int, error)

	// Elevated says whether the process runs with administrator rights.
	Elevated() (bool, error)

	// UILanguage answers the language of the Windows user interface as
	// "en" or "ru". Every other language Windows may speak answers "en",
	// which is the fallback the whole program falls back on.
	UILanguage() string
}
