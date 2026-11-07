// Seals and opens Linetta Works sync blobs with Apple's CryptoKit, so the iOS
// build encrypts with the operating system's own implementation. The engine
// (engine/internal/vault/aead_cryptokit.go) calls these two functions; the
// iOS build compiles this file into the app (apps/desktop/src-tauri/build.rs).
//
// The layout is the one Go's cipher.NewGCMWithRandomNonce writes, which is
// also CryptoKit's AES.GCM.SealedBox.combined: a 12-byte nonce, the
// ciphertext, then the 16-byte tag. A blob sealed on any device opens on any
// other. scripts/test-cryptokit-aead.sh checks that against Go's AES-GCM.
//
// Both return the number of bytes written to out, or -1.

import CryptoKit
import Foundation

private func bytes(_ pointer: UnsafePointer<UInt8>?, _ count: Int) -> Data {
    guard let pointer, count > 0 else { return Data() }
    return Data(bytes: pointer, count: count)
}

private func write(_ data: Data, _ out: UnsafeMutablePointer<UInt8>?, _ capacity: Int) -> Int {
    guard data.count <= capacity else { return -1 }
    if data.isEmpty { return 0 }
    guard let out else { return -1 }
    data.copyBytes(to: out, count: data.count)
    return data.count
}

@_cdecl("linetta_cryptokit_seal")
public func linettaCryptoKitSeal(
    _ key: UnsafePointer<UInt8>?, _ keyLength: Int,
    _ plain: UnsafePointer<UInt8>?, _ plainLength: Int,
    _ aad: UnsafePointer<UInt8>?, _ aadLength: Int,
    _ out: UnsafeMutablePointer<UInt8>?, _ outCapacity: Int
) -> Int {
    guard keyLength == 32 else { return -1 }
    let symmetric = SymmetricKey(data: bytes(key, keyLength))
    guard let box = try? AES.GCM.seal(bytes(plain, plainLength), using: symmetric, authenticating: bytes(aad, aadLength)),
          let combined = box.combined
    else { return -1 }
    return write(combined, out, outCapacity)
}

@_cdecl("linetta_cryptokit_open")
public func linettaCryptoKitOpen(
    _ key: UnsafePointer<UInt8>?, _ keyLength: Int,
    _ sealed: UnsafePointer<UInt8>?, _ sealedLength: Int,
    _ aad: UnsafePointer<UInt8>?, _ aadLength: Int,
    _ out: UnsafeMutablePointer<UInt8>?, _ outCapacity: Int
) -> Int {
    guard keyLength == 32 else { return -1 }
    let symmetric = SymmetricKey(data: bytes(key, keyLength))
    guard let box = try? AES.GCM.SealedBox(combined: bytes(sealed, sealedLength)),
          let plain = try? AES.GCM.open(box, using: symmetric, authenticating: bytes(aad, aadLength))
    else { return -1 }
    return write(plain, out, outCapacity)
}
