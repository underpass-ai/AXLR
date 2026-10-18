#!/usr/bin/env python3
"""Measure and check GeoJSON (RFC 7946): bounding box, centroid, planar and spherical area,
perimeter and length, point-in-polygon, and validation (closed rings of at least four
positions, right-hand-rule winding, coordinate ranges, self-intersecting rings).

Input (JSON on stdin):
  operation  "summary" (default) | "bbox" | "centroid" | "area" | "perimeter" | "contains" | "validate"
  geojson    a GeoJSON object inline: Geometry, Feature or FeatureCollection
  text       the same as a JSON string (instead of geojson)
  point      contains: [lon, lat] (GeoJSON order)
  points     contains: [[lon, lat], ...]
  radius_km  sphere radius for spherical measures, default 6371.0088 (IUGG mean radius)
"""
import json
import math
import sys
from fractions import Fraction

MEAN_RADIUS_KM = 6371.0088
MAX_POSITIONS = 100000
MAX_SELF_CHECK = 1000
OPERATIONS = ("summary", "bbox", "centroid", "area", "perimeter", "contains", "validate")
GEOMETRIES = ("Point", "MultiPoint", "LineString", "MultiLineString", "Polygon", "MultiPolygon", "GeometryCollection")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(x):
    """A float for display, at 12 significant digits."""
    value = float(x)
    if value == 0:
        return 0.0
    return float(f"{value:.12g}") if math.isfinite(value) else None


class Reader:
    """Walks a GeoJSON object, collecting geometries (with their feature index) and issues."""

    def __init__(self):
        self.issues = []
        self.positions = 0
        self.polygons = []   # (feature, path, [ring, ...]) rings as lists of (Fraction lon, Fraction lat)
        self.lines = []      # (feature, path, [pos, ...])
        self.points = []     # (feature, path, pos)
        self.types = {}

    def issue(self, level, path, message):
        self.issues.append({"level": level, "path": path, "message": message})

    def position(self, value, path):
        if not isinstance(value, list) or len(value) < 2:
            self.issue("error", path, "a position must be an array of at least two numbers")
            return None
        if any(isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v) for v in value):
            self.issue("error", path, "position members must be finite numbers")
            return None
        if len(value) > 3:
            self.issue("warning", path, "positions should not have more than three members (RFC 7946 3.1.1)")
        self.positions += 1
        if self.positions > MAX_POSITIONS:
            fail(f"at most {MAX_POSITIONS} positions")
        lon, lat = value[0], value[1]
        if not -180 <= lon <= 180 or not -90 <= lat <= 90:
            self.issue("error", path, f"[{lon}, {lat}] is outside longitude -180..180 / latitude -90..90")
        return (Fraction(str(lon)) if isinstance(lon, float) else Fraction(lon),
                Fraction(str(lat)) if isinstance(lat, float) else Fraction(lat))

    def positions_of(self, value, path, minimum, what):
        if not isinstance(value, list):
            self.issue("error", path, f"{what} must be an array of positions")
            return None
        out = [self.position(p, f"{path}[{i}]") for i, p in enumerate(value)]
        if any(p is None for p in out):
            return None
        if len(out) < minimum:
            self.issue("error", path, f"{what} needs at least {minimum} positions, has {len(out)}")
            return None
        return out

    def ring(self, value, path, hole):
        ring = self.positions_of(value, path, 4, "a linear ring")
        if ring is None:
            return None
        if ring[0] != ring[-1]:
            self.issue("error", path, "linear ring is not closed (first and last positions differ)")
            return None
        if abs(abs(ring_excess(ring)[1]) - 360.0) < 1e-6:
            self.issue("warning", path, "ring encircles a pole: winding and self-intersection are not checked and planar measures are not meaningful")
            return ring
        area = shoelace(ring)
        if area == 0:
            self.issue("warning", path, "ring has zero planar area")
        elif hole and area > 0:
            self.issue("warning", path, "hole is counter-clockwise; RFC 7946 3.1.6 wants holes clockwise")
        elif not hole and area < 0:
            self.issue("warning", path, "exterior ring is clockwise; RFC 7946 3.1.6 wants it counter-clockwise")
        if len(ring) - 1 <= MAX_SELF_CHECK:
            crossing = self_intersection(ring)
            if crossing is not None:
                self.issue("error", path, f"ring intersects itself (segments {crossing[0]} and {crossing[1]})")
        else:
            self.issue("warning", path, f"self-intersection not checked on rings over {MAX_SELF_CHECK} vertices")
        return ring

    def polygon(self, coords, path, feature):
        if not isinstance(coords, list) or not coords:
            self.issue("error", path, "a Polygon needs an array of at least one linear ring")
            return
        rings = [self.ring(r, f"{path}[{i}]", i > 0) for i, r in enumerate(coords)]
        if any(r is None for r in rings):
            return
        for i, hole in enumerate(rings[1:], 1):
            if not any(point_in_ring(p, rings[0]) == 1 for p in hole[:-1]):
                self.issue("warning", f"{path}[{i}]", "hole is not inside the exterior ring")
        self.polygons.append((feature, path, rings))

    def geometry(self, geom, path, feature):
        if geom is None:
            return
        if not isinstance(geom, dict):
            self.issue("error", path, "a geometry must be an object")
            return
        kind = geom.get("type")
        if kind not in GEOMETRIES:
            self.issue("error", path, f"unknown geometry type {kind!r}")
            return
        self.types[kind] = self.types.get(kind, 0) + 1
        if kind == "GeometryCollection":
            members = geom.get("geometries")
            if not isinstance(members, list):
                self.issue("error", path, "a GeometryCollection needs a geometries array")
                return
            for i, g in enumerate(members):
                self.geometry(g, f"{path}.geometries[{i}]", feature)
            return
        if "coordinates" not in geom:
            self.issue("error", path, f"a {kind} needs coordinates")
            return
        coords, cpath = geom["coordinates"], f"{path}.coordinates"
        if kind == "Point":
            p = self.position(coords, cpath)
            if p is not None:
                self.points.append((feature, path, p))
        elif kind == "MultiPoint":
            pts = self.positions_of(coords, cpath, 0, "a MultiPoint")
            for p in pts or []:
                self.points.append((feature, path, p))
        elif kind == "LineString":
            line = self.positions_of(coords, cpath, 2, "a LineString")
            if line is not None:
                self.lines.append((feature, path, line))
        elif kind == "MultiLineString":
            if not isinstance(coords, list):
                self.issue("error", cpath, "a MultiLineString needs an array of LineString coordinates")
                return
            for i, c in enumerate(coords):
                line = self.positions_of(c, f"{cpath}[{i}]", 2, "a LineString")
                if line is not None:
                    self.lines.append((feature, path, line))
        elif kind == "Polygon":
            self.polygon(coords, cpath, feature)
        else:
            if not isinstance(coords, list):
                self.issue("error", cpath, "a MultiPolygon needs an array of Polygon coordinates")
                return
            for i, c in enumerate(coords):
                self.polygon(c, f"{cpath}[{i}]", feature)

    def document(self, doc):
        if not isinstance(doc, dict):
            fail("geojson must be a GeoJSON object")
        kind = doc.get("type")
        if kind == "FeatureCollection":
            features = doc.get("features")
            if not isinstance(features, list):
                fail("a FeatureCollection needs a features array")
            for i, f in enumerate(features):
                self.feature(f, f"features[{i}]", i)
            return len(features)
        if kind == "Feature":
            self.feature(doc, "feature", 0)
            return 1
        if kind in GEOMETRIES:
            self.geometry(doc, "geometry", None)
            return None
        fail(f"type must be a GeoJSON geometry, Feature or FeatureCollection, not {kind!r}")

    def feature(self, f, path, index):
        if not isinstance(f, dict) or f.get("type") != "Feature":
            self.issue("error", path, "not a Feature object")
            return
        if "geometry" not in f:
            self.issue("error", path, "a Feature needs a geometry member (null allowed)")
        if "properties" not in f:
            self.issue("warning", path, "a Feature should have a properties member (null allowed)")
        self.geometry(f.get("geometry"), f"{path}.geometry", index)


def shoelace(ring):
    """The signed planar area of a closed ring, exact; positive counter-clockwise."""
    return sum(x1 * y2 - x2 * y1 for (x1, y1), (x2, y2) in zip(ring, ring[1:])) / 2


def ring_moments(ring):
    """Signed area and first moments (Σ(xi+xi+1)·cross/6, Σ(yi+yi+1)·cross/6) of a closed ring."""
    a = mx = my = Fraction(0)
    for (x1, y1), (x2, y2) in zip(ring, ring[1:]):
        cross = x1 * y2 - x2 * y1
        a += cross
        mx += (x1 + x2) * cross
        my += (y1 + y2) * cross
    return a / 2, mx / 6, my / 6


def orient(p, q, r):
    value = (q[0] - p[0]) * (r[1] - p[1]) - (q[1] - p[1]) * (r[0] - p[0])
    return (value > 0) - (value < 0)


def on_segment(p, q, r):
    """r lies on segment pq (given collinear)."""
    return min(p[0], q[0]) <= r[0] <= max(p[0], q[0]) and min(p[1], q[1]) <= r[1] <= max(p[1], q[1])


def segments_cross(p1, p2, q1, q2):
    d1, d2, d3, d4 = orient(q1, q2, p1), orient(q1, q2, p2), orient(p1, p2, q1), orient(p1, p2, q2)
    if d1 != d2 and d3 != d4 and 0 not in (d1, d2, d3, d4):
        return True
    return ((d1 == 0 and on_segment(q1, q2, p1)) or (d2 == 0 and on_segment(q1, q2, p2))
            or (d3 == 0 and on_segment(p1, p2, q1)) or (d4 == 0 and on_segment(p1, p2, q2)))


def self_intersection(ring):
    n = len(ring) - 1
    for i in range(n):
        for j in range(i + 1, n):
            if j == i + 1 or (i == 0 and j == n - 1):
                continue
            if segments_cross(ring[i], ring[i + 1], ring[j], ring[j + 1]):
                return i, j
    return None


def point_in_ring(pt, ring):
    """1 inside, 0 on the boundary, -1 outside (even-odd rule, planar in lon/lat)."""
    x, y = pt
    inside = False
    for a, b in zip(ring, ring[1:]):
        if orient(a, b, pt) == 0 and on_segment(a, b, pt):
            return 0
        if (a[1] > y) != (b[1] > y):
            xcross = a[0] + (y - a[1]) * (b[0] - a[0]) / (b[1] - a[1])
            if x < xcross:
                inside = not inside
    return 1 if inside else -1


def point_in_polygon(pt, rings):
    where = point_in_ring(pt, rings[0])
    if where != 1:
        return where
    for hole in rings[1:]:
        h = point_in_ring(pt, hole)
        if h == 0:
            return 0
        if h == 1:
            return -1
    return 1


def ring_excess(ring):
    """Signed spherical excess (steradians) of the region between each great-circle edge, the
    meridians through its ends and the equator, summed over the ring; and the summed Δλ."""
    total = dlon_sum = 0.0
    for (l1, p1), (l2, p2) in zip(ring, ring[1:]):
        dl = (float(l2) - float(l1) + 540.0) % 360.0 - 180.0
        dlon_sum += dl
        t1, t2 = math.tan(math.radians(float(p1)) / 2), math.tan(math.radians(float(p2)) / 2)
        total += 2 * math.atan2(math.tan(math.radians(dl) / 2) * (t1 + t2), 1 + t1 * t2)
    return total, dlon_sum


def ring_area_sr(ring):
    excess, dlon = ring_excess(ring)
    if abs(abs(dlon) - 360.0) < 1e-6:   # the ring goes round a pole: its interior is on its left
        return 2 * math.pi - excess, True
    return abs(excess), False


def haversine_km(a, b, radius):
    p1, p2 = math.radians(float(a[1])), math.radians(float(b[1]))
    dl = math.radians(float(b[0]) - float(a[0]))
    h = math.sin((p2 - p1) / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * radius * math.asin(min(1.0, math.sqrt(h)))


def path_length(line, radius):
    planar = sum(math.hypot(float(b[0] - a[0]), float(b[1] - a[1])) for a, b in zip(line, line[1:]))
    return planar, sum(haversine_km(a, b, radius) for a, b in zip(line, line[1:]))


def exact(value):
    text = str(value)
    return text if len(text) <= 64 else None


def measure(reader, radius, feature=None):
    """bbox, centroid, area, perimeter and length of the geometries (of one feature, if given)."""
    pick = (lambda items: items) if feature is None else (lambda items: [g for g in items if g[0] == feature])
    polygons, lines, points = pick(reader.polygons), pick(reader.lines), pick(reader.points)
    every = [p for _, _, rings in polygons for r in rings for p in r] + [p for _, _, l in lines for p in l] + [p for _, _, p in points]
    out = {"bbox": None, "centroid": None}
    if every:
        out["bbox"] = [num(min(p[0] for p in every)), num(min(p[1] for p in every)),
                       num(max(p[0] for p in every)), num(max(p[1] for p in every))]
    warnings = []
    if polygons:
        planar = mx = my = Fraction(0)
        steradians = 0.0
        perimeter_deg = perimeter_km = 0.0
        for _, path, rings in polygons:
            for i, ring in enumerate(rings):
                a, x, y = ring_moments(ring)
                sign = 1 if (a >= 0) == (i == 0) else -1   # exterior adds, holes subtract, whatever the winding
                planar, mx, my = planar + sign * a, mx + sign * x, my + sign * y
                sr, polar = ring_area_sr(ring)
                steradians += sr if i == 0 else -sr
                if polar:
                    warnings.append(f"{path}[{i}] encircles a pole: its spherical area is taken on its left (counter-clockwise interior); planar measures are not meaningful")
                d, k = path_length(ring, radius)
                perimeter_deg, perimeter_km = perimeter_deg + d, perimeter_km + k
        area_km2 = steradians * radius * radius
        out["area"] = {"spherical_km2": num(area_km2), "spherical_m2": num(area_km2 * 1e6), "planar_deg2": num(planar),
                       "planar_deg2_exact": exact(planar), "radius_km": num(radius), "model": "sphere"}
        out["perimeter"] = {"spherical_km": num(perimeter_km), "planar_deg": num(perimeter_deg)}
        if planar != 0:
            cx, cy = mx / planar, my / planar
            out["centroid"] = {"lon": num(cx), "lat": num(cy), "exact": [exact(cx), exact(cy)] if exact(cx) and exact(cy) else None,
                               "method": "area-weighted planar centroid in lon/lat of the polygons"}
    if lines:
        planar_len, km = 0.0, 0.0
        sx = sy = 0.0
        for _, _, line in lines:
            for a, b in zip(line, line[1:]):
                seg = math.hypot(float(b[0] - a[0]), float(b[1] - a[1]))
                planar_len += seg
                km += haversine_km(a, b, radius)
                sx += seg * float(a[0] + b[0]) / 2
                sy += seg * float(a[1] + b[1]) / 2
        out["length"] = {"spherical_km": num(km), "planar_deg": num(planar_len)}
        if out["centroid"] is None and planar_len > 0:
            out["centroid"] = {"lon": num(sx / planar_len), "lat": num(sy / planar_len), "method": "length-weighted planar centroid of the lines"}
    if points and out["centroid"] is None:
        n = len(points)
        cx, cy = sum(p[0] for _, _, p in points) / n, sum(p[1] for _, _, p in points) / n
        out["centroid"] = {"lon": num(cx), "lat": num(cy), "exact": [exact(cx), exact(cy)] if exact(cx) and exact(cy) else None,
                           "method": "mean of the points"}
    if warnings:
        out["warnings"] = warnings
    return out


def read_point(value, field):
    if (not isinstance(value, list) or len(value) not in (2, 3)
            or any(isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v) for v in value)):
        fail(f"{field} must be [lon, lat]")
    lon, lat = value[0], value[1]
    if not -180 <= lon <= 180 or not -90 <= lat <= 90:
        fail(f"{field} is outside longitude -180..180 / latitude -90..90")
    return Fraction(str(lon)) if isinstance(lon, float) else Fraction(lon), Fraction(str(lat)) if isinstance(lat, float) else Fraction(lat)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = sorted(set(request) - {"operation", "geojson", "text", "point", "points", "radius_km"})
    if unknown:
        fail(f"unknown field(s): {', '.join(unknown)}")
    operation = request.get("operation", "summary")
    if operation not in OPERATIONS:
        fail(f"operation must be one of {', '.join(OPERATIONS)}")
    if ("geojson" in request) == ("text" in request):
        fail("give exactly one of geojson (an object) or text (a JSON string)")
    doc = request.get("geojson")
    if "text" in request:
        if not isinstance(request["text"], str):
            fail("text must be a string holding GeoJSON")
        try:
            doc = json.loads(request["text"])
        except ValueError as error:
            fail(f"text is not JSON: {error}")
    radius = request.get("radius_km", MEAN_RADIUS_KM)
    if isinstance(radius, bool) or not isinstance(radius, (int, float)) or not math.isfinite(radius) or radius <= 0:
        fail("radius_km must be a positive number")
    reader = Reader()
    features = reader.document(doc)
    errors = [i for i in reader.issues if i["level"] == "error"]
    result = {"operation": operation, "type": doc.get("type")}
    if features is not None:
        result["features"] = features
    result["geometry_types"] = reader.types
    if operation == "validate":
        result.update({"valid": not errors, "errors": len(errors), "warnings": len(reader.issues) - len(errors),
                       "issues": reader.issues})
        print(json.dumps(result, ensure_ascii=False))
        return
    if errors:
        fail("invalid GeoJSON: " + "; ".join(f"{e['path']}: {e['message']}" for e in errors[:5])
             + (f" (and {len(errors) - 5} more)" if len(errors) > 5 else "") + "; run operation validate for the full list")
    if operation == "contains":
        if ("point" in request) == ("points" in request):
            fail("contains needs point [lon, lat] or points [[lon, lat], ...]")
        if not reader.polygons:
            fail("contains needs at least one Polygon or MultiPolygon")
        if "point" in request:
            pts, single = [read_point(request["point"], "point")], True
        else:
            if not isinstance(request["points"], list) or not request["points"] or len(request["points"]) > 10000:
                fail("points must be a list of 1 to 10000 [lon, lat] pairs")
            pts, single = [read_point(p, f"points[{i}]") for i, p in enumerate(request["points"])], False
        answers = []
        for pt in pts:
            where = [(f, point_in_polygon(pt, rings)) for f, _, rings in reader.polygons]
            inside = any(w == 1 for _, w in where)
            boundary = not inside and any(w == 0 for _, w in where)
            answer = {"point": [num(pt[0]), num(pt[1])], "inside": inside, "on_boundary": boundary}
            if features is not None:
                answer["features"] = sorted({f for f, w in where if w >= 0})
            answers.append(answer)
        result.update(answers[0] if single else {"results": answers})
        result["method"] = "even-odd ray casting in the lon/lat plane, exact over fractions"
        print(json.dumps(result, ensure_ascii=False))
        return
    whole = measure(reader, radius)
    keys = {"summary": None, "bbox": ("bbox",), "centroid": ("centroid",), "area": ("area",),
            "perimeter": ("perimeter", "length")}[operation]
    if operation in ("area", "perimeter") and not reader.polygons and not (operation == "perimeter" and reader.lines):
        fail(f"{operation} needs a Polygon or MultiPolygon" + (" or a line" if operation == "perimeter" else ""))
    for key, value in whole.items():
        if keys is None or key in keys or key == "warnings":
            result[key] = value
    if operation == "summary":
        result["valid"] = True
        result["issues"] = reader.issues
        if features is not None and features > 1:
            result["per_feature"] = []
            for i in range(features):
                m = measure(reader, radius, i)
                entry = {"index": i, "bbox": m["bbox"]}
                if "area" in m:
                    entry["spherical_km2"] = m["area"]["spherical_km2"]
                if "length" in m:
                    entry["length_km"] = m["length"]["spherical_km"]
                result["per_feature"].append(entry)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
