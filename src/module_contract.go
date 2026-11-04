package main

// Modules use only the shared HTTPS/session layer of the APIClient.
// No feature module may call another feature module, including on errors.
// A failed optional name lookup is represented as a warning: primary data
// remains available, rather than being misreported as an empty collection.
type moduleRows struct {
	items    []map[string]any
	warnings []string
}
