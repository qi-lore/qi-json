// Package covname deliberately has a name that differs from its directory
// (covalias), so the generator emits an aliased import for it.
package covname

// Aliased is a plain struct bound from another package through an aliased
// import.
type Aliased struct {
	Name string `json:"name"`
}
