import './wasm_exec.js';

const localStorageKeys = {
    dkgResult: 'dkgResult',
    address: 'address'
};

export class Wallet {
    constructor(host, dkgResult, address, authData) {
        this.host = host;
        this.dkgResult = dkgResult;
        this.address = address;
        this.authData = authData;
    }

    From() {
        return this.address;
    }


    // SignEthTransaction signs an Ethereum transaction (formatted as a json of parameters) using TSS
    async SignEthTransaction(raw, chainId) {
        const dkgResult = this.dkgResult;

        let signedTx = ""

        try {
            signedTx = await window.SignEthTransaction(this.host, JSON.stringify(raw), dkgResult, this.authData, String(chainId));
        } catch (error) {
            console.log("error while signing tx:", error)
            throw error;
        }

        return "0x"+signedTx;
    }

    // SignBytes signs hex encoded bytes using TSS
    async SignBytes(raw) {
        const dkgResult = this.dkgResult;

        if (!(/^0x[0-9a-fA-F]+$/i.test(raw))) {
            throw new Error("Incorrect format. Requires hex encoded data.");
        }

        let signature = ""

        try {
            signature = await window.SignBytes(this.host, raw, dkgResult, this.authData);
        } catch (error) {
            console.log("error while signing tx:", error)
            throw error;
        }

        return "0x"+signature;
    }

    // Recover recovers the private key based on client and server shares
    async Recover() {

        let privateKey = ""

        try {
            privateKey = await window.Recover(this.host, this.dkgResult, this.authData);
        } catch (error) {
            console.log("error while recovering private key:", error)
            throw error;
        }

        return "0x"+privateKey;
    }
}

export default class Meemaw {

    constructor(serverUrl, wasmModule, go) {
        this.host = serverUrl;
        this.wasmModule = wasmModule;
        this.go = go;
    }

    static async init(serverUrl, wasmUrl = '') {
        if (wasmUrl === '') {
            var myWasmUrl = new URL("/meemaw.wasm", serverUrl);
            wasmUrl = myWasmUrl.toString()
        }

        const go = new Go();
        const wasmModule = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);
        go.run(wasmModule.instance);
        console.log("wasm loaded");
        return new Meemaw(serverUrl, wasmModule, go);
    }

    // GetWallet returns the wallet if it exists or creates a new one
    async GetWallet(authData) {
        if (!authData) {
            throw new Error('authData is empty');
        }

        // Check if wallet already exists
        let userId;
        try {
            userId = await window.Identify(this.host, authData)
        } catch (error) {
            console.log("error getting userId:", error)
            throw error;
        }

        const dkgResult = window.localStorage.getItem(localStorageKeys.dkgResult+"-"+userId);
        const address = window.localStorage.getItem(localStorageKeys.address+"-"+userId);

        // If it does, return the wallet
        if (dkgResult !== null && address !== null) {
            console.log("Loading existing wallet")
            return new Wallet(this.host, dkgResult, address, authData);
        }

        console.log("Creating new wallet")

        // Else, DKG
        try {
            const newDkgResult = await window.Dkg(this.host, authData);

            const parsedDkgResult = JSON.parse(newDkgResult);
            const addr = parsedDkgResult.Address;

            this.storeDkgResults(userId, newDkgResult, addr);
            return new Wallet(this.host, newDkgResult, addr, authData);
        } catch (error) {
            console.log("error while dkg:", error)
            throw error;
        }
    }

    storeDkgResults(userId, dkgResult, address) {
        window.localStorage.setItem(localStorageKeys.dkgResult+"-"+userId, dkgResult);
        window.localStorage.setItem(localStorageKeys.address+"-"+userId, address);
    }
}