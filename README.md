# wasm-runtime

Install `wasm-pack` from:

https://rustwasm.github.io/wasm-pack/installer/#

Compile binaries in `testing/rust_scripts/hello`:

    wasm-pack build --target web && cp target/wasm32-unknown-unknown/release/hello_wasm.wasm ../testdata/
