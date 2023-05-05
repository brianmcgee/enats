# ENATS - Ethereum NATS

![Build](https://github.com/brianmcgee/enats/actions/workflows/check-flake.yml/badge.svg)
![License: Apache 2](https://img.shields.io/badge/License-Apache%202-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)

Status: _EXPERIMENTAL_

ENATS is an exploration of how [NATS](https://nats.io) can be used within the Ethereum ecosystem.

Its current focus is on connecting Ethereum clients to NATS and making their chain data available.

## Quick Start

For the smoothest experience you should have [Nix](https://nixos.org) and [Direnv](https://direnv.net/) installed.

To start a NATS server and an [Anvil](https://book.getfoundry.sh/reference/anvil/) instance:

```terminal
$ dev-services
```

To start a sidecar and have it connect to Anvil:

```terminal
$ nix run .#enats -- sidecar --client-id=anvil --log-development --log-level info
2023-05-05T10:07:50.467+0100    INFO    sidecar/sidecar.go:147  connected to nats       {"url": "ns://127.0.0.1:4222"}
2023-05-05T10:07:50.468+0100    INFO    sidecar/sidecar.go:197  connected to web3 client        {"url": "ws://127.0.0.1:8545"}
2023-05-05T10:07:50.468+0100    INFO    storage/blocks.go:45    init    {"path": "./data/anvil.db"}
2023-05-05T10:07:50.486+0100    INFO    storage/blocks.go:73    init complete   {"path": "./data/anvil.db", "stats": "[count=81] 0x1adc16b94861be4f1408d37ccd238aa9dd76e0d35ce0b7437324724187a7a1e6(0) -> 0x591ce7788b317c300632a90265521541a0fc7573f9184d72b81b9aaf66acab67(80)"}
2023-05-05T10:07:50.486+0100    INFO    storage/blocks.go:241   pruning {"maxSize": 128}
2023-05-05T10:07:50.504+0100    INFO    storage/blocks.go:260   pruning complete        {"maxSize": 128, "size": 81, "removed": 0}
2023-05-05T10:07:50.524+0100    INFO    storage/blocks.go:350   filling gaps in history {"start": 0, "end": 103}
2023-05-05T10:07:50.553+0100    INFO    storage/blocks.go:428   filling gaps in history complete        {"start": 0, "end": 103, "fetched": 23}
2023-05-05T10:07:50.553+0100    INFO    rpc/handlers.go:73      init    {"subjectPrefix": "web3.rpc.request.31337.31337"}
2023-05-05T10:07:50.554+0100    INFO    rpc/handlers.go:116     init complete   {"subjectPrefix": "web3.rpc.request.31337.31337", "blockCount": 104}
```

To view all connected sidecars for network `31337` and chain `31337`:

```terminal
$ nats request web3.rpc.request.31337.31337.info ''
10:10:13 Sending request on "web3.rpc.request.31337.31337.info"
10:10:13 Received with rtt 252.563µs
{"NetworkId":31337,"ChainId":31337,"clientId":"anvil","clientVersion":{"name":"anvil","version":"v0.1.0","os":"","language":""}}
```

To send a json rpc request to all connected sidecars for network `31337` and chain `31337` and listen for at most `10` replies:

```terminal
$ nats request --replies 10 web3.rpc.request.31337.31337 -H method:eth_blockNumber '[]'
10:12:51 Sending request on "web3.rpc.request.31337.31337"
10:12:51 Received with rtt 448.985µs
10:12:51 clientSubject: web3.rpc.request.31337.31337.client.anvil
10:12:51 clientVersion: anvil/v0.1.0

"0x194"
```

To send a json rpc request to a sidecar that has block data for network `31337`, chain `31337` and block number `0x256`:

```terminal
$ nats request --inbox-prefix web3.rpc.response --replies 10 web3.rpc.request.31337.31337.number.256 -H method:eth_getBlockByNumber '["0x256", false]'
10:16:24 Sending request on "web3.rpc.request.31337.31337.number.256"
10:16:24 Received with rtt 602.286µs
10:16:24 clientSubject: web3.rpc.request.31337.31337.client.anvil
10:16:24 clientVersion: anvil/v0.1.0

{"hash":"0xef43b00100e450a69332826c2976e4eff709eaffc7901e3b337834eb586d4453","parentHash":"0x3d4f4f5e63eb9d785fbc96c7cd526b21400750fd6ab654afa754018310d97108","sha3Uncles":"0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347","miner":"0x0000000000000000000000000000000000000000","stateRoot":"0x10baabfe446f34ffd15cf66bb1d4969f4499deea67207244c6a675ba2b5a6b88","transactionsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421","receiptsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421","number":"0x256","gasUsed":"0x0","gasLimit":"0x1c9c380","extraData":"0x","logsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000","timestamp":"0x6454c955","difficulty":"0x0","totalDifficulty":"0x0","sealFields":["0x0000000000000000000000000000000000000000000000000000000000000000","0x0000000000000000"],"uncles":[],"transactions":[],"size":"0x200","mixHash":"0x0000000000000000000000000000000000000000000000000000000000000000","nonce":"0x0000000000000000","baseFeePerGas":"0x0"}
```

To send a json rpc request to a sidecar that has block data for network `31337`, chain `31337` and block hash `88911d2f3ae12d067b662120f045b958a53dfbb2f877c6be488fe28bca71bf6f`:

```terminal
$ nats request --inbox-prefix web3.rpc.response --replies 10 web3.rpc.request.31337.31337.hash.88911d2f3ae12d067b662120f045b958a53dfbb2f877c6be488fe28bca71bf6f -H method:eth_getBlockByHash '["0x88911d2f3ae12d067b662120f045b958a53dfbb2f877c6be488fe28bca71bf6f", false]'
10:18:35 Sending request on "web3.rpc.request.31337.31337.hash.88911d2f3ae12d067b662120f045b958a53dfbb2f877c6be488fe28bca71bf6f"
10:18:35 Received with rtt 532.336µs
10:18:35 clientVersion: anvil/v0.1.0
10:18:35 clientSubject: web3.rpc.request.31337.31337.client.anvil

{"hash":"0x88911d2f3ae12d067b662120f045b958a53dfbb2f877c6be488fe28bca71bf6f","parentHash":"0xaf19b34ef6a05eb02626d4a0b1756de8c403b09f81d6bc22fc1178d98e7e34c1","sha3Uncles":"0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347","miner":"0x0000000000000000000000000000000000000000","stateRoot":"0x10baabfe446f34ffd15cf66bb1d4969f4499deea67207244c6a675ba2b5a6b88","transactionsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421","receiptsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421","number":"0x2b3","gasUsed":"0x0","gasLimit":"0x1c9c380","extraData":"0x","logsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000","timestamp":"0x6454c9b2","difficulty":"0x0","totalDifficulty":"0x0","sealFields":["0x0000000000000000000000000000000000000000000000000000000000000000","0x0000000000000000"],"uncles":[],"transactions":[],"size":"0x200","mixHash":"0x0000000000000000000000000000000000000000000000000000000000000000","nonce":"0x0000000000000000","baseFeePerGas":"0x0"}
```
