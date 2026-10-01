# Specyfikacja Techniczna Projektu RedWolf

## 1. Wstęp i Zakres Systemu

**RedWolf** to system automatyzacji wdrożeń i inwentaryzacji dedykowany dla środowisk Data Center, platform bare metal (ze szczególnym uwzględnieniem **Dell PowerEdge R640**) oraz maszyn wirtualnych.

Niniejszy dokument opisuje szczegółowe wymagania funkcjonalne, protokoły komunikacyjne, schematy danych i architekturę procesów.

---

## 2. Topologia Sieciowa i Faza Montażu

### 2.1 Podłączenia fizyczne (Platforma Dell PowerEdge R640)
W szafie serwerowej technik realizuje podłączenie fizyczne serwera:
1. **Zasilanie:** PSU1 + PSU2 (redundantne zasilanie z szyn A i B).
2. **Port BMC / iDRAC:** Dedykowany port RJ-45 iDRAC 9 podłączony do podsieci zarządzania (Out-of-band management).
3. **Port Provisioningowy:** Port `NIC1` (pierwszy interfejs zintegrowanej karty sieciowej LOM / rNDC, np. `eno1` / `eth0`) podłączony do dedykowanego VLAN-u provisioningowego.

```text
+---------------------------------------------------------------+
|                      DELL PowerEdge R640                      |
|                                                               |
|  [PSU 1] [PSU 2]     [iDRAC RJ-45]     [NIC 1] [NIC 2] [NIC 3]|
+-----|-------|--------------|--------------|-------------------+
      |       |              |              |
   Zasilanie A/B         VLAN OOB       VLAN Provisioning
                      (Zarządzanie)      (DHCP/TFTP RedWolf)
```

---

## 3. Auto-Discovery i Boot Sieciowy (PXE / iPXE)

### 3.1 Kontrola Usług Sieciowych przez RedWolf Core
RedWolf automatycznie uruchamia i nadzoruje dwa komponenty w sieci provisioningowej:
1. **Serwer DHCP:**
   - Nasłuchuje żądań DHCP Discover na interfejsie provisioningowym.
   - Identyfikuje architekturę klienta (DHCP Option 93: x86 BIOS vs UEFI x86_64).
   - Zwraca `Next-Server` (IP RedWolf) oraz `Bootfile-Name` (`ipxe.efi` dla UEFI / `undionly.kpxe` dla BIOS).
2. **Serwer TFTP & HTTP Boot:**
   - Serwuje binarkę loader'a iPXE przez TFTP.
   - Następnie iPXE przełącza się na szybki transfer HTTP w celu pobrania jądra Linuksa (`vmlinuz`) oraz ramdysku (`initramfs.img`) agenta **RedWolf Discovery**.

### 3.2 Działanie Agenta RedWolf Discovery
Agent uruchamia się w pamięci RAM węzła bez dotykania zainstalowanych dysków twardych.

#### A. Zbieranie inwentarza sprzętowego:
- **Procesor (CPU):**
  - Wykrywany z `/proc/cpuinfo` oraz `lscpu`.
  - Model (np. `Intel(R) Xeon(R) Gold 6140 CPU @ 2.30GHz`).
  - Liczba gniazd (sockets), rdzeni fizycznych (cores) i wątków (threads).
  - Flagi wirtualizacji (VT-x) i AES-NI.
- **Model platformy i płyty głównej:**
  - Wykrywany przez `dmidecode -s system-manufacturer`, `system-product-name`, `system-serial-number`.
  - Weryfikacja: `Dell Inc. PowerEdge R640` wraz z Service Tag.
- **Pamięć RAM:**
  - Całkowity rozmiar w GiB/GB.
  - Obsadzenie banków pamięci (`dmidecode -t memory`): taktowanie, typ (DDR4 ECC Reg), liczba modułów.
- **Karty Sieciowe (NIC):**
  - Wykrywane przez `/sys/class/net/*` oraz `ip -j link`.
  - Lista wszystkich fizycznych interfejsów wraz z adresami MAC, prędkościami (1GbE / 10GbE / 25GbE) i modelem kontrolera (PCI ID).

#### B. Konfiguracja BMC / iDRAC (Lokalnie przez IPMI KCS):
- W środowisku fabrycznym lub po resecie BMC może nie posiadać adresu IP lub mieć domyślne dane logowania.
- Agent Discovery ładuje moduły jądra `ipmi_si` oraz `ipmi_devintf`.
- Przez lokalny kanał KCS (bez konieczności wcześniejszej znajomości IP czy hasła iDRAC) agent wykonuje:
  ```bash
  # 1. Konfiguracja konta administratora BMC
  ipmitool user set name 2 <REDWOLF_BMC_USER>
  ipmitool user set password 2 <REDWOLF_BMC_PASSWORD>
  ipmitool user enable 2
  ipmitool channel setaccess 1 2 callin=on ipmi=on link=on privilege=4

  # 2. Przełączenie BMC na pobieranie adresu z DHCP
  ipmitool lan set 1 ipsrc dhcp

  # 3. Oczekiwanie i odpytanie o przydzielony adres IP
  ipmitool lan print 1 | grep "IP Address"
  ```
- Odczytany adres IP iDRAC, MAC iDRAC oraz status zostają dołączone do raportu.

#### C. Wysłanie raportu telemetrycznego:
Agent wysyła pakiet JSON na endpoint API RedWolf Core: `POST /api/v1/discovery/report`.

```json
{
  "system": {
    "manufacturer": "Dell Inc.",
    "model": "PowerEdge R640",
    "serial_number": "4X9Z8Y2",
    "bios_version": "2.16.0"
  },
  "cpu": {
    "model": "Intel(R) Xeon(R) Gold 6140 CPU @ 2.30GHz",
    "sockets": 2,
    "cores_per_socket": 18,
    "threads_per_socket": 36,
    "total_threads": 72
  },
  "memory": {
    "total_bytes": 137438953472,
    "total_human": "128 GiB",
    "slots_used": 4,
    "slots_total": 24,
    "type": "DDR4 ECC Registered"
  },
  "network_interfaces": [
    {
      "name": "eno1",
      "mac": "b0:4f:13:2a:44:80",
      "speed_mbps": 10000,
      "link_detected": true,
      "pci_slot": "Embedded LOM 1"
    },
    {
      "name": "eno2",
      "mac": "b0:4f:13:2a:44:81",
      "speed_mbps": 10000,
      "link_detected": false,
      "pci_slot": "Embedded LOM 2"
    }
  ],
  "bmc": {
    "type": "iDRAC9",
    "mac": "b0:4f:13:2a:44:8e",
    "ip_address": "192.168.100.45",
    "dhcp_enabled": true,
    "credentials_updated": true
  },
  "storage_devices": [
    {
      "name": "sda",
      "size_bytes": 960197124096,
      "size_human": "960 GB",
      "type": "SSD",
      "model": "DELL BOSS-S1"
    },
    {
      "name": "sdb",
      "size_bytes": 1920383410176,
      "size_human": "1.92 TB",
      "type": "NVMe",
      "model": "Samsung PM9A3"
    }
  ]
}
```

---

## 4. Prezentacja w GUI RedWolf

Gdy węzeł prześle raport:
1. Rekord w bazie danych przechodzi w stan `READY_FOR_PROVISIONING`.
2. W panelu webowym pojawia się karta serwera z badge'em **"Gotowy do wdrożenia"**.
3. Administrator widzi:
   - Pełną specyfikację podzespołów (CPU, RAM, MACi, dyski).
   - Bezpośredni link do konsoli webowej iDRAC (`https://<BMC_IP>`).
   - Przycisk **"Rozpocznij Provisioning"** uruchamiający kreator wdrożenia.

---

## 5. Silnik Wdrożenia Systemu (Cloud-Init)

### 5.1 Wspierane Systemy Operacyjne
RedWolf wspiera instalację następujących systemów z pełną automatyzacją:
- **AlmaLinux 8**
- **AlmaLinux 9**
- **AlmaLinux 10**
- **Debian 12 (Bookworm)**
- **Debian 13 (Trixie)**

### 5.2 Formularz Konfiguracyjny w GUI:
1. **Wybór dystrybucji i wersji.**
2. **Partycjonowanie dysków:**
   - Dysk docelowy (np. `/dev/sda` lub RAID BOSS-S1).
   - Układ: Standardowy (`/boot/efi`, `/boot`, `/`, `swap`) lub zaawansowany LVM.
3. **Bezpieczeństwo i Dostęp:**
   - Hasło użytkownika `root` (haszowane algorytmem SHA-512 crypt `$6$`).
   - Lista autoryzowanych kluczy SSH dla `root` i użytkownika administracyjnego.
4. **Docelowa Konfiguracja Sieciowa:**
   - Wybór interfejsu produkcyjnego lub konfiguracji typu Bond (np. `bond0` z `eno1` + `eno2` w trybie LACP 802.3ad).
   - Tryb IP: Statyczny (Adres IP, Maska CIDR, Brama domyślna, Serwery DNS) lub DHCP.
   - Tagowanie VLAN (opcjonalne).

### 5.3 Generowanie Metadanych Cloud-Init i Instalacja
RedWolf generuje pakiet konfiguracyjny serwowany przez wbudowane API HTTP (`http://<REDWOLF_IP>/cloud-init/<MAC>/`):
- `user-data`: definicja użytkowników, kluczy SSH, hasła root, pakietów początkowych (`qemu-guest-agent`, `curl`, `htop`).
- `meta-data`: `instance-id`, `local-hostname`.
- `network-config`: standard Netplan v2 / NetworkManager / ifupdown zależnie od wybranej dystrybucji.

Instalator pobiera obraz bazowy (raw/qcow2 cloud-image), rozpakowuje go bezpośrednio na dyski docelowe węzła, aplikuje Cloud-Init, a następnie restartuje serwer do gotowego systemu produkcyjnego.
