================================================================
 EXO  -  Minecraft Mod & Log Integrity Scanner
================================================================

Exo is a lightweight, dependency-free command-line tool that scans
the Minecraft installations on a PC and reports mods that look like
known cheat clients, have been tampered with, or cannot be verified.

It is the kind of tool server staff use during a "screenshare" check.


----------------------------------------------------------------
 WHAT IT DOES
----------------------------------------------------------------

  * Finds mod folders across every common launcher, even when the
    launcher has been relocated to another drive via junctions:
      - Default .minecraft
      - OneClient (clusters)
      - Modrinth App / Theseus
      - Prism Launcher, ElyPrism, Fjord, Freesm, PolyMC, UltimMC,
        MultiMC
      - CurseForge (current + legacy)
      - GDLauncher / GDLauncher Carbon
      - ATLauncher, Technic, FTB App, XMCL, HMCL
      - Lunar, Feather, Badlion, LabyMod, Salwyrr, TLauncher,
        SKLauncher

  * Sweeps every local drive (fixed and removable) for launcher
    folders installed outside AppData, e.g. D:\Minecraft\OneClient
    or G:\Games\PrismLauncher. Only folders that actually look like
    a Minecraft install are deep-scanned, so the sweep stays fast.

  * Inspects jars in parallel with 4 worker threads, with a live
    dashboard showing what each worker is doing.

  * For every .jar it finds:
      - Computes SHA-1 and SHA-512 hashes.
      - Verifies the hash against the Modrinth database. A match
        means the file is an unmodified, published mod.
      - Inspects the jar's internal package paths and file name
        against a database of known cheat clients
        (Wurst, Meteor, Impact, LiquidBounce, Prestige, Vape,
        Sigma, Future, and many more).
      - Flags heavy obfuscation and jars that contain class files
        but no mod metadata (a common sign of an injected client).

  * Scans Minecraft logs for cheat-client startup markers.

  * Writes a full, shareable report to  result.txt  in the folder
    the program was run from.


----------------------------------------------------------------
 HOW TO USE
----------------------------------------------------------------

  1. Download  exo-v1.0.0.exe  from the Releases page.
  2. Run it (double-click, or run from a terminal).
  3. Wait for the scan to finish.
  4. Read the on-screen summary, or open  result.txt  next to the
     program for the full report.

  No installation and no admin rights are required - everything it
  reads lives inside the current user's profile.


----------------------------------------------------------------
 READING THE RESULT
----------------------------------------------------------------

  VERIFIED  - hash matched a published file on Modrinth.
  UNKNOWN   - not found on Modrinth. Usually a CurseForge-only or
              private/custom mod. Not proof of anything by itself.
  WARN      - obfuscated, or contains code but no mod metadata.
  FLAGGED   - matched a known cheat-client signature.

  Overall verdicts:
    CLEAN         - all mods verified, nothing suspicious.
    INCONCLUSIVE  - no known cheats, but unverifiable/obfuscated
                    mods are present.
    SUSPICIOUS    - a known cheat signature was matched.


----------------------------------------------------------------
 IMPORTANT - WHAT THIS TOOL CANNOT DO
----------------------------------------------------------------

  Exo detects KNOWN cheat clients, tampered or unpublished mods,
  and heavy obfuscation. It does NOT read process memory.

  Private, renamed, custom-obfuscated or self-unloading cheats can
  pass this scan without being flagged. A CLEAN result is NOT a
  guarantee that no cheating occurred.

  Treat Exo's output as ONE signal in a manual review, never as
  definitive proof of innocence or guilt.


----------------------------------------------------------------
 BUILDING FROM SOURCE
----------------------------------------------------------------

  Requires Go 1.21 or newer.

      go build -ldflags "-s -w" -o exo.exe ./src

  Or use the build script, which wipes dist/ and writes both
  dist\exo.exe and a versioned dist\exo-v<version>.exe:

      .\build.ps1 -Version 1.0.0

  The result is a single static .exe with no runtime dependencies.


----------------------------------------------------------------
 LICENSE
----------------------------------------------------------------

  MIT License. See the LICENSE file.
