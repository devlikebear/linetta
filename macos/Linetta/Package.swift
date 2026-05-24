// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "Linetta",
    platforms: [
        .macOS(.v15)
    ],
    products: [
        .executable(name: "Linetta", targets: ["Linetta"]),
        .library(name: "LinettaCore", targets: ["LinettaCore"])
    ],
    targets: [
        .target(name: "LinettaCore"),
        .executableTarget(
            name: "Linetta",
            dependencies: ["LinettaCore"]
        ),
        .testTarget(
            name: "LinettaCoreTests",
            dependencies: ["LinettaCore"]
        )
    ]
)
