Synthetic native Telegram session fixtures. No real Telegram credentials or messages.
Auth key: bytes 0..255; user 42; DC 2; record 123. Database key/salt: bytes 0..47.
Generated independently using Python sqlite3/plistlib/hashlib and OpenSSL AES-CBC.
Encrypted SQLite page fixtures have reserve 48 (SQLCipher 3) and 80 (SQLCipher 4),
a 32-byte plaintext header, authenticated little-endian page numbers, and no WAL.

foundation-authorization.bplist uses the same synthetic key and is generated
by macOS Foundation's NSKeyedArchiver, including empty salt and attribute
collections. Regenerate on macOS (from this directory):
  clang -framework Foundation -Wno-deprecated-declarations foundation-authorization.m -o /tmp/ta-native-archive
  /tmp/ta-native-archive foundation-authorization.bplist
  /tmp/ta-native-archive foundation-mutable-authorization.bplist mutable
The mutable fixture encodes the synthetic key as NSMutableData (NS.data wrapper).
