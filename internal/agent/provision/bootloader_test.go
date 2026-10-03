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

func TestParseBootmgrState(t *testing.T) {
	sampleOutput := `BootCurrent: 0001
Timeout: 1 seconds
BootOrder: 0001,0002,0004
Boot0001* UEFI: PXE IP4 Intel(R) I350 Gigabit Network Connection
Boot0002* UEFI: PXE IP6 Intel(R) I350 Gigabit Network Connection
Boot0003* RedWolf (almalinux9)
Boot0004* UEFI: Built-in EFI Shell
`
	entryID, bootOrder := parseBootmgrState(sampleOutput, "RedWolf")
	if entryID != "0003" {
		t.Fatalf("expected entryID '0003', got %q", entryID)
	}
	expectedOrder := []string{"0001", "0002", "0004"}
	if len(bootOrder) != len(expectedOrder) {
		t.Fatalf("expected %d boot order entries, got %d", len(expectedOrder), len(bootOrder))
	}
	for i, v := range expectedOrder {
		if bootOrder[i] != v {
			t.Fatalf("expected bootOrder[%d] == %s, got %s", i, v, bootOrder[i])
		}
	}
}

func TestReorderBootOrder(t *testing.T) {
	currentOrder := []string{"0001", "0002", "0003", "0004"}
	newOrder := reorderBootOrder("0003", currentOrder)
	if newOrder != "0003,0001,0002,0004" {
		t.Fatalf("expected '0003,0001,0002,0004', got %q", newOrder)
	}

	// When target is not yet in BootOrder
	notInOrder := []string{"0001", "0002"}
	newOrder2 := reorderBootOrder("0005", notInOrder)
	if newOrder2 != "0005,0001,0002" {
		t.Fatalf("expected '0005,0001,0002', got %q", newOrder2)
	}

	// When BootOrder is empty
	newOrder3 := reorderBootOrder("0001", nil)
	if newOrder3 != "0001" {
		t.Fatalf("expected '0001', got %q", newOrder3)
	}
}

