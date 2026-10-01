# AGENTS.md - Instrukcje i kontekst projektu RedWolf dla AI

Ten plik stanowi główne źródło wiedzy architektonicznej i kontekstu projektowego dla modeli AI asystujących przy rozwoju projektu **RedWolf**.

---

## 🐺 Czym jest RedWolf?

**RedWolf** to nowoczesne, otwarte (open source) narzędzie do automatycznego provisioningu serwerów fizycznych (**bare metal**) oraz maszyn wirtualnych (**VM**), zaprojektowane z myślą o prostocie wdrożenia w szafach serwerowych i centrach danych.

### Główny use-case (Workflow technika w Data Center):
1. **Montaż fizyczny:** Technik wsuwa serwer (np. platformę referencyjną **Dell PowerEdge R640**) do szafy rack i podłącza:
   - Zasilanie (PSU1 / PSU2)
   - Dedykowany port BMC / iDRAC do sieci zarządzania
   - Pierwszą kartę sieciową (NIC1 / eth0 / onboard 1GbE/10GbE) do sieci provisioningowej
2. **Boot PXE:** Technik uruchamia serwer z opcją bootowania sieciowego PXE (lub serwer bootuje z PXE domyślnie przy braku systemu na dyskach).
3. **Akcja RedWolf (Autonomiczne przejęcie):**
   - RedWolf zarządza zintegrowanymi usługami **DHCP** i **TFTP** w dedykowanej sieci provisioningowej.
   - Po starcie PXE serwer pobiera i uruchamia lekki obraz micro-OS (RedWolf Discovery Agent / ramdisk).
   - Obraz automatycznie:
     - **Zbiera inwentarz sprzętowy:** model platformy/obudowy (np. Dell PowerEdge R640), procesor (model, rdzenie, wątki), pamięć RAM (pojemność, konfiguracja kanałów/banków), adresy MAC wszystkich interfejsów sieciowych.
     - **Automatyzuje konfigurację BMC / iDRAC:** za pośrednictwem lokalnego interfejsu KCS / `ipmitool` / OpenIPMI / Redfish konfiguruje bezpieczny login i hasło administratora BMC, wymusza tryb DHCP na interfejsie BMC oraz pobiera przydzielony adres IP BMC.
4. **Prezentacja w GUI:**
   - Agent discovery odsyła pełen raport telemetryczny przez API do RedWolf Core.
   - Serwer pojawia się w GUI RedWolf ze statusem **"Gotowy do provisioningu"** (*Ready for Provisioning*).
5. **Instalacja systemu (Cloud-Init):**
   - Użytkownik w GUI wybiera system operacyjny:
     - **AlmaLinux 8, 9, 10**
     - **Debian 12, 13**
   - Użytkownik konfiguruje parametry wdrożenia:
     - Schemat partycjonowania dysków (LVM, RAID software, dyski NVMe/SSD/HDD)
     - Hasło root oraz opcjonalne klucze SSH
     - Docelową konfigurację sieciową (IP statyczne/DHCP, bonding LACP, VLANy, bramę, serwery DNS)
   - RedWolf generuje metadane `cloud-init` / kickstart / preseed, instruuje węzeł do instalacji, a po restarcie serwer jest w pełni skonfigurowany i produkcyjny.

---

## 🏛️ Architektura Systemu RedWolf

Projekt składa się z następujących modułów:

1. **`redwolf-core` (Backend & Orchestrator):**
   - Zarządza stanem serwerów (FSM: `DISCOVERED` -> `READY_FOR_PROVISIONING` -> `PROVISIONING` -> `INSTALLED` -> `FAILED`).
   - Wbudowane lub zarządzane usługi sieciowe:
     - **DHCP Server:** przydzielanie adresów IP w podsieci provisioningowej oraz opcji PXE (Next-Server, Bootfile Name dla iPXE / GRUB2).
     - **TFTP / HTTP Boot Server:** serwowanie loaderów iPXE, kernela i ramdysku agenta discovery.
   - REST / gRPC / WebSocket API dla agenta discovery i interfejsu graficznego (GUI).
   - Silnik szablonowania instalatorów i Cloud-Init (`user-data`, `meta-data`, `network-config`).

2. **`redwolf-discovery` (Discovery Agent & Live Boot Image):**
   - Minimalny obraz Linuksa uruchamiany w RAM (initramfs oparty o Alpine Linux lub minimalny Busybox/Buildroot/Debian kernel).
   - Skrypt/demon discovery zbierający dane przez `dmidecode`, `lshw`, `/proc/cpuinfo`, `sysfs`, `ip link`.
   - Moduł komunikacji z BMC (moduł jądra `ipmi_si`, `ipmi_devintf`, narzędzie `ipmitool` / Redfish API).
   - Komunikacja zwrotna z `redwolf-core` via HTTP POST (JSON payload).

3. **`redwolf-ui` (Web GUI Dashboard):**
   - Interaktywny, nowoczesny pulpit nawigacyjny (SPA).
   - Real-time aktualizacja wykrytych maszyn w szafie.
   - Kreator instalacji (Wizard): wybór OS (AlmaLinux 8/9/10, Debian 12/13), konfiguracja dysków, hasła root, sieci.
   - Wizualizacja parametrów CPU, RAM, dysków, kart sieciowych i iDRAC.

4. **`redwolf-profiles` & `templates`:**
   - Szablony i skrypty kickstart / preseed / cloud-init.
   - Profile sprzętowe (np. reguły specyficzne dla Dell PowerEdge R640 / iDRAC 9).

---

## 🛠️ Wytyczne Techniczne dla Asystentów AI

Podczas pisania kodu i modyfikowania projektu trzymaj się następujących reguł:

1. **Obsługa Dell PowerEdge i BMC:**
   - Węzły bazowe mogą nie mieć skonfigurowanego adresu IP w BMC przy pierwszym uruchomieniu, dlatego konfiguracja loginu, hasła i trybu DHCP w BMC odbywa się z poziomu systemu operacyjnego załadowanego przez PXE za pomocą interfejsu KCS (Keyboard Controller Style) i protokołu IPMI (`ipmitool lan set ...`, `ipmitool user set ...`).
   - Zachowaj idempotentność skryptów konfiguracyjnych BMC.

2. **Niezawodność PXE & Cloud-Init:**
   - Każde wdrożenie musi być powtarzalne.
   - Cloud-init wymaga prawidłowej struktury plików: `user-data`, `meta-data`, `network-config` (wersja 2).
   - Generowane hasła root muszą być bezpiecznie hashowane (np. SHA-512 crypt `$6$`).

3. **Interfejs Użytkownika:**
   - Estetyka klasy enterprise: ciemny/jasny motyw, przejrzyste tabele urządzeń, wskaźniki statusu LED/aktywności, responsywność.
   - Zawsze używaj oficjalnego wektorowego logo RedWolf z katalogu `assets/logo/`.

4. **Czystość kodu:**
   - Modułowa architektura, jasny podział na backend, discovery agent i frontend.
   - Kompletna dokumentacja API i procedur uruchomieniowych.
