package collector

import (
	"testing"
)

func TestParseLANPrintOutput_DHCPWithLink(t *testing.T) {
	rawOutput := []byte(`Set in Progress         : Set Complete
Auth Type Support       : NONE MD2 MD5 PASSWORD 
Auth Type Enable        : Callback : NONE MD2 MD5 PASSWORD 
IP Address Source       : DHCP Address
IP Address              : 10.10.20.52
Subnet Mask             : 255.255.255.0
MAC Address             : ac:1f:6b:80:99:aa
802.1q VLAN ID          : Disabled
Default Gateway IP      : 10.10.20.1
Link Status             : Link Detected
`)

	mac, ip, isDHCP, linkDetected := parseLANPrintOutput(rawOutput)
	if mac != "ac:1f:6b:80:99:aa" {
		t.Errorf("expected mac 'ac:1f:6b:80:99:aa', got %s", mac)
	}
	if ip != "10.10.20.52" {
		t.Errorf("expected ip '10.10.20.52', got %s", ip)
	}
	if !isDHCP {
		t.Errorf("expected isDHCP true, got %v", isDHCP)
	}
	if !linkDetected {
		t.Errorf("expected linkDetected true, got %v", linkDetected)
	}
}

func TestParseLANPrintOutput_StaticNoLink(t *testing.T) {
	rawOutput := []byte(`IP Address Source       : Static Address
IP Address              : 192.168.1.100
MAC Address             : 00:25:90:ab:cd:ef
Link Status             : Link Down
`)

	mac, ip, isDHCP, linkDetected := parseLANPrintOutput(rawOutput)
	if mac != "00:25:90:ab:cd:ef" {
		t.Errorf("expected mac '00:25:90:ab:cd:ef', got %s", mac)
	}
	if ip != "192.168.1.100" {
		t.Errorf("expected ip '192.168.1.100', got %s", ip)
	}
	if isDHCP {
		t.Errorf("expected isDHCP false, got %v", isDHCP)
	}
	if linkDetected {
		t.Errorf("expected linkDetected false, got %v", linkDetected)
	}
}
