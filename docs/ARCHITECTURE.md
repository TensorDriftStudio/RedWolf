# Architektura Systemu RedWolf

## 1. Przegląd Komponentów

System **RedWolf** został zaprojektowany w architekturze modułowej:

```text
+-----------------------------------------------------------------------------------+
|                                  REDWOLF CORE                                     |
|                                                                                   |
|  +--------------------+   +---------------------+   +--------------------------+  |
|  |    DHCP Service    |   |     TFTP Service    |   |     HTTP Boot / Assets   |  |
|  | (Provisioning Net) |   |    (iPXE Kernel)    |   | (vmlinuz, initramfs, OS) |  |
|  +--------------------+   +---------------------+   +--------------------------+  |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  |                               Core API Engine                               |  |
|  |  - Inwentaryzacja i Baza Węzłów                                             |  |
|  |  - FSM Maszyny Stanów (Discovery -> Ready -> Provisioning -> Active)       |  |
|  |  - Generator Cloud-Init (user-data, meta-data, network-config)              |  |
|  +-----------------------------------------------------------------------------+  |
+----------------------------------------|------------------------------------------+
                                         |
                       +-----------------+-----------------+
                       |                                   |
                       v                                   v
        +----------------------------+      +----------------------------+
        |        REDWOLF UI          |      |     REDWOLF DISCOVERY      |
        |      (Web Dashboard)       |      |     (In-Memory Agent)      |
        | - Podgląd maszyn i stanu   |      | - dmidecode, lscpu, ip     |
        | - Wizualizacja specyfikacji|      | - ipmitool KCS (iDRAC conf)|
        | - Kreator instalacji OS    |      | - Raportowanie JSON        |
        +----------------------------+      +----------------------------+
```

---

## 2. Podział Odpowiedzialności

### 2.1 `redwolf-core`
- **DHCP/TFTP Orchestrator:** Zarządza podsiecią provisioningową. Odpowiada wyłącznie na żądania z autoryzowanych portów lub automatycznie rejestruje nowe urządzenia w trybie auto-discovery.
- **State Machine (FSM):**
  - `DISCOVERED`: Wykryto nowe urządzenie w sieci PXE.
  - `COLLECTING_TELEMETRY`: Agent RAM zbiera dane sprzętowe i konfiguruje BMC.
  - `READY_FOR_PROVISIONING`: Sprzęt zidentyfikowany, czeka na decyzję administratora w GUI.
  - `PROVISIONING`: Trwa partycjonowanie i instalacja obrazu OS.
  - `ACTIVE`: Serwer zainstalowany, zweryfikowany, oddany do produkcji.
  - `ERROR`: Wystąpił błąd podczas instalacji lub konfiguracji.

### 2.2 `redwolf-discovery`
- Lekki ramdisk (bazujący na minimalnym kernelu Linux + Alpine / Busybox).
- Narzędzia: `ipmitool`, `dmidecode`, `ethtool`, `util-linux`, `curl`, `jq`.
- Skrypt agenta (`redwolf-agent`):
  1. Załadowanie modułów IPMI (`modprobe ipmi_si`, `modprobe ipmi_devintf`).
  2. Pobranie metryk sprzętowych.
  3. Konfiguracja loginu i hasła do iDRAC/BMC.
  4. Wymuszenie trybu DHCP na interfejsie BMC oraz odczyt przydzielonego IP.
  5. Wysłanie danych do Core API i przejście w tryb nasłuchiwania na sygnał startu instalacji.

### 2.3 `redwolf-ui`
- Interfejs graficzny oparty o nowoczesne standardy webowe.
- Spójna identyfikacja wizualna z logotypami RedWolf (ciemny motyw jako domyślny dla środowisk NOC/DC).
- Widok listy urządzeń (Device Matrix) z podziałem na statusy.
- Szczegółowa karta serwera:
  - Specyfikacja CPU, RAM, dysków, kart sieciowych.
  - Adres IP BMC z bezpośrednim łączem do interfejsu webowego iDRAC.
  - Formularz konfiguracji wdrożenia (wybór AlmaLinux 8/9/10 lub Debian 12/13).
