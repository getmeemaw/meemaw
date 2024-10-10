---
sidebar_position: 2
---

# Repo structure

The Meemaw repo is organised in a few folders :

* **server:** implements the server of Meemaw including the transport layer, and uses the `utils/tss` package
* **client:** implements the SDKs for the clients of Meemaw
    * **client.go:** implements the `client` package, responsible for the common processes used by all clients, including the transport layer, and uses the `utils/tss` and `utils/tx` packages
    * **web:** 
        * **wasm:** implements the wasm module of the Meemaw Javascript SDK, using the `client` package
        * **meemaw-js:** uses the wasm module to expose the actual Meemaw Javascript SDK
    * **ios:** 
        * **ios.go:** Swift specific glue code around the `client` package
        * **meemaw-ios:** Swift package exposing the Meemaw iOS SDK, using a `gomobile` build of `ios.go` as resource
    * **android:** (empty for now)
* **utils:**
    * **tss:** contains the code for all TSS specific functions and helpers
    * **tx:** contains the code for the Ethereum transaction specific helpers
    * **types:** contains some agnostic types used in multiple packages (e.g. error types)
* **docs:** sources for the docs and blog, compiled into static pages for the website you're currently looking at
* **test:** integration tests, for example testing a full dkg process between server and client (note: unit tests live in their respective packages)

