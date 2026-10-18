#!/usr/bin/env python3
"""Estimate the entropy of a password given in the input: the character-pool bound
(length x log2(pool)) and a pattern-adjusted estimate that charges common passwords,
keyboard runs, sequences, repeats, dates, l33t substitutions and context words at their
real guessing cost; crack-time estimates at stated guess rates; and NIST SP 800-63B advice.

The password and every substring of it are NEVER written to the output: findings carry the
pattern type, its position and its length only.

Input (JSON on stdin):
  password           the password (string, at most 4096 characters)
  context            words specific to the user or the service (name, e-mail, site), optional
  guesses_per_second extra guess rates to estimate, {label: rate}, optional
"""
import json
import math
import pathlib
import sys
import unicodedata

HERE = pathlib.Path(__file__).resolve().parent
KEYBOARD_ROWS = ["`1234567890-=", "qwertyuiop[]\\", "asdfghjkl;'", "zxcvbnm,./", "1qaz2wsx3edc4rfv5tgb6yhn7ujm8ik9ol0p",
                 "qazwsxedcrfvtgbyhnujmikolp", "azertyuiop", "qwertzuiop", "yxcvbnm"]
SEQUENCES = ["abcdefghijklmnopqrstuvwxyz", "0123456789"]
LEET = {"4": "a", "@": "a", "8": "b", "(": "c", "3": "e", "6": "g", "9": "g", "1": "i", "!": "i", "|": "l", "0": "o",
        "5": "s", "$": "s", "7": "t", "+": "t", "2": "z", "%": "x"}
SCENARIOS = [
    ("online_throttled", 100 / 3600, "online attack against a rate-limited login (100 guesses per hour)"),
    ("online_unthrottled", 10, "online attack without rate limiting (10 guesses per second)"),
    ("offline_slow_hash", 1e4, "offline attack on a slow hash such as bcrypt, scrypt or Argon2 (10^4 per second)"),
    ("offline_fast_hash", 1e10, "offline attack on a fast unsalted hash on GPUs (10^10 per second)"),
]


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load_common():
    ranks = {}
    try:
        lines = (HERE / "common_passwords.txt").read_text(encoding="utf-8").splitlines()
    except OSError:
        return ranks, {}
    source = {}
    for line in lines:
        if line.startswith("#"):
            text = line.lstrip("# ").strip()
            if text.startswith("Source:"):
                source["source"] = text[len("Source:"):].strip().rstrip(",")
            elif text.startswith("https://"):
                source["url"] = text
            elif text.startswith("Fetched"):
                source["fetched"] = text.split()[1]
            continue
        rank, _, word = line.partition("\t")
        if word and rank.isdigit():
            ranks.setdefault(word, int(rank))
    return ranks, source


def pool_of(chars):
    classes = {"lowercase": 0, "uppercase": 0, "digits": 0, "symbols": 0, "space": 0, "other_unicode": 0}
    for char in chars:
        if "a" <= char <= "z":
            classes["lowercase"] += 1
        elif "A" <= char <= "Z":
            classes["uppercase"] += 1
        elif "0" <= char <= "9":
            classes["digits"] += 1
        elif char == " ":
            classes["space"] += 1
        elif ord(char) < 128 and char.isprintable():
            classes["symbols"] += 1
        else:
            classes["other_unicode"] += 1
    sizes = {"lowercase": 26, "uppercase": 26, "digits": 10, "symbols": 32, "space": 1, "other_unicode": 100}
    pool = sum(sizes[name] for name, count in classes.items() if count)
    return classes, pool


def unleet_variants(text):
    """The lower-cased text with l33t characters replaced; '1' is tried as both i and l."""
    base = "".join(LEET.get(c, c) for c in text)
    variants = {base}
    if "1" in text:
        variants.add("".join("l" if c == "1" else LEET.get(c, c) for c in text))
    return variants


def matches(chars, common, context):
    """Candidate patterns: (start, end, kind, bits)."""
    n = len(chars)
    lower = "".join(chars).lower()
    found = []
    # Common passwords and context words, plain or with l33t substitutions, case changes cost one bit.
    for i in range(n):
        for j in range(i + 4, min(n, i + 32) + 1):
            piece = lower[i:j]
            original = "".join(chars[i:j])
            case_bit = 1 if original != piece else 0
            for word, leet in [(piece, False)] + [(v, True) for v in unleet_variants(piece) if v != piece]:
                if word in common:
                    found.append((i, j, "common_password" if not leet else "common_password_l33t",
                                  math.log2(common[word]) + case_bit + (1 if leet else 0) + 1))
                if word in context:
                    found.append((i, j, "context_word" if not leet else "context_word_l33t",
                                  math.log2(len(context) + 1) + case_bit + (1 if leet else 0) + 1))
    # Keyboard runs and alphabetic or numeric sequences, forwards or backwards, 4 or more.
    for kind, lines, starts in (("keyboard_run", KEYBOARD_ROWS, 47), ("sequence", SEQUENCES, 36)):
        for line in lines:
            for direction in (line, line[::-1]):
                i = 0
                while i < n:
                    j = i
                    while j < n and j - i < len(direction):
                        if j == i:
                            if lower[j] not in direction:
                                break
                        else:
                            position = direction.find(lower[j - 1])
                            if position < 0 or position + 1 >= len(direction) or direction[position + 1] != lower[j]:
                                break
                        j += 1
                    if j - i >= 4:
                        found.append((i, j, kind, math.log2(starts * 2) + math.log2(j - i)))
                        i = j
                    else:
                        i += 1
    # Repeats: one character three or more times, or a block repeated.
    i = 0
    while i < n:
        j = i
        while j < n and chars[j] == chars[i]:
            j += 1
        if j - i >= 3:
            found.append((i, j, "repeated_character", math.log2(pool_of(chars[i])[1] or 1) + math.log2(j - i)))
        i = max(j, i + 1)
    for size in range(2, n // 2 + 1):
        for i in range(0, n - 2 * size + 1):
            block = chars[i:i + size]
            j = i + size
            while j + size <= n and chars[j:j + size] == block:
                j += size
            if j - i >= 2 * size and len(set(block)) > 1:
                found.append((i, j, "repeated_block", size * math.log2(pool_of(block)[1] or 1) + math.log2((j - i) // size)))
    # Dates and years: 19xx or 20xx, with or without day and month, with or without separators.
    digits = "".join(chars)
    for i in range(n):
        for length in (10, 8, 6, 4):
            piece = digits[i:i + length]
            if len(piece) != length:
                continue
            kind = date_kind(piece)
            if kind:
                found.append((i, i + length, kind, math.log2(200) if kind == "year" else math.log2(366 * (100 if length == 6 else 200)) + (2 if length == 10 else 0)))
    return found


def date_kind(piece):
    if len(piece) == 4 and piece.isdigit():
        return "year" if 1900 <= int(piece) <= 2099 else None
    stripped = piece
    if len(piece) == 10 and piece[2] == piece[5] and piece[2] in "-/._" or len(piece) == 10 and piece[4] == piece[7] and piece[4] in "-/._":
        stripped = piece.replace(piece[2] if piece[2] in "-/._" else piece[4], "")
    if not stripped.isdigit():
        return None
    if len(stripped) == 8:
        y1, m1, d1 = int(stripped[:4]), int(stripped[4:6]), int(stripped[6:])
        y2, m2, d2 = int(stripped[4:]), int(stripped[2:4]), int(stripped[:2])
        y3, m3, d3 = int(stripped[4:]), int(stripped[:2]), int(stripped[2:4])
        if (1900 <= y1 <= 2099 and 1 <= m1 <= 12 and 1 <= d1 <= 31) or any(1900 <= y <= 2099 and 1 <= m <= 12 and 1 <= d <= 31 for y, m, d in ((y2, m2, d2), (y3, m3, d3))):
            return "date"
    if len(stripped) == 6 and len(piece) == 6:
        a, b, c = int(piece[:2]), int(piece[2:4]), int(piece[4:])
        if (1 <= b <= 12 and 1 <= a <= 31) or (1 <= a <= 12 and 1 <= b <= 31) or (1 <= b <= 12 and 1 <= c <= 31):
            return "date_short"
    return None


def choose(found, per_char):
    """Non-overlapping patterns that save the most bits, greedily."""
    scored = sorted(((max(0.0, (end - start) * per_char - bits), start, end, kind, bits) for start, end, kind, bits in found),
                    key=lambda item: (-item[0], item[1]))
    taken, used = [], set()
    for saving, start, end, kind, bits in scored:
        if saving <= 0 or any(k in used for k in range(start, end)):
            continue
        used.update(range(start, end))
        taken.append({"type": kind, "start": start, "length": end - start, "bits": round(bits, 2), "saving_bits": round(saving, 2)})
    return sorted(taken, key=lambda item: item["start"])


def duration(seconds):
    if seconds < 1:
        return "less than a second"
    units = [("century", 100 * 365.25 * 86400), ("year", 365.25 * 86400), ("month", 30.44 * 86400), ("day", 86400),
             ("hour", 3600), ("minute", 60), ("second", 1)]
    for name, size in units:
        if seconds >= size:
            value = seconds / size
            if name == "century" and value >= 1e6:
                return f"{value:.2e} centuries"
            whole = int(value)
            plural = "centuries" if name == "century" else name + "s"
            return f"{whole} {name if whole == 1 else plural}"
    return "less than a second"


def rating(bits):
    for limit, label in ((28, "very weak"), (36, "weak"), (60, "fair"), (80, "strong")):
        if bits < limit:
            return label
    return "very strong"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError:
        fail("stdin is not a JSON object")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"password", "context", "guesses_per_second"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    password = request.get("password")
    if not isinstance(password, str):
        fail("password is required (a string)")
    if not password:
        fail("password is empty")
    if len(password) > 4096:
        fail("password is longer than 4096 characters")
    context = request.get("context", [])
    if not isinstance(context, list) or not all(isinstance(word, str) for word in context):
        fail("context must be a list of strings")
    context_words = set()
    for word in context:
        for part in [word] + word.replace("@", " ").replace(".", " ").replace("_", " ").replace("-", " ").split():
            if len(part.strip()) >= 4:
                context_words.add(part.strip().lower())
    rates = list(SCENARIOS)
    extra = request.get("guesses_per_second", {})
    if not isinstance(extra, dict):
        fail("guesses_per_second must be an object {label: guesses per second}")
    for label, rate in extra.items():
        if isinstance(rate, bool) or not isinstance(rate, (int, float)) or not rate > 0 or len(label) > 64:
            fail("each guesses_per_second value must be a positive number and each label at most 64 characters")
        rates.append((label, float(rate), "given in the input"))

    # SP 800-63B counts each Unicode code point as one character, after normalisation.
    normalized = unicodedata.normalize("NFKC", password)
    chars = list(normalized)
    common, source = load_common()
    classes, pool = pool_of(chars)
    per_char = math.log2(pool)
    pool_bits = len(chars) * per_char
    scanned = chars[:256]  # patterns are searched in the first 256 characters
    patterns = choose(matches(scanned, common, context_words), per_char)
    adjusted = max(pool_bits - sum(p["saving_bits"] for p in patterns), 0.0)
    whole_common = normalized.lower() in common
    if whole_common:
        adjusted = min(adjusted, math.log2(common[normalized.lower()]) + 1)
    estimate = adjusted

    advice = []
    if len(chars) < 8:
        advice.append("Too short: SP 800-63B requires at least 8 characters, and 15 when the password is the only authentication factor (rev. 4).")
    elif len(chars) < 15:
        advice.append("SP 800-63B rev. 4 asks for at least 15 characters when the password is the only factor; 8 is the minimum with a second factor.")
    if whole_common or any(p["type"].startswith("common_password") for p in patterns):
        advice.append("Contains a common or previously breached password: SP 800-63B requires verifiers to reject passwords found in such blocklists.")
    if any(p["type"] in ("repeated_character", "repeated_block", "sequence", "keyboard_run") for p in patterns):
        advice.append("Contains repetitive or sequential characters (SP 800-63B lists these among the values to reject).")
    if any(p["type"].startswith("context_word") for p in patterns):
        advice.append("Contains a context-specific word (user name, service name): SP 800-63B lists these among the values to reject.")
    if any(p["type"] in ("date", "date_short", "year") for p in patterns):
        advice.append("Contains a date or year: dates are among the first things guessed.")
    if any(p["type"].endswith("_l33t") for p in patterns):
        advice.append("Character substitutions (a->@, o->0) add almost nothing: crackers try them by rule.")
    if estimate < 60:
        advice.append("Prefer a long passphrase of several random words, or a random password from a password manager.")
    advice.append("SP 800-63B: no composition rules, no periodic changes without evidence of compromise, allow at least 64 characters and paste from password managers.")

    crack = {}
    for label, rate, description in rates:
        try:
            seconds = (2 ** estimate) / 2 / rate
        except OverflowError:
            seconds = float("inf")
        crack[label] = {"guesses_per_second": float(f"{rate:.6g}"), "description": description,
                        "seconds": float(f"{seconds:.4g}") if math.isfinite(seconds) else None,
                        "display": duration(seconds) if math.isfinite(seconds) else "longer than the age of the universe"}

    print(json.dumps({
        "length": len(chars),
        "normalized": normalized != password,
        "character_classes": {name: count > 0 for name, count in classes.items()},
        "pool_size": pool,
        "bits_per_character": round(per_char, 4),
        "pool_entropy_bits": round(pool_bits, 2),
        "estimated_entropy_bits": round(estimate, 2),
        "rating": rating(estimate),
        "whole_password_is_common": whole_common,
        "patterns": patterns,
        "crack_time": crack,
        "advice": advice,
        "method": "pool bound = length x log2(pool); estimate replaces each detected pattern by its guessing cost (zxcvbn-style); crack time = 2^(bits-1) / rate (average case)",
        "blocklist": dict({"entries": len(common)}, **(source or {})),
        "note": "The password is not echoed: patterns report only their type, position and length.",
    }))


if __name__ == "__main__":
    main()
