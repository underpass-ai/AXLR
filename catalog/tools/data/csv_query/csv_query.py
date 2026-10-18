#!/usr/bin/env python3
"""SQL over one or more CSVs: each CSV becomes a table in an in-memory SQLite database
(columns typed INTEGER, REAL or TEXT by inference, empty cells NULL) and one read-only
SELECT runs against them. The result comes back as JSON rows and as CSV.

Read-only by construction: PRAGMA query_only, an authorizer that allows only reading
(no ATTACH, PRAGMA, writes or load_extension), one statement only, a step budget and a row
limit.

Input (JSON on stdin):
  sql          one SELECT (or WITH ... SELECT) statement
  tables       {name: csv text}
  csv          a single CSV, as table "data" (or the name in table)
  table        name for csv, default "data"
  paths        {name: relative path} CSV files in the working directory
  delimiter    override the sniffed delimiter for every table
  infer_types  true (default): INTEGER / REAL / TEXT by inference; false: every column TEXT
  limit        maximum rows returned, default 1000 (at most 100000)
"""
import csv
import io
import json
import pathlib
import re
import sqlite3
import sys
import time

INT = re.compile(r"^[+-]?\d{1,18}$")
NUM = re.compile(r"^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$")
NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]{0,63}$")
MAX_BYTES = 20_000_000
ALLOWED = {sqlite3.SQLITE_SELECT, sqlite3.SQLITE_READ, sqlite3.SQLITE_FUNCTION}
if hasattr(sqlite3, "SQLITE_RECURSIVE"):
    ALLOWED.add(sqlite3.SQLITE_RECURSIVE)
DENIED_FUNCTIONS = {"load_extension", "readfile", "writefile", "edit", "fts3_tokenizer"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_path(path):
    if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path) or ".." in re.split(r"[\\/]", path):
        fail("paths must be relative paths inside the working directory, without '..'")
    target = pathlib.Path(path)
    if not target.is_file():
        fail(f"no such file: {path}")
    if target.stat().st_size > MAX_BYTES:
        fail(f"{path} is larger than 20 MB")
    return target.read_bytes().decode("utf-8-sig", errors="replace")


def parse_csv(name, text, delimiter):
    if text.startswith("﻿"):
        text = text[1:]
    if delimiter is None:
        try:
            delimiter = csv.Sniffer().sniff(text[:65536], delimiters=",;\t|").delimiter
        except csv.Error:
            delimiter = ","
    try:
        rows = [row for row in csv.reader(io.StringIO(text), delimiter=delimiter) if row and any(c.strip() for c in row)]
    except csv.Error as error:
        fail(f"table {name}: the CSV could not be parsed: {error}")
    if not rows:
        fail(f"table {name} has no header row")
    header, data = rows[0], rows[1:]
    width = max(len(r) for r in rows)
    columns, seen = [], {}
    for index in range(width):
        base = header[index].strip() if index < len(header) and header[index].strip() else f"column_{index + 1}"
        label = base
        while label.lower() in seen:
            seen[base.lower()] = seen.get(base.lower(), 1) + 1
            label = f"{base}_{seen[base.lower()]}"
        seen[label.lower()] = 1
        columns.append(label)
    data = [row + [""] * (width - len(row)) for row in data]
    return columns, data, delimiter


def column_type(values):
    present = [v.strip() for v in values if v.strip() != ""]
    if not present:
        return "TEXT"
    if all(INT.match(v) for v in present):
        return "INTEGER"
    if all(NUM.match(v) for v in present):
        return "REAL"
    return "TEXT"


def convert(value, kind):
    if value.strip() == "":
        return None
    if kind == "INTEGER":
        return int(value.strip())
    if kind == "REAL":
        return float(value.strip())
    return value


def quote(identifier):
    return '"' + identifier.replace('"', '""') + '"'


def authorizer(action, arg1, arg2, database, trigger):
    if action == sqlite3.SQLITE_FUNCTION:
        return sqlite3.SQLITE_DENY if (arg2 or "").lower() in DENIED_FUNCTIONS else sqlite3.SQLITE_OK
    return sqlite3.SQLITE_OK if action in ALLOWED else sqlite3.SQLITE_DENY


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"sql", "tables", "csv", "table", "paths", "delimiter", "infer_types", "limit"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    sql = request.get("sql")
    if not isinstance(sql, str) or not sql.strip():
        fail("sql is required: one SELECT statement")
    sources = {}
    tables = request.get("tables", {})
    if not isinstance(tables, dict) or not all(isinstance(v, str) for v in tables.values()):
        fail("tables must be an object {name: csv text}")
    sources.update(tables)
    if "csv" in request:
        if not isinstance(request["csv"], str):
            fail("csv must be a string")
        name = request.get("table", "data")
        if not isinstance(name, str):
            fail("table must be a string")
        sources[name] = request["csv"]
    paths = request.get("paths", {})
    if not isinstance(paths, dict):
        fail("paths must be an object {name: relative path}")
    for name, path in paths.items():
        sources[name] = read_path(path)
    if not sources:
        fail("give at least one table: tables, csv or paths")
    if sum(len(t) for t in sources.values()) > MAX_BYTES:
        fail("the CSVs are larger than 20 MB in total")
    for name in sources:
        if not isinstance(name, str) or not NAME.match(name) or name.lower().startswith("sqlite_"):
            fail(f"table name {name!r} must be a letter or _ followed by letters, digits or _ (and not sqlite_...)")
    if len({n.lower() for n in sources}) != len(sources):
        fail("table names must differ in more than case")
    delimiter = request.get("delimiter")
    if delimiter is not None:
        if not isinstance(delimiter, str) or len(delimiter.replace("\\t", "\t")) != 1:
            fail("delimiter must be a single character")
        delimiter = delimiter.replace("\\t", "\t")
    infer_types = request.get("infer_types", True)
    if not isinstance(infer_types, bool):
        fail("infer_types must be true or false")
    limit = request.get("limit", 1000)
    if isinstance(limit, bool) or not isinstance(limit, int) or not 1 <= limit <= 100000:
        fail("limit must be an integer from 1 to 100000")

    statement = sql.strip()
    while statement.endswith(";"):
        statement = statement[:-1].rstrip()
    if not re.match(r"^(\(\s*)*(select|with|values)\b", statement, re.I):
        fail("only one SELECT (or WITH ... SELECT, VALUES) statement is allowed")

    db = sqlite3.connect(":memory:")
    schema = {}
    for name, text in sources.items():
        columns, data, used = parse_csv(name, text, delimiter)
        kinds = [column_type([row[i] for row in data]) if infer_types else "TEXT" for i in range(len(columns))]
        db.execute(f"CREATE TABLE {quote(name)} (" + ", ".join(f"{quote(c)} {k}" for c, k in zip(columns, kinds)) + ")")
        try:
            db.executemany(f"INSERT INTO {quote(name)} VALUES (" + ", ".join("?" * len(columns)) + ")",
                           [[convert(row[i], kinds[i]) if infer_types else (row[i] if row[i] != "" else None) for i in range(len(columns))] for row in data])
        except (OverflowError, sqlite3.Error) as error:
            fail(f"table {name} could not be loaded: {error}")
        schema[name] = {"columns": [{"name": c, "type": k} for c, k in zip(columns, kinds)], "rows": len(data), "delimiter": used}
    db.commit()
    db.execute("PRAGMA query_only = ON")
    db.set_authorizer(authorizer)
    deadline = time.monotonic() + 10
    db.set_progress_handler(lambda: 1 if time.monotonic() > deadline else 0, 10000)
    try:
        cursor = db.execute(statement)
        rows = cursor.fetchmany(limit + 1)
    except sqlite3.Warning as error:
        fail(f"only one statement is allowed: {error}")
    except sqlite3.ProgrammingError as error:
        if "one statement" in str(error):
            fail(f"only one statement is allowed: {error}")
        fail(f"SQL error: {error}")
    except sqlite3.DatabaseError as error:
        message = str(error)
        if "interrupted" in message:
            fail("the query ran longer than 10 seconds")
        if "not authorized" in message or "prohibited" in message:
            fail(f"the query is not allowed (read-only: no ATTACH, PRAGMA, writes or extensions): {message}")
        fail(f"SQL error: {message}")
    except (OverflowError, ValueError, MemoryError) as error:
        fail(f"SQL error: {error}")
    columns = [d[0] for d in cursor.description] if cursor.description else []
    truncated = len(rows) > limit
    rows = rows[:limit]

    def plain(value):
        return value.hex() if isinstance(value, bytes) else value

    records = [{columns[i]: plain(v) for i, v in enumerate(row)} for row in rows]
    if len(set(columns)) != len(columns):
        records = [[plain(v) for v in row] for row in rows]
    out = io.StringIO()
    writer = csv.writer(out, lineterminator="\n")
    writer.writerow(columns)
    for row in rows:
        writer.writerow(["" if v is None else (v.hex() if isinstance(v, bytes) else v) for v in row])
    print(json.dumps({"columns": columns, "rows": records, "row_count": len(rows), "truncated": truncated, "csv": out.getvalue(),
                      "tables": schema, "sqlite_version": sqlite3.sqlite_version}))


if __name__ == "__main__":
    main()
