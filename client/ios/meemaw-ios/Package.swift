// swift-tools-version: 5.6

import PackageDescription

let package = Package(
    name: "meemaw",
    platforms: [
        SupportedPlatform.iOS(.v13),
        SupportedPlatform.macOS(.v11)
    ],
    products: [
        .library(
            name: "meemaw",
            targets: ["meemaw"]),
    ],
    dependencies: [
        .package(url: "https://github.com/argentlabs/web3.swift", from: "1.1.0")
    ],
    targets: [
        .target(
            name: "meemaw",
            dependencies: [.target(name: "Tsslib"), "web3.swift"]),
        .binaryTarget(
            name: "Tsslib",
            url: "https://github.com/getmeemaw/meemaw/releases/download/v0.1.1/Tsslib.xcframework.zip",
            checksum: "b81a5a32d83403b125bdc4f6a0dbc53f2ba32eb26d1fc56f32eb147cae0a8d41")
    ]
)
