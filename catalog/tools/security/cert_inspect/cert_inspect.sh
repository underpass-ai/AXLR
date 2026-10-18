#!/usr/bin/env bash
# Inspect a PEM X.509 certificate with openssl: subject, issuer, serial, validity, SHA-256
# fingerprint, subject alternative names, key type and size, and the days left until it
# expires relative to a date given in the input (the clock is never read).
#
# Input (JSON on stdin):
#   pem   the certificate in PEM form, newlines as JSON \n escapes (the first certificate is used)
#   now   "YYYY-MM-DD" or "YYYY-MM-DDTHH:MM:SSZ" (UTC), required for days_to_expiry
# Output: one JSON object. Usage errors print {"error": ...} and exit 2.
set -u
export LC_ALL=C

fail() {
  printf '{"error": %s}\n' "$(json_str "$1")"
  exit 2
}

# JSON string literal: backslashes and quotes escaped, tabs and newlines as escapes,
# other control characters dropped.
json_str() {
  local s=$1
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\t'/\\t}
  s=${s//$'\r'/}
  s=${s//$'\n'/\\n}
  s=$(printf '%s' "$s" | tr -d '\000-\010\013-\037')
  printf '"%s"' "$s"
}

# The raw (still escaped) value of a top-level string field, or nothing.
field() {
  printf '%s' "$flat" | sed -n -E 's/.*"'"$1"'"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)".*/\1/p'
}

command -v openssl >/dev/null 2>&1 || fail "openssl is not installed"
input=$(cat)
flat=$(printf '%s' "$input" | tr '\n\r' '  ')
case "$flat" in
  *[![:space:]]*) ;;
  *) fail "stdin is empty; expected a JSON object with pem" ;;
esac
trimmed=$(printf '%s' "$flat" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')
case "$trimmed" in
  \{*\}) ;;
  *) fail "stdin is not a JSON object" ;;
esac

raw_pem=$(field pem)
[ -n "$raw_pem" ] || fail "pem is required: a PEM certificate as a JSON string"
# JSON unescape for the characters a PEM can hold: \n, \r, \/ and \\.
pem=$(printf '%s' "$raw_pem" | sed -e 's/\\r//g' -e 's/\\n/\n/g' -e 's/\\\//\//g' -e 's/\\\\/\\/g')
case "$pem" in
  *"-----BEGIN CERTIFICATE-----"*"-----END CERTIFICATE-----"*) ;;
  *) fail "pem must contain a -----BEGIN CERTIFICATE----- ... -----END CERTIFICATE----- block" ;;
esac
# Keep only the first certificate block.
pem=$(printf '%s\n' "$pem" | sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' | sed '/-----END CERTIFICATE-----/q')

now=$(field now)
if [ -z "$now" ]; then
  if printf '%s' "$flat" | grep -q '"now"[[:space:]]*:'; then
    fail "now must be a string YYYY-MM-DD or YYYY-MM-DDTHH:MM:SSZ"
  fi
  fail "now is required (YYYY-MM-DD or YYYY-MM-DDTHH:MM:SSZ, UTC); the clock is never read"
fi
if printf '%s' "$now" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'; then
  now_text="$now 00:00:00"
elif printf '%s' "$now" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'; then
  now_text=$(printf '%s' "$now" | sed -e 's/T/ /' -e 's/Z$//')
else
  fail "now must be YYYY-MM-DD or YYYY-MM-DDTHH:MM:SSZ (UTC)"
fi
now_epoch=$(date -u -d "$now_text UTC" +%s 2>/dev/null) || fail "now is not a valid date"
case "$(date -u -d "@$now_epoch" +%Y-%m-%d)" in
  "${now:0:10}") ;;
  *) fail "now is not a valid calendar date" ;;
esac

x509() {
  printf '%s\n' "$pem" | openssl x509 -noout "$@" 2>/dev/null
}

fields=$(x509 -nameopt RFC2253 -subject -issuer -serial -startdate -enddate) ||
  fail "openssl could not parse the certificate (not a valid X.509 PEM)"
value_of() {
  printf '%s\n' "$fields" | sed -n "s/^$1=//p" | head -n 1
}
subject=$(value_of subject)
issuer=$(value_of issuer)
serial=$(value_of serial)
not_before=$(value_of notBefore)
not_after=$(value_of notAfter)
# The fingerprint label differs between openssl versions ("SHA256 Fingerprint", "sha256 Fingerprint").
fingerprint=$(x509 -fingerprint -sha256 | sed -n 's/^[^=]*=//p' | head -n 1)
text=$(x509 -text)
key_algorithm=$(printf '%s\n' "$text" | sed -n 's/^[[:space:]]*Public Key Algorithm:[[:space:]]*//p' | head -n 1)
key_bits=$(printf '%s\n' "$text" | sed -n -E 's/^[[:space:]]*(RSA |EC |DSA )?Public-Key:[[:space:]]*\(([0-9]+) bit\).*/\2/p' | head -n 1)
curve=$(printf '%s\n' "$text" | sed -n 's/^[[:space:]]*NIST CURVE:[[:space:]]*//p' | head -n 1)
[ -n "$curve" ] || curve=$(printf '%s\n' "$text" | sed -n 's/^[[:space:]]*ASN1 OID:[[:space:]]*//p' | head -n 1)
signature_algorithm=$(printf '%s\n' "$text" | sed -n 's/^[[:space:]]*Signature Algorithm:[[:space:]]*//p' | head -n 1)
version=$(printf '%s\n' "$text" | sed -n -E 's/^[[:space:]]*Version:[[:space:]]*([0-9]+).*/\1/p' | head -n 1)
# SANs: the indented line after the extension header; "No extensions" or nothing when absent.
san_line=$(x509 -ext subjectAltName | sed -n '2,$p' | sed -E 's/^[[:space:]]+//' | tr '\n' ' ')
is_ca=false
if printf '%s\n' "$text" | grep -q 'CA:TRUE'; then is_ca=true; fi

case "$key_algorithm" in
  rsaEncryption) key_type=RSA ;;
  id-ecPublicKey) key_type=EC ;;
  ED25519) key_type=Ed25519; key_bits=${key_bits:-256} ;;
  ED448) key_type=Ed448; key_bits=${key_bits:-456} ;;
  dsaEncryption) key_type=DSA ;;
  *) key_type=$key_algorithm ;;
esac

before_epoch=$(date -u -d "$not_before" +%s 2>/dev/null) || fail "could not read notBefore: $not_before"
after_epoch=$(date -u -d "$not_after" +%s 2>/dev/null) || fail "could not read notAfter: $not_after"
iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; }
seconds_left=$((after_epoch - now_epoch))
# Whole days, rounded down (floor, also for negative values).
if [ "$seconds_left" -ge 0 ]; then
  days_left=$((seconds_left / 86400))
else
  days_left=$(( -((-seconds_left + 86399) / 86400) ))
fi
validity_days=$(( (after_epoch - before_epoch) / 86400 ))
expired=false; not_yet_valid=false; status=valid
if [ "$now_epoch" -gt "$after_epoch" ]; then expired=true; status=expired; fi
if [ "$now_epoch" -lt "$before_epoch" ]; then not_yet_valid=true; status=not_yet_valid; fi
self_issued=false
[ "$subject" = "$issuer" ] && self_issued=true

sans="["
first=1
if [ -n "$san_line" ] && [ "${san_line#No extensions}" = "$san_line" ]; then
  IFS=',' read -r -a items <<< "$san_line"
  for item in "${items[@]}"; do
    item=$(printf '%s' "$item" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')
    [ -n "$item" ] || continue
    [ $first -eq 1 ] || sans+=", "
    sans+=$(json_str "$item")
    first=0
  done
fi
sans+="]"

warnings="["
add_warning() {
  [ "$warnings" = "[" ] || warnings+=", "
  warnings+=$(json_str "$1")
}
[ "$expired" = true ] && add_warning "the certificate has expired"
[ "$not_yet_valid" = true ] && add_warning "the certificate is not valid yet"
if [ "$status" = valid ] && [ "$days_left" -lt 30 ]; then add_warning "expires in less than 30 days"; fi
if [ "$key_type" = RSA ] && [ -n "$key_bits" ] && [ "$key_bits" -lt 2048 ]; then add_warning "RSA key shorter than 2048 bits"; fi
case "$signature_algorithm" in
  *md5*|*MD5*|sha1With*|ecdsa-with-SHA1) add_warning "weak signature algorithm $signature_algorithm" ;;
esac
[ "$sans" = "[]" ] && add_warning "no subjectAltName: browsers ignore the CN for host names"
warnings+="]"

num_or_null() { if [ -n "$1" ]; then printf '%s' "$1"; else printf 'null'; fi; }
str_or_null() { if [ -n "$1" ]; then json_str "$1"; else printf 'null'; fi; }

printf '{'
printf '"subject": %s, ' "$(json_str "$subject")"
printf '"issuer": %s, ' "$(json_str "$issuer")"
printf '"serial": %s, ' "$(json_str "$serial")"
printf '"version": %s, ' "$(num_or_null "$version")"
printf '"not_before": %s, ' "$(json_str "$(iso "$before_epoch")")"
printf '"not_after": %s, ' "$(json_str "$(iso "$after_epoch")")"
printf '"validity_days": %s, ' "$validity_days"
printf '"now": %s, ' "$(json_str "$(iso "$now_epoch")")"
printf '"days_to_expiry": %s, ' "$days_left"
printf '"status": "%s", "expired": %s, "not_yet_valid": %s, ' "$status" "$expired" "$not_yet_valid"
printf '"sha256_fingerprint": %s, ' "$(json_str "$fingerprint")"
printf '"subject_alt_names": %s, ' "$sans"
printf '"key": {"type": %s, "bits": %s, "curve": %s}, ' "$(str_or_null "$key_type")" "$(num_or_null "$key_bits")" "$(str_or_null "$curve")"
printf '"signature_algorithm": %s, ' "$(str_or_null "$signature_algorithm")"
printf '"self_issued": %s, "ca": %s, ' "$self_issued" "$is_ca"
printf '"warnings": %s, ' "$warnings"
printf '"note": "Fields as read by openssl x509; the signature chain and revocation are not checked."'
printf '}\n'
