#!/usr/bin/env python3
"""Check a software bill of materials in CycloneDX JSON (spec 1.4, 1.5, 1.6) or SPDX JSON
(2.2, 2.3): the fields each specification requires, components without a version, a purl or
a licence, duplicate components and identifiers, dangling references, and a licence summary
(SPDX ids counted and checked against the bundled SPDX License List, NOASSERTION, NONE,
LicenseRef-, free-text names, expressions).

Input (JSON on stdin):
  sbom       the SBOM as a JSON object
  sbom_text  or the SBOM as JSON text
  path       or a relative path to a JSON file in the working directory
  max_list   longest list of component names to return per category (default 100)
"""
import json
import pathlib
import re
import sys

HERE = pathlib.Path(__file__).resolve().parent
CDX_TYPES = {
    "1.4": {"application", "framework", "library", "container", "operating-system", "device", "firmware", "file"},
}
CDX_TYPES["1.5"] = CDX_TYPES["1.4"] | {"platform", "device-driver", "machine-learning-model", "data"}
CDX_TYPES["1.6"] = CDX_TYPES["1.5"] | {"cryptographic-asset"}
CDX_HASHES = {"MD5", "SHA-1", "SHA-256", "SHA-384", "SHA-512", "SHA3-256", "SHA3-384", "SHA3-512", "BLAKE2b-256",
              "BLAKE2b-384", "BLAKE2b-512", "BLAKE3"}
UUID_URN = re.compile(r"^urn:uuid:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$")
PURL = re.compile(r"^pkg:[A-Za-z.+-][A-Za-z0-9.+-]*/[^@?#\s]+(@[^?#\s]+)?(\?[^#\s]*)?(#\S*)?$")
DATETIME = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$")
SPDX_ID = re.compile(r"^SPDXRef-[A-Za-z0-9.-]+$")
SPDX_CHECKSUMS = {"SHA1", "SHA224", "SHA256", "SHA384", "SHA512", "MD2", "MD4", "MD5", "MD6", "SHA3-256", "SHA3-384",
                  "SHA3-512", "BLAKE2b-256", "BLAKE2b-384", "BLAKE2b-512", "BLAKE3", "ADLER32"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load_license_list():
    try:
        data = json.loads((HERE / "spdx_licenses.json").read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}, {}, None
    licenses = {key.lower(): (key, value) for key, value in data["licenses"].items()}
    exceptions = {key.lower(): (key, value) for key, value in data["exceptions"].items()}
    meta = {"version": data.get("license_list_version"), "release_date": data.get("release_date"), "source": data.get("source_url"),
            "fetched": data.get("fetched")}
    return licenses, exceptions, meta


class Report:
    def __init__(self, max_list):
        self.errors, self.warnings = [], []
        self.max_list = max_list
        self.components = []  # dicts: name, version, purl, has_license, supplier, path, ref
        self.license_counts, self.ref_counts, self.name_counts = {}, {}, {}
        self.unknown_ids, self.deprecated_ids, self.exception_ids = {}, {}, {}
        self.noassertion = self.none = self.expressions = 0

    def error(self, path, message):
        self.errors.append({"path": path, "message": message})

    def warn(self, path, message):
        self.warnings.append({"path": path, "message": message})


def bump(table, key):
    table[key] = table.get(key, 0) + 1


def tokenize(expression):
    return re.findall(r"\(|\)|[^\s()]+", expression)


def check_expression(report, expression, path, licenses, exceptions, refs_known=None):
    """SPDX license expression (SPDX 2.3 Annex D): ids, LicenseRef-, WITH exceptions, AND / OR, parentheses."""
    text = expression.strip()
    if text == "NOASSERTION":
        report.noassertion += 1
        return
    if text == "NONE":
        report.none += 1
        return
    tokens = tokenize(text)
    if not tokens:
        report.error(path, "empty licence expression")
        return
    operators = [t for t in tokens if t.upper() in ("AND", "OR", "WITH")]
    if operators:
        report.expressions += 1
        if not (all(t.isupper() for t in operators) or all(t.islower() for t in operators)):
            report.warn(path, "licence expression operators must be all upper case or all lower case")
    depth, expect_operand, after_with = 0, True, False
    for token in tokens:
        upper = token.upper()
        if token == "(":
            if not expect_operand:
                report.error(path, f"licence expression {text!r}: '(' where an operator was expected")
                return
            depth += 1
        elif token == ")":
            depth -= 1
            if depth < 0 or expect_operand:
                report.error(path, f"licence expression {text!r}: unbalanced or empty parentheses")
                return
        elif upper in ("AND", "OR", "WITH"):
            if expect_operand:
                report.error(path, f"licence expression {text!r}: operator {token} without an operand")
                return
            expect_operand, after_with = True, upper == "WITH"
        else:
            if not expect_operand:
                report.error(path, f"licence expression {text!r}: two licences without an operator between them")
                return
            if after_with:
                found = exceptions.get(token.lower())
                if found:
                    bump(report.exception_ids, found[0])
                elif not token.startswith(("AdditionRef-", "DocumentRef-")):
                    bump(report.unknown_ids, token)
            elif token.startswith(("LicenseRef-", "DocumentRef-")):
                bump(report.ref_counts, token)
                if refs_known is not None and token.startswith("LicenseRef-") and token not in refs_known:
                    report.error(path, f"{token} is not defined in hasExtractedLicensingInfos")
            else:
                base = token[:-1] if token.endswith("+") else token
                found = licenses.get(base.lower())
                if found:
                    bump(report.license_counts, found[0] + ("+" if token.endswith("+") else ""))
                    if found[1].get("deprecated"):
                        bump(report.deprecated_ids, found[0])
                elif licenses:
                    bump(report.unknown_ids, token)
                else:
                    bump(report.license_counts, token)
            expect_operand, after_with = False, False
    if depth != 0 or expect_operand:
        report.error(path, f"licence expression {text!r} is incomplete or has unbalanced parentheses")


def cyclonedx(bom, report, licenses, exceptions):
    path = ""
    spec = bom.get("specVersion")
    if bom.get("bomFormat") != "CycloneDX":
        report.error("/bomFormat", "bomFormat must be \"CycloneDX\"")
    if not isinstance(spec, str):
        report.error("/specVersion", "specVersion is required (a string such as \"1.5\")")
        spec = "1.6"
    elif spec not in CDX_TYPES:
        report.error("/specVersion", f"specVersion {spec} is not supported here (1.4, 1.5 or 1.6)")
        spec = "1.6"
    serial = bom.get("serialNumber")
    if serial is None:
        report.warn("/serialNumber", "no serialNumber: recommended (urn:uuid:...) so that BOM versions can be told apart")
    elif not isinstance(serial, str) or not UUID_URN.match(serial):
        report.error("/serialNumber", "serialNumber must be an RFC 4122 URN: urn:uuid:xxxxxxxx-xxxx-Mxxx-Nxxx-xxxxxxxxxxxx")
    version = bom.get("version", 1)
    if isinstance(version, bool) or not isinstance(version, int) or version < 1:
        report.error("/version", "version must be an integer >= 1")
    metadata = bom.get("metadata", {})
    if not isinstance(metadata, dict):
        report.error("/metadata", "metadata must be an object")
        metadata = {}
    timestamp = metadata.get("timestamp")
    if timestamp is None:
        report.warn("/metadata/timestamp", "no metadata.timestamp (an NTIA minimum element)")
    elif not isinstance(timestamp, str) or not DATETIME.match(timestamp):
        report.error("/metadata/timestamp", "metadata.timestamp must be an ISO 8601 date-time")
    if not (metadata.get("authors") or metadata.get("manufacture") or metadata.get("manufacturer") or metadata.get("supplier") or metadata.get("tools")):
        report.warn("/metadata", "no metadata.authors, supplier or tools: the author of the SBOM data is an NTIA minimum element")
    refs = {}

    def walk(component, where):
        if not isinstance(component, dict):
            report.error(where, "a component must be an object")
            return
        ctype, name = component.get("type"), component.get("name")
        if not isinstance(ctype, str):
            report.error(where + "/type", "type is required")
        elif ctype not in CDX_TYPES[spec]:
            report.error(where + "/type", f"type {ctype!r} is not allowed in CycloneDX {spec}")
        if not isinstance(name, str) or not name:
            report.error(where + "/name", "name is required")
        ref = component.get("bom-ref")
        if ref is not None:
            if not isinstance(ref, str) or not ref:
                report.error(where + "/bom-ref", "bom-ref must be a non-empty string")
            elif ref in refs:
                report.error(where + "/bom-ref", f"duplicate bom-ref {ref!r} (also at {refs[ref]})")
            else:
                refs[ref] = where
        version_value = component.get("version")
        if version_value is not None and not isinstance(version_value, str):
            report.error(where + "/version", "version must be a string")
        purl = component.get("purl")
        if purl is not None and (not isinstance(purl, str) or not PURL.match(purl)):
            report.error(where + "/purl", "purl is not a valid package URL (pkg:type/namespace/name@version)")
        has_license = False
        entries = component.get("licenses")
        if entries is not None:
            if not isinstance(entries, list):
                report.error(where + "/licenses", "licenses must be an array")
                entries = []
            if any(isinstance(e, dict) and "expression" in e for e in entries) and len(entries) > 1:
                report.error(where + "/licenses", "an expression must be the only item of licenses")
            for index, entry in enumerate(entries):
                at = f"{where}/licenses/{index}"
                if not isinstance(entry, dict) or len({"license", "expression"} & set(entry)) != 1:
                    report.error(at, "each licenses item must have exactly one of license or expression")
                    continue
                has_license = True
                if "expression" in entry:
                    if isinstance(entry["expression"], str):
                        check_expression(report, entry["expression"], at + "/expression", licenses, exceptions)
                    else:
                        report.error(at + "/expression", "expression must be a string")
                else:
                    lic = entry["license"]
                    if not isinstance(lic, dict) or len({"id", "name"} & set(lic)) != 1:
                        report.error(at + "/license", "license must have exactly one of id or name")
                        continue
                    if "id" in lic:
                        lid = lic["id"]
                        found = licenses.get(str(lid).lower())
                        if found:
                            bump(report.license_counts, found[0])
                            if found[1].get("deprecated"):
                                bump(report.deprecated_ids, found[0])
                        else:
                            report.error(at + "/license/id", f"{lid!r} is not an SPDX License List id (use name for other licences)")
                            bump(report.unknown_ids, str(lid))
                    else:
                        lname = str(lic["name"])
                        if lname.upper() == "NOASSERTION":
                            report.noassertion += 1
                        else:
                            bump(report.name_counts, lname)
                            if licenses.get(lname.lower()):
                                report.warn(at + "/license/name", f"{lname!r} is an SPDX id: put it in id rather than name")
        hashes = component.get("hashes", [])
        if isinstance(hashes, list):
            for index, entry in enumerate(hashes):
                if not isinstance(entry, dict) or entry.get("alg") not in CDX_HASHES or not isinstance(entry.get("content"), str):
                    report.error(f"{where}/hashes/{index}", "hash needs alg (MD5, SHA-1, SHA-256, ...) and content")
        supplier = component.get("supplier") or component.get("publisher") or component.get("author") or component.get("manufacturer")
        report.components.append({"name": name if isinstance(name, str) else None, "version": version_value if isinstance(version_value, str) else None,
                                  "purl": purl if isinstance(purl, str) else None, "has_license": has_license,
                                  "supplier": bool(supplier), "path": where, "ref": ref, "unique_id": bool(purl or component.get("cpe") or component.get("swid"))})
        for index, child in enumerate(component.get("components", []) if isinstance(component.get("components", []), list) else []):
            walk(child, f"{where}/components/{index}")

    meta_component = metadata.get("component")
    if meta_component is not None:
        walk(meta_component, "/metadata/component")
        primary = report.components.pop() if report.components and report.components[-1]["path"] == "/metadata/component" else None
    else:
        primary = None
        report.warn("/metadata/component", "no metadata.component: the BOM does not say what it describes")
    components = bom.get("components", [])
    if not isinstance(components, list):
        report.error("/components", "components must be an array")
        components = []
    for index, component in enumerate(components):
        walk(component, f"/components/{index}")
    dependencies = bom.get("dependencies")
    has_dependencies = isinstance(dependencies, list) and bool(dependencies)
    if isinstance(dependencies, list):
        for index, dep in enumerate(dependencies):
            if not isinstance(dep, dict) or not isinstance(dep.get("ref"), str):
                report.error(f"/dependencies/{index}", "a dependency needs ref")
                continue
            for target in [dep["ref"]] + [d for d in dep.get("dependsOn", []) if isinstance(d, str)]:
                if target not in refs:
                    report.error(f"/dependencies/{index}", f"{target!r} is not the bom-ref of any component")
    return {"format": "CycloneDX", "spec_version": spec, "serial_number": serial if isinstance(serial, str) else None,
            "describes": primary["name"] if primary else None, "has_dependencies": has_dependencies,
            "timestamp": bool(timestamp)}


def spdx(doc, report, licenses, exceptions):
    version = doc.get("spdxVersion")
    if version not in ("SPDX-2.2", "SPDX-2.3"):
        report.error("/spdxVersion", f"spdxVersion must be SPDX-2.2 or SPDX-2.3 (found {version!r})")
        version = "SPDX-2.3"
    v23 = version == "SPDX-2.3"
    if doc.get("dataLicense") != "CC0-1.0":
        report.error("/dataLicense", "dataLicense must be CC0-1.0 (SPDX 2.x clause 6.2)")
    if doc.get("SPDXID") != "SPDXRef-DOCUMENT":
        report.error("/SPDXID", "the document SPDXID must be SPDXRef-DOCUMENT (clause 6.3)")
    if not isinstance(doc.get("name"), str) or not doc.get("name"):
        report.error("/name", "name is required (clause 6.4)")
    namespace = doc.get("documentNamespace")
    if not isinstance(namespace, str) or not re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*:\S+$", namespace or ""):
        report.error("/documentNamespace", "documentNamespace is required: a unique absolute URI (clause 6.5)")
    elif "#" in namespace:
        report.error("/documentNamespace", "documentNamespace must not contain '#'")
    creation = doc.get("creationInfo")
    timestamp = False
    if not isinstance(creation, dict):
        report.error("/creationInfo", "creationInfo is required with created and creators (clauses 6.8 and 6.9)")
    else:
        created = creation.get("created")
        if not isinstance(created, str) or not re.match(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$", created):
            report.error("/creationInfo/created", "created is required as YYYY-MM-DDThh:mm:ssZ (clause 6.9)")
        else:
            timestamp = True
        creators = creation.get("creators")
        if not isinstance(creators, list) or not creators:
            report.error("/creationInfo/creators", "creators is required: one or more 'Tool: ', 'Organization: ' or 'Person: ' entries (clause 6.8)")
        else:
            for index, creator in enumerate(creators):
                if not isinstance(creator, str) or not re.match(r"^(Tool|Organization|Person): \S", creator):
                    report.error(f"/creationInfo/creators/{index}", "a creator must start with 'Tool: ', 'Organization: ' or 'Person: '")
    known_refs = set()
    for index, info in enumerate(doc.get("hasExtractedLicensingInfos", []) or []):
        lid = info.get("licenseId") if isinstance(info, dict) else None
        if not isinstance(lid, str) or not lid.startswith("LicenseRef-"):
            report.error(f"/hasExtractedLicensingInfos/{index}", "licenseId must start with LicenseRef-")
        else:
            known_refs.add(lid)
            if not info.get("extractedText"):
                report.error(f"/hasExtractedLicensingInfos/{index}", "extractedText is required")
    ids = {"SPDXRef-DOCUMENT": "/"}
    packages = doc.get("packages", [])
    if not isinstance(packages, list):
        report.error("/packages", "packages must be an array")
        packages = []
    for index, package in enumerate(packages):
        at = f"/packages/{index}"
        if not isinstance(package, dict):
            report.error(at, "a package must be an object")
            continue
        name = package.get("name")
        if not isinstance(name, str) or not name:
            report.error(at + "/name", "name is required (clause 7.1)")
        sid = package.get("SPDXID")
        if not isinstance(sid, str) or not SPDX_ID.match(sid):
            report.error(at + "/SPDXID", "SPDXID is required: SPDXRef- followed by letters, digits, '.' or '-' (clause 7.2)")
        elif sid in ids:
            report.error(at + "/SPDXID", f"duplicate SPDXID {sid} (also at {ids[sid]})")
        else:
            ids[sid] = at
        location = package.get("downloadLocation")
        if not isinstance(location, str) or not location:
            report.error(at + "/downloadLocation", "downloadLocation is required: a URL, NONE or NOASSERTION (clause 7.7)")
        files_analyzed = package.get("filesAnalyzed", True)
        if files_analyzed is True and "packageVerificationCode" not in package:
            report.error(at + "/packageVerificationCode", "packageVerificationCode is required when filesAnalyzed is true, its default (clause 7.9); set filesAnalyzed false if files were not analysed")
        has_license = False
        for field in ("licenseConcluded", "licenseDeclared"):
            if field in package:
                if isinstance(package[field], str):
                    check_expression(report, package[field], f"{at}/{field}", licenses, exceptions, known_refs)
                    if package[field] not in ("NOASSERTION", "NONE"):
                        has_license = True
                else:
                    report.error(f"{at}/{field}", f"{field} must be a licence expression string")
            elif not v23:
                report.error(f"{at}/{field}", f"{field} is required in SPDX 2.2 (clauses 7.13 and 7.15); NOASSERTION is allowed")
        if "copyrightText" not in package and not v23:
            report.error(at + "/copyrightText", "copyrightText is required in SPDX 2.2 (clause 7.17); NOASSERTION is allowed")
        purls = []
        for rindex, ref in enumerate(package.get("externalRefs", []) or []):
            if not isinstance(ref, dict) or not all(isinstance(ref.get(k), str) for k in ("referenceCategory", "referenceType", "referenceLocator")):
                report.error(f"{at}/externalRefs/{rindex}", "externalRefs need referenceCategory, referenceType and referenceLocator")
                continue
            if ref["referenceType"] == "purl":
                purls.append(ref["referenceLocator"])
                if not PURL.match(ref["referenceLocator"]):
                    report.error(f"{at}/externalRefs/{rindex}", "purl is not a valid package URL")
                category = ref["referenceCategory"]
                expected = "PACKAGE-MANAGER" if v23 else "PACKAGE_MANAGER"
                if category.replace("_", "-") != "PACKAGE-MANAGER":
                    report.warn(f"{at}/externalRefs/{rindex}", f"purl references belong to the {expected} category")
        for cindex, checksum in enumerate(package.get("checksums", []) or []):
            if not isinstance(checksum, dict) or checksum.get("algorithm") not in SPDX_CHECKSUMS or not isinstance(checksum.get("checksumValue"), str):
                report.error(f"{at}/checksums/{cindex}", "checksum needs algorithm (SHA1, SHA256, ...) and checksumValue")
        version_info = package.get("versionInfo")
        report.components.append({"name": name if isinstance(name, str) else None, "version": version_info if isinstance(version_info, str) else None,
                                  "purl": purls[0] if purls else None, "has_license": has_license, "supplier": bool(package.get("supplier") or package.get("originator")),
                                  "path": at, "ref": sid, "unique_id": bool(purls or package.get("externalRefs"))})
    for index, file in enumerate(doc.get("files", []) or []):
        at = f"/files/{index}"
        if not isinstance(file, dict):
            report.error(at, "a file must be an object")
            continue
        if not isinstance(file.get("fileName"), str):
            report.error(at + "/fileName", "fileName is required (clause 8.1)")
        sid = file.get("SPDXID")
        if not isinstance(sid, str) or not SPDX_ID.match(sid):
            report.error(at + "/SPDXID", "SPDXID is required (clause 8.2)")
        elif sid in ids:
            report.error(at + "/SPDXID", f"duplicate SPDXID {sid} (also at {ids[sid]})")
        else:
            ids[sid] = at
        checksums = file.get("checksums")
        if not isinstance(checksums, list) or not any(isinstance(c, dict) and c.get("algorithm") == "SHA1" for c in checksums):
            report.error(at + "/checksums", "a SHA1 checksum is required for every file (clause 8.4)")
        if isinstance(file.get("licenseConcluded"), str):
            check_expression(report, file["licenseConcluded"], at + "/licenseConcluded", licenses, exceptions, known_refs)
        elif not v23:
            report.error(at + "/licenseConcluded", "licenseConcluded is required in SPDX 2.2 (clause 8.5)")
        if "copyrightText" not in file and not v23:
            report.error(at + "/copyrightText", "copyrightText is required in SPDX 2.2 (clause 8.8)")
        for lindex, item in enumerate(file.get("licenseInfoInFiles", []) or []):
            if isinstance(item, str):
                check_expression(report, item, f"{at}/licenseInfoInFiles/{lindex}", licenses, exceptions, known_refs)
    for index, snippet in enumerate(doc.get("snippets", []) or []):
        if isinstance(snippet, dict) and isinstance(snippet.get("SPDXID"), str):
            ids.setdefault(snippet["SPDXID"], f"/snippets/{index}")
    relationships = doc.get("relationships", []) or []
    describes = list(doc.get("documentDescribes", []) or [])
    has_dependencies = False
    for index, rel in enumerate(relationships):
        at = f"/relationships/{index}"
        if not isinstance(rel, dict) or not all(isinstance(rel.get(k), str) for k in ("spdxElementId", "relationshipType", "relatedSpdxElement")):
            report.error(at, "a relationship needs spdxElementId, relationshipType and relatedSpdxElement")
            continue
        for key in ("spdxElementId", "relatedSpdxElement"):
            target = rel[key]
            if target not in ids and target not in ("NONE", "NOASSERTION") and not target.startswith("DocumentRef-"):
                report.error(at + "/" + key, f"{target} is not the SPDXID of any element of the document")
        if rel["relationshipType"] == "DESCRIBES" and rel["spdxElementId"] == "SPDXRef-DOCUMENT":
            describes.append(rel["relatedSpdxElement"])
        if "DEPENDS_ON" in rel["relationshipType"] or "DEPENDENCY_OF" in rel["relationshipType"] or rel["relationshipType"] == "CONTAINS":
            has_dependencies = True
    for target in doc.get("documentDescribes", []) or []:
        if target not in ids:
            report.error("/documentDescribes", f"{target} is not the SPDXID of any element")
    if not describes and (packages or doc.get("files")):
        report.error("/relationships", "the document must DESCRIBE at least one element (a DESCRIBES relationship from SPDXRef-DOCUMENT or documentDescribes)")
    return {"format": "SPDX", "spec_version": version[5:], "document_namespace": namespace if isinstance(namespace, str) else None,
            "describes": describes, "has_dependencies": has_dependencies, "timestamp": timestamp}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"sbom", "sbom_text", "path", "max_list"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    given = [k for k in ("sbom", "sbom_text", "path") if k in request]
    if len(given) != 1:
        fail("give exactly one of sbom (object), sbom_text (JSON text) or path (relative)")
    if "sbom" in request:
        document = request["sbom"]
    else:
        if "path" in request:
            path = request["path"]
            if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or ".." in re.split(r"[\\/]", path) or re.match(r"^[A-Za-z]:", path):
                fail("path must be a relative path inside the working directory, without '..'")
            target = pathlib.Path(path)
            if not target.is_file():
                fail(f"no such file: {path}")
            if target.stat().st_size > 20_000_000:
                fail("the file is larger than 20 MB")
            text = target.read_text(encoding="utf-8", errors="replace")
        else:
            text = request["sbom_text"]
            if not isinstance(text, str):
                fail("sbom_text must be a string")
        try:
            document = json.loads(text)
        except ValueError as error:
            fail(f"the SBOM is not valid JSON: {error}")
    if not isinstance(document, dict):
        fail("the SBOM must be a JSON object")
    max_list = request.get("max_list", 100)
    if isinstance(max_list, bool) or not isinstance(max_list, int) or max_list < 0:
        fail("max_list must be an integer >= 0")
    licenses, exceptions, meta = load_license_list()
    report = Report(max_list)
    if "bomFormat" in document or "specVersion" in document:
        info = cyclonedx(document, report, licenses, exceptions)
    elif "spdxVersion" in document:
        info = spdx(document, report, licenses, exceptions)
    else:
        fail("not a CycloneDX (bomFormat, specVersion) or SPDX (spdxVersion) JSON document")

    comps = report.components

    def names(items):
        labels = [c["name"] or c["path"] for c in items]
        return labels[:max_list]

    no_version = [c for c in comps if not c["version"]]
    no_purl = [c for c in comps if not c["purl"]]
    no_license = [c for c in comps if not c["has_license"]]
    no_supplier = [c for c in comps if not c["supplier"]]
    groups = {}
    for c in comps:
        if c["name"]:
            groups.setdefault((c["name"].lower(), c["version"] or ""), []).append(c["path"])
    duplicates = [{"name": key[0], "version": key[1] or None, "paths": paths} for key, paths in sorted(groups.items()) if len(paths) > 1]
    purl_groups = {}
    for c in comps:
        if c["purl"]:
            purl_groups.setdefault(c["purl"], []).append(c["path"])
    duplicate_purls = [{"purl": key, "paths": paths} for key, paths in sorted(purl_groups.items()) if len(paths) > 1]
    if duplicates:
        report.warn("/components", f"{len(duplicates)} component name and version pairs appear more than once")
    ntia = {
        "supplier_name": bool(comps) and not no_supplier,
        "component_name": all(c["name"] for c in comps),
        "version": bool(comps) and not no_version,
        "unique_identifiers": bool(comps) and all(c["unique_id"] for c in comps),
        "dependency_relationship": info["has_dependencies"],
        "author_of_sbom_data": not any(w["path"] == "/metadata" for w in report.warnings) if info["format"] == "CycloneDX" else not any(e["path"].startswith("/creationInfo") for e in report.errors),
        "timestamp": info["timestamp"],
    }
    output = dict(info)
    output.update({
        "valid": not report.errors,
        "error_count": len(report.errors),
        "errors": report.errors,
        "warnings": report.warnings,
        "components": {
            "count": len(comps),
            "without_version": {"count": len(no_version), "names": names(no_version)},
            "without_purl": {"count": len(no_purl), "names": names(no_purl)},
            "without_license": {"count": len(no_license), "names": names(no_license)},
            "without_supplier": {"count": len(no_supplier)},
            "duplicates": duplicates[:max_list],
            "duplicate_purls": duplicate_purls[:max_list],
        },
        "licenses": {
            "spdx_ids": dict(sorted(report.license_counts.items(), key=lambda kv: (-kv[1], kv[0]))),
            "license_refs": dict(sorted(report.ref_counts.items())),
            "names_not_spdx": dict(sorted(report.name_counts.items())),
            "exceptions": dict(sorted(report.exception_ids.items())),
            "noassertion": report.noassertion,
            "none": report.none,
            "expressions": report.expressions,
            "unknown_ids": dict(sorted(report.unknown_ids.items())),
            "deprecated_ids": dict(sorted(report.deprecated_ids.items())),
        },
        "ntia_minimum_elements": ntia,
        "license_list": meta,
        "note": "Structural checks of the fields the specification requires, not full JSON Schema validation.",
    })
    print(json.dumps(output))


if __name__ == "__main__":
    main()
