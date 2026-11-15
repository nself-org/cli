# Vendored wasmtime archives

The wasmtime-go v25 archives for Linux x86_64, Linux aarch64 and Windows
x86_64 contain `.llvmbc` and `.llvmcmd` sections. Go 1.26.11 and later refuse
these LLVM bitcode sections as cgo linker inputs. The macOS x86_64 and aarch64
archives also contain Mach-O `__LLVM,__bitcode` and `__LLVM,__cmdline` sections,
which the same toolchain check rejects.
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
On macOS, install LLVM and put its tools on PATH, then select the native archive:

```sh
brew install llvm
export PATH="$(brew --prefix llvm)/bin:$PATH"
bash scripts/ci/strip-lto-bitcode.sh vendor/github.com/bytecodealliance/wasmtime-go/v25/build/macos-aarch64/libwasmtime.a
```

Use `macos-x86_64` on Intel. On macOS the script requires `llvm-objcopy` and
removes `__LLVM,__bitcode` and `__LLVM,__cmdline` from every archive member.
The Linux command and its section selection are unchanged. The scanner and
shell control cover both formats, including names crossing a read boundary,
both members, the symbol index, and byte-identical clean archives.
Embedded Linux and macOS CI jobs run the control and strip their job-local
archive before cgo tests; the macOS CI smoke leg also runs the control.
Never commit stripped archives: vendor stays identical to
upstream. Restore the archive from Git after a local cgo build.

When upstream ships clean archives, remove their entries from the list and
remove the corresponding strip steps. Windows remains listed for detection;
there is no Windows cgo CI job or Windows stripping step.
