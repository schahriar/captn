// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "multidep",
    targets: [
        .target(name: "Dep1"),
        .executableTarget(name: "App", dependencies: ["Dep1"])
    ]
)
