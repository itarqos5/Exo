package main

// Signature is a known cheat-client fingerprint. We match it three ways:
//   - against the jar's file name
//   - against entry (package) paths inside the jar
//   - against log file contents
// All matches are case-insensitive substring matches on lowercased input.
type Signature struct {
	Name     string   // human-readable client name
	Package  []string // distinctive package / class path fragments inside jars
	FileHint []string // distinctive file-name fragments
	LogHint  []string // distinctive strings that show up in logs at launch
}

// cheatSignatures lists publicly-known Minecraft cheat clients. These names are
// common knowledge in the anti-cheat / screenshare community. Matching flags a
// mod for a human to review — it is not proof by itself.
var cheatSignatures = []Signature{
	// --- Major / well-known ---
	{Name: "Wurst", Package: []string{"net/wurstclient", "wurstclient"}, FileHint: []string{"wurst"}, LogHint: []string{"wurstclient", "wurst client"}},
	{Name: "Meteor Client", Package: []string{"meteordevelopment/meteorclient", "minegame159/meteorclient"}, FileHint: []string{"meteor-client", "meteorclient"}, LogHint: []string{"meteor client"}},
	{Name: "Impact", Package: []string{"com/github/impactdevelopment"}, FileHint: []string{"impactclient", "impact-client"}, LogHint: []string{"impact client"}},
	{Name: "Aristois", Package: []string{"me/kaimn/aristois", "aristois"}, FileHint: []string{"aristois"}, LogHint: []string{"aristois"}},
	{Name: "LiquidBounce", Package: []string{"net/ccbluex/liquidbounce", "ccbluex"}, FileHint: []string{"liquidbounce"}, LogHint: []string{"liquidbounce"}},
	{Name: "Sigma", Package: []string{"sigmaclient", "com/sigma"}, FileHint: []string{"sigma"}, LogHint: []string{"sigma client"}},
	{Name: "Future", Package: []string{"future/client"}, FileHint: []string{"future"}, LogHint: []string{"future client"}},
	{Name: "Vape", Package: []string{"vape/", "me/vape"}, FileHint: []string{"vape"}, LogHint: []string{"vape client", "vapelite", "vape v4", "vape lite"}},
	{Name: "Novoline", Package: []string{"novoline"}, FileHint: []string{"novoline"}, LogHint: []string{"novoline"}},
	{Name: "Rise", Package: []string{"rise/client"}, FileHint: []string{"rise"}, LogHint: []string{"rise client"}},
	{Name: "Inertia", Package: []string{"inertia/client", "me/alpha/inertia"}, FileHint: []string{"inertia"}, LogHint: []string{"inertia client"}},
	{Name: "Konas", Package: []string{"me/konas", "konas/"}, FileHint: []string{"konas"}, LogHint: []string{"konas"}},
	{Name: "Baritone (automation)", Package: []string{"baritone/"}, FileHint: []string{"baritone"}, LogHint: []string{"baritone"}},

	// --- Niche / community clients ---
	{Name: "Prestige", Package: []string{"prestige/client", "me/prestige", "dev/prestige"}, FileHint: []string{"prestige"}, LogHint: []string{"prestige client"}},
	{Name: "Moon", Package: []string{"moonclient", "moon/client"}, FileHint: []string{"moonclient"}, LogHint: []string{"moon client"}},
	{Name: "Entropy", Package: []string{"entropy/client"}, FileHint: []string{"entropy"}, LogHint: []string{"entropy client"}},
	{Name: "Pyro", Package: []string{"pyro/client"}, FileHint: []string{"pyroclient"}, LogHint: []string{"pyro client"}},
	{Name: "Rusing / Ruse", Package: []string{"rusherhack", "ruse/client"}, FileHint: []string{"rusherhack", "ruse"}, LogHint: []string{"rusherhack", "ruse client"}},
	{Name: "Expensive", Package: []string{"expensive/client", "wtf/expensive"}, FileHint: []string{"expensive"}, LogHint: []string{"expensive"}},
	{Name: "Doomsday", Package: []string{"doomsday/client"}, FileHint: []string{"doomsday"}, LogHint: []string{"doomsday client"}},
	{Name: "Tenacity", Package: []string{"tenacity/client"}, FileHint: []string{"tenacity"}, LogHint: []string{"tenacity client"}},
	{Name: "Raven", Package: []string{"keystrokesmod", "raven/client"}, FileHint: []string{"raven-b", "ravenb"}, LogHint: []string{"raven client", "raven b"}},
	{Name: "Astral", Package: []string{"astral/client"}, FileHint: []string{"astralclient", "astral-"}, LogHint: []string{"astral client"}},
	{Name: "Nurik", Package: []string{"nurik/client"}, FileHint: []string{"nurik"}, LogHint: []string{"nurik"}},
	{Name: "Zeton", Package: []string{"zeton/client"}, FileHint: []string{"zeton"}, LogHint: []string{"zeton"}},
	{Name: "Nova", Package: []string{"nova/client", "wtf/nova"}, FileHint: []string{"novaclient", "nova-client"}, LogHint: []string{"nova client"}},
	{Name: "Augustus", Package: []string{"augustus/client"}, FileHint: []string{"augustus"}, LogHint: []string{"augustus client"}},
	{Name: "Exhibition", Package: []string{"exhibition/client"}, FileHint: []string{"exhibition"}, LogHint: []string{"exhibition client"}},
	{Name: "Slinky", Package: []string{"slinky/client"}, FileHint: []string{"slinky"}, LogHint: []string{"slinky client"}},
	{Name: "Bedless (Bedwars)", Package: []string{"bedless/client"}, FileHint: []string{"bedlessclient", "bedless-"}, LogHint: []string{"bedless client"}},
	{Name: "Sekai", Package: []string{"sekai/client"}, FileHint: []string{"sekaiclient"}, LogHint: []string{"sekai client"}},
	{Name: "Rapid", Package: []string{"rapid/client"}, FileHint: []string{"rapidclient"}, LogHint: []string{"rapid client"}},
	{Name: "Chief / ChiefKeef", Package: []string{"chief/client"}, FileHint: []string{"chiefclient", "chiefkeef"}, LogHint: []string{"chief client"}},
	{Name: "Nighthawk", Package: []string{"nighthawk/client"}, FileHint: []string{"nighthawk"}, LogHint: []string{"nighthawk"}},
	{Name: "Nebula", Package: []string{"nebula/client"}, FileHint: []string{"nebulaclient"}, LogHint: []string{"nebula client"}},
	{Name: "Ares", Package: []string{"ares/client"}, FileHint: []string{"aresclient"}, LogHint: []string{"ares client"}},
	{Name: "Flux", Package: []string{"flux/client"}, FileHint: []string{"fluxclient"}, LogHint: []string{"flux client"}},
	{Name: "Spexion", Package: []string{"spexion/client"}, FileHint: []string{"spexion"}, LogHint: []string{"spexion"}},
	{Name: "Hyperium (modified)", Package: []string{"cc/hyperium/mods/autogg"}, FileHint: []string{"hyperium-cheat"}, LogHint: []string{"hyperium cheat"}},

	// --- Known cheat-utility / injection mods ---
	{Name: "AutoClicker mod", Package: []string{"autoclicker/", "me/autoclicker"}, FileHint: []string{"autoclicker"}, LogHint: []string{"autoclicker enabled"}},
	{Name: "Reach/Hitbox mod", Package: []string{"reachmod", "hitbox/expander"}, FileHint: []string{"reachmod", "hitboxexpander"}, LogHint: []string{"reach extended"}},
	{Name: "X-Ray mod", Package: []string{"xray/", "me/xray"}, FileHint: []string{"xray", "x-ray"}, LogHint: []string{"xray enabled"}},
	{Name: "Generic injection hint", Package: []string{"cheat/", "hack/client", "hacks/module"}, FileHint: []string{"hackclient", "cheatclient"}, LogHint: []string{"injecting into minecraft", "bypass detected", "hack module"}},
}
