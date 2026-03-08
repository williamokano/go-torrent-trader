package service

import "fmt"

// knownClients maps Azureus-style 2-char client codes to human-readable names.
var knownClients = map[string]string{
	"7T": "aTorrent",
	"AB": "AnyEvent::BitTorrent",
	"AG": "Ares",
	"A~": "Ares",
	"AR": "Arctic",
	"AT": "Artemis",
	"AV": "Avicora",
	"AX": "BitPump",
	"AZ": "Azureus/Vuze",
	"BB": "BitBuddy",
	"BC": "BitComet",
	"BE": "Baretorrent",
	"BF": "Bitflu",
	"BG": "BTG",
	"BL": "BitCometLite",
	"BP": "BitTorrent Pro",
	"BR": "BitRocket",
	"BS": "BTSlave",
	"BT": "mainline BitTorrent",
	"Bt": "Bt",
	"BW": "BitWombat",
	"BX": "Bittorrent X",
	"CD": "Enhanced CTorrent",
	"CT": "CTorrent",
	"DE": "Deluge",
	"DP": "Propagate Data Client",
	"EB": "EBit",
	"ES": "electric sheep",
	"FC": "FileCroc",
	"FD": "Free Download Manager",
	"FT": "FoxTorrent",
	"FX": "Freebox BitTorrent",
	"GS": "GSTorrent",
	"HK": "Hekate",
	"HL": "Halite",
	"HM": "hMule",
	"HN": "Hydranode",
	"IL": "iLivid",
	"JS": "Justseed.it",
	"JT": "JavaTorrent",
	"KG": "KGet",
	"KT": "KTorrent",
	"LC": "LeechCraft",
	"LH": "LH-ABC",
	"LP": "Lphant",
	"LT": "libtorrent",
	"lt": "libTorrent (Rakshasa)",
	"LW": "LimeWire",
	"MK": "Meerkat",
	"MO": "MonoTorrent",
	"MP": "MooPolice",
	"MR": "Miro",
	"MT": "MoonlightTorrent",
	"NB": "Net::BitTorrent",
	"NX": "Net Transport",
	"OS": "OneSwarm",
	"OT": "OmegaTorrent",
	"PB": "Protocol::BitTorrent",
	"PD": "Pando",
	"PI": "PicoTorrent",
	"PT": "PHPTracker",
	"qB": "qBittorrent",
	"QD": "QQDownload",
	"QT": "Qt 4 Torrent example",
	"RT": "Retriever",
	"RZ": "RezTorrent",
	"S~": "Shareaza alpha/beta",
	"SB": "Swiftbit",
	"SD": "Thunder (Xunlei)",
	"SM": "SoMud",
	"SP": "BitSpirit",
	"SS": "SwarmScope",
	"ST": "SymTorrent",
	"st": "sharktorrent",
	"SZ": "Shareaza",
	"TB": "Torch",
	"TE": "terasaur Seed Bank",
	"TL": "Tribler",
	"TN": "TorrentDotNET",
	"TR": "Transmission",
	"TS": "Torrentstorm",
	"TT": "TuoTu",
	"UL": "uLeecher!",
	"UM": "uTorrent for Mac",
	"UT": "uTorrent",
	"UW": "uTorrent Web",
	"VG": "Vagaa",
	"WD": "WebTorrent Desktop",
	"WT": "BitLet",
	"WW": "WebTorrent",
	"WY": "FireTorrent",
	"XF": "Xfplay",
	"XL": "Xunlei",
	"XS": "XSwifter",
	"XT": "XanTorrent",
	"XX": "Xtorrent",
	"ZT": "ZipTorrent",
}

// ParsePeerIDClient extracts the BitTorrent client name and version from a
// 20-byte peer_id. It recognises the Azureus-style encoding (-XX1234-) and
// falls back to a prefix-based unknown label for anything else.
func ParsePeerIDClient(peerID []byte) string {
	if len(peerID) >= 8 && peerID[0] == '-' && peerID[7] == '-' {
		code := string(peerID[1:3])
		name, ok := knownClients[code]
		if !ok {
			return fmt.Sprintf("Unknown (%s)", code)
		}
		v0 := peerID[3]
		v1 := peerID[4]
		v2 := peerID[5]
		v3 := peerID[6]
		return fmt.Sprintf("%s %c.%c.%c.%c", name, v0, v1, v2, v3)
	}

	// Non-Azureus peer ID — show a short prefix for identification.
	if len(peerID) >= 8 {
		return fmt.Sprintf("Unknown (%s)", string(peerID[:8]))
	}
	if len(peerID) > 0 {
		return fmt.Sprintf("Unknown (%s)", string(peerID))
	}
	return "Unknown"
}
