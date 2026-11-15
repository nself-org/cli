# Vendored wasmtime archives

The wasmtime-go v25 archives for Linux x86_64, Linux aarch64 and Windows
x86_64 contain `.llvmbc` and `.llvmcmd` sections. Go 1.26.11 and later refuse
these LLVM bitcode sections as cgo linker inputs. The macOS archives are clean.
`internal/repoqa/testdata/lto_archives.txt` records the affected archives and
the repo check compares that list against every vendored archive.

Release builds set `CGO_ENABLED=0` and do not link wasmtime. Developers building
with cgo on Linux can prepare their checkout with:

```sh
bash scripts/ci/strip-lto-bitcode.sh vendor/github.com/bytecodealliance/wasmtime-go/v25/build/linux-x86_64/libwasmtime.a
```

Use `linux-aarch64` on an arm64 machine. The script needs GNU `objcopy`, or
`llvm-objcopy` if GNU objcopy is absent. It removes only the LTO sections from
each member, preserving the machine code, and prints before/after SHA-256 sums.
The shell control checks both members, the symbol index and clean archives.
Embedded Linux CI jobs run that control and strip their job-local archive
before cgo tests. Never commit stripped archives: vendor stays identical to
upstream. Restore the archive from Git after a local cgo build.

When upstream ships clean archives, remove their entries from the list and
remove the corresponding strip steps. Windows remains listed for detection;
there is no Windows cgo CI job or Windows stripping step.
