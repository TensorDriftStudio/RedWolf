package collector

import (
	"encoding/json"
	"testing"
)

func TestFormatSizeHuman(t *testing.T) {
	cases := []struct {
		bytes    uint64
		expected string
	}{
		{500 * 1000 * 1000 * 1000, "500 GB"},
		{960 * 1000 * 1000 * 1000, "960 GB"},
		{1920 * 1000 * 1000 * 1000, "1.92 TB"},
		{3840 * 1000 * 1000 * 1000, "3.84 TB"},
	}

	for _, c := range cases {
		result := formatSizeHuman(c.bytes)
		if result != c.expected {
			t.Errorf("formatSizeHuman(%d) = %s, expected %s", c.bytes, result, c.expected)
		}
	}
}

func TestClassifyStorageType(t *testing.T) {
	bossDev := lsblkDevice{
		Name:       "sda",
		Size:       json.Number("960197124096"),
		Model:      "DELL BOSS-S1 RAID1",
		Rotational: false,
	}
	if got := classifyStorageType(bossDev, 960197124096); got != "Dell BOSS RAID 1" {
		t.Errorf("expected 'Dell BOSS RAID 1', got %s", got)
	}

	nvmeDev := lsblkDevice{
		Name:       "nvme0n1",
		Size:       json.Number("1920383410176"),
		Transport:  "nvme",
		Rotational: false,
	}
	if got := classifyStorageType(nvmeDev, 1920383410176); got != "NVMe PCIe SSD" {
		t.Errorf("expected 'NVMe PCIe SSD', got %s", got)
	}

	satadomDev := lsblkDevice{
		Name:       "sda",
		Model:      "Supermicro SATADOM-SL",
		Rotational: false,
	}
	if got := classifyStorageType(satadomDev, 64000000000); got != "Supermicro SATADOM" {
		t.Errorf("expected 'Supermicro SATADOM', got %s", got)
	}
}
