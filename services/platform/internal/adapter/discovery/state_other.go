//go:build !linux

package discovery

func StateDirectory() (string, error)        { return "", errUnsafe }
func PrepareStateDirectory() (string, error) { return "", errUnsafe }
