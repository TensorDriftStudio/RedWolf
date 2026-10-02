package domain

import "errors"

// PowerState represents the current physical power state of a server chassis.
type PowerState string

const (
	PowerStateOn      PowerState = "POWERED_ON"
	PowerStateOff     PowerState = "POWERED_OFF"
	PowerStateUnknown PowerState = "UNKNOWN"
)

// PowerAction represents an out-of-band power command issued to the BMC.
type PowerAction string

const (
	PowerActionOn               PowerAction = "on"
	PowerActionOff              PowerAction = "off"
	PowerActionReset            PowerAction = "reset"
	PowerActionGracefulShutdown PowerAction = "graceful_shutdown"
	PowerActionPXEReboot        PowerAction = "pxe_reboot"
)

// Validate checks whether the power action is recognized.
func (a PowerAction) Validate() error {
	switch a {
	case PowerActionOn, PowerActionOff, PowerActionReset, PowerActionGracefulShutdown, PowerActionPXEReboot:
		return nil
	default:
		return errors.New("unsupported power action: " + string(a))
	}
}

// PowerStatusResponse represents the response payload for a power state query.
type PowerStatusResponse struct {
	NodeID     string     `json:"nodeId"`
	PowerState PowerState `json:"powerState"`
	BMCIP      string     `json:"bmcIp"`
}
