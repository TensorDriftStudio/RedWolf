package ipmi

import (
	"context"
	"fmt"
	"log/slog"
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

// UpdateCredentials updates and enables the specified user slot password on the physical BMC via IPMI 2.0 RMCP+.
func (c *RemoteController) UpdateCredentials(ctx context.Context, bmc domain.BMCInfo, currentCreds *domain.BMCCredential, newUsername, newPassword string, slot int) error {
	if bmc.IP == "" {
		return fmt.Errorf("bmc ip address is empty")
	}
	if slot <= 0 {
		slot = 2 // Standard administrator user slot
	}

	args := c.buildBaseArgs(bmc.IP, currentCreds)

	// Step 1: Set username if provided
	if newUsername != "" {
		nameArgs := append([]string{}, args...)
		nameArgs = append(nameArgs, "user", "set", "name", fmt.Sprintf("%d", slot), newUsername)
		cmd := exec.CommandContext(ctx, "ipmitool", nameArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.DebugContext(ctx, "ipmitool user set name non-fatal warning", "slot", slot, "output", string(out), "error", err)
		}
	}

	// Step 2: Set user password (strictly 14-16 characters)
	passArgs := append([]string{}, args...)
	passArgs = append(passArgs, "user", "set", "password", fmt.Sprintf("%d", slot), newPassword)
	cmdPass := exec.CommandContext(ctx, "ipmitool", passArgs...)
	if out, err := cmdPass.CombinedOutput(); err != nil {
		return fmt.Errorf("ipmitool user set password failed: %w (output: %s)", err, string(out))
	}

	// Step 3: Enable user account
	enableArgs := append([]string{}, args...)
	enableArgs = append(enableArgs, "user", "enable", fmt.Sprintf("%d", slot))
	cmdEnable := exec.CommandContext(ctx, "ipmitool", enableArgs...)
	if out, err := cmdEnable.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "ipmitool user enable warning", "slot", slot, "output", string(out), "error", err)
	}

	// Step 4: Grant administrator privilege on LAN channel
	channel := bmc.Channel
	if channel <= 0 {
		channel = 1
	}
	privArgs := append([]string{}, args...)
	privArgs = append(privArgs, "channel", "setaccess", fmt.Sprintf("%d", channel), fmt.Sprintf("%d", slot), "callin=on", "ipmi=on", "link=on", "privilege=4")
	cmdPriv := exec.CommandContext(ctx, "ipmitool", privArgs...)
	if out, err := cmdPriv.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "ipmitool channel setaccess privilege warning", "channel", channel, "slot", slot, "output", string(out), "error", err)
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
