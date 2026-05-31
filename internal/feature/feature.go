package feature

type Feature int

const (
	// Enable the user permissions on a connection.
	SyncConnectionToUser Feature = iota
	// Enable the user group permissions on a connection.
	SyncConnectionToUserGroup
)

type Flag map[Feature]bool

func (f Flag) Has(feature Feature) bool {
	if enabled, ok := f[feature]; ok {
		return enabled
	}

	return false
}

func (f Flag) Set(feature Feature, enabled bool) {
	if f == nil {
		f = make(Flag)
	}

	f[feature] = enabled
}
