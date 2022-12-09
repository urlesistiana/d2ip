# d2ip

d2ip 是一个零配置的 DNS 服务器。可将任意包含 IP 的域名映射到该 IP。

- IPv4: `1.2.3.4.your.domain` -> A `1.2.3.4`
- IPv6: `2000--1.your.domain` -> AAAA `2000::1`

## 参数

```shell
d2ip -l :53 -d your.domain,your.another.domain -m 127.0.0.1:8080
```

- `-l` 监听地址。默认 `:53` 。
- `-d` (必需) 提供服务的域名。多个域名用 `,` 分隔。
- `-m` prometheus metrics http 地址。

## 使用

1. 在服务器上启动 d2ip 。
2. 将域名的 NS 记录指向服务器。

## docker

```shell
docker run -d --name d2ip --restart unless-stopped -p 53:53/udp -p 53:53/tcp ghcr.io/urlesistiana/d2ip:main d2ip -d your.domain
```
