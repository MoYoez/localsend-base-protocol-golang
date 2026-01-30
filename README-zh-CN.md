# Localsend-base-protocol-golang

**语言:** [English](../README.md) | 简体中文

---

## 概述

这是基于 [LocalSend 协议](https://github.com/localsend/protocol) 的服务器客户端实现，使用 Go 语言编写。

本实现提供了一个基于 Go 的服务器和客户端，遵循 LocalSend 协议 v2.1 规范。

实际上这是为 [decky-localsend](https://github.com/moyoez/decky-localsend) 编写的工具

## 项目结构

```
.
├── api/              # API 服务器实现
│   ├── controllers/  # 请求处理器
│   ├── models/       # 数据模型
│   └── middlewares/  # HTTP 中间件
├── boardcast/        # 组播发现
├── transfer/         # 文件传输逻辑
├── share/            # 共享工具
├── tool/             # 辅助工具
└── types/            # 类型定义
```

### Flags

| 参数                            | 类型    | 默认值   | 说明                                                                                         |
|---------------------------------|---------|----------|----------------------------------------------------------------------------------------------|
| `-log`                         | string   | (空)     | 日志模式：`dev` 或 `prod`                                                                    |
| `-useMultcastAddress`          | string   | (空)     | 覆盖默认组播地址                                                                             |
| `-useMultcastPort`             | int     | 0        | 覆盖默认组播端口                                                                             |
| `-useConfigPath`               | string   | (空)     | 指定其他配置文件路径                                                                         |
| `-useDefaultUploadFolder`      | string   | (空)     | 指定默认上传文件夹                                                                           |
| `-useLegacyMode`               | Boolean   | false    | 使用旧版 HTTP 模式扫描设备（每 30 秒扫描一次）                                               |
| `-useReferNetworkInterface`    | string   | "*"      | 指定使用的网络接口（如 `"en0"`、`"eth0"`，或 `"*"` 表示所有接口）                             |
| `-usePin`                      | string   | (空)     | 指定上传时需要的 PIN
| `-useAutoSave`                 | Boolean  | true     | 若为 false，则在接收文件时需要手动确认                |
| `-useAlias`                    | string  | (空) | 指定别名以在互联网上显示 |
| `-useHttps`                   | bool    | true    | 若为 true，使用 https（加密）；若为 false，使用 http（不加密）。protocol 配置的别名。 |
| `-useMixedScan`               | bool    | false   | 使用混合模式扫描 (UDP+HTTP)
| `-skipNotify`                 | bool    | false   | 跳过对 Decky 的 unix 的通信      

#### 小提示

> 大多数情况下，使用 Mixed Mode 就可以了xwx :(

## TODO

1. bug 修复
2. 网页支持，但可能会咕咕咕咕咕咕咕

## 已知 BUG

1. Web Localsend 用不了 （本来就用不了）
2. 在一些情况下，Localsend 其他客户端 因为省电策略或者别的乱七八糟的原因，可能会扫不到，需要重新开下(

## 快速开始

```bash
# 构建项目
go build -o localsend-server

# 运行服务器
./localsend-server
```

## 性能测试

基准测试位于 `api/perf_test.go`，使用 `httptest` 直接压测 API 处理器。

```bash
# prepare-upload 基准（JSON 解析 + 会话准备）
go test ./api -run '^$' -bench BenchmarkPrepareUpload -benchmem -benchtime=3s

# upload 基准（磁盘写入 + 哈希）
go test ./api -run '^$' -bench BenchmarkUpload -benchmem -benchtime=3s

# 覆盖上传负载大小（单位：bytes）
BENCH_UPLOAD_SIZE=10485760 go test ./api -run '^$' -bench BenchmarkUpload -benchmem -benchtime=3s
```

## 配置

服务器可以通过命令行参数或配置文件进行配置。请查看代码了解可用选项。

## 许可证

本项目实现了 LocalSend 协议。有关协议规范，请参考 [LocalSend 协议仓库](https://github.com/localsend/protocol)。
