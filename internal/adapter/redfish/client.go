package redfish

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

var _ port.BMCController = (*Client)(nil)

// Client implements port.BMCController via standard DMTF Redfish REST APIs.
type Client struct {
	httpClient *http.Client
}

// NewClient initializes a Redfish REST client with custom timeouts and TLS configuration.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 2500 * time.Millisecond,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // Allow self-signed BMC certificates
				},
				DisableKeepAlives: true,
			},
		},
	}
}

// candidateSystemURIs lists vendor-specific paths to the primary ComputerSystem resource.
var candidateSystemURIs = []string{
	"/redfish/v1/Systems/System.Embedded.1", // Dell PowerEdge iDRAC 8/9
	"/redfish/v1/Systems/1",                 // Supermicro AMI MegaRAC
	"/redfish/v1/Systems/Self",              // ASRock Rack AST2500/2600
}

// GetPowerState queries the current chassis power state via Redfish.
func (c *Client) GetPowerState(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential) (domain.PowerState, error) {
	if bmc.IP == "" {
		return domain.PowerStateUnknown, fmt.Errorf("bmc ip address is empty")
	}

	for _, systemURI := range candidateSystemURIs {
		url := fmt.Sprintf("https://%s%s", bmc.IP, systemURI)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		if creds != nil && creds.Username != "" {
			req.SetBasicAuth(creds.Username, creds.Password)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var payload struct {
			PowerState string `json:"PowerState"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			continue
		}

		switch strings.ToUpper(payload.PowerState) {
		case "ON":
			return domain.PowerStateOn, nil
		case "OFF":
			return domain.PowerStateOff, nil
		default:
			return domain.PowerStateUnknown, nil
		}
	}

	return domain.PowerStateUnknown, fmt.Errorf("no responsive redfish endpoint found on %s", bmc.IP)
}

// ExecutePowerAction sends a ComputerSystem.Reset action to the BMC.
func (c *Client) ExecutePowerAction(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential, action domain.PowerAction) error {
	if bmc.IP == "" {
		return fmt.Errorf("bmc ip address is empty")
	}

	var resetType string
	switch action {
	case domain.PowerActionOn:
		resetType = "On"
	case domain.PowerActionOff:
		resetType = "ForceOff"
	case domain.PowerActionGracefulShutdown:
		resetType = "GracefulShutdown"
	case domain.PowerActionReset, domain.PowerActionPXEReboot:
		resetType = "ForceRestart"
	default:
		return fmt.Errorf("unsupported redfish power action: %s", action)
	}

	// If PXE reboot requested, first set one-time boot override
	if action == domain.PowerActionPXEReboot {
		if err := c.SetOneTimePXEBoot(ctx, bmc, creds); err != nil {
			return fmt.Errorf("failed setting one-time pxe boot before reset: %w", err)
		}
	}

	bodyData, _ := json.Marshal(map[string]string{
		"ResetType": resetType,
	})

	for _, systemURI := range candidateSystemURIs {
		actionURL := fmt.Sprintf("https://%s%s/Actions/ComputerSystem.Reset", bmc.IP, systemURI)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, actionURL, bytes.NewReader(bodyData))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if creds != nil && creds.Username != "" {
			req.SetBasicAuth(creds.Username, creds.Password)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
	}

	return fmt.Errorf("failed executing redfish power action %s on %s", action, bmc.IP)
}

// SetOneTimePXEBoot sets the UEFI/BIOS boot override to Network PXE for the next boot cycle.
func (c *Client) SetOneTimePXEBoot(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential) error {
	if bmc.IP == "" {
		return fmt.Errorf("bmc ip address is empty")
	}

	patchPayload := map[string]any{
		"Boot": map[string]string{
			"BootSourceOverrideTarget":  "Pxe",
			"BootSourceOverrideEnabled": "Once",
			"BootSourceOverrideMode":    "UEFI",
		},
	}
	bodyData, _ := json.Marshal(patchPayload)

	for _, systemURI := range candidateSystemURIs {
		url := fmt.Sprintf("https://%s%s", bmc.IP, systemURI)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(bodyData))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if creds != nil && creds.Username != "" {
			req.SetBasicAuth(creds.Username, creds.Password)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
	}

	return fmt.Errorf("failed setting one-time pxe boot via redfish on %s", bmc.IP)
}

// UpdateCredentials updates credentials on the physical BMC using standard Redfish AccountService.
func (c *Client) UpdateCredentials(ctx context.Context, bmc domain.BMCInfo, currentCreds *domain.BMCCredential, newUsername, newPassword string, slot int) error {
	if bmc.IP == "" {
		return fmt.Errorf("bmc ip address is empty")
	}
	if slot <= 0 {
		slot = 2
	}

	patchPayload := map[string]any{
		"Password": newPassword,
		"Enabled":  true,
	}
	if newUsername != "" {
		patchPayload["UserName"] = newUsername
	}
	bodyData, _ := json.Marshal(patchPayload)

	accountURIs := []string{
		fmt.Sprintf("/redfish/v1/AccountService/Accounts/%d", slot),
		"/redfish/v1/AccountService/Accounts/2",
		"/redfish/v1/AccountService/Accounts/1",
	}

	for _, accountURI := range accountURIs {
		url := fmt.Sprintf("https://%s%s", bmc.IP, accountURI)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(bodyData))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if currentCreds != nil && currentCreds.Username != "" {
			req.SetBasicAuth(currentCreds.Username, currentCreds.Password)
		} else {
			req.SetBasicAuth("ADMIN", "ADMIN")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
	}

	return fmt.Errorf("failed updating bmc credentials via redfish on %s", bmc.IP)
}
