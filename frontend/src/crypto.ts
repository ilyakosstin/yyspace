/**
 * Client-side encryption of text content (titles, descriptions, timer names).
 * AES-256-GCM; the key is generated in the browser and never sent to the server.
 * Encrypted values are stored as `enc:v1:` + base64(iv || ciphertext), so they can be told apart
 * from older plaintext values, which are shown as they are.
 */

export const PREFIX = "enc:v1:";
export const LOCKED = "🔒 Encrypted (add your key in Settings)";
export const BROKEN = "🔒 Can’t decrypt (wrong key?)";

const KEY_STORE = "fh.key";
const ENABLED_STORE = "fh.encrypt";
const IV_BYTES = 12;

const b64 = (bytes: Uint8Array): string => btoa(String.fromCharCode(...bytes));
const unb64 = (s: string): Uint8Array<ArrayBuffer> => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

export const isEncrypted = (s: string | null | undefined): boolean => !!s && s.startsWith(PREFIX);

export function importKey(rawB64: string): Promise<CryptoKey> {
  const raw = unb64(rawB64.trim());
  if (raw.length !== 32) throw new Error("A key is 32 bytes (44 characters of base64)");
  return crypto.subtle.importKey("raw", raw, "AES-GCM", false, ["encrypt", "decrypt"]);
}

export function generateKeyText(): string {
  return b64(crypto.getRandomValues(new Uint8Array(32)));
}

export async function encryptWith(key: CryptoKey, text: string): Promise<string> {
  const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES));
  const ct = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, new TextEncoder().encode(text)));
  const out = new Uint8Array(IV_BYTES + ct.length);
  out.set(iv);
  out.set(ct, IV_BYTES);
  return PREFIX + b64(out);
}

/** Throws if the value is not valid ciphertext for this key. */
export async function decryptWith(key: CryptoKey, value: string): Promise<string> {
  const data = unb64(value.slice(PREFIX.length));
  const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv: data.slice(0, IV_BYTES) }, key, data.slice(IV_BYTES));
  return new TextDecoder().decode(pt);
}

// --- the browser's own key ------------------------------------------------

let keyText: string | null = null;
let key: CryptoKey | null = null;
let enabled = false;

const read = (k: string): string | null => {
  try {
    return localStorage.getItem(k);
  } catch {
    return null;
  }
};
const write = (k: string, v: string | null) => {
  try {
    if (v === null) localStorage.removeItem(k);
    else localStorage.setItem(k, v);
  } catch {
    /* storage unavailable: the key lasts until the tab closes */
  }
};

export const available = (): boolean => typeof crypto !== "undefined" && !!crypto.subtle;
export const hasKey = (): boolean => key !== null;
export const getKeyText = (): string | null => keyText;
export const isEnabled = (): boolean => enabled && key !== null;

export const ready: Promise<void> = (async () => {
  const saved = read(KEY_STORE);
  if (saved && available()) {
    try {
      key = await importKey(saved);
      keyText = saved;
    } catch {
      /* corrupt key: treated as absent */
    }
  }
  enabled = read(ENABLED_STORE) === "1";
})();

export async function useKeyText(text: string): Promise<void> {
  key = await importKey(text);
  keyText = text.trim();
  write(KEY_STORE, keyText);
  setEnabled(true);
}

export function setEnabled(on: boolean): void {
  enabled = on;
  write(ENABLED_STORE, on ? "1" : "0");
}

export function removeKey(): void {
  key = null;
  keyText = null;
  enabled = false;
  write(KEY_STORE, null);
  write(ENABLED_STORE, null);
}

/** Encrypts when encryption is on; empty values stay empty. */
export async function seal(text: string): Promise<string>;
export async function seal(text: string | null | undefined): Promise<string | null | undefined>;
export async function seal(text: string | null | undefined) {
  return key && enabled && text ? encryptWith(key, text) : text;
}

/** Plaintext passes through unchanged, so old unencrypted data keeps working. */
export async function open(value: string): Promise<string>;
export async function open(value: string | null): Promise<string | null>;
export async function open(value: string | null) {
  if (!isEncrypted(value)) return value;
  if (!key) return LOCKED;
  try {
    return await decryptWith(key, value!);
  } catch {
    return BROKEN;
  }
}
