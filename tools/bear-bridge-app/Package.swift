// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "BearBridge",
    platforms: [
        .macOS(.v13),
    ],
    targets: [
        .executableTarget(
            name: "BearBridge",
            path: "Sources/BearBridge"
        ),
        .testTarget(
            name: "BearBridgeTests",
            dependencies: ["BearBridge"],
            path: "Tests/BearBridgeTests"
        ),
    ]
)
