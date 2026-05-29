# Linphon phase-one migration contract

Phase one splits bootstrap, system install, provider configuration, and runtime control while keeping the existing Psiphon behavior compatible. VPNGate is explicitly out of scope for this phase.

## Command ownership

- `install.sh` is bootstrap-only by default. It installs the `linph` command and prints the next steps. It must not install tunnel-core, config, aliases, provider state, or start slots unless the temporary `--legacy-full-install` path is used.
- `linph install` owns system/runtime artifacts: install directories, `linph`, compatibility aliases, Psiphon core/config assets, manifest, and initial provider state. It does not start slots by default.
- `linph psi set` owns Psiphon provider configuration: slot count, regions, and HTTP/SOCKS base ports.
- `linph provider get` and `linph provider set psi` own active-provider inspection and selection.
- `linph start`, `restart`, `stop`, `port`, `ctry`, `log`, `switch-port`, and `switch-ctry` operate on the active provider. In phase one the only supported provider is `psi`, so legacy behavior remains compatible.
- `linph run` and the `psiphon` alias remain Psiphon-only compatibility entrypoints.
- `plinstaller2` remains a Psiphon/system install compatibility shim. It must never silently introduce VPNGate behavior.

## Provider state

The canonical provider state is a versioned JSON document under the installed runtime root, currently `linph-profile.json`.

Required fields:

- `schema_version`
- `active_provider`
- `providers.psi`

The legacy `linph-installed-profile.json` is migrated into `providers.psi` when a managed command first needs state and no canonical state exists. The legacy file is preserved as backup/import source; once the canonical profile exists, callers must not write the legacy file directly.

If a command requires migration or repair but cannot write the canonical state, it must fail before starting, stopping, or mutating runtime state. The repair path is `linph install --repair` when a future transaction repair mode is available; phase one must at least print a clear privileged-command remediation.

## Bootstrap trust boundary

`bash <(curl ...)` is a convenience bootstrap, not a cryptographically trusted install method. The secure path is a verified release package or installer plus detached signature / signed metadata checked against a pinned public key or package-manager trust root.

Same-channel checksums are not sufficient as the only authenticity check for `linph` or runtime assets. Until release signing infrastructure exists, source/local artifact mode must be described as using local reviewed artifacts rather than pretending to provide remote supply-chain verification.

## VPNGate non-goals

Phase one must not implement VPNGate, OpenVPN, L2TP/IPsec, SoftEther, provider protocol flags, or VPNGate docs beyond noting that VPNGate is a later phase. The first VPNGate phase, when implemented later, is OpenVPN-only.
