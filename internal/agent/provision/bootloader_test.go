package provision

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestFindInstalledKernel_AlmaLinux(t *testing.T) {
	tempBoot := t.TempDir()

	// Create AlmaLinux kernel files including a rescue kernel
	_ = os.WriteFile(filepath.Join(tempBoot, "vmlinuz-0-rescue-abcdef"), []byte("rescue-kernel"), 0755)
	_ = os.WriteFile(filepath.Join(tempBoot, "initramfs-0-rescue-abcdef.img"), []byte("rescue-initramfs"), 0644)
	_ = os.WriteFile(filepath.Join(tempBoot, "vmlinuz-5.14.0-427.el9.x86_64"), []byte("real-kernel"), 0755)
	_ = os.WriteFile(filepath.Join(tempBoot, "initramfs-5.14.0-427.el9.x86_64.img"), []byte("real-initramfs"), 0644)

	kInfo, err := FindInstalledKernel(tempBoot)
	if err != nil {
		t.Fatalf("unexpected error finding kernel: %v", err)
	}

	if kInfo.KernelFile != "vmlinuz-5.14.0-427.el9.x86_64" {
		t.Errorf("expected kernel vmlinuz-5.14.0-427.el9.x86_64, got %s", kInfo.KernelFile)
	}
	if kInfo.InitrdFile != "initramfs-5.14.0-427.el9.x86_64.img" {
		t.Errorf("expected initramfs-5.14.0-427.el9.x86_64.img, got %s", kInfo.InitrdFile)
	}
}

func TestFindInstalledKernel_Debian(t *testing.T) {
	tempBoot := t.TempDir()

	_ = os.WriteFile(filepath.Join(tempBoot, "vmlinuz-6.1.0-28-amd64"), []byte("debian-kernel"), 0755)
	_ = os.WriteFile(filepath.Join(tempBoot, "initrd.img-6.1.0-28-amd64"), []byte("debian-initrd"), 0644)

	kInfo, err := FindInstalledKernel(tempBoot)
	if err != nil {
		t.Fatalf("unexpected error finding debian kernel: %v", err)
	}

	if kInfo.KernelFile != "vmlinuz-6.1.0-28-amd64" {
		t.Errorf("expected vmlinuz-6.1.0-28-amd64, got %s", kInfo.KernelFile)
	}
	if kInfo.InitrdFile != "initrd.img-6.1.0-28-amd64" {
		t.Errorf("expected initrd.img-6.1.0-28-amd64, got %s", kInfo.InitrdFile)
	}
}

func TestComputeKernelArgs(t *testing.T) {
	// 1. LVM on Software RAID 1
	layoutRAIDLVM := &StorageLayoutResult{
		IsLVM:          true,
		IsSoftwareRAID: true,
		RootPartition:  "/dev/mapper/vg_system-root",
	}
	args := ComputeKernelArgs(layoutRAIDLVM, "")
	if !strings.Contains(args, "root=/dev/mapper/vg_system-root") {
		t.Errorf("expected root=/dev/mapper/vg_system-root in args, got %s", args)
	}
	if !strings.Contains(args, "rd.lvm=1") {
		t.Errorf("expected rd.lvm=1 in args, got %s", args)
	}
	if !strings.Contains(args, "rd.auto=1") {
		t.Errorf("expected rd.auto=1 in args, got %s", args)
	}
	if !strings.Contains(args, "rd.md=1") {
		t.Errorf("expected rd.md=1 in args, got %s", args)
	}
	if !strings.Contains(args, "plymouth.enable=0") {
		t.Errorf("expected plymouth.enable=0 in args, got %s", args)
	}

	// 2. Software RAID 1 without LVM
	layoutRAID := &StorageLayoutResult{
		IsSoftwareRAID: true,
		RootPartition:  "/dev/md1",
	}
	argsRAID := ComputeKernelArgs(layoutRAID, "1234-abcd")
	if !strings.Contains(argsRAID, "root=UUID=1234-abcd") {
		t.Errorf("expected root=UUID=1234-abcd in args, got %s", argsRAID)
	}
	if !strings.Contains(argsRAID, "rd.auto=1") {
		t.Errorf("expected rd.auto=1 in args, got %s", argsRAID)
	}
	if !strings.Contains(argsRAID, "rd.md=1") {
		t.Errorf("expected rd.md=1 in args, got %s", argsRAID)
	}
}

func TestGenerateUniversalGrubConfig(t *testing.T) {
	targetRoot := t.TempDir()
	bootDir := filepath.Join(targetRoot, "boot")
	_ = os.MkdirAll(bootDir, 0755)

	// Create kernel and EFI hierarchy
	_ = os.WriteFile(filepath.Join(bootDir, "vmlinuz-5.14.0-427.el9.x86_64"), []byte("kernel"), 0755)
	_ = os.WriteFile(filepath.Join(bootDir, "initramfs-5.14.0-427.el9.x86_64.img"), []byte("initramfs"), 0644)

	efiDir := filepath.Join(bootDir, "efi", "EFI", "almalinux")
	_ = os.MkdirAll(efiDir, 0755)

	layout := &StorageLayoutResult{
		IsLVM:          true,
		IsSoftwareRAID: true,
		RootPartition:  "/dev/mapper/vg_system-root",
		BootPartition:  "/dev/md0",
	}

	err := GenerateUniversalGrubConfig(context.Background(), targetRoot, layout, "almalinux9")
	if err != nil {
		t.Fatalf("unexpected error generating grub config: %v", err)
	}

	// 1. Verify /boot/grub/grub.cfg (BIOS location)
	biosCfgPath := filepath.Join(bootDir, "grub", "grub.cfg")
	biosData, err := os.ReadFile(biosCfgPath)
	if err != nil {
		t.Fatalf("failed reading BIOS grub.cfg: %v", err)
	}
	biosContent := string(biosData)
	if !strings.Contains(biosContent, `set default="0"`) {
		t.Errorf("expected set default=0 in grub.cfg")
	}
	if !strings.Contains(biosContent, "menuentry \"RedWolf (almalinux9 - Direct Boot)\"") {
		t.Errorf("expected direct menuentry in grub.cfg")
	}
	if !strings.Contains(biosContent, "linux /vmlinuz-5.14.0-427.el9.x86_64") {
		t.Errorf("expected linux line with kernel file in grub.cfg")
	}
	if !strings.Contains(biosContent, "initrd /initramfs-5.14.0-427.el9.x86_64.img") {
		t.Errorf("expected initrd line in grub.cfg")
	}

	// 2. Verify /boot/grub2/grub.cfg (RHEL/AlmaLinux location)
	rhelCfgPath := filepath.Join(bootDir, "grub2", "grub.cfg")
	if _, err := os.Stat(rhelCfgPath); err != nil {
		t.Errorf("missing grub2/grub.cfg: %v", err)
	}

	// 3. Verify symlink /boot/boot -> .
	bootLink, err := os.Readlink(filepath.Join(bootDir, "boot"))
	if err != nil || bootLink != "." {
		t.Errorf("expected symlink /boot/boot -> '.', got link: %s (err: %v)", bootLink, err)
	}

	// 4. Verify EFI stub /boot/efi/EFI/almalinux/grub.cfg
	efiData, err := os.ReadFile(filepath.Join(efiDir, "grub.cfg"))
	if err != nil {
		t.Fatalf("failed reading EFI grub.cfg: %v", err)
	}
	efiContent := string(efiData)
	if !strings.Contains(efiContent, "configfile ($dev)/grub2/grub.cfg") {
		t.Errorf("expected configfile chainload in EFI grub.cfg, got: %s", efiContent)
	}
	// Verify no broken `if [ ! -f ... ]` syntax
	if strings.Contains(efiContent, "test ! -f") || strings.Contains(efiContent, "[ ! -f") {
		t.Errorf("EFI grub.cfg must not contain broken '! -f' test syntax, got: %s", efiContent)
	}
}

