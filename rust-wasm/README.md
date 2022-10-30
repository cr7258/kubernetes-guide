
create a new project
```bash
cargo new --lib mywasm
```

compile
```bash
wasm-pack build --target nodejs
```
call wasm module with nodejs
```bash
node test.js
```