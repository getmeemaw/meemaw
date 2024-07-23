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
            url: "https://github.com/getmeemaw/meemaw/releases/download/v1.2.0/Tsslib.xcframework.zip",
            checksum: "0295c9719995037ebb2a91c7e2cac3d64d9051ff4d73caae2b2730655a3bef6c"
            // path: "Tsslib.xcframework" // dev
        )
    ]
)
