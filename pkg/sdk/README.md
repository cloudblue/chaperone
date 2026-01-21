# Public SDK

This directory will contain the public SDK (`chaperone-sdk`) that distributors use to implement their plugins.

The SDK defines the interfaces that plugins must implement:

- `CredentialProvider` - For credential injection
- `CertificateSigner` - For certificate signing operations
- `ResponseModifier` - For response manipulation

See the Design Specification for interface details.
