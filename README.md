# d2ip

d2ip 是一个 DNS 服务器。可将任意 ip 域名转换成 A/AAAA 记录。

示例:

- `1.2.3.4.your.domain` -> `1.2.3.4`
- `2000--1.your.domain` -> `2000::1`

## 参数

`d2ip -l :53 -d your.domain,your.another.domain -m 127.0.0.1:8080`

- -l 监听地址。默认 :53 。
- -d (必需) 提供服务的域名。多个域名用 `,` 分隔。
- -m prometheus metrics http 地址。

## docker

```shell
docker run -d --name d2ip --restart unless-stopped -p 53:53/udp -p 53:53/tcp ghcr.io/urlesistiana/d2ip:main d2ip -d your.domain
```