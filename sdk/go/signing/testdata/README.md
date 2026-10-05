# signing test fixtures

Test-only material. The private key below was generated for these tests and
protects nothing; no production key is ever placed here (P7-PROD-59 owns those).

Commands that produced the files (OpenSSL 3.x):

```sh
openssl genpkey -algorithm ed25519 -out fixture-ed25519.pkcs8.txt
openssl pkey -in fixture-ed25519.pkcs8.txt -pubout -outform DER | tail -c 32 | base64 | tr -d '\n' > fixture.pub.b64
printf '%s' 'nself-signing-fixture-v1
purpose=plugins
hello' > fixture.msg
openssl pkeyutl -sign -inkey fixture-ed25519.pkcs8.txt -rawin -in fixture.msg | base64 | tr -d '\n' > fixture.sig.b64
```

- `fixture-ed25519.pkcs8.txt`: PKCS#8 PEM private key (`.txt` because the repo ignores `*.pem`).
- `fixture.pub.b64`: raw 32-byte public key, base64 standard.
- `fixture.msg`: the signed message (domain-separated: label, purpose, text).
- `fixture.sig.b64`: raw Ed25519 signature over `fixture.msg`, base64 standard.
