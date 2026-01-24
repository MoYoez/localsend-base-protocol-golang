<div align="center">

# Localsend-base-protocol-golang

**Language / 语言:** English | [简体中文](README-zh-CN.md)

</div>

---

### Overview

This is a server-client implementation of the [LocalSend Protocol](https://github.com/localsend/protocol) written in Go.

This implementation provides a Go-based server and client that follows the LocalSend Protocol v2.1 specification.

Actually it used for [decky-localsend](https://github.com/moyoez/decky-localsend)

### Project Structure

```
.
├── api/              # API server implementation
│   ├── controllers/  # Request handlers
│   ├── models/       # Data models
│   └── middlewares/  # HTTP middlewares
├── boardcast/        # Multicast discovery
├── transfer/         # File transfer logic
├── share/            # Shared utilities
├── tool/             # Helper tools
└── types/            # Type definitions
```

### Command-Line Flags

| Flag                          | Type    | Default | Description                                                                                  |
|-------------------------------|---------|---------|----------------------------------------------------------------------------------------------|
| `-log`                        | string  | (empty) | Log mode: `dev` or `prod`                                                                    |
| `-useMultcastAddress`         | string  | (empty) | Override the default multicast address                                                       |
| `-useMultcastPort`            | int     | 0       | Override the default multicast port                                                          |
| `-useConfigPath`              | string  | (empty) | Specify an alternative config file path                                                      |
| `-useDefaultUploadFolder`     | string  | (empty) | Specify the default folder for uploads                                                       |
| `-useLegacyMode`              | bool    | false   | Use legacy HTTP mode to scan devices (scans every 30 seconds)                                |
| `-useReferNetworkInterface`   | string  | "*"     | Specify the network interface for use (e.g., `"en0"`, `"eth0"`, or `"*"` for all interfaces) |
| `-usePin`                    | string  | (empty) | Specify a PIN to require for uploads |
| `-useAutoSave`               | bool    | true    | If false, requires manual confirmation to receive files |
| `-useAlias`                    | string  | (empty) | Specify a Alias to shown in net. |


> Generally, if you are using a hotspot for the Steam Deck, using -useLegacyMode can help avoid the issue of not being able to scan.

### TODO

1. **Manual confirmation as receiver** - Implement manual confirmation mechanism for receiving files
2. **API parameter modifications** - Review and update API parameters as needed
3. **Bug fixes and performance improvements** - Address potential bugs and optimize performance

### Getting Started

```bash
# Build the project
go build -o localsend-server

# Run the server
./localsend-server
```

### Configuration

The server can be configured through command-line flags or configuration files. See the code for available options.

### License

This project implements the LocalSend Protocol. Please refer to the [LocalSend Protocol repository](https://github.com/localsend/protocol) for protocol specifications.
