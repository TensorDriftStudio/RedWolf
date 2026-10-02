package domain

import "time"

// LDAPConfig holds connection and query parameters for OpenLDAP/FreeIPA servers.
type LDAPConfig struct {
	Enabled            bool   `json:"enabled"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	UseTLS             bool   `json:"useTls"`
	StartTLS           bool   `json:"startTls"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	BindDN             string `json:"bindDn"`
	BindPassword       string `json:"bindPassword"`
	BaseDN             string `json:"baseDn"`
	UserFilter         string `json:"userFilter"`
	GroupSearchDN      string `json:"groupSearchDn"`
	AdminGroupDN       string `json:"adminGroupDn"`
	OperatorGroupDN    string `json:"operatorGroupDn"`
}

// ActiveDirectoryConfig specifies parameters for Microsoft Active Directory Domain Services.
type ActiveDirectoryConfig struct {
	Enabled            bool   `json:"enabled"`
	Domain             string `json:"domain"`
	DomainController   string `json:"domainController"`
	Port               int    `json:"port"`
	UseLDAPS           bool   `json:"useLdaps"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	BindDN             string `json:"bindDn"`
	BindPassword       string `json:"bindPassword"`
	BaseDN             string `json:"baseDn"`
	UserSearchFilter   string `json:"userSearchFilter"`
	AdminGroup         string `json:"adminGroup"`
	OperatorGroup      string `json:"operatorGroup"`
}

// NetworkSettings holds DHCP and PXE network configuration.
type NetworkSettings struct {
	SubnetCIDR           string   `json:"subnetCidr"`
	DHCPRangeStart       string   `json:"dhcpRangeStart"`
	DHCPRangeEnd         string   `json:"dhcpRangeEnd"`
	Gateway              string   `json:"gateway"`
	DNSServers           []string `json:"dnsServers"`
	LeaseDurationMinutes int      `json:"leaseDurationMinutes"`
}

// GeneralSettings holds core appliance settings.
type GeneralSettings struct {
	ApplianceName         string          `json:"applianceName"`
	ServerURL             string          `json:"serverUrl"`
	ProvisioningInterface string          `json:"provisioningInterface"`
	DefaultOS             OperatingSystem `json:"defaultOs"`
}

// AuthSettings aggregates identity providers.
type AuthSettings struct {
	LocalAuthEnabled bool                  `json:"localAuthEnabled"`
	LDAP             LDAPConfig            `json:"ldap"`
	ActiveDirectory  ActiveDirectoryConfig `json:"activeDirectory"`
}

// StorageSettings captures OS distribution caching and paths.
type StorageSettings struct {
	ImageStorageDir string `json:"imageStorageDir"`
	MaxCacheSizeGB  int    `json:"maxCacheSizeGb"`
}

// SystemSettings encapsulates all configurable system parameters.
type SystemSettings struct {
	General   GeneralSettings `json:"general"`
	Network   NetworkSettings `json:"network"`
	Auth      AuthSettings    `json:"auth"`
	Storage   StorageSettings `json:"storage"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// DirectoryTestRequest represents payload sent to verify directory connectivity.
type DirectoryTestRequest struct {
	Source          AuthSource            `json:"source"`
	LDAP            *LDAPConfig           `json:"ldap,omitempty"`
	ActiveDirectory *ActiveDirectoryConfig `json:"activeDirectory,omitempty"`
	TestUsername    string                `json:"testUsername,omitempty"`
	TestPassword    string                `json:"testPassword,omitempty"`
}

// DirectoryTestResult encapsulates diagnostics from directory connection tests.
type DirectoryTestResult struct {
	Success      bool      `json:"success"`
	LatencyMs    int64     `json:"latencyMs"`
	Message      string    `json:"message"`
	EntriesFound int       `json:"entriesFound"`
	TestedAt     time.Time `json:"testedAt"`
}
