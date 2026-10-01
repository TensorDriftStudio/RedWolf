# Reguły Domenowe Projektu RedWolf

## Cel projektu
RedWolf automatyzuje proces przygotowania, inwentaryzacji i instalacji serwerów fizycznych (bare metal, m.in. Dell PowerEdge R640) oraz maszyn wirtualnych.

## Standardowy Przepływ Wdrożeniowy
1. Podłączenie fizyczne serwera:
   - Zasilanie
   - Sieć iDRAC / BMC (zarządzanie out-of-band)
   - Pierwsza karta sieciowa (NIC 1) podpięta pod sieć provisioningową
2. Start przez PXE z sieci provisioningowej.
3. Usługi DHCP i TFTP zarządzane przez RedWolf dostarczają obraz agenta Discovery:
   - Zbieranie metryk: Model procesora, model platformy (DMI), pojemność RAM, adresy MAC kart sieciowych.
   - Konfiguracja BMC: Ustawienie loginu i hasła, przełączenie w tryb DHCP, odczyt pobranego adresu IP.
4. Aktualizacja GUI RedWolf i oznaczenie serwera jako gotowy do instalacji ("Ready for provisioning").
5. Wdrożenie wybranego systemu:
   - Obsługiwane dystrybucje: AlmaLinux (8, 9, 10), Debian (12, 13).
   - Metoda instalacji: Cloud-Init (partycjonowanie dysków, hasło root, docelowa konfiguracja sieciowa).
