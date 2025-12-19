# Contributing to Distributed Key-Value Store with Tunable Consistency

Thank you for your interest in contributing to Distributed Key-Value Store with Tunable Consistency! This document provides guidelines and information for contributors.

## 📋 Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Documentation](#documentation)

---

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment. Please:

- Be respectful and constructive in discussions
- Welcome newcomers and help them get started
- Focus on what is best for the community
- Show empathy towards other community members

---

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Git
- Make

### Setup Development Environment

```bash
# Fork the repository on GitHub

# Clone your fork
git clone https://github.com/YOUR_USERNAME/Distributed-Key-Value-Store-with-Tunable-Consistency.git
cd Distributed-Key-Value-Store-with-Tunable-Consistency

# Add upstream remote
git remote add upstream https://github.com/bhushanasati25/Distributed-Key-Value-Store-with-Tunable-Consistency.git

# Install dependencies
go mod download

# Install development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Verify setup
make build
make test
```

---

## Development Workflow

### 1. Create a Branch

```bash
# Sync with upstream
git fetch upstream
git checkout main
git merge upstream/main

# Create feature branch
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

### 2. Make Changes

- Write clean, documented code
- Follow the [coding standards](#coding-standards)
- Add tests for new functionality
- Update documentation as needed

### 3. Test Your Changes

```bash
# Run all tests
make test

# Run linter
make lint

# Run with coverage
make coverage

# Test manually with a cluster
make cluster
```

### 4. Commit Your Changes

Use conventional commit messages:

```
feat: add new feature description
fix: resolve issue with X
docs: update README with Y
test: add tests for Z
refactor: improve code structure
perf: optimize performance of X
```

Example:
```bash
git add .
git commit -m "feat: add support for TTL expiration"
```

### 5. Push and Create PR

```bash
git push origin feature/your-feature-name
```

Then open a Pull Request on GitHub.

---

## Pull Request Process

### Before Submitting

- [ ] Code compiles without errors (`make build`)
- [ ] All tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Documentation updated if needed
- [ ] Commit messages follow conventions

### PR Description Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
Describe how you tested these changes

## Checklist
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] No breaking changes (or documented)
```

### Review Process

1. A maintainer will review your PR
2. Address any requested changes
3. Once approved, your PR will be merged

---

## Coding Standards

### Go Style Guide

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Run `golangci-lint` before committing

### Naming Conventions

```go
// Package names: lowercase, single word
package storage

// Exported functions: PascalCase with descriptive names
func (bc *Bitcask) GetValue(key string) ([]byte, error)

// Unexported functions: camelCase
func (bc *Bitcask) rebuildIndex() error

// Constants: PascalCase for exported, camelCase for unexported
const MaxFileSize = 100 * 1024 * 1024
const defaultBufferSize = 64 * 1024
```

### Error Handling

```go
// Always wrap errors with context
if err != nil {
    return fmt.Errorf("failed to open data file: %w", err)
}

// Define package-level errors for known conditions
var (
    ErrKeyNotFound = errors.New("key not found")
    ErrStorageFull = errors.New("storage is full")
)
```

### Comments

```go
// Package storage implements the Bitcask storage engine.
package storage

// Bitcask implements an append-only log-structured storage engine.
// It provides O(1) lookups using an in-memory hash index.
type Bitcask struct {
    // ...
}

// Get retrieves a value by key.
// Returns ErrKeyNotFound if the key doesn't exist.
func (bc *Bitcask) Get(key string) ([]byte, error) {
    // ...
}
```

---

## Testing Guidelines

### Unit Tests

- Test each function/method independently
- Use table-driven tests for multiple cases
- Mock external dependencies

```go
func TestBitcaskGet(t *testing.T) {
    tests := []struct {
        name    string
        key     string
        want    []byte
        wantErr error
    }{
        {"existing key", "key1", []byte("value1"), nil},
        {"missing key", "missing", nil, ErrKeyNotFound},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Tests

- Use build tags: `// +build integration`
- Test multi-node scenarios
- Include failure scenarios

### Test Coverage

- Aim for >80% coverage on critical paths
- Focus on edge cases and error conditions

---

## Documentation

### Code Documentation

- Add GoDoc comments for all exported types and functions
- Include examples for complex APIs

### README Updates

- Update feature lists when adding features
- Keep examples current and working
- Update configuration options

### Architecture Docs

For significant changes, update:
- Architecture diagrams
- Technical deep dive sections
- API reference

---

## Questions?

- Open an issue for bugs or feature requests
- Start a discussion for questions or ideas
- Check existing issues before creating new ones

Thank you for contributing! 🎉
