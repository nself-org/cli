Interop fixture for contract plugins.release-signature v1.1 (platform signatures). Made by
scripts/release/platform-signatures-test.sh --regen-testdata with scripts/release/sign.sh and a THROWAWAY key (fixture.pub is its
public half; the private half was discarded). Not a release key: it is not in scripts/release/keys/ and no CLI pins it.

- demo-1.0.0.tar.gz / demo-1.0.0-linux-amd64.tar.gz: stub tarballs. releases.json: the release-data rows for them.
- vectors.json: slug, version, platform, tarball sha256, signature object, and cases (each overrides fields of the vector and says
  whether the signature must verify). A verifier recomputes the message
  `nself-plugin-bin-v1\n<slug>\n<version>\n<platform>\n<sha256 hex of the platform tarball>\n` from the case, and verifies the raw
  Ed25519 signature (base64 `sig`) with fixture.pub. The `tarball_append_byte` case verifies the signature over the sha256 of the
  tarball with one byte appended; `signature: source` uses `source_signature` as the platform signature.

Copied unchanged into the cli (P7-PLUG-84) from nself-org/plugins scripts/release/testdata/platform-v1/ at plugins main 839d28593b2409647f172553274791852aacd8b3 (plugins#414, P7-PLUG-83). platform_test.go TestPlatformInteropSignSh verifies these vectors with signature.VerifyPlatform.
