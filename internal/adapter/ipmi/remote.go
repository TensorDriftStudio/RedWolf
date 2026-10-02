package ipmi

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

var _ port.BMCController = (*RemoteController)(nil)

// RemoteController executes out-of-band IPMI 2.0 commands via RMCP+ (lanplus).
type RemoteController struct{}

// NewRemoteController initializes the remote IPMI RMCP+ controller.
func NewRemoteController() *RemoteController {
	return &RemoteController{}
}

// GetPowerState executes 'chassis power status' over IPMI 2.0 LAN.
func (c *RemoteController) GetPowerState(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential) (domain.PowerState, error) {
	if bmc.IP == "" {
		return domain.PowerStateUnknown, fmt.Errorf("bmc ip address is empty")
	}

	args := c.buildBaseArgs(bmc.IP, creds)
	args = append(args, "chassis", "power", "status")

	cmd := exec.CommandContext(ctx, "ipmitool", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return domain.PowerStateUnknown, fmt.Errorf("ipmitool chassis power status failed: %w (output: %s)", err, string(out))
	}

	output := strings.ToLower(string(out))
	if strings.Contains(output, "is on") {
		return domain.PowerStateOn, nil
	}
	if strings.Contains(output, "is off") {
		return domain.PowerStateOff, nil
	}

	return domain.PowerStateUnknown, nil
}

// ExecutePowerAction issues power control commands over IPMI 2.0 LAN.
func (c *RemoteController) ExecutePowerAction(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential, action domain.PowerAction) error {
	if bmc.IP == "" {
		return fmt.Errorf("bmc ip address is empty")
	}

	if action == domain.PowerActionPXEReboot {
		if err := c.SetOneTimePXEBoot(ctx, bmc, creds); err != nil {
			return fmt.Errorf("failed setting one-time pxe boot before reset: %w", err)
		}
	}

	var ipmiPowerCmd string
	switch action {
	case domain.PowerActionOn:
		ipmiPowerCmd = "on"
	case domain.PowerActionOff:
		ipmiPowerCmd = "off"
	case domain.PowerActionGracefulShutdown:
		ipmiPowerCmd = "soft"
	case domain.PowerActionReset, domain.PowerActionPXEReboot:
		ipmiPowerCmd = "cycle"
	default:
		return fmt.Errorf("unsupported ipmi power action: %s", action)
	}

	args := c.buildBaseArgs(bmc.IP, creds)
	args = append(args, "chassis", "power", ipmiPowerCmd)

	cmd := exec.CommandContext(ctx, "ipmitool", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ipmitool chassis power %s failed: %w (output: %s)", ipmiPowerCmd, err, string(out))
	}

	return nil
}

// SetOneTimePXEBoot instructs the BIOS/BMC to boot from network PXE on the next reboot cycle.
func (c *RemoteController) SetOneTimePXEBoot(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential) error {
	if bmc.IP == "" {
		return fmt.Errorf("bmc ip address is empty")
	}

	args := c.buildBaseArgs(bmc.IP, creds)
	args = append(args, "chassis", "bootdev", "pxe", "options=persistent=no")

	cmd := exec.CommandContext(ctx, "ipmitool", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ipmitool chassis bootdev pxe failed: %w (output: %s)", err, string(out))
	}

	return nil
}

func (c *RemoteController) buildBaseArgs(ip string, creds *domain.BMCCredential) []string {
	user := "ADMIN"
	pass := "ADMIN"
	if creds != nil && creds.Username != "" {
		user = creds.Username
		pass = creds.Password
	}

	return []string{
		"-I", "lanplus",
		"-H", ip,
		"-U", user,
		"-P", pass,
	}
}
