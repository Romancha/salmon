import Foundation
import Security

/// Errors from Keychain operations.
enum KeychainError: Error, Equatable {
    case unexpectedData
    case osStatus(Int32)
}

/// Protocol for Keychain operations, enabling test mocking.
protocol KeychainServiceProtocol {
    func save(key: String, value: String) throws
    func load(key: String) -> String?
    func delete(key: String)
}

/// Stores and retrieves secrets from the macOS Keychain.
final class KeychainService: KeychainServiceProtocol {

    private let service: String

    /// - Parameter service: The Keychain service name (defaults to the app bundle ID).
    init(service: String = Bundle.main.bundleIdentifier ?? "com.salmon-run") {
        self.service = service
    }

    func save(key: String, value: String) throws {
        guard let data = value.data(using: .utf8) else {
            throw KeychainError.unexpectedData
        }

        // Delete existing item first to avoid errSecDuplicateItem.
        delete(key: key)

        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
            kSecValueData as String: data,
        ]

        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw KeychainError.osStatus(status)
        }
    }

    func load(key: String) -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]

        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess, let data = item as? Data else {
            return nil
        }
        return String(data: data, encoding: .utf8)
    }

    func delete(key: String) {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]
        SecItemDelete(query as CFDictionary)
    }
}
