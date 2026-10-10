package domain

// Vendor identifies supported server manufacturers.
type Vendor string

const (
	VendorDell       Vendor = "Dell Inc."
	VendorSupermicro Vendor = "Supermicro"
	VendorASRockRack Vendor = "ASRockRack"
	VendorGeneric    Vendor = "Generic"
)

// FirmwareMode indicates whether the host booted in UEFI or BIOS mode.
type FirmwareMode string

const (
	FirmwareAuto FirmwareMode = "AUTO"
	FirmwareUEFI FirmwareMode = "UEFI"
	FirmwareBIOS FirmwareMode = "BIOS"
)

// CPUInfo represents host processor topology and microarchitecture.
type CPUInfo struct {
	Model            string `json:"model"`
	Sockets          int    `json:"sockets"`
	CoresPerSocket   int    `json:"coresPerSocket"`
	ThreadsPerSocket int    `json:"threadsPerSocket"`
	TotalThreads     int    `json:"totalThreads"`
	Arch             string `json:"arch"`
}

// MemoryInfo catalogs host RAM capacity and slot population.
type MemoryInfo struct {
	TotalBytes uint64 `json:"totalBytes"`
	TotalHuman string `json:"totalHuman"`
	SlotsUsed  int    `json:"slotsUsed"`
	SlotsTotal int    `json:"slotsTotal"`
	Type       string `json:"type"`
	SpeedMHz   int    `json:"speedMhz"`
}

// StorageDevice contains machine-readable block device attributes.
type StorageDevice struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	ByID       string `json:"byId"`
	SizeBytes  uint64 `json:"sizeBytes"`
	SizeHuman  string `json:"sizeHuman"`
	Type       string `json:"type"`
	Transport  string `json:"transport"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
}

// NetworkInterface records physical NIC attributes and boot link state.
type NetworkInterface struct {
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	SpeedMbps int    `json:"speedMbps"`
	Carrier   bool   `json:"carrier"`
	IsBoot    bool   `json:"isBoot"`
	Driver    string `json:"driver"`
	PCISlot   string `json:"pciSlot"`
}

// PortMode defines the out-of-band management interface mode.
type PortMode string

const (
	PortModeDedicated PortMode = "Dedicated"
	PortModeShared    PortMode = "Shared"
	PortModeFailover  PortMode = "Failover"
)

// BMCInfo encapsulates baseboard management controller telemetry.
type BMCInfo struct {
	Vendor             string   `json:"vendor"`
	IP                 string   `json:"ip"`
	MAC                string   `json:"mac"`
	DHCP               bool     `json:"dhcp"`
	Channel            int      `json:"channel"`
	PortMode           PortMode `json:"portMode"`
	CredentialsUpdated bool     `json:"credentialsUpdated"`
}
