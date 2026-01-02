# Geth 节点常用命令

本文档用于记录当前服务器上的 Geth/Lighthouse 常用命令。

## 服务器信息

- IP: 66.45.249.58
- 用户: root
- 密码: %kg%*7V$

## 二创仓库维护与更新（最重要）

仓库位置：

```bash
cd /root/my-go-ethereum
```

拉取最新二创代码并编译：

```bash
git fetch --all
git checkout release/1.16.lottery
git pull --ff-only origin release/1.16.lottery

export PATH=/usr/local/go/bin:$PATH
make geth
ln -sf /root/my-go-ethereum/build/bin/geth /usr/local/bin/geth
```

重启 Geth 生效：

```bash
systemctl restart geth
systemctl status geth --no-pager
```

如果需要完整重跑（包含依赖、Lighthouse、配置刷新），执行：

```bash
bash /root/deploy_geth_node.sh
```

## 服务状态与重启

```bash
systemctl status geth --no-pager
systemctl restart geth
systemctl stop geth
```

```bash
systemctl status lighthouse-beacon --no-pager
systemctl restart lighthouse-beacon
systemctl stop lighthouse-beacon
```

## 查看日志

```bash
journalctl -fu geth.service -o cat
```

```bash
journalctl -fu lighthouse-beacon.service -o cat
```

## 同步进度

推荐使用已生成的便捷脚本：

```bash
/root/geth-node/sync.sh
```

等价命令：

```bash
/root/my-go-ethereum/build/bin/geth attach --exec "eth.syncing" /var/lib/geth-mainnet/geth.ipc
/root/my-go-ethereum/build/bin/geth attach --exec "eth.blockNumber" /var/lib/geth-mainnet/geth.ipc
```

说明：
- `eth.syncing` 返回 `false` 表示执行层同步完成。
- 日志中的 `Indexing transactions` 和 `Generating snapshot` 属于后台索引/快照任务，不影响余额查询。

## 查询余额 (ETH)

```bash
curl -s -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0x地址","latest"],"id":1}'
```

返回是 wei（16 进制），可用下列命令换算为 ETH：

```bash
python3 - << 'PY'
val = int("0x000000000000", 16)  # 替换为返回的 hex
print(val)
print(val / 10**18)
PY
```

## 数据目录

```bash
ls -lah /var/lib/geth-mainnet
ls -lah /var/lib/lighthouse-mainnet
```

## 便捷脚本位置

```bash
ls -lah /root/geth-node
```

包含：
- `start.sh` / `stop.sh` / `status.sh`
- `logs.sh` / `logs-beacon.sh`
- `sync.sh`
