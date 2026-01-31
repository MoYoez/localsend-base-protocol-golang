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
| `-log`                        | string  | (empty) | Log mode: `dev` or `prod` or `none`                                                               |
| `-useMultcastAddress`         | string  | (empty) | Override the default multicast address                                                       |
| `-useMultcastPort`            | int     | 0       | Override the default multicast port                                                          |
| `-useConfigPath`              | string  | (empty) | Specify an alternative config file path                                                      |
| `-useDefaultUploadFolder`     | string  | (empty) | Specify the default folder for uploads                                                       |
| `-useLegacyMode`              | bool    | false   | Use legacy HTTP mode to scan devices (scans every 30 seconds)                                |
| `-useReferNetworkInterface`   | string  | "*"     | Specify the network interface for use (e.g., `"en0"`, `"eth0"`, or `"*"` for all interfaces) |
| `-usePin`                    | string  | (empty) | Specify a PIN to require for uploads |
| `-useAutoSave`               | bool    | true    | If false, requires manual confirmation to receive files |
| `-useAlias`                    | string  | (empty) | Specify a Alias to shown in net. |
| `-useHttps`                   | bool    | true    | If true, use https (encrypted); if false, use http (unencrypted). Alias for protocol config. |
| `-useMixedScan`               | bool    | false   | Use mixed scan mode (both UDP and HTTP for discovery)                                        |
| `-skipNotify`                 | bool    | false   | Skip notification mode                                                                       |


> Most of cases, mixed mode works well for most cases, if you prefer to reduce the power cost for your machine, switching to (Normal Mode - UDP Detected.) ,it will not make scan to the whole net.

> Sometimes Application cannot scan other localsend if the online too long time, **consider trigger "scan" on other localsend**.

### TODO

1. (maybe) Website Support, but decky-send make it, so i guess I dont make website for this, getting API for your local machine accessing it not a good choice.

### Known BUGS

- Cannot use web localsend (Due to localsend use v3 but not explained in protocol document.)
- In some cases (e.g. localsend in backend too long time (I guess), being scanned machine cannot detected the service, restart service is helpful. )

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
