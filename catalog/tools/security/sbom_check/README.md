# sbom_check

Reads a software bill of materials and says whether it carries what its specification
requires, which components are poorly described, and which licences it declares. Give the SBOM
as a JSON object (`sbom`), as JSON text (`sbom_text`) or as a relative `path`. The format is
recognised by its keys: `bomFormat`/`specVersion` for CycloneDX, `spdxVersion` for SPDX.

## CycloneDX 1.4, 1.5, 1.6

Errors: `bomFormat` not `CycloneDX`; `specVersion` missing or not 1.4–1.6; `serialNumber` not an
RFC 4122 `urn:uuid:`; `version` not an integer ≥ 1; a component without `type` or `name`, or
with a `type` that its spec version does not allow (`platform`, `device-driver`,
`machine-learning-model` and `data` arrived in 1.5, `cryptographic-asset` in 1.6); a
duplicate `bom-ref`; an invalid purl; a `licenses` item without exactly one of `license` or
`expression`, a `license` without exactly one of `id` or `name`, an `id` that is not on the SPDX
License List, an expression that is not alone in its array or does not parse; a hash without a
known `alg`; a `dependencies` entry whose `ref` or `dependsOn` is not a known `bom-ref`.
Warnings: no `serialNumber`, no `metadata.timestamp`, no author, tool or supplier, no
`metadata.component`, an SPDX id written in `name`. Nested `components` are walked.

## SPDX 2.2 and 2.3

Errors (clause numbers of the 2.3 specification): `spdxVersion` (6.1), `dataLicense` must be
`CC0-1.0` (6.2), the document `SPDXID` must be `SPDXRef-DOCUMENT` (6.3), `name` (6.4),
`documentNamespace` an absolute URI without `#` (6.5), `creationInfo` with `created` as
`YYYY-MM-DDThh:mm:ssZ` and `Tool:`/`Organization:`/`Person:` `creators` (6.8, 6.9); every package
with `name`, a well-formed unique `SPDXID` and `downloadLocation` (7.1, 7.2, 7.7), and a
`packageVerificationCode` when `filesAnalyzed` is true, its default (7.9); in 2.2 only,
`licenseConcluded`, `licenseDeclared` and `copyrightText` are mandatory too; files need
`fileName`, `SPDXID` and a SHA1 checksum; licence expressions must parse and every
`LicenseRef-` must be defined in `hasExtractedLicensingInfos`; relationships must point at
existing elements; and the document must DESCRIBE something.

## Summaries

`components`: the count, and those without a version, a purl (SPDX: an `externalRefs` of type
`purl`), a licence or a supplier, plus duplicates by name and version and by purl.
`licenses`: SPDX ids counted (matched case-insensitively and written in their canonical form,
`+` kept), `LicenseRef-` ids, free-text names, exceptions after `WITH`, NOASSERTION, NONE,
the number of compound expressions, unknown ids and deprecated ids (such as `GPL-2.0`, replaced
by `GPL-2.0-only`). `ntia_minimum_elements`: whether supplier, name, version, unique
identifiers, dependency relationships, author and timestamp are present (NTIA, 2021).

The SPDX License List (3.29.0, released 2026-09-16) is bundled as `spdx_licenses.json`,
written by `tools/catalog/data/sbom_check.py` from spdx/license-list-data on 2026-10-18; the
output names its version in `license_list`.

## Tests

The CycloneDX specification's own test file `valid-minimal-viable-1.5.json` (valid; one
component with no version, purl or licence); a CycloneDX 1.4 BOM with only the two required
properties; a minimal SPDX 2.3 document built with exactly the fields clauses 6 and 7 require
(valid), and the same as SPDX 2.2 (invalid: `copyrightText` is mandatory there); a broken SPDX
document with ten errors; a CycloneDX 1.6 BOM exercising the licence summary, duplicates and a
dangling dependency; and usage errors. Outside the tests, the official examples
`SPDXJSONExample-v2.2/2.3.spdx.json` (spdx/tools-java) and `valid-bom-1.4/1.6.json` (CycloneDX)
were checked to give no errors.

## Limits

Structural checks of the required fields, not complete JSON Schema validation (formats of
every optional field are not checked); XML, tag-value, protobuf and SPDX 3.0 are not read;
vulnerability (VEX) data is not analysed; licence compatibility is out of scope.
