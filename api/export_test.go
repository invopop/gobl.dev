package api

// SetBodyLimits sets the import and export body limits for a test, and
// returns a function that restores them.
func SetBodyLimits(importLimit, exportLimit int64) func() {
	i, e := importBodyLimit, exportBodyLimit
	importBodyLimit, exportBodyLimit = importLimit, exportLimit
	return func() {
		importBodyLimit, exportBodyLimit = i, e
	}
}
