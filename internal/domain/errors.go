package domain

import "errors"

// Domain sentinel errors for RedWolf Core.
var (
	ErrNodeNotFound         = errors.New("node not found")
	ErrNodeAlreadyExists    = errors.New("node already exists")
	ErrInvalidNodeStatus    = errors.New("invalid node status transition")
	ErrInvalidStorageDevice = errors.New("invalid storage device target")
	ErrInvalidMACAddress    = errors.New("invalid MAC address format")
	ErrInvalidIPAddress     = errors.New("invalid IP address format")
	ErrMissingCredentials   = errors.New("missing or empty credentials")
	ErrInvalidPasswordLen   = errors.New("BMC password must be between 14 and 16 characters")
	ErrInvalidYAMLConfig    = errors.New("invalid YAML syntax in cloud-init configuration")
	ErrInsufficientStorage  = errors.New("insufficient storage capacity for requested layout")
	ErrBootloaderFailed     = errors.New("bootloader installation or registration failed")
)

// ErrNodeNotFoundIs checks whether an error unwraps to ErrNodeNotFound.
func ErrNodeNotFoundIs(err error) bool {
	return errors.Is(err, ErrNodeNotFound)
}
