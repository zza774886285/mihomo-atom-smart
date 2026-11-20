# mihomo-atom-smart

mihomo **smart** 内核 + **atom 网关 FakeIP 出站**补丁，给 atom（jasonxtt/atom）那套「atomdns 决策 + mihomo 执行」的机器用。

分支 `atom-smart` 每天都在跟上游，`Alpha` 保持上游原样（不碰）。

## 为什么有这么个仓库

atom 自带的内核是 `jasonxtt/mihomo` 的 `atom/fakeip-outbound` 分支（基于 v1.19.34，3 个 commit），
认两个上游没有的 DNS 字段：

- `dns.atom-mixed-dns`
- `dns.atom-fake-ip-final`

换成普通 mihomo / vernesong 的 smart 内核，这两个字段直接不认，atomdns 的 FakeIP 决策链路就断了。
反过来把 smart（约 25 万字节源码：`adapter/outboundgroup/smart.go` + `component/smart/*`）搬进 atom 的 fork，
工作量大且要长期跟两边。

所以做法是：**以 vernesong 的 smart 分支为基线，把 atom 那 3 个补丁打上去**。实测只有 1 处冲突
（`hub/route/cache.go` 两行路由，保留两边即可）。

## 分支与 CI

- `Alpha`：纯上游，不提交
- `atom-smart`：上游 Alpha + 3 个 atom 补丁（`git log --grep atom`）
- `Atom Smart Build`：推 `atom-smart` 就编 v1/v3 + 跑门禁，不发版
- `Atom Smart Sync`：每 6 小时查上游 Alpha，合并 → 编译 → 门禁 → 发 release + 更新 `releases.json`；
  失败只开 issue，不发版

## 门禁（`.github/ci/verify.sh`，只看真二进制）

1. 版本自报包含 tag
2. atom 私有字段配置 `-t` 通过
3. **反向对照**：删掉 `atom-mixed-dns` 必须失败（防止补丁悄悄失效后空转）
4. `type: smart` 组配置 `-t` 通过
5. 起进程、查 Clash API，必须出现 `type=Smart` 的组

## 产物

- `mihomo-linux-amd64-v1.gz` — GOAMD64=v1，无 AVX 的 CPU（142 / CHR / QEMU 默认 CPU）
- `mihomo-linux-amd64-v3.gz` — GOAMD64=v3，支持 AVX2 的 CPU
- `atom-smart-manifest.json` — 版本、上游 commit、两个包的 sha256

装法就是替换 `/opt/atom/bin/mihomo`（先备份，`systemctl restart atom-mihomo`）。
smart 组首次启用会自己下 `Model.bin`（9MB）和 `ASN.mmdb`（8.7MB），国内大概率拉不动，需要预置到数据目录。

## 上游同步失败时

`git fetch upstream Alpha && git checkout atom-smart && git rebase upstream/Alpha`，
冲突集中在 `dns/server.go`、`tunnel/tunnel.go`、`hub/route/cache.go`、`constant/metadata.go`，
解完跑 `bash .github/ci/verify.sh ./mihomo` 再手动触发 `Atom Smart Sync`。