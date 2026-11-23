import { describe, expect, it } from "vitest";
import { decryptWith, encryptWith, generateKeyText, importKey, isEncrypted, PREFIX } from "./crypto";

describe("text encryption", () => {
  it("round-trips unicode and marks ciphertext with a prefix", async () => {
    const key = await importKey(generateKeyText());
    const ct = await encryptWith(key, "Купить молоко 🥛");
    expect(ct.startsWith(PREFIX)).toBe(true);
    expect(isEncrypted(ct)).toBe(true);
    expect(isEncrypted("plain text")).toBe(false);
    expect(await decryptWith(key, ct)).toBe("Купить молоко 🥛");
  });

  it("uses a fresh nonce each time", async () => {
    const key = await importKey(generateKeyText());
    expect(await encryptWith(key, "a")).not.toBe(await encryptWith(key, "a"));
  });

  it("rejects the wrong key and malformed keys", async () => {
    const ct = await encryptWith(await importKey(generateKeyText()), "secret");
    await expect(decryptWith(await importKey(generateKeyText()), ct)).rejects.toThrow();
    expect(() => importKey("c2hvcnQ=")).toThrow();
  });
});
