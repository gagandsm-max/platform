# OEM / API version compatibility

- Record Redfish version, OEM product (iDRAC/iLO/CIMC/other), firmware family and schema OEM extensions.
- Feature support is dimensional: inventory, sensors, storage, firmware inventory, eventing.
- Unsupported version ⇒ explicit UNSUPPORTED/UNVERIFIED—not guessed mappings.
- Partial OEM extension failure must degrade subsystem evidence, not invent values.
- Listed integration target ≠ certified Capability Pack dimension.
