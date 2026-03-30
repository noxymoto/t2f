# t2f
linux scripting language that compiles to asm. written in go

# T2F

T2F is an experimental compiled language that emits native assembly directly.

It is designed around a few simple ideas:

- keep the toolchain small
- stay close to the machine
- make the generated assembly inspectable
- build tiny native programs without a heavyweight runtime

Today, T2F can compile to:

- Linux x86_64 assembly
- Windows x64 assembly

It is not a production language yet. It is a fast-moving experimental systems language and runtime project.

## Why T2F Exists

Most languages either hide the low-level details completely or make native work feel heavy and indirect.

T2F tries a different path:

- a small, readable language surface
- native code generation
- direct syscall-oriented programming on Linux
- transparent output you can actually inspect

One of the nicest parts of the project is that you can look at the generated `.asm` and understand what the compiler is doing.

## Current Status

T2F already supports enough to build real programs:

- functions
- globals and locals
- `if` and `while`
- arrays and indexing
- strings
- raw memory access
- file I/O
- process spawning on Linux
- socket/server primitives on Linux
- package install/update support in the `t2f` CLI

There is also early cross-platform support:

- Linux backend: the strongest backend today
- Windows backend: core language support exists, but networking is still limited

## Project Shape

This repository currently includes:

- the compiler itself
- Linux and Windows backends
- runtime helpers
- example programs and demos
- `t2fetch`, a tiny native system info utility
- `microsh`, an experimental shell project
- web/server demos
- Micro editor syntax highlighting for `.t2f`

## Example

```t2f
let greeting = "Hello from T2F";
print_str(greeting);

fn add_one(x) {
    return x + 1;
}

print add_one(41);
```

## Building The Compiler

From the repo root:

```bash
go build -o t2f .
```

On Windows:

```powershell
go build -o t2f.exe .
```

## CLI

Current commands:

```bash
t2f compile [--target linux|windows] <file.t2f>
t2f run [--target linux|windows] <file.t2f>
t2f repl [--target linux|windows]
t2f install <path-or-git-url>
t2f update [package-name]
t2f list
```

## Basic Usage

Compile a Linux program:

```bash
t2f compile hello.t2f
./hello
```

Compile explicitly for Linux:

```bash
t2f compile --target linux hello.t2f
```

Compile explicitly for Windows:

```bash
t2f compile --target windows hello.t2f
```

## Toolchain Requirements

### Linux

You will generally want:

- `go`
- `nasm`
- `ld`

### Windows

You will generally want:

- `go`
- `nasm`
- a linker such as `gcc` or `clang`

## Generated Output

T2F writes:

- `<name>.asm`
- `<name>.o`
- the final executable

That output is part of the workflow, not an implementation detail hidden from you.

## Package Management

The `t2f` command now includes a small built-in package flow.

Right now it supports:

- installing from a local path
- installing from a git URL
- updating all installed packages
- updating one installed package
- listing installed packages

On Linux, installed binaries go to:

```bash
~/.local/bin
```

Package metadata is stored under:

```bash
~/.t2f/packages
```

This is intentionally small and simple right now. A central package index/registry is a natural next step, but the current flow is already enough to install and rebuild T2F programs as native commands.

## Notable Projects In This Repo

### `t2fetch`

A tiny native fetch utility written in T2F.

It currently shows:

- `username@computer`
- OS
- environment
- kernel
- uptime
- CPU
- shell
- memory
- root disk usage
- battery when available

It is a good example of the kind of utility T2F can already build well.

### `microsh`

An experimental shell project written in T2F.

It is not a full shell yet, but it demonstrates that T2F can already support command-driven native terminal tools.

### `website`

A Linux-side native socket/server demo showing that the language can already drive raw server-style programs.

This is still experimental and not production-hardened.

## Editor Support

There is a Micro syntax package in:

`t2f-micro-syntax/`

It provides first-pass syntax highlighting for `.t2f` files.

## What T2F Is Good At Right Now

T2F is already a good fit for:

- tiny native utilities
- terminal tools
- systems scripting experiments
- fetch tools
- simple native demos
- low-level learning and compiler experimentation

## What Is Still Weak

T2F still needs work in several important areas:

- stronger semantic analysis
- more regular memory ergonomics
- richer stdlib support
- more polished editor/tooling support
- better cross-platform parity
- more hardened networking/runtime behavior

## Philosophy

T2F is not trying to be a giant general-purpose language on day one.

It is closer to:

- an experimental native systems scripting language
- a direct-to-assembly language playground
- a small language for native tools and runtime ideas

That is the best way to understand the project today.

## Roadmap Direction

The current repository roadmap includes work in areas like:

- better types and memory ergonomics
- stronger runtime safety
- improved networking
- ARM64 support
- tooling such as formatting and debugging
- more advanced actor/distributed ideas

See `ROADMAP.txt`.

## Experimental Warning

T2F is experimental.

Expect:

- breaking changes
- evolving builtins
- backend-specific behavior
- incomplete Windows parity
- rough edges in runtime semantics

That said, it is already capable of building surprisingly fast, tiny native programs.

## Contributing

If you want to explore the project, the best way is:

1. build the compiler
2. read the example projects
3. compile a few `.t2f` programs
4. inspect the generated assembly

That workflow explains the language better than a large theory document ever could.
