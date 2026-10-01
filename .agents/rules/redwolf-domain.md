# RedWolf Domain Rules

## Project Objective
RedWolf automates preparation, hardware inventory discovery, and operating system provisioning for bare-metal servers (specifically Dell PowerEdge R640 as reference) and virtual machines.

## Standard Deployment Workflow
1. Physical Server Connection:
   - Redundant Power Supplies
   - iDRAC / BMC dedicated management port
   - First network adapter (NIC 1) connected to the provisioning network
2. Node boots via PXE from the provisioning network.
3. DHCP and TFTP/HTTP services managed by RedWolf deliver the in-memory Discovery Agent:
   - Hardware telemetry extraction: CPU model/cores/threads, platform model/chassis (DMI/SMBIOS), total RAM, network interface MAC addresses, storage devices.
   - BMC Configuration: Set secure credentials, enable DHCP mode on the BMC interface, retrieve assigned BMC IP.
4. RedWolf Web GUI updates and marks node status as "Ready for Provisioning".
5. Target OS deployment:
   - Supported distributions: AlmaLinux (8, 9, 10), Debian (12, 13).
   - Deployment mechanism: Cloud-Init (storage partitioning, root password, target network configuration).
