package provision

import (
	"testing"
)

func TestParseEFIPartitionFromJSON(t *testing.T) {
	// AlmaLinux 9: ESP on partition 2
	almaJSON := []byte(`{
		"blockdevices": [
			{
				"name": "nvme0n1",
				"path": "/dev/nvme0n1",
				"type": "disk",
				"children": [
					{
						"name": "nvme0n1p1",
						"path": "/dev/nvme0n1p1",
						"type": "part",
						"fstype": null,
						"label": null
					},
					{
						"name": "nvme0n1p2",
						"path": "/dev/nvme0n1p2",
						"type": "part",
						"fstype": "vfat",
						"label": "EFI"
					},
					{
						"name": "nvme0n1p3",
						"path": "/dev/nvme0n1p3",
						"type": "part",
						"fstype": "xfs",
						"label": "root"
					}
				]
			}
		]
	}`)

	partNum := parseEFIPartitionFromJSON(almaJSON)
	if partNum != 2 {
		t.Fatalf("expected EFI partition 2 for AlmaLinux, got %d", partNum)
	}

	// Debian 12: ESP on partition 15
	debianJSON := []byte(`{
		"blockdevices": [
			{
				"name": "sda",
				"path": "/dev/sda",
				"type": "disk",
				"children": [
					{
						"name": "sda1",
						"path": "/dev/sda1",
						"type": "part",
						"fstype": "ext4",
						"label": "rootfs"
					},
					{
						"name": "sda15",
						"path": "/dev/sda15",
						"type": "part",
						"fstype": "vfat",
						"label": "ESP"
					}
				]
			}
		]
	}`)

	partNumDebian := parseEFIPartitionFromJSON(debianJSON)
	if partNumDebian != 15 {
		t.Fatalf("expected EFI partition 15 for Debian, got %d", partNumDebian)
	}
}
